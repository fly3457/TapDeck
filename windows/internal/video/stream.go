package video

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrBusy      = errors.New("另一台手机正在查看电脑画面")
	ErrNoEncoder = errors.New("这台电脑没有可用的 H.264 编码器")
	ErrNoStream  = errors.New("视频流不存在或已过期")
)

// Source 逐帧产出编码后的画面；Close 结束编码进程。
type Source interface {
	Next() (Frame, error)
	Close() error
}

// Launcher 负责选择显示器与编码器、启动编码并读取光标位置。
type Launcher interface {
	Available() bool
	Prepare(Request) (Plan, error)
	Launch(context.Context, Plan) (Source, error)
	// Cursor 返回相对于 d 左上角的光标坐标；光标不在 d 上或隐藏时 visible 为 false。
	Cursor(d Display) (x, y int, visible bool)
}

// Writer 发送一条二进制 WebSocket 消息。
type Writer func(ctx context.Context, b []byte) error

const (
	MessageFrame  byte = 1
	MessageCursor byte = 2
	// MessageTiming 紧接在对应画面帧之前发送，供手机统计端到端延迟；旧版手机忽略未知类型。
	MessageTiming byte = 3
	// MessageAudio 是电脑声音：类型 4 后接 48 kHz、立体声、16 位小端 PCM，每包 10 ms。
	MessageAudio byte = 4

	FlagKey    byte = 1
	FlagConfig byte = 2

	// queueFrames 约 100ms@60fps；积压超过即丢弃到下一个关键帧。
	queueFrames = 6
	attachTTL   = 30 * time.Second
	cursorEvery = 8 * time.Millisecond
	// audioPackets 是电脑声音的发送积压上限（每包 10ms）。
	audioPackets = 10
)

// Stream 是一次已授权的视频请求，Owner 为控制会话 ID。
type Stream struct {
	ID      string
	Token   []byte
	Owner   uint64
	Plan    Plan
	created time.Time
	ctx     context.Context
	cancel  context.CancelFunc
	claimed atomic.Bool
	Sent    atomic.Uint64
	Dropped atomic.Uint64
	// Timing 为真时每个画面帧前附带帧时间消息（Manager.Start 创建的流都开启）。
	Timing bool
	// audio 为真时转发电脑声音；audioWake 在开关变化时通知采集循环。
	audio     atomic.Bool
	audioWake chan struct{}
}

// SetAudio 打开或关闭本流的电脑声音转发。
func (st *Stream) SetAudio(on bool) {
	if st.audio.Swap(on) != on {
		select {
		case st.audioWake <- struct{}{}:
		default:
		}
	}
}

func (st *Stream) Done() <-chan struct{} { return st.ctx.Done() }

type Manager struct {
	mu       sync.Mutex
	launcher Launcher
	active   *Stream
	now      func() time.Time
	// Loopback 采集电脑声音（Windows 为 audio.Loopback）；为 nil 时不提供声音转发。
	Loopback func(ctx context.Context, emit func(pcm []byte)) error
}

// AudioAvailable 表示能否把电脑声音随画面转发。
func (m *Manager) AudioAvailable() bool { return m != nil && m.Loopback != nil }

// SetAudio 打开或关闭 owner 当前视频流的电脑声音。
func (m *Manager) SetAudio(owner uint64, on bool) {
	if m == nil {
		return
	}
	m.mu.Lock()
	st := m.active
	m.mu.Unlock()
	if st != nil && st.Owner == owner {
		st.SetAudio(on)
	}
}

func NewManager(l Launcher) *Manager { return &Manager{launcher: l, now: time.Now} }

func (m *Manager) Available() bool { return m != nil && m.launcher != nil && m.launcher.Available() }

// Prewarm 在后台预先探测默认画质的编码器，手机首次打开视频模式时不必等待逐个尝试。
func (m *Manager) Prewarm() {
	if m.Available() {
		go func() { _, _ = m.launcher.Prepare(Request{Quality: "smooth"}) }()
	}
}

// Start 为 owner 分配新的视频流。同一 owner 的旧流被替换；其他会话持有
// 未过期的流时返回 ErrBusy。Prepare 可能耗时（首次探测编码器），不持锁执行。
func (m *Manager) Start(owner uint64, r Request) (*Stream, error) {
	if !m.Available() {
		return nil, ErrNoEncoder
	}
	m.mu.Lock()
	busy := m.busyLocked(owner)
	m.mu.Unlock()
	if busy {
		return nil, ErrBusy
	}
	plan, err := m.launcher.Prepare(r)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	st := &Stream{ID: randomHex(8), Token: randomBytes(32), Owner: owner, Plan: plan, created: m.now(), ctx: ctx, cancel: cancel, Timing: true, audioWake: make(chan struct{}, 1)}
	st.audio.Store(r.Audio)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.busyLocked(owner) {
		cancel()
		return nil, ErrBusy
	}
	if m.active != nil {
		m.active.cancel()
	}
	m.active = st
	return st, nil
}

func (m *Manager) busyLocked(owner uint64) bool {
	a := m.active
	if a == nil || a.Owner == owner || a.ctx.Err() != nil {
		return false
	}
	// 分配后迟迟没有连接的流视为过期，不阻塞其他手机。
	return a.claimed.Load() || m.now().Sub(a.created) < attachTTL
}

// Stop 结束 owner 的视频流（控制会话停止视频、断开或解除配对时调用）。
func (m *Manager) Stop(owner uint64) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active != nil && m.active.Owner == owner {
		m.active.cancel()
		m.active = nil
	}
}

func (m *Manager) StopAll() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active != nil {
		m.active.cancel()
		m.active = nil
	}
}

// DisplayFor 返回 owner 当前视频流捕获的显示器，用于把手机上的点击换算为桌面坐标。
func (m *Manager) DisplayFor(owner uint64) (Display, bool) {
	if m == nil {
		return Display{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active == nil || m.active.Owner != owner || m.active.ctx.Err() != nil {
		return Display{}, false
	}
	return m.active.Plan.Display, true
}

// Owner 返回当前视频流的会话 ID，没有时为 0。
func (m *Manager) Owner() uint64 {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active == nil || m.active.ctx.Err() != nil {
		return 0
	}
	return m.active.Owner
}

// Claim 校验视频连接的流 ID、令牌与所属会话；每个流只能被连接一次。
func (m *Manager) Claim(id string, token []byte, owner uint64) (*Stream, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.active
	if st == nil || st.ctx.Err() != nil || st.Owner != owner || subtle.ConstantTimeCompare([]byte(st.ID), []byte(id)) != 1 ||
		subtle.ConstantTimeCompare(st.Token, token) != 1 || m.now().Sub(st.created) > attachTTL || !st.claimed.CompareAndSwap(false, true) {
		return nil, ErrNoStream
	}
	return st, nil
}

// Run 启动编码并把画面与光标写给手机，直到 ctx 或流结束。
func (m *Manager) Run(ctx context.Context, st *Stream, w Writer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-st.ctx.Done():
			cancel()
		case <-ctx.Done():
		}
	}()
	defer func() {
		m.mu.Lock()
		if m.active == st {
			m.active = nil
		}
		m.mu.Unlock()
		st.cancel()
	}()
	src, err := m.launcher.Launch(ctx, st.Plan)
	if err != nil {
		return err
	}
	defer src.Close()
	go func() { <-ctx.Done(); src.Close() }()

	// 开启帧时间时每帧占两条（时间 + 画面），容量翻倍以保持约 100ms 的积压上限。
	queue := make(chan []byte, queueFrames*2)
	readErr := make(chan error, 1)
	go func() { readErr <- pump(src, queue, st) }()
	// 电脑声音单独排队，最多积压约 100ms（10 包），满了丢最旧的，声音不会越拖越晚。
	audioQ := make(chan []byte, audioPackets)
	if m.Loopback != nil && st.audioWake != nil {
		go m.audioLoop(ctx, st, audioQ)
	}

	cursor := time.NewTicker(cursorEvery)
	defer cursor.Stop()
	lastX, lastY, lastVisible, sentCursor := 0, 0, false, false
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-readErr:
			if ctx.Err() != nil {
				return nil
			}
			return err
		case b := <-queue:
			if b[0] == MessageTiming {
				stampSend(b, time.Now())
			}
			if err := w(ctx, b); err != nil {
				return err
			}
			if b[0] == MessageFrame {
				st.Sent.Add(1)
			}
		case b := <-audioQ:
			if err := w(ctx, b); err != nil {
				return err
			}
		case <-cursor.C:
			x, y, visible := m.launcher.Cursor(st.Plan.Display)
			if sentCursor && x == lastX && y == lastY && visible == lastVisible {
				continue
			}
			lastX, lastY, lastVisible, sentCursor = x, y, visible, true
			if err := w(ctx, CursorMessage(x, y, visible)); err != nil {
				return err
			}
		}
	}
}

// pump 读取编码帧放入有界队列。队列满时清空积压并丢弃后续帧，直到下一个
// 关键帧，保证手机画面延迟不会累积；配置帧从不丢弃。
func pump(src Source, queue chan []byte, st *Stream) error {
	needKey := false
	var clock captureClock
	for {
		f, err := src.Next()
		if err != nil {
			return err
		}
		recv := time.Now()
		msg := FrameMessage(f)
		if f.Config {
			needKey = true
			drain(queue, st)
			queue <- msg
			continue
		}
		if needKey && !f.Key {
			st.Dropped.Add(1)
			continue
		}
		// 帧时间与画面成对入队：只有一个生产者，空位只会变多，检查后连续放入不会拆散。
		batch := [][]byte{msg}
		if st.Timing {
			batch = [][]byte{TimingMessage(f.PTS, clock.encode(recv, f.PTS), recv), msg}
		}
		if cap(queue)-len(queue) >= len(batch) {
			for _, b := range batch {
				queue <- b
			}
			needKey = false
			continue
		}
		drain(queue, st)
		st.Dropped.Add(1)
		needKey = true
		if f.Key {
			for _, b := range batch {
				queue <- b
			}
			needKey = false
		}
		log.Printf("video: 网络积压，丢弃到下一个关键帧")
	}
}

// captureClock 估计 ffmpeg 内部（截屏到编码输出）的耗时。ddagrab 的 pts 是截屏时刻相对第一帧的
// 单调时钟，起点无法直接得知：取“收到时刻 − pts”的历史最小值作为起点，因此得到的是相对最快一帧
// 多出的耗时，会少算最快一帧本身约几毫秒的编码时间。
type captureClock struct {
	base   time.Time
	origin time.Duration
	ready  bool
}

func (c *captureClock) encode(recv time.Time, pts time.Duration) time.Duration {
	if c.base.IsZero() {
		c.base = recv
	}
	est := recv.Sub(c.base) - pts
	if !c.ready || est < c.origin {
		c.origin, c.ready = est, true
	}
	return est - c.origin
}

// TimingMessage：类型 3、int64 pts（微秒）、uint32 截屏到编码输出（微秒）、uint32 发送队列等待（微秒，
// 发送时填写）、int64 发送时刻（PC 墙钟 Unix 微秒；入队时暂存收到时刻），均为小端。
func TimingMessage(pts, encode time.Duration, recv time.Time) []byte {
	b := make([]byte, 25)
	b[0] = MessageTiming
	binary.LittleEndian.PutUint64(b[1:], uint64(pts.Microseconds()))
	binary.LittleEndian.PutUint32(b[9:], uint32(max(0, min(encode.Microseconds(), 1<<31))))
	binary.LittleEndian.PutUint64(b[17:], uint64(recv.UnixMicro()))
	return b
}

// stampSend 在写出帧时间消息前填入发送队列等待时长与发送时刻。
func stampSend(b []byte, now time.Time) {
	recv := int64(binary.LittleEndian.Uint64(b[17:]))
	wait := max(0, min(now.UnixMicro()-recv, 1<<31))
	binary.LittleEndian.PutUint32(b[13:], uint32(wait))
	binary.LittleEndian.PutUint64(b[17:], uint64(now.UnixMicro()))
}

func drain(queue chan []byte, st *Stream) {
	for {
		select {
		case b := <-queue:
			// 已排队的配置帧必须保留：解码器没有它无法开始。
			if len(b) > 1 && b[1]&FlagConfig != 0 {
				defer func() { queue <- b }()
				continue
			}
			st.Dropped.Add(1)
		default:
			return
		}
	}
}

// FrameMessage：类型 1、flags、int64 pts（微秒，小端）、Annex-B 数据。
func FrameMessage(f Frame) []byte {
	b := make([]byte, 10, 10+len(f.Data))
	b[0] = MessageFrame
	if f.Key {
		b[1] |= FlagKey
	}
	if f.Config {
		b[1] |= FlagConfig
	}
	binary.LittleEndian.PutUint64(b[2:], uint64(f.PTS.Microseconds()))
	return append(b, f.Data...)
}

// CursorMessage：类型 2、int32 x、int32 y（小端）、uint8 可见。
func CursorMessage(x, y int, visible bool) []byte {
	b := make([]byte, 10)
	b[0] = MessageCursor
	binary.LittleEndian.PutUint32(b[1:], uint32(int32(x)))
	binary.LittleEndian.PutUint32(b[5:], uint32(int32(y)))
	if visible {
		b[9] = 1
	}
	return b
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}

func randomHex(n int) string { return hex.EncodeToString(randomBytes(n)) }

// audioLoop 在开关打开时采集电脑声音放入 out，关闭时停止采集；随 ctx 结束。
// 采集失败（例如默认播放设备是虚拟声卡）时记录一次日志，等开关再次变化或 10 秒后重试。
func (m *Manager) audioLoop(ctx context.Context, st *Stream, out chan []byte) {
	emit := func(pcm []byte) {
		msg := AudioMessage(pcm)
		for {
			select {
			case out <- msg:
				return
			default:
			}
			select {
			case <-out: // 网络跟不上：丢最旧的一包
			default:
			}
		}
	}
	var lastErr string
	for ctx.Err() == nil {
		if !st.audio.Load() {
			select {
			case <-ctx.Done():
				return
			case <-st.audioWake:
			}
			continue
		}
		run, stop := context.WithCancel(ctx)
		go func() {
			// 开关变成关闭时结束本轮采集；打开状态下的重复通知忽略。
			for {
				select {
				case <-run.Done():
					return
				case <-st.audioWake:
					if !st.audio.Load() {
						stop()
						return
					}
				}
			}
		}()
		err := m.Loopback(run, emit)
		stop()
		if err != nil && ctx.Err() == nil {
			if err.Error() != lastErr {
				log.Printf("video: 电脑声音转发：%v", err)
				lastErr = err.Error()
			}
			select {
			case <-ctx.Done():
			case <-st.audioWake:
			case <-time.After(10 * time.Second):
			}
		}
	}
}

// AudioMessage：类型 4 后接 PCM（48 kHz、立体声、16 位小端）。
func AudioMessage(pcm []byte) []byte {
	b := make([]byte, 1, 1+len(pcm))
	b[0] = MessageAudio
	return append(b, pcm...)
}

// AudioOn 表示本流当前是否转发电脑声音。
func (st *Stream) AudioOn() bool { return st.audio.Load() }

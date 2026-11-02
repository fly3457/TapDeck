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

	FlagKey    byte = 1
	FlagConfig byte = 2

	// queueFrames 约 100ms@60fps；积压超过即丢弃到下一个关键帧。
	queueFrames = 6
	attachTTL   = 30 * time.Second
	cursorEvery = 8 * time.Millisecond
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
}

func (st *Stream) Done() <-chan struct{} { return st.ctx.Done() }

type Manager struct {
	mu       sync.Mutex
	launcher Launcher
	active   *Stream
	now      func() time.Time
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
	st := &Stream{ID: randomHex(8), Token: randomBytes(32), Owner: owner, Plan: plan, created: m.now(), ctx: ctx, cancel: cancel}
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

	queue := make(chan []byte, queueFrames)
	readErr := make(chan error, 1)
	go func() { readErr <- pump(src, queue, st) }()

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
			if err := w(ctx, b); err != nil {
				return err
			}
			st.Sent.Add(1)
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
	for {
		f, err := src.Next()
		if err != nil {
			return err
		}
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
		select {
		case queue <- msg:
			needKey = false
		default:
			drain(queue, st)
			st.Dropped.Add(1)
			needKey = true
			if f.Key {
				queue <- msg
				needKey = false
			}
			log.Printf("video: 网络积压，丢弃到下一个关键帧")
		}
	}
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

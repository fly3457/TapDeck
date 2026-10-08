package server

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/coder/websocket"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"tapdeck/internal/audio"
	"tapdeck/internal/config"
	"tapdeck/internal/input"
	"tapdeck/internal/keyboard"
	"tapdeck/internal/protocol"
	"tapdeck/internal/secure"
	"time"
)

const ControlVersion = 2

type Message struct {
	Type        string `json:"type"`
	Version     int    `json:"version,omitempty"`
	DeviceID    string `json:"device_id,omitempty"`
	Name        string `json:"name,omitempty"`
	Token       string `json:"token,omitempty"`
	ClientNonce string `json:"client_nonce,omitempty"`
	Secret      string `json:"secret,omitempty"`
	Code        string `json:"code,omitempty"`
	Epoch       uint32 `json:"epoch,omitempty"`
	NextEpoch   uint32 `json:"next_epoch,omitempty"`
	X           int64  `json:"x,omitempty"`
	Y           int64  `json:"y,omitempty"`
	ScrollX     int64  `json:"scroll_x,omitempty"`
	ScrollY     int64  `json:"scroll_y,omitempty"`
	Button      string `json:"button,omitempty"`
	Down        bool   `json:"down,omitempty"`
	Slot        int    `json:"slot,omitempty"`
	Revision    uint64 `json:"revision,omitempty"`
	Hold        string `json:"hold,omitempty"`
	Text        string `json:"text,omitempty"`
	Recording   string `json:"recording,omitempty"`
	Mode        string `json:"mode,omitempty"`
	ProfileID   string `json:"profile_id,omitempty"`
	Tick        int64  `json:"tick,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Steps       int    `json:"steps,omitempty"`
	Action      string `json:"action,omitempty"`
}
type Pending struct {
	ID       string
	DeviceID string
	Name     string
	Code     string
	Expires  time.Time
	answer   chan bool
	cancel   context.CancelFunc
	revoke   func()
}

// MaxSessions 是接收端同时接受的控制端数量上限。
const MaxSessions = 5

type Snapshot struct {
	Running bool
	URL     string
	// Device 是所有已连接控制端名字的拼接（旧字段，运行时统计与界面兼容用）。
	Device string
	// Devices 是当前已连接控制端的名字（最多 MaxSessions 个）。
	Devices           []string
	Pending           []Pending
	PairedDevices     []PairedDevice
	AudioStatus       string
	AudioReady        bool
	Level             float64
	Received          uint64
	Concealed         uint64
	MousePackets      uint64
	AudioPackets      uint64
	Error             string
	InjectionP95MS    float64
	BufferedFrames    int
	MaxBufferedFrames int
}
type InputController interface {
	Move(int32, int32, int32, int32) error
	Button(string, bool) error
	Chord(string) error
	Hold(string, bool) error
	ReleaseAll()
}
type asyncKeyboard interface {
	ChordAsync(string, func(error)) error
	HoldAsync(string, bool, func(error)) error
	// KeyAsync carries a full-keyboard key state: "key_down" or "key_up".
	KeyAsync(string, string, func(error)) error
	StartVoice(string, string, string, string, func(error)) error
	StopVoice(string, func(error)) error
	KeyboardStatus() keyboard.Status
	ConfigureBackend(string) error
}
type asyncTouchpad interface {
	ZoomAsync(int, func(error)) error
	GestureAsync(string, func(error)) error
}
type AudioEngine interface {
	Configure(string, float64)
	Status() (string, bool, float64, uint64, uint64)
	Begin(uint64) error
	End(uint64, time.Duration, func())
	Abort()
	AbortRecording(uint64)
	Run(context.Context)
	Push(uint64, uint64, []byte)
	BufferStats() (int, int)
}
type recordingVoice struct {
	Mode, Start, Stop string
	ProfileID, Name   string
	StopDelayMS       int
}

func voiceFor(c config.Config, m Message) (recordingVoice, error) {
	var p config.VoiceProfile
	var found bool
	if m.ProfileID != "" {
		if m.Revision != c.Revision {
			return recordingVoice{}, fmt.Errorf("语音配置已更新，请重试")
		}
		for _, candidate := range c.Voice.Profiles {
			if candidate.ID == m.ProfileID && candidate.Enabled {
				p, found = candidate, true
				break
			}
		}
	} else {
		p, found = c.Voice.FirstEnabled(m.Mode)
	}
	if !found {
		return recordingVoice{}, fmt.Errorf("该语音配置未启用或已失效")
	}
	v := recordingVoice{ProfileID: p.ID, Name: p.Name, Mode: p.Mode, StopDelayMS: c.Voice.StopDelayMS}
	if p.Mode == "hold" {
		v.Start = p.HoldKey
	} else {
		v.Start, v.Stop = p.ToggleStartKey, p.ToggleStopKey
	}
	return v, nil
}

type Server struct {
	mu           sync.Mutex
	lifecycleMu  sync.Mutex
	ctx          context.Context
	generation   uint64
	cfg          config.Config
	dir          string
	host         string
	name         string
	pin          string
	cert         tls.Certificate
	paired       map[string]pairedRecord
	pairEpoch    map[string]uint64
	buttonMu     sync.Mutex
	buttonOwners map[string]map[uint64]bool
	pending      map[string]*Pending
	// sessions 是当前已连接的控制端，按会话 id 索引，最多 MaxSessions 个。
	sessions     map[uint64]*session
	http         *http.Server
	wss          *http.Server
	udp          *net.UDPConn
	cancel       context.CancelFunc
	audioDone    chan struct{}
	running      bool
	lastError    string
	Input        InputController
	Audio        AudioEngine
	mousePackets atomic.Uint64
	audioPackets atomic.Uint64
	processingNS atomic.Uint64
	processingN  atomic.Uint64
	latencyMu    sync.Mutex
	latencies    [4096]int64
	latencyCount uint64
	// apkData/apkName/apkSHA 是内嵌的 Android 安装包，供配对网页扫码下载。
	apkName    string
	apkData    []byte
	apkSHA     string
	apkVersion string
}
type session struct {
	closeOnce                sync.Once
	mu                       sync.Mutex
	writeMu                  sync.Mutex
	s                        *Server
	ctx                      context.Context
	cancel                   context.CancelFunc
	ws                       *websocket.Conn
	id                       uint64
	deviceID, name           string
	mouse, audio             *protocol.Codec
	mouseReplay, audioReplay protocol.ReplayWindow
	active                   bool
	lastSeen                 atomic.Int64
	epoch                    uint32
	last                     protocol.Movement
	pending                  *protocol.Movement
	recording                uint64
	voice                    recordingVoice
	ending                   bool
	preparing                bool
	highestMouse             uint64
	hasMouse                 bool
	// holds 记录当前被手机按住的快捷键，会话结束或收到 hold_stop 时释放。
	holds          map[string]string
	keys           map[string]int
	input          InputController
	audioRecording uint64
}

func LocalIPs() []net.IP {
	result := []net.IP{net.ParseIP("127.0.0.1")}
	is, _ := net.Interfaces()
	for _, i := range is {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		as, _ := i.Addrs()
		for _, a := range as {
			ip, _, e := net.ParseCIDR(a.String())
			if e == nil && ip.To4() != nil && !ip.IsLinkLocalUnicast() {
				result = append(result, ip.To4())
			}
		}
	}
	sort.Slice(result[1:], func(i, j int) bool { return result[i+1].String() < result[j+1].String() })
	return result
}
func New(dir string) (*Server, error) {
	c, err := config.Load(dir)
	if err != nil {
		return nil, err
	}
	ips := LocalIPs()
	cert, pin, err := secure.Certificate(dir, ips)
	if err != nil {
		return nil, err
	}
	host := "127.0.0.1"
	if len(ips) > 1 {
		host = ips[1].String()
	}
	name, _ := os.Hostname()
	s := &Server{cfg: c, dir: dir, host: host, name: name, pin: pin, cert: cert, pairEpoch: map[string]uint64{}, pending: map[string]*Pending{}, sessions: map[uint64]*session{}, Input: input.New(), Audio: audio.New()}
	if s.paired, err = loadPaired(dir); err != nil {
		return nil, err
	}
	s.Audio.Configure(c.AudioDevice, c.Gain)
	return s, nil
}
func (s *Server) Config() config.Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.cfg
	c.Sensitivity = config.PointerBaseSensitivity
	c.Shortcuts = append([]config.Shortcut(nil), c.Shortcuts...)
	c.Voice = c.Voice.Normalized()
	return c
}
func (s *Server) Update(c config.Config) error {
	c.Sensitivity = config.PointerBaseSensitivity
	c.Voice = c.Voice.Normalized()
	if err := c.Validate(); err != nil {
		return err
	}
	for _, k := range c.Shortcuts {
		if !k.Enabled {
			continue
		}
		if _, e := input.ParseChord(k.Chord); e != nil {
			return e
		}
	}
	for _, k := range c.Shortcuts {
		if k.Enabled {
			if e := keyboard.Validate(k.Chord, c.KeyboardBackend); e != nil {
				return e
			}
		}
	}
	for _, p := range c.Voice.Profiles {
		if !p.Enabled {
			continue
		}
		for _, k := range p.Keys() {
			if _, e := input.ParseChord(k); e != nil {
				return fmt.Errorf("%s: %w", p.Name, e)
			}
			if e := keyboard.Validate(k, c.KeyboardBackend); e != nil {
				return fmt.Errorf("%s: %w", p.Name, e)
			}
		}
	}
	s.mu.Lock()
	old := s.cfg
	if a, ok := s.Input.(asyncKeyboard); ok && old.KeyboardBackend != c.KeyboardBackend {
		if e := a.ConfigureBackend(c.KeyboardBackend); e != nil {
			s.mu.Unlock()
			return e
		}
	}
	c.Revision = old.Revision + 1
	if e := config.Save(s.dir, c); e != nil {
		if a, ok := s.Input.(asyncKeyboard); ok && old.KeyboardBackend != c.KeyboardBackend {
			_ = a.ConfigureBackend(old.KeyboardBackend)
		}
		s.mu.Unlock()
		return e
	}
	s.cfg = c
	sessions := make([]*session, 0, len(s.sessions))
	for _, ss := range s.sessions {
		sessions = append(sessions, ss)
	}
	running := s.running
	s.mu.Unlock()
	s.Audio.Configure(c.AudioDevice, c.Gain)
	if old.HTTPPort != c.HTTPPort || old.WSSPort != c.WSSPort || old.UDPPort != c.UDPPort {
		if running {
			s.Stop()
			return s.Start()
		}
	} else {
		for _, ss := range sessions {
			_ = ss.send(map[string]any{"type": "config", "config": c})
		}
	}
	return nil
}
func (s *Server) PairURL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fmt.Sprintf("http://%s:%d/pair", s.host, s.cfg.HTTPPort)
}
func (s *Server) QRURI() string {
	return s.PairURL()
}
func (s *Server) metadata() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return map[string]any{"version": ControlVersion, "host": s.host, "wss_port": s.cfg.WSSPort, "http_port": s.cfg.HTTPPort, "pin": s.pin, "name": s.name}
}
func (s *Server) Start() error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return nil
	}
	h, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", s.cfg.HTTPPort))
	if err != nil {
		return err
	}
	w, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", s.cfg.WSSPort))
	if err != nil {
		h.Close()
		return err
	}
	u, err := net.ListenUDP("udp", &net.UDPAddr{Port: s.cfg.UDPPort})
	if err != nil {
		h.Close()
		w.Close()
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.ctx = ctx
	s.cancel = cancel
	s.udp = u
	_ = u.SetReadBuffer(256 * 1024)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, map[string]any{"app": "TapDeck", "version": ControlVersion})
	})
	mux.HandleFunc("GET /api/pair-info", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		jsonResponse(w, s.metadata())
	})
	mux.HandleFunc("GET /pair", s.pairPage)
	mux.HandleFunc("GET /apk", s.apkFile)
	mux.HandleFunc("GET /apk/qr.png", s.apkQR)
	s.http = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	wm := http.NewServeMux()
	wm.HandleFunc("GET /ws", s.connect)
	s.wss = &http.Server{Handler: wm, ReadHeaderTimeout: 5 * time.Second, TLSConfig: &tls.Config{Certificates: []tls.Certificate{s.cert}, MinVersion: tls.VersionTLS12}}
	hs := s.http
	ws := s.wss
	certCfg := ws.TLSConfig
	s.running = true
	s.lastError = ""
	go func() {
		if e := hs.Serve(h); e != nil && !errors.Is(e, http.ErrServerClosed) {
			log.Print(e)
		}
	}()
	go func() {
		if e := ws.Serve(tls.NewListener(w, certCfg)); e != nil && !errors.Is(e, http.ErrServerClosed) {
			log.Print(e)
		}
	}()
	go s.udpLoop(ctx, u)
	s.audioDone = make(chan struct{})
	audioDone := s.audioDone
	go func() { defer close(audioDone); s.Audio.Run(ctx) }()
	log.Printf("listening http=%d wss=%d udp=%d", s.cfg.HTTPPort, s.cfg.WSSPort, s.cfg.UDPPort)
	return nil
}
func (s *Server) Stop() {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.generation++
	cancel := s.cancel
	h, w, u, sessions, audioDone := s.http, s.wss, s.udp, s.sessions, s.audioDone
	s.sessions = map[uint64]*session{}
	s.pending = map[string]*Pending{}
	s.mu.Unlock()
	cancel()
	for _, ss := range sessions {
		ss.close("接收端已停止")
	}
	h.Close()
	w.Close()
	u.Close()
	s.Audio.Abort()
	s.Input.ReleaseAll()
	<-audioDone
}
func (s *Server) Snapshot() Snapshot {
	s.mu.Lock()
	v := Snapshot{Running: s.running, URL: fmt.Sprintf("http://%s:%d/pair", s.host, s.cfg.HTTPPort), Error: s.lastError}
	for _, ss := range s.sessions {
		v.Devices = append(v.Devices, ss.name)
	}
	sort.Strings(v.Devices)
	v.Device = strings.Join(v.Devices, "、")
	for _, p := range s.pending {
		v.Pending = append(v.Pending, *p)
	}
	sort.Slice(v.Pending, func(i, j int) bool { return v.Pending[i].Expires.Before(v.Pending[j].Expires) })
	v.PairedDevices = s.pairedDevicesLocked()
	s.mu.Unlock()
	v.AudioStatus, v.AudioReady, v.Level, v.Received, v.Concealed = s.Audio.Status()
	v.MousePackets = s.mousePackets.Load()
	v.AudioPackets = s.audioPackets.Load()
	v.BufferedFrames, v.MaxBufferedFrames = s.Audio.BufferStats()
	s.latencyMu.Lock()
	n := min(s.latencyCount, uint64(len(s.latencies)))
	samples := append([]int64(nil), s.latencies[:n]...)
	s.latencyMu.Unlock()
	if len(samples) > 0 {
		sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
		v.InjectionP95MS = float64(samples[int(float64(len(samples)-1)*0.95)]) / 1e6
	}
	return v
}
func (s *Server) Approve(id string, allow bool) {
	s.mu.Lock()
	p := s.pending[id]
	s.mu.Unlock()
	if p != nil {
		select {
		case p.answer <- allow:
		default:
		}
	}
}
func jsonResponse(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
func randomID() uint64 {
	v := binary.LittleEndian.Uint64(secure.Random(8))
	if v == 0 {
		return 1
	}
	return v
}
func Code(pin string, client, server []byte) string {
	p, _ := hex.DecodeString(pin)
	b := append([]byte("tapdeck-pair-v1"), p...)
	b = append(b, client...)
	b = append(b, server...)
	h := sha256.Sum256(b)
	hexCode := strings.ToUpper(hex.EncodeToString(h[:16]))
	groups := []string{}
	for i := 0; i < len(hexCode); i += 4 {
		groups = append(groups, hexCode[i:i+4])
	}
	return strings.Join(groups, " ")
}
func read(ctx context.Context, c *websocket.Conn) (Message, error) {
	_, b, e := c.Read(ctx)
	if e != nil {
		return Message{}, e
	}
	var m Message
	e = json.Unmarshal(b, &m)
	return m, e
}
func write(ctx context.Context, c *websocket.Conn, v any) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	return c.Write(ctx, websocket.MessageText, b)
}
func (s *Server) connect(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Origin") != "" {
		http.Error(w, "native client required", http.StatusForbidden)
		return
	}
	ws, e := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
	if e != nil {
		return
	}
	defer ws.CloseNow()
	ws.SetReadLimit(8192)
	s.mu.Lock()
	parent, generation, running := s.ctx, s.generation, s.running
	s.mu.Unlock()
	if !running || parent == nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	firstCtx, firstCancel := context.WithTimeout(ctx, 10*time.Second)
	hello, e := read(firstCtx, ws)
	firstCancel()
	if e != nil || hello.Type != "hello" {
		return
	}
	if hello.Version != ControlVersion {
		_ = write(ctx, ws, map[string]any{"type": "error", "code": "version_mismatch", "reason": "协议版本不匹配，请同时升级 PC 和 Android 至 TapDeck 0.2", "version": ControlVersion})
		return
	}
	if len(hello.DeviceID) < 8 || len(hello.DeviceID) > 128 || len(hello.Name) > 128 {
		return
	}
	// 名额检查放在配对之前：满了就立刻拒绝，不再让新设备走一遍配对。
	// 同一台设备自己重连不受限（下面会替换它的旧会话）。
	s.mu.Lock()
	full := s.running && len(s.sessions) >= MaxSessions
	owns := false
	for _, old := range s.sessions {
		if old.deviceID == hello.DeviceID {
			owns = true
		}
	}
	s.mu.Unlock()
	if full && !owns {
		_ = write(ctx, ws, map[string]any{"type": "error", "code": "too_many_clients", "reason": fmt.Sprintf("接收端最多同时连接 %d 个控制端，请先断开其中一个", MaxSessions), "limit": MaxSessions})
		return
	}
	s.mu.Lock()
	hash := s.paired[hello.DeviceID].Hash
	authorizationEpoch := s.pairEpoch[hello.DeviceID]
	trusted := hash != "" && subtle.ConstantTimeCompare([]byte(hash), []byte(secure.Hash(hello.Token))) == 1
	s.mu.Unlock()
	if hello.Token != "" && !trusted {
		_ = write(ctx, ws, map[string]any{"type": "error", "code": "pairing_revoked", "reason": "配对凭据已失效，请重新连接并由电脑允许"})
		return
	}
	token := ""
	if !trusted {
		client, e := base64.RawURLEncoding.DecodeString(hello.ClientNonce)
		if e != nil || len(client) != 32 {
			return
		}
		serverNonce := secure.Random(32)
		id := fmt.Sprintf("%016x", randomID())
		p := &Pending{ID: id, DeviceID: hello.DeviceID, Name: hello.Name, Code: Code(s.pin, client, serverNonce), Expires: time.Now().Add(120 * time.Second), answer: make(chan bool, 1)}
		pairCtx, pairCancel := context.WithDeadline(ctx, p.Expires)
		defer pairCancel()
		p.cancel = pairCancel
		p.revoke = func() {
			noticeCtx, stopNotice := context.WithTimeout(ctx, time.Second)
			defer stopNotice()
			_ = write(noticeCtx, ws, map[string]any{"type": "error", "code": "pairing_revoked", "reason": "电脑已解除此设备配对，请重新连接并由电脑允许"})
			pairCancel()
		}
		s.mu.Lock()
		if len(s.pending) >= 4 || s.generation != generation || s.pairEpoch[hello.DeviceID] != authorizationEpoch {
			s.mu.Unlock()
			return
		}
		s.pending[id] = p
		s.mu.Unlock()
		defer func() { s.mu.Lock(); delete(s.pending, id); s.mu.Unlock() }()
		if e = write(pairCtx, ws, map[string]any{"type": "pair_challenge", "request_id": id, "server_nonce": base64.RawURLEncoding.EncodeToString(serverNonce), "code": p.Code, "qr_verified": false}); e != nil {
			return
		}
		// 手机上只需要看到校验码，不再需要点“一致”：这里直接等 PC 端确认。
		// （老版本 App 仍会发一条 pair_confirm，控制循环会把它当未知消息忽略。）
		select {
		case allow := <-p.answer:
			if !allow {
				return
			}
		case <-pairCtx.Done():
			return
		}
		s.mu.Lock()
		delete(s.pending, id)
		s.mu.Unlock()
		token = base64.RawURLEncoding.EncodeToString(secure.Random(32))
	}
	ss := &session{s: s, ctx: ctx, cancel: cancel, ws: ws, id: randomID(), deviceID: hello.DeviceID, name: hello.Name, active: true, holds: map[string]string{}, keys: map[string]int{}}
	if agent, ok := s.Input.(*keyboard.Agent); ok {
		ss.input = agent.ForOwner(fmt.Sprintf("%016x", ss.id))
	}
	mouseKey, audioKey := secure.Random(32), secure.Random(32)
	var mp, ap [4]byte
	copy(mp[:], secure.Random(4))
	copy(ap[:], secure.Random(4))
	ss.mouse, _ = protocol.NewCodec(mouseKey, mp, ss.id, protocol.Mouse)
	ss.audio, _ = protocol.NewCodec(audioKey, ap, ss.id, protocol.Audio)
	ss.lastSeen.Store(time.Now().UnixNano())
	s.lifecycleMu.Lock()
	s.mu.Lock()
	if !s.running || ctx.Err() != nil {
		s.mu.Unlock()
		s.lifecycleMu.Unlock()
		return
	}
	if s.generation != generation || s.pairEpoch[hello.DeviceID] != authorizationEpoch || (trusted && s.paired[hello.DeviceID].Hash != hash) {
		s.mu.Unlock()
		s.lifecycleMu.Unlock()
		_ = write(ctx, ws, map[string]any{"type": "error", "code": "pairing_revoked", "reason": "配对授权已变化，请重新连接并由电脑允许"})
		return
	}
	// 同一台设备重连时替换掉它的旧会话，避免占掉两个名额。
	var replaced []*session
	for _, old := range s.sessions {
		if old.deviceID == ss.deviceID {
			replaced = append(replaced, old)
		}
	}
	if len(s.sessions)-len(replaced) >= MaxSessions {
		s.mu.Unlock()
		s.lifecycleMu.Unlock()
		_ = write(ctx, ws, map[string]any{"type": "error", "code": "too_many_clients", "reason": fmt.Sprintf("接收端最多同时连接 %d 个控制端，请先断开其中一个", MaxSessions), "limit": MaxSessions})
		return
	}
	next := clonePaired(s.paired)
	record := next[hello.DeviceID]
	if !trusted {
		record.Hash, record.PairedAt = secure.Hash(token), time.Now().UTC()
	}
	if strings.TrimSpace(hello.Name) != "" {
		record.Name = hello.Name
	}
	record.LastConnectedAt = time.Now().UTC()
	next[hello.DeviceID] = record
	if e = savePaired(s.dir, next); e != nil {
		s.mu.Unlock()
		s.lifecycleMu.Unlock()
		_ = write(ctx, ws, map[string]any{"type": "error", "code": "pairing_save_failed", "reason": "配对信息保存失败：" + e.Error()})
		return
	}
	s.paired = next
	if !trusted {
		s.pairEpoch[hello.DeviceID]++
	}
	for _, old := range replaced {
		delete(s.sessions, old.id)
	}
	s.sessions[ss.id] = ss
	cfg := s.cfg
	s.mu.Unlock()
	for _, old := range replaced {
		old.close("同一设备重新连接")
	}
	s.lifecycleMu.Unlock()
	defer func() {
		s.lifecycleMu.Lock()
		defer s.lifecycleMu.Unlock()
		s.mu.Lock()
		_, present := s.sessions[ss.id]
		delete(s.sessions, ss.id)
		s.mu.Unlock()
		if present {
			ss.close("连接结束")
		}
	}()
	if e = ss.send(map[string]any{"type": "ready", "session": fmt.Sprintf("%016x", ss.id), "token": token, "config": cfg, "udp_port": cfg.UDPPort, "mouse_key": base64.RawURLEncoding.EncodeToString(mouseKey), "audio_key": base64.RawURLEncoding.EncodeToString(audioKey), "mouse_prefix": base64.RawURLEncoding.EncodeToString(mp[:]), "audio_prefix": base64.RawURLEncoding.EncodeToString(ap[:]), "features": append(ss.touchpadFeatures(), "voice_profiles"), "double_click_ms": input.DoubleClickTime()}); e != nil {
		return
	}
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if time.Since(time.Unix(0, ss.lastSeen.Load())) > time.Second {
					ss.close("心跳超时")
					return
				}
				ss.mu.Lock()
				if ss.active && ss.recording != 0 {
					status, ready, _, _, _ := s.Audio.Status()
					if a, ok := ss.controller().(asyncKeyboard); ok && !a.KeyboardStatus().Ready {
						ready = false
						status = a.KeyboardStatus().Error
					}
					if !ready {
						id, voice := ss.recording, ss.voice
						ss.recording = 0
						ss.ending = false
						if id != 0 {
							ss.abortAudio()
						}
						if a, ok := ss.controller().(asyncKeyboard); ok {
							_ = a.StopVoice(ss.voiceToken(id), func(error) {})
						} else if voice.Mode == "hold" {
							_ = ss.controller().Hold(voice.Start, false)
						}
						if _, ok := ss.controller().(asyncKeyboard); !ok && voice.Mode == "toggle" {
							_ = ss.controller().Chord(voice.Stop)
						}
						_ = ss.send(map[string]any{"type": "mic_error", "recording": fmt.Sprintf("%016x", id), "reason": status})
					}
				}
				ss.mu.Unlock()
			}
		}
	}()
	for ctx.Err() == nil {
		m, e := read(ctx, ws)
		if e != nil {
			return
		}
		ss.lastSeen.Store(time.Now().UnixNano())
		if e = ss.handle(m); e != nil {
			_ = ss.send(map[string]any{"type": "error", "reason": e.Error()})
			// A failed barrier cannot leave the peers on different mouse epochs.
			if m.Type == "mouse_button" || m.Type == "zoom" || m.Type == "gesture" {
				return
			}
		}
	}
}
func (ss *session) send(v any) error {
	ss.writeMu.Lock()
	defer ss.writeMu.Unlock()
	ctx, cancel := context.WithTimeout(ss.ctx, 2*time.Second)
	defer cancel()
	return write(ctx, ss.ws, v)
}
func (ss *session) close(reason string) {
	ss.closeOnce.Do(func() {
		ss.mu.Lock()
		ss.active = false
		recording, voice := ss.recording, ss.voice
		holds, keys := ss.holds, ss.keys
		ss.recording = 0
		ss.ending = false
		ss.holds = map[string]string{}
		if recording != 0 {
			ss.abortAudio()
		}
		ss.mu.Unlock()
		ss.cancel()
		ss.ws.CloseNow()
		ss.releaseInputs(holds, keys, recording, voice)
		log.Printf("session closed: %s", reason)
	})
}
func (ss *session) move(m protocol.Movement) error {
	if m.Epoch < ss.epoch {
		return nil
	}
	if m.Epoch > ss.epoch {
		if m.Epoch <= ss.epoch+4 {
			cp := m
			ss.pending = &cp
		}
		return nil
	}
	px := func(v int64) int64 { return int64(math.Round(float64(v) / 1024)) }
	dx, dy, sx, sy := px(m.X)-px(ss.last.X), px(m.Y)-px(ss.last.Y), px(m.ScrollX)-px(ss.last.ScrollX), px(m.ScrollY)-px(ss.last.ScrollY)
	for _, v := range []int64{dx, dy, sx, sy} {
		if v > 100000 || v < -100000 {
			return fmt.Errorf("位移跨度异常")
		}
	}
	if e := ss.controller().Move(int32(dx), int32(dy), int32(sx), int32(sy)); e != nil {
		return e
	}
	ss.last = m
	return nil
}
func (ss *session) handle(m Message) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if !ss.active {
		return fmt.Errorf("会话已关闭")
	}
	switch m.Type {
	case "heartbeat":
		return ss.send(map[string]any{"type": "heartbeat", "tick": m.Tick})
	case "mouse_button":
		return ss.controlBarrier(m, func() error { return ss.mouseButton(m.Button, m.Down) })
	case "zoom", "gesture":
		if m.Type == "zoom" && (m.Steps == 0 || m.Steps < -4 || m.Steps > 4) {
			return fmt.Errorf("无效缩放步进")
		}
		if m.Type == "gesture" && m.Action != "up" && m.Action != "down" {
			return fmt.Errorf("无效三指手势")
		}
		control, ok := ss.controller().(asyncTouchpad)
		if !ok {
			return fmt.Errorf("电脑端不支持触控板扩展手势，请升级")
		}
		done := func(err error) {
			if err != nil {
				_ = ss.send(map[string]any{"type": "error", "code": "touchpad_error", "reason": err.Error()})
			}
		}
		return ss.controlBarrier(m, func() error {
			if m.Type == "zoom" {
				return control.ZoomAsync(m.Steps, done)
			}
			return control.GestureAsync(m.Action, done)
		})
	case "shortcut":
		c := ss.s.Config()
		if m.Revision != c.Revision {
			_ = ss.send(map[string]any{"type": "config", "config": c})
			return fmt.Errorf("配置已更新，请重新点击")
		}
		if m.Slot < 0 || m.Slot >= len(c.Shortcuts) {
			return fmt.Errorf("无效快捷键")
		}
		if !c.Shortcuts[m.Slot].Enabled {
			return fmt.Errorf("此快捷键已禁用")
		}
		if a, ok := ss.controller().(asyncKeyboard); ok {
			return a.ChordAsync(c.Shortcuts[m.Slot].Chord, func(e error) {
				if e != nil {
					_ = ss.send(map[string]any{"type": "error", "reason": e.Error()})
				}
			})
		}
		return ss.controller().Chord(c.Shortcuts[m.Slot].Chord)
	case "shortcut_hold_start":
		// 按住快捷键时发送：键保持按下，直到收到对应的 hold_stop 或会话结束。
		c := ss.s.Config()
		if m.Revision != c.Revision {
			_ = ss.send(map[string]any{"type": "config", "config": c})
			return fmt.Errorf("配置已更新，请重新按住")
		}
		if m.Hold == "" || len(m.Hold) > 64 {
			return fmt.Errorf("无效按住编号")
		}
		if m.Slot < 0 || m.Slot >= len(c.Shortcuts) {
			return fmt.Errorf("无效快捷键")
		}
		if !c.Shortcuts[m.Slot].Enabled {
			return fmt.Errorf("此快捷键已禁用")
		}
		if _, held := ss.holds[m.Hold]; held {
			return nil
		}
		chord := c.Shortcuts[m.Slot].Chord
		if a, ok := ss.controller().(asyncKeyboard); ok {
			if e := a.HoldAsync(chord, true, func(e error) {
				if e != nil {
					_ = ss.send(map[string]any{"type": "error", "reason": e.Error()})
				}
			}); e != nil {
				return e
			}
		} else if e := ss.controller().Hold(chord, true); e != nil {
			return e
		}
		ss.holds[m.Hold] = chord
		return nil
	case "key_down", "key_up":
		// 全键盘：单个按键的按下 / 抬起，修饰键与普通键都是独立状态，
		// 因此可以按住不放（Windows 会重复）或同时按住多个键。
		c := ss.s.Config()
		if m.Revision != c.Revision {
			_ = ss.send(map[string]any{"type": "config", "config": c})
			return fmt.Errorf("配置已更新，请重试")
		}
		text := strings.TrimSpace(m.Text)
		if text == "" || len(text) > 128 {
			return fmt.Errorf("无效按键")
		}
		if _, err := input.ParseChord(text); err != nil {
			return err
		}
		if a, ok := ss.controller().(asyncKeyboard); ok {
			if ss.keys == nil {
				ss.keys = map[string]int{}
			}
			if m.Type == "key_up" && ss.keys[text] == 0 {
				return nil
			}
			err := a.KeyAsync(m.Type, text, func(e error) {
				if e != nil {
					_ = ss.send(map[string]any{"type": "error", "reason": e.Error()})
				}
			})
			if err == nil {
				if m.Type == "key_down" {
					ss.keys[text]++
				} else {
					ss.keys[text]--
				}
			}
			return err
		}
		return fmt.Errorf("键盘不可用")
	case "shortcut_hold_stop":
		if m.Hold == "" {
			return fmt.Errorf("无效按住编号")
		}
		chord, held := ss.holds[m.Hold]
		if !held {
			return nil
		}
		delete(ss.holds, m.Hold)
		if a, ok := ss.controller().(asyncKeyboard); ok {
			return a.HoldAsync(chord, false, func(e error) {
				if e != nil {
					_ = ss.send(map[string]any{"type": "error", "reason": e.Error()})
				}
			})
		}
		return ss.controller().Hold(chord, false)
	case "mic_start":
		id, e := strconv.ParseUint(m.Recording, 16, 64)
		if e != nil || id == 0 {
			return fmt.Errorf("无效录音编号")
		}
		if ss.recording != 0 {
			return fmt.Errorf("上一轮语音尚未结束")
		}
		cfg := ss.s.Config()
		voice, e := voiceFor(cfg, m)
		if e != nil {
			if err := ss.send(map[string]any{"type": "mic_error", "reason": e.Error(), "recording": m.Recording}); err != nil {
				return err
			}
			return ss.send(map[string]any{"type": "config", "config": cfg})
		}
		audioID := randomID()
		if e = ss.s.Audio.Begin(audioID); e != nil {
			return ss.send(map[string]any{"type": "mic_error", "reason": e.Error(), "recording": m.Recording})
		}
		ss.recording = id
		ss.audioRecording = audioID
		ss.ending = false
		ss.voice = voice
		if a, ok := ss.controller().(asyncKeyboard); ok {
			ss.preparing = true
			done := func(e error) {
				ss.mu.Lock()
				defer ss.mu.Unlock()
				if !ss.active || ss.recording != id || ss.ending {
					return
				}
				ss.preparing = false
				if e != nil {
					ss.abortAudio()
					ss.recording = 0
					_ = ss.send(map[string]any{"type": "mic_error", "reason": e.Error(), "recording": m.Recording})
					return
				}
				_ = ss.send(map[string]any{"type": "mic_ready", "recording": m.Recording})
			}
			if e = a.StartVoice(ss.voiceToken(id), voice.Mode, voice.Start, voice.Stop, done); e != nil {
				ss.abortAudio()
				ss.recording = 0
				ss.preparing = false
				return ss.send(map[string]any{"type": "mic_error", "reason": e.Error(), "recording": m.Recording})
			}
			return nil
		}
		if ss.voice.Mode == "hold" {
			e = ss.controller().Hold(ss.voice.Start, true)
		} else if ss.voice.Mode == "toggle" {
			e = ss.controller().Chord(ss.voice.Start)
		}
		if e != nil {
			ss.abortAudio()
			ss.recording = 0
			if ss.voice.Mode == "hold" {
				_ = ss.controller().Hold(ss.voice.Start, false)
			}
			return ss.send(map[string]any{"type": "mic_error", "reason": e.Error(), "recording": m.Recording})
		}
		return ss.send(map[string]any{"type": "mic_ready", "recording": m.Recording})
	case "mic_stop", "mic_abort":
		id, e := strconv.ParseUint(m.Recording, 16, 64)
		if e != nil || id == 0 || id != ss.recording {
			return nil
		}
		if m.Type == "mic_stop" && ss.ending {
			return nil
		}
		v := ss.voice
		if a, ok := ss.controller().(asyncKeyboard); ok {
			finish := func() {
				ss.mu.Lock()
				if !ss.active || ss.recording != id {
					ss.mu.Unlock()
					return
				}
				ss.mu.Unlock()
				e := a.StopVoice(ss.voiceToken(id), func(e error) { ss.completeVoice(id, m.Recording, e) })
				if e != nil {
					ss.completeVoice(id, m.Recording, e)
				}
			}
			ss.ending = true
			if m.Type == "mic_abort" || ss.preparing {
				ss.abortAudio()
				go finish()
			} else {
				ss.s.Audio.End(ss.audioRecording, time.Duration(v.StopDelayMS)*time.Millisecond, finish)
			}
			return nil
		}
		finish := func() {
			ss.mu.Lock()
			defer ss.mu.Unlock()
			if !ss.active || ss.recording != id {
				return
			}
			if v.Mode == "hold" {
				_ = ss.controller().Hold(v.Start, false)
			} else if v.Mode == "toggle" {
				_ = ss.controller().Chord(v.Stop)
			}
			ss.recording = 0
			ss.ending = false
			_ = ss.send(map[string]any{"type": "mic_stopped", "recording": m.Recording})
		}
		if m.Type == "mic_abort" {
			ss.abortAudio()
			if v.Mode == "hold" {
				_ = ss.controller().Hold(v.Start, false)
			} else if v.Mode == "toggle" {
				_ = ss.controller().Chord(v.Stop)
			}
			ss.recording = 0
			ss.ending = false
			return ss.send(map[string]any{"type": "mic_stopped", "recording": m.Recording})
		}
		ss.ending = true
		ss.s.Audio.End(ss.audioRecording, time.Duration(v.StopDelayMS)*time.Millisecond, finish)
	case "pair_confirm":
		// 老版本 App 配对时仍会发这条消息（现在改由 PC 端确认）：收到就忽略。
		return nil
	default:
		return fmt.Errorf("未知消息类型")
	}
	return nil
}
func (ss *session) voiceToken(id uint64) string { return fmt.Sprintf("%016x/%016x", ss.id, id) }
func (ss *session) completeVoice(id uint64, recording string, e error) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if !ss.active || ss.recording != id {
		return
	}
	ss.recording = 0
	ss.ending = false
	ss.preparing = false
	typeName := "mic_stopped"
	reason := ""
	if e != nil {
		typeName = "mic_error"
		reason = e.Error()
	}
	_ = ss.send(map[string]any{"type": typeName, "recording": recording, "reason": reason})
}
func (s *Server) udpLoop(ctx context.Context, u *net.UDPConn) {
	packets := make(chan []byte, 12)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case p := <-packets:
				s.audioPacket(p)
			}
		}
	}()
	b := make([]byte, 1201)
	for ctx.Err() == nil {
		n, _, e := u.ReadFromUDP(b)
		if e != nil {
			return
		}
		if n < protocol.HeaderSize || n > 1200 {
			continue
		}
		if b[4] == protocol.Audio {
			p := append([]byte(nil), b[:n]...)
			select {
			case packets <- p:
			default:
			}
		} else if b[4] == protocol.Mouse {
			s.mousePacket(b[:n])
		}
	}
}
func (s *Server) activeSession(p []byte) *session {
	s.mu.Lock()
	defer s.mu.Unlock()
	// UDP 包头里带会话 id，多台控制端各自路由到自己的会话。
	return s.sessions[binary.LittleEndian.Uint64(p[8:])]
}
func (s *Server) mousePacket(p []byte) {
	start := time.Now()
	ss := s.activeSession(p)
	if ss == nil {
		return
	}
	seq, b, e := ss.mouse.Open(p)
	if e != nil {
		return
	}
	m, e := protocol.ParseMovement(b)
	if e != nil {
		return
	}
	ss.mu.Lock()
	if ss.active && (!ss.hasMouse || seq > ss.highestMouse) {
		ss.highestMouse = seq
		ss.hasMouse = true
		_ = ss.move(m)
		s.mousePackets.Add(1)
	}
	ss.mu.Unlock()
	elapsed := time.Since(start)
	s.processingNS.Add(uint64(elapsed))
	s.processingN.Add(1)
	s.latencyMu.Lock()
	s.latencies[s.latencyCount%uint64(len(s.latencies))] = int64(elapsed)
	s.latencyCount++
	s.latencyMu.Unlock()
}
func (s *Server) audioPacket(p []byte) {
	ss := s.activeSession(p)
	if ss == nil {
		return
	}
	seq, b, e := ss.audio.Open(p)
	if e != nil || len(b) != 976 {
		return
	}
	ss.mu.Lock()
	ok := ss.active && ss.recording != 0 && binary.LittleEndian.Uint64(b) == ss.recording && ss.audioReplay.Accept(seq)
	if !ok {
		ss.mu.Unlock()
		return
	}
	s.Audio.Push(ss.audioRecording, binary.LittleEndian.Uint64(b[8:]), b[16:])
	ss.mu.Unlock()
	s.audioPackets.Add(1)
}
func (s *Server) pairPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; frame-ancestors 'none'")
	fmt.Fprint(w, strings.Replace(pairHTML, "<!--APK-->", s.apkSection(r), 1))
}

const pairHTML = `<!doctype html><html lang="zh"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>TapDeck 配对</title><style>body{font:18px system-ui;margin:0;background:#f2f5f9;color:#172331}main{max-width:520px;margin:8vh auto;padding:28px}h1{font-size:36px}h2{font-size:22px;margin:36px 0 8px}a,button{display:block;padding:18px;margin:20px 0;border:0;border-radius:12px;background:#175cd3;color:white;text-align:center;text-decoration:none;font:inherit}a.plain{display:inline;padding:0;margin:0;background:none;color:#175cd3;text-decoration:underline}input{box-sizing:border-box;width:100%;padding:12px;font:inherit}p{line-height:1.7}p.hint{font-size:15px;color:#5b6675;word-break:break-all}img.qr{display:block;margin:16px auto;background:white;border:1px solid #cbd5e1;border-radius:12px;padding:8px}code{font-size:16px}</style><main><h1>TapDeck</h1><p>手机与电脑连接同一 Wi-Fi。首次使用先安装 Android 端，再打开应用，核对校验码并在电脑允许连接。</p><!--APK--><h2>连接电脑</h2><a id="open" href="#">打开 TapDeck 连接</a><p>如果浏览器无法打开应用，请在 TapDeck 连接页输入下面的网址：</p><input readonly id="address"><p id="status">正在获取连接信息…</p></main><script>document.getElementById('address').value=location.origin+'/pair';fetch('/api/pair-info',{cache:'no-store'}).then(r=>r.json()).then(m=>{const q=new URLSearchParams({v:m.version,host:location.hostname,wss:m.wss_port,http:m.http_port,pin:m.pin});let uri='tapdeck://pair?'+q;const a=document.getElementById('open');a.href=uri;if(/Chrome/.test(navigator.userAgent))a.href='intent://pair?'+q+'#Intent;scheme=tapdeck;package=com.yuncii.tapdeck;end';document.getElementById('status').textContent='电脑：'+m.name}).catch(()=>document.getElementById('status').textContent='无法连接电脑，请检查网络后刷新页面。')</script></html>`

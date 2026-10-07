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
	"net/url"
	"os"
	"path/filepath"
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
	Tick        int64  `json:"tick,omitempty"`
	Reason      string `json:"reason,omitempty"`
}
type Pending struct {
	ID      string
	Name    string
	Code    string
	Expires time.Time
	answer  chan bool
}
type Snapshot struct {
	Running           bool
	URL               string
	Device            string
	Pending           []Pending
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
type AudioEngine interface {
	Configure(string, float64)
	Status() (string, bool, float64, uint64, uint64)
	Begin(uint64) error
	End(uint64, time.Duration, func())
	Abort()
	Run(context.Context)
	Push(uint64, uint64, []byte)
	BufferStats() (int, int)
}
type recordingVoice struct {
	Mode, Start, Stop string
	StopDelayMS       int
}

func voiceFor(c config.Voice, mode string) (recordingVoice, error) {
	switch mode {
	case "hold":
		return recordingVoice{Mode: mode, Start: c.HoldKey, StopDelayMS: c.StopDelayMS}, nil
	case "toggle":
		return recordingVoice{Mode: mode, Start: c.ToggleStartKey, Stop: c.ToggleStopKey, StopDelayMS: c.StopDelayMS}, nil
	default:
		return recordingVoice{}, fmt.Errorf("无效语音模式")
	}
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
	tokens       map[string]string
	secret       string
	secretUntil  time.Time
	pending      map[string]*Pending
	current      *session
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
	apkName string
	apkData []byte
	apkSHA  string
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
	holds map[string]string
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
	s := &Server{cfg: c, dir: dir, host: host, name: name, pin: pin, cert: cert, tokens: map[string]string{}, pending: map[string]*Pending{}, Input: input.New(), Audio: audio.New()}
	if b, e := os.ReadFile(filepath.Join(dir, "paired.json")); e == nil {
		if e = json.Unmarshal(b, &s.tokens); e != nil {
			return nil, e
		}
	} else if !os.IsNotExist(e) {
		return nil, e
	}
	s.Audio.Configure(c.AudioDevice, c.Gain)
	s.RefreshQR()
	return s, nil
}
func (s *Server) Config() config.Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.cfg
	c.Shortcuts = append([]config.Shortcut(nil), c.Shortcuts...)
	return c
}
func (s *Server) Update(c config.Config) error {
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
	for _, k := range []string{c.Voice.HoldKey, c.Voice.ToggleStartKey, c.Voice.ToggleStopKey} {
		if _, e := input.ParseChord(k); e != nil {
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
	for _, k := range []string{c.Voice.HoldKey, c.Voice.ToggleStartKey, c.Voice.ToggleStopKey} {
		if e := keyboard.Validate(k, c.KeyboardBackend); e != nil {
			return e
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
	cur := s.current
	running := s.running
	s.mu.Unlock()
	s.Audio.Configure(c.AudioDevice, c.Gain)
	if old.HTTPPort != c.HTTPPort || old.WSSPort != c.WSSPort || old.UDPPort != c.UDPPort {
		if running {
			s.Stop()
			return s.Start()
		}
	} else if cur != nil {
		_ = cur.send(map[string]any{"type": "config", "config": c})
	}
	return nil
}
func (s *Server) RefreshQR() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.secret = base64.RawURLEncoding.EncodeToString(secure.Random(32))
	s.secretUntil = time.Now().Add(120 * time.Second)
}
func (s *Server) PairURL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fmt.Sprintf("http://%s:%d/pair", s.host, s.cfg.HTTPPort)
}
func (s *Server) QRURI() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	q := url.Values{"v": {strconv.Itoa(ControlVersion)}, "host": {s.host}, "wss": {strconv.Itoa(s.cfg.WSSPort)}, "http": {strconv.Itoa(s.cfg.HTTPPort)}, "pin": {s.pin}, "secret": {s.secret}}
	return "tapdeck://pair?" + q.Encode()
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
	h, w, u, cur, audioDone := s.http, s.wss, s.udp, s.current, s.audioDone
	s.current = nil
	s.pending = map[string]*Pending{}
	s.mu.Unlock()
	cancel()
	if cur != nil {
		cur.close("接收端已停止")
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
	if s.current != nil {
		v.Device = s.current.name
	}
	for _, p := range s.pending {
		v.Pending = append(v.Pending, *p)
	}
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
func (s *Server) Unpair() error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	s.mu.Lock()
	s.generation++
	s.tokens = map[string]string{}
	b, _ := json.Marshal(s.tokens)
	e := config.AtomicWrite(filepath.Join(s.dir, "paired.json"), b)
	cur := s.current
	s.current = nil
	for _, p := range s.pending {
		select {
		case p.answer <- false:
		default:
		}
	}
	s.secret = ""
	s.mu.Unlock()
	if cur != nil {
		cur.close("已解除配对")
	}
	s.RefreshQR()
	return e
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
	s.mu.Lock()
	hash := s.tokens[hello.DeviceID]
	trusted := hash != "" && subtle.ConstantTimeCompare([]byte(hash), []byte(secure.Hash(hello.Token))) == 1
	qr := hello.Secret != "" && time.Now().Before(s.secretUntil) && subtle.ConstantTimeCompare([]byte(hello.Secret), []byte(s.secret)) == 1
	if qr {
		s.secret = ""
	}
	s.mu.Unlock()
	token := ""
	if !trusted {
		client, e := base64.RawURLEncoding.DecodeString(hello.ClientNonce)
		if e != nil || len(client) != 32 {
			return
		}
		serverNonce := secure.Random(32)
		id := fmt.Sprintf("%016x", randomID())
		p := &Pending{ID: id, Name: hello.Name, Code: Code(s.pin, client, serverNonce), Expires: time.Now().Add(120 * time.Second), answer: make(chan bool, 1)}
		s.mu.Lock()
		if len(s.pending) >= 4 {
			s.mu.Unlock()
			return
		}
		s.pending[id] = p
		s.mu.Unlock()
		defer func() { s.mu.Lock(); delete(s.pending, id); s.mu.Unlock() }()
		pairCtx, pairCancel := context.WithDeadline(ctx, p.Expires)
		defer pairCancel()
		if e = write(pairCtx, ws, map[string]any{"type": "pair_challenge", "request_id": id, "server_nonce": base64.RawURLEncoding.EncodeToString(serverNonce), "code": p.Code, "qr_verified": qr}); e != nil {
			return
		}
		confirm, e := read(pairCtx, ws)
		if e != nil || confirm.Type != "pair_confirm" || confirm.Code != p.Code {
			return
		}
		if !qr {
			select {
			case allow := <-p.answer:
				if !allow {
					return
				}
			case <-pairCtx.Done():
				return
			}
		}
		token = base64.RawURLEncoding.EncodeToString(secure.Random(32))
		s.mu.Lock()
		if !s.running || s.generation != generation || ctx.Err() != nil {
			s.mu.Unlock()
			return
		}
		s.tokens[hello.DeviceID] = secure.Hash(token)
		b, _ := json.Marshal(s.tokens)
		e = config.AtomicWrite(filepath.Join(s.dir, "paired.json"), b)
		delete(s.pending, id)
		s.mu.Unlock()
		if e != nil {
			log.Print(e)
			return
		}
	}
	ss := &session{s: s, ctx: ctx, cancel: cancel, ws: ws, id: randomID(), deviceID: hello.DeviceID, name: hello.Name, active: true, holds: map[string]string{}}
	mouseKey, audioKey := secure.Random(32), secure.Random(32)
	var mp, ap [4]byte
	copy(mp[:], secure.Random(4))
	copy(ap[:], secure.Random(4))
	ss.mouse, _ = protocol.NewCodec(mouseKey, mp, ss.id, protocol.Mouse)
	ss.audio, _ = protocol.NewCodec(audioKey, ap, ss.id, protocol.Audio)
	ss.lastSeen.Store(time.Now().UnixNano())
	s.lifecycleMu.Lock()
	s.mu.Lock()
	if !s.running || s.generation != generation || ctx.Err() != nil {
		s.mu.Unlock()
		s.lifecycleMu.Unlock()
		return
	}
	old := s.current
	s.current = nil
	s.mu.Unlock()
	if old != nil {
		old.close("新的设备连接")
	}
	s.mu.Lock()
	s.current = ss
	cfg := s.cfg
	s.mu.Unlock()
	s.lifecycleMu.Unlock()
	defer func() {
		s.lifecycleMu.Lock()
		defer s.lifecycleMu.Unlock()
		s.mu.Lock()
		current := s.current == ss
		if current {
			s.current = nil
		}
		s.mu.Unlock()
		if current {
			ss.close("连接结束")
		}
	}()
	if e = ss.send(map[string]any{"type": "ready", "session": fmt.Sprintf("%016x", ss.id), "token": token, "config": cfg, "udp_port": cfg.UDPPort, "mouse_key": base64.RawURLEncoding.EncodeToString(mouseKey), "audio_key": base64.RawURLEncoding.EncodeToString(audioKey), "mouse_prefix": base64.RawURLEncoding.EncodeToString(mp[:]), "audio_prefix": base64.RawURLEncoding.EncodeToString(ap[:])}); e != nil {
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
					if a, ok := s.Input.(asyncKeyboard); ok && !a.KeyboardStatus().Ready {
						ready = false
						status = a.KeyboardStatus().Error
					}
					if !ready {
						id, voice := ss.recording, ss.voice
						ss.recording = 0
						ss.ending = false
						s.Audio.Abort()
						if a, ok := s.Input.(asyncKeyboard); ok {
							_ = a.StopVoice(ss.voiceToken(id), func(error) {})
						} else if voice.Mode == "hold" {
							_ = s.Input.Hold(voice.Start, false)
						}
						if _, ok := s.Input.(asyncKeyboard); !ok && voice.Mode == "toggle" {
							_ = s.Input.Chord(voice.Stop)
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
			if m.Type == "mouse_button" {
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
		ss.recording = 0
		ss.ending = false
		ss.holds = map[string]string{}
		ss.mu.Unlock()
		ss.cancel()
		ss.ws.CloseNow()
		ss.s.Audio.Abort()
		if _, ok := ss.s.Input.(asyncKeyboard); !ok && recording != 0 && voice.Mode == "toggle" {
			_ = ss.s.Input.Chord(voice.Stop)
		}
		ss.s.Input.ReleaseAll()
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
	if e := ss.s.Input.Move(int32(dx), int32(dy), int32(sx), int32(sy)); e != nil {
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
		if m.Epoch != ss.epoch || m.NextEpoch != m.Epoch+1 {
			return fmt.Errorf("鼠标控制分段失配")
		}
		snap := protocol.Movement{Epoch: m.Epoch, X: m.X, Y: m.Y, ScrollX: m.ScrollX, ScrollY: m.ScrollY}
		if e := ss.move(snap); e != nil {
			return e
		}
		if e := ss.s.Input.Button(m.Button, m.Down); e != nil {
			return e
		}
		ss.epoch = m.NextEpoch
		ss.last.Epoch = ss.epoch
		if ss.pending != nil {
			p := ss.pending
			ss.pending = nil
			return ss.move(*p)
		}
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
		if a, ok := ss.s.Input.(asyncKeyboard); ok {
			return a.ChordAsync(c.Shortcuts[m.Slot].Chord, func(e error) {
				if e != nil {
					_ = ss.send(map[string]any{"type": "error", "reason": e.Error()})
				}
			})
		}
		return ss.s.Input.Chord(c.Shortcuts[m.Slot].Chord)
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
		if a, ok := ss.s.Input.(asyncKeyboard); ok {
			if e := a.HoldAsync(chord, true, func(e error) {
				if e != nil {
					_ = ss.send(map[string]any{"type": "error", "reason": e.Error()})
				}
			}); e != nil {
				return e
			}
		} else if e := ss.s.Input.Hold(chord, true); e != nil {
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
		if a, ok := ss.s.Input.(asyncKeyboard); ok {
			return a.KeyAsync(m.Type, text, func(e error) {
				if e != nil {
					_ = ss.send(map[string]any{"type": "error", "reason": e.Error()})
				}
			})
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
		if a, ok := ss.s.Input.(asyncKeyboard); ok {
			return a.HoldAsync(chord, false, func(e error) {
				if e != nil {
					_ = ss.send(map[string]any{"type": "error", "reason": e.Error()})
				}
			})
		}
		return ss.s.Input.Hold(chord, false)
	case "mic_start":
		id, e := strconv.ParseUint(m.Recording, 16, 64)
		if e != nil || id == 0 {
			return fmt.Errorf("无效录音编号")
		}
		if ss.recording != 0 {
			return fmt.Errorf("上一轮语音尚未结束")
		}
		voice, e := voiceFor(ss.s.Config().Voice, m.Mode)
		if e != nil {
			return e
		}
		if e = ss.s.Audio.Begin(id); e != nil {
			return ss.send(map[string]any{"type": "mic_error", "reason": e.Error(), "recording": m.Recording})
		}
		ss.recording = id
		ss.ending = false
		ss.voice = voice
		if a, ok := ss.s.Input.(asyncKeyboard); ok {
			ss.preparing = true
			done := func(e error) {
				ss.mu.Lock()
				defer ss.mu.Unlock()
				if !ss.active || ss.recording != id || ss.ending {
					return
				}
				ss.preparing = false
				if e != nil {
					ss.s.Audio.Abort()
					ss.recording = 0
					_ = ss.send(map[string]any{"type": "mic_error", "reason": e.Error(), "recording": m.Recording})
					return
				}
				_ = ss.send(map[string]any{"type": "mic_ready", "recording": m.Recording})
			}
			if e = a.StartVoice(ss.voiceToken(id), voice.Mode, voice.Start, voice.Stop, done); e != nil {
				ss.s.Audio.Abort()
				ss.recording = 0
				ss.preparing = false
				return ss.send(map[string]any{"type": "mic_error", "reason": e.Error(), "recording": m.Recording})
			}
			return nil
		}
		if ss.voice.Mode == "hold" {
			e = ss.s.Input.Hold(ss.voice.Start, true)
		} else if ss.voice.Mode == "toggle" {
			e = ss.s.Input.Chord(ss.voice.Start)
		}
		if e != nil {
			ss.s.Audio.Abort()
			ss.recording = 0
			if ss.voice.Mode == "hold" {
				_ = ss.s.Input.Hold(ss.voice.Start, false)
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
		if a, ok := ss.s.Input.(asyncKeyboard); ok {
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
				ss.s.Audio.Abort()
				go finish()
			} else {
				ss.s.Audio.End(id, time.Duration(v.StopDelayMS)*time.Millisecond, finish)
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
				_ = ss.s.Input.Hold(v.Start, false)
			} else if v.Mode == "toggle" {
				_ = ss.s.Input.Chord(v.Stop)
			}
			ss.recording = 0
			ss.ending = false
			_ = ss.send(map[string]any{"type": "mic_stopped", "recording": m.Recording})
		}
		if m.Type == "mic_abort" {
			ss.s.Audio.Abort()
			if v.Mode == "hold" {
				_ = ss.s.Input.Hold(v.Start, false)
			} else if v.Mode == "toggle" {
				_ = ss.s.Input.Chord(v.Stop)
			}
			ss.recording = 0
			ss.ending = false
			return ss.send(map[string]any{"type": "mic_stopped", "recording": m.Recording})
		}
		ss.ending = true
		ss.s.Audio.End(id, time.Duration(v.StopDelayMS)*time.Millisecond, finish)
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
	ss := s.current
	if ss == nil || binary.LittleEndian.Uint64(p[8:]) != ss.id {
		return nil
	}
	return ss
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
	ok := ss.active && ss.audioReplay.Accept(seq)
	ss.mu.Unlock()
	if !ok {
		return
	}
	s.Audio.Push(binary.LittleEndian.Uint64(b), binary.LittleEndian.Uint64(b[8:]), b[16:])
	s.audioPackets.Add(1)
}
func (s *Server) pairPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; frame-ancestors 'none'")
	fmt.Fprint(w, strings.Replace(pairHTML, "<!--APK-->", s.apkSection(r), 1))
}

const pairHTML = `<!doctype html><html lang="zh"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>TapDeck 配对</title><style>body{font:18px system-ui;margin:0;background:#f2f5f9;color:#172331}main{max-width:520px;margin:8vh auto;padding:28px}h1{font-size:36px}h2{font-size:22px;margin:36px 0 8px}a,button{display:block;padding:18px;margin:20px 0;border:0;border-radius:12px;background:#175cd3;color:white;text-align:center;text-decoration:none;font:inherit}a.plain{display:inline;padding:0;margin:0;background:none;color:#175cd3;text-decoration:underline}input{box-sizing:border-box;width:100%;padding:12px;font:inherit}p{line-height:1.7}p.hint{font-size:15px;color:#5b6675;word-break:break-all}img.qr{display:block;margin:16px auto;background:white;border:1px solid #cbd5e1;border-radius:12px;padding:8px}code{font-size:16px}</style><main><h1>TapDeck</h1><p>打开应用后，核对电脑和 Android 显示的校验码，并在电脑允许连接。</p><a id="open" href="#">打开 TapDeck 配对</a><p>如果浏览器无法打开应用，请在 TapDeck 连接页输入下面的网址：</p><input readonly id="address"><p id="status">正在获取连接信息…</p><!--APK--></main><script>document.getElementById('address').value=location.origin+'/pair';fetch('/api/pair-info',{cache:'no-store'}).then(r=>r.json()).then(m=>{const q=new URLSearchParams({v:m.version,host:location.hostname,wss:m.wss_port,http:m.http_port,pin:m.pin});let uri='tapdeck://pair?'+q;const a=document.getElementById('open');a.href=uri;if(/Chrome/.test(navigator.userAgent))a.href='intent://pair?'+q+'#Intent;scheme=tapdeck;package=com.yuncii.tapdeck;end';document.getElementById('status').textContent='电脑：'+m.name}).catch(()=>document.getElementById('status').textContent='无法连接电脑，请检查网络后刷新页面。')</script></html>`

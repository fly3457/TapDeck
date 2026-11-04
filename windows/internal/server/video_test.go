package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"tapdeck/internal/video"
)

type testVideoSource struct{ frames chan video.Frame }

func (s *testVideoSource) Next() (video.Frame, error) {
	f, ok := <-s.frames
	if !ok {
		return video.Frame{}, io.EOF
	}
	return f, nil
}
func (s *testVideoSource) Close() error { return nil }

type testVideoLauncher struct{ launched chan struct{} }

func (testVideoLauncher) Available() bool { return true }
func (testVideoLauncher) Prepare(r video.Request) (video.Plan, error) {
	return video.NewPlan(video.Display{Width: 2560, Height: 1440}, r, "h264_nvenc")
}
func (l testVideoLauncher) Launch(ctx context.Context, p video.Plan) (video.Source, error) {
	src := &testVideoSource{frames: make(chan video.Frame, 4)}
	src.frames <- video.Frame{Config: true, Data: []byte{0, 0, 0, 1, 0x67}}
	src.frames <- video.Frame{Key: true, Data: []byte{0, 0, 0, 1, 0x65}}
	close(l.launched)
	return src, nil
}
func (testVideoLauncher) Cursor(video.Display) (int, int, bool) { return 5, 6, true }

func readUntil(t *testing.T, ctx context.Context, conn *websocket.Conn, kind string) map[string]json.RawMessage {
	t.Helper()
	for {
		m := testRead(t, ctx, conn)
		if stringField(m, "type") == kind {
			return m
		}
	}
}

func TestVideoStreamLifecycle(t *testing.T) {
	launcher := testVideoLauncher{launched: make(chan struct{})}
	s, client := testReceiver(t, func(s *Server) { s.Video = video.NewManager(launcher) })
	conn, ctx := testSocket(t, s, client, "")
	token := testPair(t, s, conn, ctx)
	conn.CloseNow()
	conn, ctx = testSocket(t, s, client, token)
	ready := testRead(t, ctx, conn)
	var features []string
	_ = json.Unmarshal(ready["features"], &features)
	if !slices.Contains(features, VideoFeature) {
		t.Fatalf("video not advertised: %v", features)
	}
	session := stringField(ready, "session")

	if err := write(ctx, conn, Message{Type: "video_start", Quality: "smooth", MaxWidth: 1080, MaxHeight: 2400}); err != nil {
		t.Fatal(err)
	}
	vr := readUntil(t, ctx, conn, "video_ready")
	if string(vr["width"]) != "1920" || string(vr["height"]) != "1080" || string(vr["screen_width"]) != "2560" {
		t.Fatalf("video_ready %v", vr)
	}

	dial := func() *websocket.Conn {
		v, _, err := websocket.Dial(ctx, fmt.Sprintf("wss://127.0.0.1:%d/video", s.cfg.WSSPort), &websocket.DialOptions{HTTPClient: client})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { v.CloseNow() })
		return v
	}
	// 错误令牌不能连接，也不会消耗这次流。
	bad := dial()
	_ = write(ctx, bad, Message{Type: "video_hello", Session: session, Stream: stringField(vr, "stream"), Token: base64.RawURLEncoding.EncodeToString(make([]byte, 32))})
	if stringField(testRead(t, ctx, bad), "code") != "unauthorized" {
		t.Fatal("wrong token accepted")
	}

	v := dial()
	if err := write(ctx, v, Message{Type: "video_hello", Session: session, Stream: stringField(vr, "stream"), Token: stringField(vr, "token")}); err != nil {
		t.Fatal(err)
	}
	if format := testRead(t, ctx, v); stringField(format, "type") != "video_format" || string(format["width"]) != "1920" {
		t.Fatalf("format %v", format)
	}
	var config, key, cursor bool
	for !(config && key && cursor) {
		kind, b, err := v.Read(ctx)
		if err != nil || kind != websocket.MessageBinary {
			t.Fatalf("read %v %v", kind, err)
		}
		switch {
		case b[0] == video.MessageFrame && b[1]&video.FlagConfig != 0:
			config = true
		case b[0] == video.MessageFrame && b[1]&video.FlagKey != 0:
			key = config
		case b[0] == video.MessageCursor:
			cursor = true
		}
	}
	if s.VideoDevice() != "Test device" {
		t.Fatalf("streaming device %q", s.VideoDevice())
	}
	// 控制连接断开后视频随之结束。
	conn.CloseNow()
	deadline, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for {
		if _, _, err := v.Read(deadline); err != nil {
			break
		}
	}
	if deadline.Err() != nil || s.Video.Owner() != 0 {
		t.Fatal("video outlived its control session")
	}
}

func TestVideoDisabledOnPC(t *testing.T) {
	s, client := testReceiver(t, func(s *Server) {
		s.Video = video.NewManager(testVideoLauncher{launched: make(chan struct{})})
		s.cfg.VideoDisabled = true
	})
	conn, ctx := testSocket(t, s, client, "")
	testPair(t, s, conn, ctx)
	_ = write(ctx, conn, Message{Type: "video_start"})
	if m := readUntil(t, ctx, conn, "video_error"); stringField(m, "code") != "disabled" {
		t.Fatalf("%v", m)
	}
}

type pointerInput struct {
	quietInput
	mu     sync.Mutex
	events []string
}

func (p *pointerInput) MoveTo(x, y int32) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, fmt.Sprintf("to:%d,%d", x, y))
	return nil
}
func (p *pointerInput) Button(b string, down bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, fmt.Sprintf("%s:%v", b, down))
	return nil
}
func (p *pointerInput) snapshot() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.events...)
}

func TestVideoPointerClicksAtDisplayPosition(t *testing.T) {
	var pauses []time.Duration
	old := pointerPause
	pointerPause = func(d time.Duration) { pauses = append(pauses, d) }
	t.Cleanup(func() { pointerPause = old })
	in := &pointerInput{}
	s, client := testReceiver(t, func(s *Server) {
		s.Input = in
		s.Video = video.NewManager(testVideoLauncher{launched: make(chan struct{})})
	})
	conn, ctx := testSocket(t, s, client, "")
	token := testPair(t, s, conn, ctx)
	conn.CloseNow()
	conn, ctx = testSocket(t, s, client, token)
	ready := testRead(t, ctx, conn)
	sid, _ := strconv.ParseUint(stringField(ready, "session"), 16, 64)

	// 画面未打开：只提示，屏障仍推进，不注入。
	_ = write(ctx, conn, Message{Type: "pointer", Action: "click", PX: 10, PY: 20, Epoch: 0, NextEpoch: 1})
	if m := readUntil(t, ctx, conn, "error"); stringField(m, "code") != "touchpad_error" {
		t.Fatalf("%v", m)
	}
	if _, err := s.Video.Start(sid, video.Request{}); err != nil {
		t.Fatal(err)
	}
	_ = write(ctx, conn, Message{Type: "pointer", Action: "click", PX: 100, PY: 200, Epoch: 1, NextEpoch: 2})
	_ = write(ctx, conn, Message{Type: "pointer", Action: "right_click", PX: 99999, PY: -5, Epoch: 2, NextEpoch: 3})
	_ = write(ctx, conn, Message{Type: "pointer", Action: "move", PX: 5, PY: 6, Epoch: 3, NextEpoch: 4})
	_ = write(ctx, conn, Message{Type: "heartbeat", Tick: 7})
	readUntil(t, ctx, conn, "heartbeat")
	// 先移入并停顿（悬停），再补一次同位置移动、按下、按住、抬起。
	want := []string{"to:100,200", "to:100,200", "left:true", "left:false", "to:2559,0", "to:2559,0", "right:true", "right:false", "to:5,6"}
	if got := in.snapshot(); !slices.Equal(got, want) {
		t.Fatalf("events %v, want %v", got, want)
	}
	if !slices.Equal(pauses, []time.Duration{pointerHoverDelay, pointerPressDuration, pointerHoverDelay, pointerPressDuration}) {
		t.Fatalf("pauses %v", pauses)
	}
}

type fakeClicker struct {
	mu    sync.Mutex
	calls []string
	fail  bool
}

func (f *fakeClicker) record(s string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, s)
}
func (f *fakeClicker) Down(d video.Display, x, y int32) error {
	f.record(fmt.Sprintf("down %d,%d", x, y))
	return nil
}
func (f *fakeClicker) Up(d video.Display, x, y int32) error {
	f.record(fmt.Sprintf("up %d,%d", x, y))
	return nil
}
func (f *fakeClicker) Release() { f.record("release") }
func (f *fakeClicker) snapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeClicker) Move(d video.Display, x, y int32) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fmt.Sprintf("move %d,%d", x, y))
	return nil
}

func (f *fakeClicker) Click(d video.Display, x, y int32, right bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fmt.Sprintf("%d,%d,%v", x, y, right))
	if f.fail {
		return fmt.Errorf("no device")
	}
	return nil
}

func TestVideoPointerPrefersHardwareClicker(t *testing.T) {
	old := pointerPause
	pointerPause = func(time.Duration) {}
	t.Cleanup(func() { pointerPause = old })
	for _, fail := range []bool{false, true} {
		in, clicker := &pointerInput{}, &fakeClicker{fail: fail}
		s, client := testReceiver(t, func(s *Server) {
			s.Input = in
			s.Pointer = clicker
			s.Video = video.NewManager(testVideoLauncher{launched: make(chan struct{})})
		})
		conn, ctx := testSocket(t, s, client, "")
		testPair(t, s, conn, ctx)
		s.mu.Lock()
		var sid uint64
		for id := range s.sessions {
			sid = id
		}
		s.mu.Unlock()
		if _, err := s.Video.Start(sid, video.Request{}); err != nil {
			t.Fatal(err)
		}
		_ = write(ctx, conn, Message{Type: "pointer", Action: "right_click", PX: 30, PY: 40, Epoch: 0, NextEpoch: 1})
		_ = write(ctx, conn, Message{Type: "heartbeat", Tick: 1})
		readUntil(t, ctx, conn, "heartbeat")
		clicker.mu.Lock()
		calls := append([]string(nil), clicker.calls...)
		clicker.mu.Unlock()
		if !slices.Equal(calls, []string{"30,40,true"}) {
			t.Fatalf("fail=%v clicker calls %v", fail, calls)
		}
		// 硬件点击成功时不再走 SendInput；失败时退回 SendInput 完成同一次点击。
		got := in.snapshot()
		if fail != (len(got) > 0) {
			t.Fatalf("fail=%v SendInput events %v", fail, got)
		}
	}
}

func TestVideoPointerDragAndReleaseOnDisconnect(t *testing.T) {
	old := pointerPause
	pointerPause = func(time.Duration) {}
	t.Cleanup(func() { pointerPause = old })
	clicker := &fakeClicker{}
	s, client := testReceiver(t, func(s *Server) {
		s.Pointer = clicker
		s.Video = video.NewManager(testVideoLauncher{launched: make(chan struct{})})
	})
	conn, ctx := testSocket(t, s, client, "")
	testPair(t, s, conn, ctx)
	s.mu.Lock()
	var sid uint64
	for id := range s.sessions {
		sid = id
	}
	s.mu.Unlock()
	if _, err := s.Video.Start(sid, video.Request{}); err != nil {
		t.Fatal(err)
	}
	// 双击后拖动：第二次按下 down，移动 move，松开 up。
	steps := []Message{
		{Type: "pointer", Action: "click", PX: 10, PY: 10},
		{Type: "pointer", Action: "down", PX: 10, PY: 10},
		{Type: "pointer", Action: "move", PX: 50, PY: 60},
		{Type: "pointer", Action: "up", PX: 50, PY: 60},
		{Type: "pointer", Action: "down", PX: 70, PY: 80},
	}
	for i, m := range steps {
		m.Epoch, m.NextEpoch = uint32(i), uint32(i+1)
		_ = write(ctx, conn, m)
	}
	_ = write(ctx, conn, Message{Type: "heartbeat", Tick: 1})
	readUntil(t, ctx, conn, "heartbeat")
	// 拖拽中断开：会话结束时松开虚拟鼠标。
	conn.CloseNow()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && !slices.Contains(clicker.snapshot(), "release") {
		time.Sleep(20 * time.Millisecond)
	}
	want := []string{"10,10,false", "down 10,10", "move 50,60", "up 50,60", "down 70,80", "release"}
	if got := clicker.snapshot(); !slices.Equal(got, want) {
		t.Fatalf("calls %v, want %v", got, want)
	}
}

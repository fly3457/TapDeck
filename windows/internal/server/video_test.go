package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"slices"
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

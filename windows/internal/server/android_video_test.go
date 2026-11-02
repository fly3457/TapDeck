package server

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"tapdeck/internal/secure"
)

// TestAndroidVideoFixture 为 Android 模拟器提供一台使用真实截屏与内嵌 ffmpeg 编码的接收端，
// 自动批准配对；鼠标与按键只计数、不注入本机。仅在设置 TAPDECK_ANDROID_VIDEO_INFO 时运行。
func TestAndroidVideoFixture(t *testing.T) {
	infoPath := os.Getenv("TAPDECK_ANDROID_VIDEO_INFO")
	if infoPath == "" {
		t.Skip("opt-in Android video fixture")
	}
	in := &androidFixtureInput{held: map[string]int{}}
	s, _ := testReceiver(t, func(s *Server) {
		s.Input = in
		s.Audio = &manualAudio{}
		s.name = "Fixture-Video-PC"
		s.host = "10.0.2.2"
		var err error
		s.cert, s.pin, err = secure.Certificate(s.dir, append(LocalIPs(), net.ParseIP("10.0.2.2")))
		if err != nil {
			t.Fatal(err)
		}
	})
	if !s.Video.Available() {
		t.Fatal("内嵌 ffmpeg 不可用")
	}
	s.Video.Prewarm()
	done := make(chan struct{})
	var finish sync.Once
	mux := http.NewServeMux()
	mux.HandleFunc("GET /state", func(w http.ResponseWriter, r *http.Request) {
		snapshot := s.Snapshot()
		held, events := in.snapshot()
		_ = json.NewEncoder(w).Encode(map[string]any{"connected": len(snapshot.Devices), "video_device": snapshot.VideoDevice,
			"mouse_packets": snapshot.MousePackets, "held": held, "events": events})
	})
	mux.HandleFunc("POST /finish", func(w http.ResponseWriter, r *http.Request) {
		finish.Do(func() { close(done) })
		w.WriteHeader(http.StatusNoContent)
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	admin := &http.Server{Handler: mux}
	go admin.Serve(listener)
	t.Cleanup(func() { admin.Close() })
	data, _ := json.Marshal(map[string]any{"url": s.PairURL(), "admin": fmt.Sprintf("http://127.0.0.1:%d", listener.Addr().(*net.TCPAddr).Port)})
	if err = os.WriteFile(infoPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Log("Android video fixture ready")
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	seen := map[string]bool{}
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			for _, p := range s.Snapshot().Pending {
				if !seen[p.ID] {
					seen[p.ID] = true
					s.Approve(p.ID, true)
				}
			}
		}
	}
}

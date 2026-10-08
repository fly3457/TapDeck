package server

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"tapdeck/internal/secure"
)

// Opt-in Android integration fixture. Independent certificates/config directories,
// real HTTP/WSS/UDP protocol, and inert input keep the user's desktop untouched.
type androidFixtureInput struct {
	mu     sync.Mutex
	held   map[string]int
	events int
}

func (p *androidFixtureInput) Move(int32, int32, int32, int32) error {
	p.mu.Lock()
	p.events++
	p.mu.Unlock()
	return nil
}
func (p *androidFixtureInput) Button(key string, down bool) error { return p.Hold("mouse:"+key, down) }
func (p *androidFixtureInput) Chord(string) error                 { p.mu.Lock(); p.events++; p.mu.Unlock(); return nil }
func (p *androidFixtureInput) Hold(key string, down bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events++
	if down {
		p.held[key]++
	} else if p.held[key] > 1 {
		p.held[key]--
	} else {
		delete(p.held, key)
	}
	return nil
}
func (p *androidFixtureInput) ReleaseAll() { p.mu.Lock(); clear(p.held); p.mu.Unlock() }
func (p *androidFixtureInput) snapshot() (int, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.held), p.events
}

func TestAndroidMultiPCFixture(t *testing.T) {
	infoPath := os.Getenv("TAPDECK_ANDROID_FIXTURE_INFO")
	if infoPath == "" {
		t.Skip("opt-in Android integration fixture")
	}
	var receivers [3]*Server
	var inputs [3]*androidFixtureInput
	var modes, approvals [3]atomic.Int32
	for i := range receivers {
		inputs[i] = &androidFixtureInput{held: map[string]int{}}
		s, _ := testReceiver(t, func(s *Server) {
			s.Input = inputs[i]
			s.Audio = &manualAudio{}
			s.name = fmt.Sprintf("Fixture-PC-%d", i+1)
			s.host = "10.0.2.2"
			s.pairTimeout = 2 * time.Second
			s.cfg.Shortcuts[0].Label = fmt.Sprintf("PC%d 按键", i+1)
			s.cfg.Voice.Profiles[0].Name = fmt.Sprintf("PC%d 语音", i+1)
			var err error
			s.cert, s.pin, err = secure.Certificate(s.dir, append(LocalIPs(), net.ParseIP("10.0.2.2")))
			if err != nil {
				t.Fatal(err)
			}
		})
		receivers[i] = s
	}
	done := make(chan struct{})
	var finish sync.Once
	mux := http.NewServeMux()
	mux.HandleFunc("GET /state", func(w http.ResponseWriter, r *http.Request) {
		var items []map[string]any
		for i, s := range receivers {
			snapshot := s.Snapshot()
			held, events := inputs[i].snapshot()
			items = append(items, map[string]any{"connected": len(snapshot.Devices), "held": held, "events": events,
				"mouse_packets": snapshot.MousePackets, "approvals": approvals[i].Load(), "paired": len(snapshot.PairedDevices)})
		}
		_ = json.NewEncoder(w).Encode(items)
	})
	mux.HandleFunc("POST /control", func(w http.ResponseWriter, r *http.Request) {
		i, _ := strconv.Atoi(r.URL.Query().Get("pc"))
		if i < 0 || i >= len(receivers) {
			http.Error(w, "unknown PC", http.StatusBadRequest)
			return
		}
		var err error
		switch r.URL.Query().Get("action") {
		case "stop":
			receivers[i].Stop()
		case "start":
			err = receivers[i].Start()
		case "revoke":
			err = receivers[i].Unpair()
		case "reject":
			modes[i].Store(1)
		case "wait":
			modes[i].Store(2)
		case "allow":
			modes[i].Store(0)
		case "finish":
			finish.Do(func() { close(done) })
		default:
			http.Error(w, "unknown action", 400)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	admin := &http.Server{Handler: mux}
	go admin.Serve(listener)
	t.Cleanup(func() { admin.Close() })
	info := map[string]any{"urls": []string{receivers[0].PairURL(), receivers[1].PairURL(), receivers[2].PairURL()},
		"admin": fmt.Sprintf("http://10.0.2.2:%d", listener.Addr().(*net.TCPAddr).Port)}
	data, _ := json.Marshal(info)
	if err = os.WriteFile(infoPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Log("Android fixture ready; only temporary pairing stores and inert inputs are used")
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	seen := map[string]bool{}
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			for i, s := range receivers {
				for _, p := range s.Snapshot().Pending {
					if modes[i].Load() == 2 || seen[p.ID] {
						continue
					}
					seen[p.ID] = true
					allow := modes[i].Load() == 0
					if allow {
						approvals[i].Add(1)
					}
					s.Approve(p.ID, allow)
				}
			}
		}
	}
}

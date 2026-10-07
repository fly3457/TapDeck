package server

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"tapdeck/internal/config"
	"testing"
	"time"
)

type voiceInput struct {
	quietInput
	mu     sync.Mutex
	events []string
}

func (v *voiceInput) Chord(k string) error {
	if k != "" {
		v.mu.Lock()
		v.events = append(v.events, "tap:"+k)
		v.mu.Unlock()
	}
	return nil
}
func (v *voiceInput) Hold(k string, down bool) error {
	if k != "" {
		v.mu.Lock()
		v.events = append(v.events, fmt.Sprintf("hold:%s:%t", k, down))
		v.mu.Unlock()
	}
	return nil
}
func (v *voiceInput) result() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return strings.Join(v.events, ",")
}

type manualAudio struct {
	mu   sync.Mutex
	done func()
	ends int
}

func (*manualAudio) Configure(string, float64) {}
func (*manualAudio) Status() (string, bool, float64, uint64, uint64) {
	return "test audio", true, 0, 0, 0
}
func (*manualAudio) Begin(uint64) error { return nil }
func (a *manualAudio) End(_ uint64, _ time.Duration, done func()) {
	a.mu.Lock()
	a.done = done
	a.ends++
	a.mu.Unlock()
}
func (*manualAudio) Abort()                      {}
func (*manualAudio) AbortRecording(uint64)       {}
func (*manualAudio) Run(ctx context.Context)     { <-ctx.Done() }
func (*manualAudio) Push(uint64, uint64, []byte) {}
func (*manualAudio) BufferStats() (int, int)     { return 0, 0 }
func (a *manualAudio) drain() {
	a.mu.Lock()
	done := a.done
	a.mu.Unlock()
	if done != nil {
		done()
	}
}

func TestVoiceModesSnapshotAndIdempotentStop(t *testing.T) {
	for _, mode := range []string{"hold", "toggle"} {
		t.Run(mode, func(t *testing.T) {
			fake, engine := &voiceInput{}, &manualAudio{}
			s, client := testReceiver(t, func(s *Server) {
				s.Input, s.Audio = fake, engine
				s.cfg.Voice = config.Voice{HoldKey: "RightAlt", ToggleStartKey: "F9", ToggleStopKey: "F10"}
			})
			conn, ctx := testSocket(t, s, client, "")
			testPair(t, s, conn, ctx)
			_ = write(ctx, conn, Message{Type: "mic_start", Recording: "1", Mode: mode})
			if stringField(testRead(t, ctx, conn), "type") != "mic_ready" {
				t.Fatal("recording not ready")
			}
			c := s.Config()
			c.Voice = config.Voice{HoldKey: "LeftCtrl", ToggleStartKey: "F11", ToggleStopKey: "F12"}
			if err := s.Update(c); err != nil {
				t.Fatal(err)
			}
			if stringField(testRead(t, ctx, conn), "type") != "config" {
				t.Fatal("missing live configuration")
			}
			_ = write(ctx, conn, Message{Type: "mic_stop", Recording: "1"})
			_ = write(ctx, conn, Message{Type: "mic_stop", Recording: "1"})
			_ = write(ctx, conn, Message{Type: "heartbeat"})
			testRead(t, ctx, conn)
			engine.mu.Lock()
			ends := engine.ends
			engine.mu.Unlock()
			if ends != 1 {
				t.Fatal("duplicate stop reset drain", ends)
			}
			engine.drain()
			if stringField(testRead(t, ctx, conn), "type") != "mic_stopped" {
				t.Fatal("missing completion")
			}
			engine.drain() // a late duplicate completion must do nothing
			_ = write(ctx, conn, Message{Type: "mic_abort", Recording: "1"})
			_ = write(ctx, conn, Message{Type: "heartbeat"})
			testRead(t, ctx, conn)
			want := "hold:RightAlt:true,hold:RightAlt:false"
			if mode == "toggle" {
				want = "tap:F9,tap:F10"
			}
			if fake.result() != want {
				t.Fatalf("snapshot or completion lost: %s", fake.result())
			}
		})
	}
}

func TestAbortDuringDrainAndCloseOnlyEndsOnce(t *testing.T) {
	fake, engine := &voiceInput{}, &manualAudio{}
	s, client := testReceiver(t, func(s *Server) {
		s.Input, s.Audio = fake, engine
		s.cfg.Voice = config.Voice{ToggleStartKey: "RightAlt", ToggleStopKey: "RightAlt"}
	})
	conn, ctx := testSocket(t, s, client, "")
	testPair(t, s, conn, ctx)
	_ = write(ctx, conn, Message{Type: "mic_start", Recording: "1", Mode: "toggle"})
	testRead(t, ctx, conn)
	_ = write(ctx, conn, Message{Type: "mic_stop", Recording: "1"})
	_ = write(ctx, conn, Message{Type: "mic_abort", Recording: "1"})
	if stringField(testRead(t, ctx, conn), "type") != "mic_stopped" {
		t.Fatal("abort failed")
	}
	engine.drain()
	s.Stop()
	if got := fake.result(); got != "tap:RightAlt,tap:RightAlt" {
		t.Fatal("duplicate end hotkey", got)
	}
}

func TestShortcutDisabledAndStableSlot(t *testing.T) {
	fake := &trackingChordInput{}
	s, client := testReceiver(t, func(s *Server) {
		s.Input = fake
		s.cfg.Shortcuts[1].Enabled = false
		s.cfg.Shortcuts[7] = config.Shortcut{Label: "第八项", Chord: "RightCtrl", Enabled: true}
	})
	conn, ctx := testSocket(t, s, client, "")
	testPair(t, s, conn, ctx)
	_ = write(ctx, conn, Message{Type: "shortcut", Slot: 1, Revision: s.Config().Revision})
	if stringField(testRead(t, ctx, conn), "type") != "error" || fake.chords.Load() != 0 {
		t.Fatal("disabled shortcut executed")
	}
	_ = write(ctx, conn, Message{Type: "shortcut", Slot: 7, Revision: s.Config().Revision})
	_ = write(ctx, conn, Message{Type: "heartbeat"})
	testRead(t, ctx, conn)
	if fake.chords.Load() != 1 {
		t.Fatal("stable eighth slot did not execute")
	}
	_ = write(ctx, conn, Message{Type: "shortcut", Slot: 7, Revision: 0})
	if stringField(testRead(t, ctx, conn), "type") != "config" {
		t.Fatal("stale revision not refreshed")
	}
	testRead(t, ctx, conn)
	if fake.chords.Load() != 1 {
		t.Fatal("stale shortcut executed")
	}
}

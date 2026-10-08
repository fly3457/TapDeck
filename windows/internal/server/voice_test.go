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
				s.cfg.Voice.Profiles[0].ToggleStartKey, s.cfg.Voice.Profiles[0].ToggleStopKey = "F9", "F10"
			})
			conn, ctx := testSocket(t, s, client, "")
			testPair(t, s, conn, ctx)
			_ = write(ctx, conn, Message{Type: "mic_start", Recording: "1", Mode: mode})
			if stringField(testRead(t, ctx, conn), "type") != "mic_ready" {
				t.Fatal("recording not ready")
			}
			c := s.Config()
			c.Voice.Profiles[0].ToggleStartKey, c.Voice.Profiles[0].ToggleStopKey = "F11", "F12"
			c.Voice.Profiles[1].HoldKey = "LeftCtrl"
			for i := range c.Voice.Profiles {
				c.Voice.Profiles[i].Enabled = false
				c.Voice.Profiles[i].Name = "录音中改名"
			}
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
		s.cfg.Voice.Profiles[0].ToggleStartKey, s.cfg.Voice.Profiles[0].ToggleStopKey = "RightAlt", "RightAlt"
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

func TestProfileIdentityRevisionAndLegacyRouting(t *testing.T) {
	fake, engine := &voiceInput{}, &manualAudio{}
	s, client := testReceiver(t, func(s *Server) {
		s.Input, s.Audio = fake, engine
		s.cfg.Voice.Profiles[2].Enabled = true
	})
	conn, ctx := testSocket(t, s, client, "")
	testPair(t, s, conn, ctx)
	// Two hold profiles must resolve by ID, irrespective of the supplied legacy mode.
	for i, id := range []string{"voice-3", "voice-2"} {
		recording := fmt.Sprintf("%x", i+1)
		_ = write(ctx, conn, Message{Type: "mic_start", Recording: recording, ProfileID: id, Mode: "toggle", Revision: s.Config().Revision})
		if stringField(testRead(t, ctx, conn), "type") != "mic_ready" {
			t.Fatal("profile not ready")
		}
		_ = write(ctx, conn, Message{Type: "mic_abort", Recording: recording})
		if stringField(testRead(t, ctx, conn), "type") != "mic_stopped" {
			t.Fatal("profile not stopped")
		}
	}
	if got := fake.result(); got != "hold:Ctrl+Shift+M:true,hold:Ctrl+Shift+M:false,hold:RightAlt:true,hold:RightAlt:false" {
		t.Fatal(got)
	}
	c := s.Config()
	c.Voice.Profiles[1].Enabled = false
	if err := s.Update(c); err != nil {
		t.Fatal(err)
	}
	testRead(t, ctx, conn)
	if s.Config().Voice.HoldKey != "Ctrl+Shift+M" {
		t.Fatal("legacy projection must use first enabled matching type")
	}
	before := fake.result()
	for _, m := range []Message{
		{ProfileID: "voice-3", Revision: c.Revision},
		{ProfileID: "voice-2", Revision: s.Config().Revision},
		{ProfileID: "missing", Revision: s.Config().Revision},
		{Mode: "invalid"},
	} {
		m.Type, m.Recording = "mic_start", "3"
		_ = write(ctx, conn, m)
		err := testRead(t, ctx, conn)
		if stringField(err, "type") != "mic_error" || stringField(err, "recording") != "3" {
			t.Fatal(err)
		}
		if stringField(testRead(t, ctx, conn), "type") != "config" {
			t.Fatal("missing recovery config")
		}
	}
	if fake.result() != before {
		t.Fatal("invalid request triggered a hotkey")
	}
	_ = write(ctx, conn, Message{Type: "mic_start", Recording: "4", Mode: "hold"})
	if stringField(testRead(t, ctx, conn), "type") != "mic_ready" {
		t.Fatal("legacy routing failed")
	}
	_ = write(ctx, conn, Message{Type: "mic_abort", Recording: "4"})
	testRead(t, ctx, conn)
	if !strings.HasSuffix(fake.result(), "hold:Ctrl+Shift+M:true,hold:Ctrl+Shift+M:false") {
		t.Fatal(fake.result())
	}
	c = s.Config()
	for i := range c.Voice.Profiles {
		c.Voice.Profiles[i].Enabled = false
	}
	if err := s.Update(c); err != nil {
		t.Fatal(err)
	}
	testRead(t, ctx, conn)
	for _, mode := range []string{"hold", "toggle"} {
		_ = write(ctx, conn, Message{Type: "mic_start", Recording: "5", Mode: mode})
		if stringField(testRead(t, ctx, conn), "type") != "mic_error" {
			t.Fatal("disabled legacy type accepted")
		}
		testRead(t, ctx, conn)
	}
}

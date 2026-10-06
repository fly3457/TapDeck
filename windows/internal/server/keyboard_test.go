package server

import (
	"fmt"
	"sync"
	"tapdeck/internal/keyboard"
	"testing"
)

type delayedKeyboard struct {
	quietInput
	mu     sync.Mutex
	start  func(error)
	active bool
	stops  int
	chords int
}

func (k *delayedKeyboard) ChordAsync(_ string, done func(error)) error {
	k.mu.Lock()
	k.chords++
	k.mu.Unlock()
	return nil
}
func (k *delayedKeyboard) StartVoice(_, _, _, _ string, done func(error)) error {
	k.mu.Lock()
	k.start = done
	k.active = true
	k.mu.Unlock()
	return nil
}
func (k *delayedKeyboard) StopVoice(_ string, done func(error)) error {
	k.mu.Lock()
	k.stops++
	k.active = false
	k.mu.Unlock()
	go done(nil)
	return nil
}
func (k *delayedKeyboard) KeyboardStatus() keyboard.Status { return keyboard.Status{Ready: true} }
func (k *delayedKeyboard) ConfigureBackend(string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.active {
		return fmt.Errorf("busy")
	}
	return nil
}
func TestCanceledPreparationSuppressesLateReady(t *testing.T) {
	k := &delayedKeyboard{}
	s, client := testReceiver(t, func(s *Server) { s.Input = k; s.Audio = &manualAudio{} })
	conn, ctx := testSocket(t, s, client, "")
	testPair(t, s, conn, ctx)
	write(ctx, conn, Message{Type: "mic_start", Recording: "1", Mode: "toggle"})
	write(ctx, conn, Message{Type: "heartbeat"})
	if stringField(testRead(t, ctx, conn), "type") != "heartbeat" {
		t.Fatal("keyboard preparation blocked control/mouse session")
	}
	c := s.Config()
	c.KeyboardBackend = "sendinput"
	if err := s.Update(c); err == nil {
		t.Fatal("backend changed while preparation owns voice")
	}
	write(ctx, conn, Message{Type: "mic_stop", Recording: "1"})
	if stringField(testRead(t, ctx, conn), "type") != "mic_stopped" {
		t.Fatal("preparing stop not completed")
	}
	k.mu.Lock()
	ready := k.start
	n := k.stops
	k.mu.Unlock()
	if n != 1 {
		t.Fatal("stop count", n)
	}
	ready(nil) // late child response must not activate AudioRecord
	write(ctx, conn, Message{Type: "heartbeat"})
	if stringField(testRead(t, ctx, conn), "type") != "heartbeat" {
		t.Fatal("late mic_ready leaked")
	}
}
func TestAsyncShortcutDoesNotBlockSession(t *testing.T) {
	k := &delayedKeyboard{}
	s, client := testReceiver(t, func(s *Server) { s.Input = k })
	conn, ctx := testSocket(t, s, client, "")
	testPair(t, s, conn, ctx)
	write(ctx, conn, Message{Type: "shortcut", Slot: 0, Revision: s.Config().Revision})
	write(ctx, conn, Message{Type: "heartbeat"})
	if stringField(testRead(t, ctx, conn), "type") != "heartbeat" {
		t.Fatal("shortcut waited for pulse before heartbeat")
	}
	k.mu.Lock()
	n := k.chords
	k.mu.Unlock()
	if n != 1 {
		t.Fatal("shortcut dispatch", n)
	}
}

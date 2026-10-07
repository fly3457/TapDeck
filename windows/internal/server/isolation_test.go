package server

import (
	"encoding/binary"
	"fmt"
	"sync"
	"tapdeck/internal/protocol"
	"testing"
)

type scopedAudio struct {
	manualAudio
	recording uint64
	pushes    int
	aborts    int
}

func (a *scopedAudio) Begin(id uint64) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.recording != 0 {
		return fmt.Errorf("busy")
	}
	a.recording = id
	return nil
}
func (a *scopedAudio) AbortRecording(id uint64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if id != 0 && id == a.recording {
		a.recording = 0
		a.aborts++
	}
}
func (a *scopedAudio) Push(id, _ uint64, _ []byte) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if id == a.recording && id != 0 {
		a.pushes++
	}
}

type scopedKeyboard struct {
	delayedKeyboard
	buttonMu sync.Mutex
	buttons  []string
}

func (k *scopedKeyboard) StartVoice(_ string, _ string, _ string, _ string, done func(error)) error {
	go done(nil)
	return nil
}
func (k *scopedKeyboard) Button(button string, down bool) error {
	k.buttonMu.Lock()
	defer k.buttonMu.Unlock()
	k.buttons = append(k.buttons, fmt.Sprintf("%s:%t", button, down))
	return nil
}

func TestUnpairPreservesOtherDragKeysAndRecording(t *testing.T) {
	k, engine := &scopedKeyboard{}, &scopedAudio{}
	s, client := testReceiver(t, func(s *Server) { s.Input = k; s.Audio = engine })
	a, ca, _ := pairDevice(t, s, client, 1)
	b, cb, _ := pairDevice(t, s, client, 2)
	for _, v := range []struct{ connIndex int }{{1}, {2}} {
		conn, ctx := a, ca
		if v.connIndex == 2 {
			conn, ctx = b, cb
		}
		_ = write(ctx, conn, Message{Type: "key_down", Text: "LeftCtrl", Revision: s.Config().Revision})
		_ = write(ctx, conn, Message{Type: "mouse_button", Button: "left", Down: true, Epoch: 0, NextEpoch: 1})
		_ = write(ctx, conn, Message{Type: "heartbeat"})
		testRead(t, ctx, conn)
	}
	_ = write(cb, b, Message{Type: "mic_start", Mode: "hold", Recording: "1"})
	if stringField(testRead(t, cb, b), "type") != "mic_ready" {
		t.Fatal("other recording not ready")
	}
	// Identical wire IDs on another device cannot claim the single stream.
	_ = write(ca, a, Message{Type: "mic_start", Mode: "hold", Recording: "1"})
	if stringField(testRead(t, ca, a), "type") != "mic_error" {
		t.Fatal("single recording limit lost")
	}
	if err := s.UnpairDevice("integration-device-1"); err != nil {
		t.Fatal(err)
	}
	if stringField(testRead(t, ca, a), "code") != "pairing_revoked" {
		t.Fatal("no revocation")
	}
	engine.mu.Lock()
	recording, aborts := engine.recording, engine.aborts
	engine.mu.Unlock()
	if recording == 0 || aborts != 0 || k.releases.Load() != 0 {
		t.Fatal("unpair aborted another stream or globally released input")
	}
	k.buttonMu.Lock()
	buttons := append([]string(nil), k.buttons...)
	k.buttonMu.Unlock()
	if len(buttons) != 1 || buttons[0] != "left:true" {
		t.Fatal("other drag released", buttons)
	}
	if keys := k.keyCount(); len(keys) != 3 || keys[2] != "key_up:LeftCtrl" {
		t.Fatal("scoped held-key cleanup failed", keys)
	}
	k.mu.Lock()
	stops := k.stops
	k.mu.Unlock()
	if stops != 0 {
		t.Fatal("other voice hotkey stopped")
	}
	_ = write(cb, b, Message{Type: "mouse_button", Button: "left", Down: false, Epoch: 1, NextEpoch: 2})
	_ = write(cb, b, Message{Type: "mic_abort", Recording: "1"})
	if stringField(testRead(t, cb, b), "type") != "mic_stopped" {
		t.Fatal("owner failed to stop")
	}
	k.buttonMu.Lock()
	defer k.buttonMu.Unlock()
	if len(k.buttons) != 2 || k.buttons[1] != "left:false" {
		t.Fatal(k.buttons)
	}
}

func TestLateAudioCannotEnterAnotherSessionsRecording(t *testing.T) {
	engine := &scopedAudio{}
	s := &Server{Audio: engine, sessions: map[uint64]*session{}}
	ss := &session{s: s, id: 1, active: true, recording: 7, audioRecording: 71}
	ss.audio, _ = protocol.NewCodec(make([]byte, 32), [4]byte{}, 1, protocol.Audio)
	s.sessions[1] = ss
	_ = engine.Begin(72) // belongs to a later or different session
	body := make([]byte, 976)
	binary.LittleEndian.PutUint64(body, 7)
	s.audioPacket(ss.audio.Seal(0, body))
	ss.abortAudio()
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if engine.recording != 72 || engine.pushes != 0 || engine.aborts != 0 {
		t.Fatal("late audio or abort crossed session ownership")
	}
}

package server

import (
	"fmt"
	"tapdeck/internal/protocol"
)

func (ss *session) touchpadFeatures() []string {
	if _, ok := ss.controller().(asyncTouchpad); ok {
		return []string{"touchpad_zoom", "three_finger"}
	}
	return []string{}
}

// Reliable controls flush the preceding cumulative movement before accepting
// an action; future-epoch UDP packets may only run after this barrier advances.
func (ss *session) controlBarrier(m Message, action func() error) error {
	if m.Epoch != ss.epoch || m.NextEpoch != m.Epoch+1 {
		return fmt.Errorf("鼠标控制分段失配")
	}
	if err := ss.move(protocol.Movement{Epoch: m.Epoch, X: m.X, Y: m.Y, ScrollX: m.ScrollX, ScrollY: m.ScrollY}); err != nil {
		return err
	}
	if err := action(); err != nil {
		return err
	}
	ss.epoch = m.NextEpoch
	ss.last.Epoch = ss.epoch
	if ss.pending != nil {
		p := ss.pending
		ss.pending = nil
		return ss.move(*p)
	}
	return nil
}

func (ss *session) controller() InputController {
	if ss.input != nil {
		return ss.input
	}
	return ss.s.Input
}
func (ss *session) mouseButton(button string, down bool) error {
	if button != "left" && button != "right" && button != "middle" {
		return fmt.Errorf("无效鼠标按钮")
	}
	s := ss.s
	s.buttonMu.Lock()
	defer s.buttonMu.Unlock()
	if s.buttonOwners == nil {
		s.buttonOwners = map[string]map[uint64]bool{}
	}
	owners := s.buttonOwners[button]
	if owners == nil {
		owners = map[uint64]bool{}
		s.buttonOwners[button] = owners
	}
	if owners[ss.id] == down {
		return nil
	}
	if (down && len(owners) == 0) || (!down && len(owners) == 1) {
		if err := s.Input.Button(button, down); err != nil {
			return err
		}
	}
	if down {
		owners[ss.id] = true
	} else {
		delete(owners, ss.id)
	}
	return nil
}
func (ss *session) releaseButtons() {
	s := ss.s
	s.buttonMu.Lock()
	defer s.buttonMu.Unlock()
	for button, owners := range s.buttonOwners {
		if owners[ss.id] {
			delete(owners, ss.id)
			if len(owners) == 0 {
				_ = s.Input.Button(button, false)
			}
		}
	}
}
func (ss *session) abortAudio() {
	if ss.audioRecording == 0 {
		return
	}
	ss.s.Audio.AbortRecording(ss.audioRecording)
}

// Production inputs own their worker queue. Synchronous test/custom inputs
// retain their own held chords and never clear another live controller.
func (ss *session) releaseInputs(holds map[string]string, keys map[string]int, recording uint64, voice recordingVoice) {
	ss.releaseButtons()
	if ss.input != nil {
		ss.input.ReleaseAll()
		return
	}
	c := ss.controller()
	if a, ok := c.(asyncKeyboard); ok {
		if recording != 0 {
			_ = a.StopVoice(ss.voiceToken(recording), func(error) {})
		}
		for _, chord := range holds {
			_ = a.HoldAsync(chord, false, func(error) {})
		}
		for chord, count := range keys {
			for range count {
				_ = a.KeyAsync("key_up", chord, func(error) {})
			}
		}
	} else {
		for _, chord := range holds {
			_ = c.Hold(chord, false)
		}
		if recording != 0 {
			if voice.Mode == "hold" {
				_ = c.Hold(voice.Start, false)
			} else {
				_ = c.Chord(voice.Stop)
			}
		}
	}
}

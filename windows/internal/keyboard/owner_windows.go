//go:build windows

package keyboard

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// Owner binds every queued keyboard action to one controller session.
type Owner struct {
	mu     sync.Mutex
	agent  *Agent
	id     string
	closed bool
}

func (a *Agent) ForOwner(id string) *Owner { return &Owner{agent: a, id: id} }
func (o *Owner) submit(r request, done func(error)) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return errCanceled
	}
	r.Owner = o.id
	return o.agent.submit(r, done)
}
func (o *Owner) sync(r request) error {
	done := make(chan error, 1)
	if err := o.submit(r, func(err error) { done <- err }); err != nil {
		return err
	}
	select {
	case err := <-done:
		return err
	case <-time.After(3 * time.Second):
		return fmt.Errorf("键盘动作超时")
	}
}
func (o *Owner) KeyboardStatus() Status             { return o.agent.KeyboardStatus() }
func (o *Owner) ConfigureBackend(mode string) error { return o.agent.ConfigureBackend(mode) }
func (o *Owner) Move(x, y, sx, sy int32) error      { return o.agent.Move(x, y, sx, sy) }
func (o *Owner) Button(b string, down bool) error   { return o.agent.Button(b, down) }
func (o *Owner) Chord(k string) error               { return o.sync(request{Action: "chord", Chord: k}) }
func (o *Owner) Hold(k string, down bool) error {
	return o.sync(request{Action: "hold", Chord: k, Down: down})
}
func (o *Owner) ChordAsync(k string, done func(error)) error {
	if err := Validate(k, o.KeyboardStatus().Configured); err != nil {
		return err
	}
	return o.submit(request{Action: "chord", Chord: k}, done)
}
func (o *Owner) ZoomAsync(steps int, done func(error)) error {
	if err := validateZoom(steps); err != nil {
		return err
	}
	return o.submit(request{Action: "zoom", Steps: steps}, done)
}
func (o *Owner) GestureAsync(direction string, done func(error)) error {
	if err := validateGesture(direction); err != nil {
		return err
	}
	return o.submit(request{Action: "gesture", Mode: direction}, done)
}
func (o *Owner) HoldAsync(k string, down bool, done func(error)) error {
	if err := Validate(k, o.KeyboardStatus().Configured); err != nil {
		return err
	}
	action := "hold_up"
	if down {
		action = "hold_async"
	}
	return o.submit(request{Action: action, Chord: k}, done)
}
func (o *Owner) KeyAsync(action, k string, done func(error)) error {
	if action != "key_down" && action != "key_up" {
		return fmt.Errorf("无效按键动作")
	}
	if err := Validate(k, o.KeyboardStatus().Configured); err != nil {
		return err
	}
	return o.submit(request{Action: action, Chord: k}, done)
}
func (o *Owner) StartVoice(token, mode, start, stop string, done func(error)) error {
	if err := Validate(start, o.KeyboardStatus().Configured); err != nil {
		return err
	}
	if err := Validate(stop, o.KeyboardStatus().Configured); err != nil {
		return err
	}
	return o.submit(request{Action: "voice_start", Token: token, Mode: mode, Chord: start, Stop: stop}, done)
}
func (o *Owner) StopVoice(token string, done func(error)) error {
	return o.submit(request{Action: "voice_stop", Token: token}, done)
}
func (o *Owner) ReleaseAll() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return
	}
	o.closed = true
	if err := o.agent.submit(request{Action: "release_owner", Owner: o.id}, func(err error) {
		if err != nil {
			log.Printf("会话键盘清理: %v", err)
		}
	}); err != nil {
		log.Print(err)
	}
}

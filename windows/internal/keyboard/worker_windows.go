//go:build windows

package keyboard

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sync"
	"time"
)

type request struct {
	ID     uint64 `json:"id"`
	Epoch  uint64 `json:"epoch"`
	Action string `json:"action"`
	Chord  string `json:"chord,omitempty"`
	Down   bool   `json:"down,omitempty"`
	Token  string `json:"token,omitempty"`
	Mode   string `json:"mode,omitempty"`
	Stop   string `json:"stop,omitempty"`
}
type response struct {
	ID       uint64   `json:"id"`
	Error    string   `json:"error,omitempty"`
	Status   Status   `json:"status"`
	SoftKeys []uint16 `json:"soft_keys,omitempty"`
}

var errCanceled = errors.New("键盘动作已取消")

func pulse(ctx context.Context, e *Engine, chord string) error {
	return pulseMarked(ctx, e, chord, nil)
}
func pulseMarked(ctx context.Context, e *Engine, chord string, onDown func()) error {
	if ctx.Err() != nil {
		return errCanceled
	}
	if chord == "" {
		if onDown != nil {
			onDown()
		}
		return nil
	}
	route, err := e.press(chord)
	if err != nil {
		return err
	}
	if onDown != nil {
		onDown()
	}
	if e.observe != nil {
		e.observe()
	}
	select {
	case <-ctx.Done():
	case <-time.After(50 * time.Millisecond):
	}
	releaseErr := e.release(chord, route)
	if ctx.Err() != nil {
		return errCanceled
	}
	return releaseErr
}
func (e *Engine) startVoice(ctx context.Context, r request) error {
	if ctx.Err() != nil {
		return errCanceled
	}
	e.mu.Lock()
	if _, found := e.voices[r.Token]; found {
		e.mu.Unlock()
		return nil
	}
	e.voices[r.Token] = voice{mode: r.Mode, start: r.Chord, stop: r.Stop}
	e.mu.Unlock()
	var err error
	mark := func() { e.mu.Lock(); v := e.voices[r.Token]; v.started = true; e.voices[r.Token] = v; e.mu.Unlock() }
	if r.Mode == "hold" {
		err = e.Hold(r.Chord, true)
		if err == nil {
			mark()
		}
	} else {
		err = pulseMarked(ctx, e, r.Chord, mark)
	}
	if err != nil && !errors.Is(err, errCanceled) {
		e.mu.Lock()
		delete(e.voices, r.Token)
		e.mu.Unlock()
	}
	return err
}
func (e *Engine) stopVoice(ctx context.Context, token string) error {
	e.mu.Lock()
	v, found := e.voices[token]
	delete(e.voices, token)
	e.mu.Unlock()
	if !found || !v.started {
		return nil
	}
	if v.mode == "hold" {
		return e.Hold(v.start, false)
	}
	return pulse(ctx, e, v.stop)
}
func (e *Engine) clearVoices() {
	e.mu.Lock()
	tokens := make([]string, 0, len(e.voices))
	for k := range e.voices {
		tokens = append(tokens, k)
	}
	e.mu.Unlock()
	for _, token := range tokens {
		_ = e.stopVoice(context.Background(), token)
	}
	e.ClearKeys()
}

// RunWorker uses inherited anonymous pipes: no TCP listener, named endpoint,
// authentication token or independently discoverable control channel.
func RunWorker(reader io.Reader, writer io.Writer) error {
	e := NewEngine("auto")
	return runWorker(reader, writer, e)
}
func runWorker(reader io.Reader, writer io.Writer, e *Engine) error {
	defer e.Close()
	enc := json.NewEncoder(writer)
	write := func(id uint64, err error) {
		s := e.Status()
		s.WorkerPID = os.Getpid()
		r := response{ID: id, Status: s, SoftKeys: e.softwareKeys()}
		if err != nil {
			r.Error = err.Error()
		}
		_ = enc.Encode(r)
	}
	e.observe = func() { write(0, nil) }
	write(0, nil)
	queue := make(chan request, 128)
	urgent := make(chan request, 128)
	parentGone := make(chan struct{})
	var mu sync.Mutex
	epoch := uint64(1)
	canceled := map[string]bool{}
	pendingStarts := map[string]int{}
	var currentCancel context.CancelFunc
	var currentToken string
	go func() {
		defer close(parentGone)
		defer func() {
			mu.Lock()
			if currentCancel != nil {
				currentCancel()
			}
			mu.Unlock()
		}()
		dec := json.NewDecoder(reader)
		for {
			var r request
			if dec.Decode(&r) != nil {
				return
			}
			mu.Lock()
			if r.Action == "voice_start" {
				pendingStarts[r.Token]++
			}
			if r.Action == "clear" {
				epoch = r.Epoch
				canceled = map[string]bool{}
				if currentCancel != nil {
					currentCancel()
				}
			}
			if r.Action == "voice_stop" {
				canceled[r.Token] = true
				if currentToken == r.Token && currentCancel != nil {
					currentCancel()
				}
			}
			mu.Unlock()
			if r.Action == "voice_stop" || r.Action == "clear" {
				urgent <- r
			} else {
				queue <- r
			}
		}
	}()
	for {
		select {
		case <-parentGone:
			e.clearVoices()
			return nil
		default:
		}
		var r request
		select {
		case r = <-urgent:
		default:
			select {
			case <-parentGone:
				e.clearVoices()
				return nil
			case r = <-urgent:
			case r = <-queue:
			}
		}
		mu.Lock()
		invalid := r.Epoch != epoch || (r.Action == "voice_start" && canceled[r.Token])
		ctx, cancel := context.WithCancel(context.Background())
		currentCancel, currentToken = cancel, r.Token
		if invalid {
			cancel()
		}
		mu.Unlock()
		var err error
		if invalid {
			err = errCanceled
		} else {
			switch r.Action {
			case "chord":
				err = pulse(ctx, e, r.Chord)
			case "hold":
				err = e.Hold(r.Chord, r.Down)
			case "hold_async":
				err = e.Hold(r.Chord, true)
			case "hold_up":
				err = e.Hold(r.Chord, false)
			case "key_down":
				err = e.PressKey(r.Chord)
			case "key_up":
				err = e.ReleaseKey(r.Chord)
			case "voice_start":
				err = e.startVoice(ctx, r)
			case "voice_stop":
				err = e.stopVoice(ctx, r.Token)
			case "clear":
				e.clearVoices()
			case "configure":
				err = e.Configure(r.Mode)
			case "status":
			default:
				err = errors.New("无效键盘工作进程消息")
			}
		}
		cancel()
		mu.Lock()
		currentCancel = nil
		currentToken = ""
		if r.Action == "voice_start" {
			pendingStarts[r.Token]--
			if pendingStarts[r.Token] == 0 {
				delete(pendingStarts, r.Token)
				delete(canceled, r.Token)
			}
		}
		if r.Action == "voice_stop" && pendingStarts[r.Token] == 0 {
			delete(canceled, r.Token)
		}
		mu.Unlock()
		write(r.ID, err)
	}
}

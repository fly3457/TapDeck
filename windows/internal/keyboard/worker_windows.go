//go:build windows

package keyboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"tapdeck/internal/input"
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
	Owner  string `json:"owner,omitempty"`
	Steps  int    `json:"steps,omitempty"`
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
	e.voices[r.Token] = voice{mode: r.Mode, start: r.Chord, stop: r.Stop, owner: r.Owner}
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
	canceledOwners := map[string]bool{}
	pendingOwners := map[string]int{}
	heldByOwner := map[string]map[string][]string{}
	canonical := func(chord string) string {
		ks, _ := input.ParseChord(chord)
		sort.Slice(ks, func(i, j int) bool { return ks[i] < ks[j] })
		parts := []string{}
		for _, k := range ks {
			parts = append(parts, fmt.Sprintf("%X", k))
		}
		return strings.Join(parts, "+")
	}
	ownedHold := func(r request, down bool) error {
		if r.Owner == "" {
			return e.Hold(r.Chord, down)
		}
		key := canonical(r.Chord)
		if down {
			if err := e.Hold(r.Chord, true); err != nil {
				return err
			}
			if heldByOwner[r.Owner] == nil {
				heldByOwner[r.Owner] = map[string][]string{}
			}
			heldByOwner[r.Owner][key] = append(heldByOwner[r.Owner][key], r.Chord)
			return nil
		}
		held := heldByOwner[r.Owner][key]
		if len(held) == 0 {
			return nil
		}
		chord := held[len(held)-1]
		if len(held) == 1 {
			delete(heldByOwner[r.Owner], key)
		} else {
			heldByOwner[r.Owner][key] = held[:len(held)-1]
		}
		return e.Hold(chord, false)
	}
	releaseOwner := func(owner string) {
		e.mu.Lock()
		var tokens []string
		for token, v := range e.voices {
			if v.owner == owner {
				tokens = append(tokens, token)
			}
		}
		e.mu.Unlock()
		for _, token := range tokens {
			_ = e.stopVoice(context.Background(), token)
		}
		for _, chords := range heldByOwner[owner] {
			for _, chord := range chords {
				_ = e.Hold(chord, false)
			}
		}
		delete(heldByOwner, owner)
	}
	pendingStarts := map[string]int{}
	var currentCancel context.CancelFunc
	var currentToken string
	var currentOwner string
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
			if r.Owner != "" {
				pendingOwners[r.Owner]++
			}
			if r.Action == "release_owner" {
				canceledOwners[r.Owner] = true
				if currentOwner == r.Owner && currentCancel != nil {
					currentCancel()
				}
			}
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
			if r.Action == "voice_stop" || r.Action == "clear" || r.Action == "release_owner" {
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
		invalid := r.Epoch != epoch || (r.Action == "voice_start" && canceled[r.Token]) || (r.Owner != "" && canceledOwners[r.Owner] && r.Action != "release_owner")
		ctx, cancel := context.WithCancel(context.Background())
		currentCancel, currentToken = cancel, r.Token
		currentOwner = r.Owner
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
			case "zoom":
				err = e.zoom(ctx, r.Steps)
			case "gesture":
				err = e.gesture(ctx, r.Mode)
			case "hold":
				err = ownedHold(r, r.Down)
			case "hold_async":
				err = ownedHold(r, true)
			case "hold_up":
				err = ownedHold(r, false)
			case "key_down":
				err = ownedHold(r, true)
			case "key_up":
				err = ownedHold(r, false)
			case "voice_start":
				err = e.startVoice(ctx, r)
			case "voice_stop":
				err = e.stopVoice(ctx, r.Token)
			case "clear":
				e.clearVoices()
				heldByOwner = map[string]map[string][]string{}
			case "release_owner":
				releaseOwner(r.Owner)
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
		currentOwner = ""
		if r.Owner != "" {
			pendingOwners[r.Owner]--
			if pendingOwners[r.Owner] == 0 {
				delete(pendingOwners, r.Owner)
				delete(canceledOwners, r.Owner)
			}
		}
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

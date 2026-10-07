//go:build windows

package keyboard

import (
	"encoding/json"
	"io"
	"sync"
	"testing"
	"time"
)

type workerHarness struct {
	encoder   *json.Encoder
	responses chan response
}

func startTestWorker(t *testing.T, e *Engine) *workerHarness {
	t.Helper()
	rin, win := io.Pipe()
	rout, wout := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- runWorker(rin, wout, e); wout.Close() }()
	h := &workerHarness{json.NewEncoder(win), make(chan response, 64)}
	go func() {
		dec := json.NewDecoder(rout)
		for {
			var r response
			if dec.Decode(&r) != nil {
				return
			}
			h.responses <- r
		}
	}()
	select {
	case <-h.responses:
	case <-time.After(time.Second):
		t.Fatal("no worker greeting")
	}
	t.Cleanup(func() {
		win.Close()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(time.Second):
			t.Error("worker did not stop")
		}
		rout.Close()
	})
	return h
}
func (h *workerHarness) send(t *testing.T, r request) {
	t.Helper()
	r.Epoch = 1
	if err := h.encoder.Encode(r); err != nil {
		t.Fatal(err)
	}
}
func (h *workerHarness) wait(t *testing.T, id uint64) response {
	t.Helper()
	for {
		select {
		case r := <-h.responses:
			if r.ID == id {
				return r
			}
		case <-time.After(time.Second):
			t.Fatalf("no response for %d", id)
		}
	}
}

func TestReleaseOwnerKeepsOtherKeysAndVoice(t *testing.T) {
	e, _, _ := fixture()
	h := startTestWorker(t, e)
	requests := []request{
		{ID: 1, Owner: "a", Action: "key_down", Chord: "Ctrl+M"},
		{ID: 2, Owner: "b", Action: "hold", Chord: "LeftCtrl+K", Down: true},
		{ID: 3, Owner: "b", Action: "voice_start", Token: "b/record", Mode: "hold", Chord: "RightAlt"},
		{ID: 4, Owner: "a", Action: "key_up", Chord: "Ctrl+K"}, // cannot release B's chord
		{ID: 5, Owner: "a", Action: "release_owner"},
	}
	for _, r := range requests {
		h.send(t, r)
		if reply := h.wait(t, r.ID); reply.Error != "" {
			t.Fatal(reply)
		}
	}
	e.mu.Lock()
	keys := copyKeys(e.hidKeys)
	voices := len(e.voices)
	e.mu.Unlock()
	if keys[0xA2] != 1 || keys['K'] != 1 || keys[0xA5] != 1 || keys['M'] != 0 || voices != 1 {
		t.Fatal("another owner lost inputs", keys, voices)
	}
	h.send(t, request{ID: 6, Owner: "b", Action: "release_owner"})
	reply := h.wait(t, 6)
	e.mu.Lock()
	remaining := len(e.hidKeys) + len(e.voices) + len(e.holds)
	e.mu.Unlock()
	if remaining != 0 || reply.Status.Busy {
		t.Fatal("owner cleanup leaked keys", remaining, reply)
	}
}

func TestReleaseOwnerCancelsExecutingAndQueuedActions(t *testing.T) {
	e, _, _ := fixture()
	entered, unblock := make(chan struct{}), make(chan struct{})
	original := e.writeHID
	var once sync.Once
	e.writeHID = func(keys map[uint16]int) error {
		if keys['A'] > 0 {
			once.Do(func() { close(entered) })
			<-unblock
		}
		return original(keys)
	}
	h := startTestWorker(t, e)
	h.send(t, request{ID: 1, Owner: "a", Action: "chord", Chord: "A"})
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("pulse did not start")
	}
	h.send(t, request{ID: 2, Owner: "a", Action: "hold_async", Chord: "B"})
	h.send(t, request{ID: 3, Owner: "a", Action: "voice_start", Token: "a/late", Mode: "toggle", Chord: "C", Stop: "D"})
	h.send(t, request{ID: 4, Owner: "a", Action: "release_owner"})
	h.send(t, request{ID: 5, Owner: "b", Action: "hold_async", Chord: "K"})
	close(unblock)
	replies := map[uint64]response{}
	for len(replies) < 5 {
		select {
		case r := <-h.responses:
			if r.ID != 0 {
				replies[r.ID] = r
			}
		case <-time.After(time.Second):
			t.Fatal("queued actions were not completed")
		}
	}
	for _, id := range []uint64{1, 2, 3} {
		if replies[id].Error == "" {
			t.Fatalf("revoked action %d executed", id)
		}
	}
	e.mu.Lock()
	keys := copyKeys(e.hidKeys)
	voices := len(e.voices)
	e.mu.Unlock()
	if keys['K'] != 1 || len(keys) != 1 || voices != 0 {
		t.Fatal("late revoked input or unrelated key loss", keys, voices)
	}
}

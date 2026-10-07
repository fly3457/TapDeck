//go:build windows

package keyboard

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"tapdeck/internal/input"
	"testing"
	"time"
)

type fakeWindowObserver struct {
	mu          sync.Mutex
	observation windowObservation
	failure     error
	changed     chan struct{}
	done        chan struct{}
}

func fakeWindows() *fakeWindowObserver {
	return &fakeWindowObserver{observation: windowObservation{Foreground: 1}, changed: make(chan struct{}, 1), done: make(chan struct{})}
}
func (f *fakeWindowObserver) Observe() windowObservation {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.observation
}
func (f *fakeWindowObserver) After(_ context.Context, target windowMode, _ uintptr) (windowObservation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failure != nil {
		return f.observation, f.failure
	}
	f.observation = windowObservation{Foreground: 1, Desktop: target == windowDesktop, TaskView: target == windowTaskView}
	return f.observation, nil
}
func (f *fakeWindowObserver) Changes() <-chan struct{} { return f.changed }
func (f *fakeWindowObserver) Done() <-chan struct{}    { return f.done }
func (f *fakeWindowObserver) Close()                   { close(f.done) }
func (f *fakeWindowObserver) external(obs windowObservation) {
	f.mu.Lock()
	f.observation = obs
	f.mu.Unlock()
	select {
	case f.changed <- struct{}{}:
	default:
	}
}

func TestThreeFingerWindowTransitionsAndRepeatedGestures(t *testing.T) {
	windows := fakeWindows()
	g := newWindowGestures(windows)
	defer g.Close()
	var keys []string
	for _, direction := range []string{"up", "up", "down", "down", "down", "up"} {
		if err := g.run(context.Background(), direction, func(key string) error { keys = append(keys, key); return nil }); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(keys, []string{"LeftWin+Tab", "Esc", "LeftWin+D", "LeftWin+D"}) {
		t.Fatal("wrong state transitions", keys)
	}
}
func TestExternalWindowSelectionAndTaskViewCloseResynchronize(t *testing.T) {
	windows := fakeWindows()
	g := newWindowGestures(windows)
	defer g.Close()
	var keys []string
	emit := func(key string) error { keys = append(keys, key); return nil }
	if err := g.run(context.Background(), "up", emit); err != nil {
		t.Fatal(err)
	}
	windows.external(windowObservation{Foreground: 42})
	deadline := time.Now().Add(time.Second)
	for {
		g.mu.Lock()
		actual := g.state
		g.mu.Unlock()
		if actual == windowNormal {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("foreground event did not clear Task View")
		}
		time.Sleep(time.Millisecond)
	}
	if err := g.run(context.Background(), "down", emit); err != nil {
		t.Fatal(err)
	}
	windows.external(windowObservation{Foreground: 99}) // user manually restores/opens an app
	if err := g.run(context.Background(), "up", emit); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(keys, []string{"LeftWin+Tab", "LeftWin+D", "LeftWin+Tab"}) {
		t.Fatal("stale state toggled desktop/task view", keys)
	}
}
func TestWindowGestureFailureCancellationAndValidation(t *testing.T) {
	windows := fakeWindows()
	g := newWindowGestures(windows)
	defer g.Close()
	failed := errors.New("input failed")
	if err := g.run(context.Background(), "up", func(string) error { return failed }); !errors.Is(err, failed) {
		t.Fatal(err)
	}
	if g.state != windowNormal {
		t.Fatal("advanced after failed injection")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := g.run(ctx, "down", func(string) error { t.Fatal("canceled gesture ran"); return nil }); !errors.Is(err, errCanceled) {
		t.Fatal(err)
	}
	if g.run(context.Background(), "left", func(string) error { t.Fatal("unknown gesture ran"); return nil }) == nil {
		t.Fatal("accepted unknown action")
	}
	windows.failure = failed
	if err := g.run(context.Background(), "up", func(string) error { return nil }); !errors.Is(err, failed) {
		t.Fatal(err)
	}
	if g.state != windowNormal {
		t.Fatal("assumed task view opened without observing it")
	}
}

type zoomSoft struct {
	*fakeSoft
	calls []bool
	steps []int
}

func (f *zoomSoft) ZoomSteps(steps int, held bool) error {
	f.steps = append(f.steps, steps)
	f.calls = append(f.calls, held)
	return nil
}
func gestureEngine() (*Engine, *zoomSoft) {
	e, soft, _ := fixture()
	s := &zoomSoft{fakeSoft: soft}
	e.soft = s
	e.readModifiers = func() input.Modifiers { return input.Modifiers{} }
	return e, s
}
func TestZoomWorksInAllBackendsAndKeepsAnotherOwnersCtrl(t *testing.T) {
	for _, mode := range []string{"auto", "hid", "sendinput"} {
		e, soft := gestureEngine()
		e.mode = mode
		if err := e.Hold("RightCtrl+M", true); err != nil {
			t.Fatal(err)
		}
		before := copyKeys(e.hidKeys)
		if err := e.zoom(context.Background(), 1); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before, e.hidKeys) || len(soft.calls) != 1 || !soft.calls[0] || e.owners[0xA3].Count != 1 {
			t.Fatal("zoom changed the held voice key", mode, soft.calls, e.owners)
		}
		if err := e.Hold("RightCtrl+M", false); err != nil {
			t.Fatal(err)
		}
		if err := e.zoom(context.Background(), -1); err != nil {
			t.Fatal(err)
		}
		if soft.calls[1] {
			t.Fatal("temporary Ctrl was not requested")
		}
	}
}
func TestPinchAndThreeFingerRejectModifierConflictsWithoutReleasingKeys(t *testing.T) {
	for _, key := range []string{"RightAlt", "LeftShift", "LeftWin"} {
		e, soft := gestureEngine()
		windows := fakeWindows()
		e.navigator = newWindowGestures(windows)
		if err := e.Hold(key, true); err != nil {
			t.Fatal(err)
		}
		before := copyKeys(e.hidKeys)
		if e.zoom(context.Background(), 1) == nil || e.gesture(context.Background(), "down") == nil {
			t.Fatal("ignored conflicting modifier", key)
		}
		if len(soft.calls) != 0 || !reflect.DeepEqual(before, e.hidKeys) {
			t.Fatal("conflict released other held keys")
		}
		e.Close()
	}
	e, soft := gestureEngine()
	e.readModifiers = func() input.Modifiers { return input.Modifiers{Ctrl: true} }
	windows := fakeWindows()
	e.navigator = newWindowGestures(windows)
	defer e.Close()
	if err := e.zoom(context.Background(), 1); err != nil || !soft.calls[0] {
		t.Fatal("physical Ctrl not respected", err)
	}
	if err := e.gesture(context.Background(), "down"); err == nil {
		t.Fatal("Win+D would become Ctrl+Win+D")
	}
}
func TestRevokedOwnerCannotExecuteQueuedZoomOrSystemGesture(t *testing.T) {
	e, soft := gestureEngine()
	windows := fakeWindows()
	e.navigator = newWindowGestures(windows)
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
	h.send(t, request{ID: 1, Owner: "revoked", Action: "chord", Chord: "A"})
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("pulse did not start")
	}
	h.send(t, request{ID: 2, Owner: "revoked", Action: "zoom", Steps: 1})
	h.send(t, request{ID: 3, Owner: "revoked", Action: "gesture", Mode: "down"})
	h.send(t, request{ID: 4, Owner: "revoked", Action: "release_owner"})
	close(unblock)
	replies := map[uint64]response{}
	for len(replies) < 4 {
		select {
		case r := <-h.responses:
			if r.ID != 0 {
				replies[r.ID] = r
			}
		case <-time.After(time.Second):
			t.Fatal("no replies")
		}
	}
	if replies[2].Error == "" || replies[3].Error == "" || len(soft.calls) != 0 || windows.Observe().Desktop {
		t.Fatal("late revoked gesture executed", replies)
	}
}

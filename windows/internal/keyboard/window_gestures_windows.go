//go:build windows

package keyboard

import (
	"context"
	"fmt"
	"sync"
)

type windowMode uint8

const (
	windowNormal windowMode = iota
	windowDesktop
	windowTaskView
)

type windowObservation struct {
	Foreground        uintptr
	Desktop, TaskView bool
}
type windowObserver interface {
	Observe() windowObservation
	After(context.Context, windowMode, uintptr) (windowObservation, error)
	Changes() <-chan struct{}
	Done() <-chan struct{}
	Close()
}

// There is one navigator in the PC worker, shared by all session owners.
type windowGestures struct {
	mu       sync.Mutex
	observer windowObserver
	state    windowMode
	closed   bool
}

func newWindowGestures(observer windowObserver) *windowGestures {
	g := &windowGestures{observer: observer}
	go func() {
		for {
			select {
			case <-observer.Done():
				return
			case <-observer.Changes():
				g.mu.Lock()
				if !g.closed {
					g.sync(observer.Observe())
				}
				g.mu.Unlock()
			}
		}
	}()
	return g
}
func (g *windowGestures) sync(obs windowObservation) {
	if obs.Foreground == 0 {
		return
	} // Ignore a transient foreground switch.
	if obs.TaskView {
		g.state = windowTaskView
	} else if obs.Desktop {
		g.state = windowDesktop
	} else {
		g.state = windowNormal
	}
}
func (g *windowGestures) run(ctx context.Context, direction string, chord func(string) error) error {
	if direction != "up" && direction != "down" {
		return fmt.Errorf("无效三指手势")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed || ctx.Err() != nil {
		return errCanceled
	}
	obs := g.observer.Observe()
	g.sync(obs)
	target, key := g.state, ""
	switch g.state {
	case windowNormal:
		if direction == "up" {
			target, key = windowTaskView, "LeftWin+Tab"
		} else {
			target, key = windowDesktop, "LeftWin+D"
		}
	case windowDesktop:
		if direction == "up" {
			target, key = windowNormal, "LeftWin+D"
		}
	case windowTaskView:
		if direction == "down" {
			target, key = windowNormal, "Esc"
		}
	}
	if key == "" {
		return nil
	}
	if err := chord(key); err != nil {
		return err
	}
	observed, err := g.observer.After(ctx, target, obs.Foreground)
	// Always re-read after the native action, including cancellation, rather
	// than assuming injection success means the shell changed its state.
	g.sync(observed)
	return err
}
func (g *windowGestures) Close() {
	g.mu.Lock()
	g.closed = true
	g.mu.Unlock()
	g.observer.Close()
}

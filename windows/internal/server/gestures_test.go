package server

import (
	"errors"
	"fmt"
	"reflect"
	"tapdeck/internal/protocol"
	"testing"
)

type gestureInput struct {
	fakeInput
	reject error
}

func (f *gestureInput) Move(x, y, sx, sy int32) error {
	if x != 0 || y != 0 || sx != 0 || sy != 0 {
		f.events = append(f.events, fmt.Sprintf("move:%d:%d:%d:%d", x, y, sx, sy))
	}
	return nil
}
func (f *gestureInput) ZoomAsync(steps int, _ func(error)) error {
	if f.reject != nil {
		return f.reject
	}
	f.events = append(f.events, fmt.Sprintf("zoom:%d", steps))
	return nil
}
func (f *gestureInput) GestureAsync(action string, _ func(error)) error {
	if f.reject != nil {
		return f.reject
	}
	f.events = append(f.events, "gesture:"+action)
	return nil
}
func TestZoomAndGestureBarrierFlushMovementAndPreserveFutureTotals(t *testing.T) {
	f := &gestureInput{}
	ss := &session{s: &Server{Input: f}, active: true}
	if err := ss.move(protocol.Movement{Epoch: 1, X: 30 * 1024, ScrollY: -40 * 1024}); err != nil {
		t.Fatal(err)
	}
	if err := ss.handle(Message{Type: "zoom", Steps: 1, Epoch: 0, NextEpoch: 1, X: 10 * 1024, ScrollY: -20 * 1024}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.events, []string{"move:10:0:0:-20", "zoom:1", "move:20:0:0:-20"}) {
		t.Fatal("cross-channel order lost", f.events)
	}
	if err := ss.handle(Message{Type: "gesture", Action: "down", Epoch: 1, NextEpoch: 2, X: 30 * 1024, ScrollY: -40 * 1024}); err != nil {
		t.Fatal(err)
	}
	if ss.last.X != 30*1024 || ss.last.ScrollY != -40*1024 || ss.epoch != 2 || f.events[3] != "gesture:down" {
		t.Fatal("barrier reset cumulative totals", ss.last, f.events)
	}
	if err := ss.move(protocol.Movement{Epoch: 0, X: 5 * 1024}); err != nil || len(f.events) != 4 {
		t.Fatal("late packet crossed gesture barrier", err, f.events)
	}
}
func TestGestureCapabilitiesAreOptionalForOldInputsAndClients(t *testing.T) {
	old := &session{s: &Server{Input: &fakeInput{}}}
	if len(old.touchpadFeatures()) != 0 {
		t.Fatal("advertised unsupported controls")
	}
	current := &session{s: &Server{Input: &gestureInput{}}}
	if !reflect.DeepEqual(current.touchpadFeatures(), []string{"touchpad_zoom", "three_finger"}) {
		t.Fatal(current.touchpadFeatures())
	}
	if ControlVersion != 2 {
		t.Fatal("gesture extension changed control version")
	}
}
func TestInvalidClosedAndFailedGestureDoNotAdvanceEpoch(t *testing.T) {
	f := &gestureInput{}
	ss := &session{s: &Server{Input: f}, active: true}
	for _, message := range []Message{
		{Type: "zoom", Steps: 0, NextEpoch: 1}, {Type: "zoom", Steps: 5, NextEpoch: 1},
		{Type: "gesture", Action: "LeftWin+D", NextEpoch: 1}, {Type: "gesture", Action: "up", Epoch: 1, NextEpoch: 2},
	} {
		if ss.handle(message) == nil {
			t.Fatal("accepted invalid gesture", message)
		}
	}
	if len(f.events) != 0 || ss.epoch != 0 {
		t.Fatal("invalid action changed state")
	}
	f.reject = errors.New("owner revoked")
	if ss.handle(Message{Type: "zoom", Steps: 1, NextEpoch: 1}) == nil || ss.epoch != 0 {
		t.Fatal("failed queue advanced epoch")
	}
	f.reject = nil
	ss.active = false
	if ss.handle(Message{Type: "gesture", Action: "down", NextEpoch: 1}) == nil || len(f.events) != 0 {
		t.Fatal("closed session injected gesture")
	}
}

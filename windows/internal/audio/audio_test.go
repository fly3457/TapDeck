package audio

import (
	"encoding/binary"
	"testing"
	"time"
)

func TestRecordingIsolationAndBoundedQueue(t *testing.T) {
	e := New()
	e.ready = true
	if err := e.Begin(1); err != nil {
		t.Fatal(err)
	}
	pcm := make([]byte, 960)
	for i := 0; i < 480; i++ {
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(1000))
	}
	e.Push(2, 0, pcm)
	if len(e.frames) != 0 {
		t.Fatal("old recording accepted")
	}
	for i := 0; i < 100; i++ {
		e.Push(1, uint64(i*480), pcm)
	}
	if len(e.frames) > 6 {
		t.Fatal("unbounded buffer")
	}
	e.Abort()
	e.Push(1, 0, pcm)
	if len(e.frames) != 0 {
		t.Fatal("audio after abort")
	}
}

func TestAbortRecordingCannotStopAnotherOwner(t *testing.T) {
	e := New()
	e.ready = true
	if err := e.Begin(22); err != nil {
		t.Fatal(err)
	}
	e.Push(22, 0, make([]byte, 960))
	e.AbortRecording(11)
	if e.recording != 22 || len(e.frames) != 1 {
		t.Fatal("another session's recording was aborted")
	}
	e.AbortRecording(22)
	if e.recording != 0 || len(e.frames) != 0 {
		t.Fatal("owner could not abort its recording")
	}
}
func TestPartialFrameArrival(t *testing.T) {
	e := New()
	e.ready = true
	_ = e.Begin(1)
	e.first = time.Now().Add(-time.Second)
	e.started = true
	e.next = 500
	e.Push(1, 480, make([]byte, 960))
	if _, ok := e.frames[480]; !ok {
		t.Fatal("partially consumed frame rejected")
	}
}
func TestDrainStops(t *testing.T) {
	e := New()
	e.ready = true
	_ = e.Begin(1)
	done := make(chan struct{})
	e.End(1, 0, func() { close(done) })
	e.end = time.Now().Add(-time.Second)
	e.render(make([]int16, 480))
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stop callback never ran")
	}
	if e.recording != 0 {
		t.Fatal("recording remained active")
	}
}
func TestClockDriftRemainsBounded(t *testing.T) {
	e := New()
	e.ready = true
	_ = e.Begin(1)
	pcm := make([]byte, 960)
	for i := 0; i < 480; i++ {
		binary.LittleEndian.PutUint16(pcm[i*2:], 1000)
	}
	for i := 0; i < 3; i++ {
		e.Push(1, uint64(i*480), pcm)
	}
	e.first = time.Now().Add(-time.Second)
	var silence int
	for i := 3; i < 10000; i++ {
		e.Push(1, uint64(i*480), pcm)
		n := 480
		if i%20 == 0 {
			n = 481
		}
		out := make([]int16, n)
		e.render(out)
		for _, v := range out {
			if v == 0 {
				silence++
			}
		}
		if len(e.frames) > 6 {
			t.Fatal("drift grew buffer")
		}
	}
	if silence > 4800 {
		t.Fatalf("clock drift caused sustained silence: %d", silence)
	}
}
func TestClockSlipEvictsSkippedFrame(t *testing.T) {
	e := New()
	e.ready = true
	_ = e.Begin(1)
	e.first = time.Now().Add(-time.Second)
	e.started = true
	e.next = 479
	e.frames[0] = make([]int16, 480)
	e.frames[2400] = make([]int16, 480)
	e.render(make([]int16, 1))
	if _, exists := e.frames[0]; exists {
		t.Fatal("skipped frame remained in bounded buffer")
	}
}

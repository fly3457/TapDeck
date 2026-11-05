package video

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

type fakeSource struct {
	frames chan Frame
	once   sync.Once
	closed chan struct{}
}

func newFakeSource() *fakeSource {
	return &fakeSource{frames: make(chan Frame, 64), closed: make(chan struct{})}
}
func (f *fakeSource) Next() (Frame, error) {
	select {
	case fr, ok := <-f.frames:
		if !ok {
			return Frame{}, io.EOF
		}
		return fr, nil
	case <-f.closed:
		return Frame{}, io.EOF
	}
}
func (f *fakeSource) Close() error { f.once.Do(func() { close(f.closed) }); return nil }

type fakeLauncher struct {
	available bool
	prepared  int
	source    *fakeSource
	err       error
}

func (l *fakeLauncher) Available() bool { return l.available }
func (l *fakeLauncher) Prepare(r Request) (Plan, error) {
	l.prepared++
	if l.err != nil {
		return Plan{}, l.err
	}
	return NewPlan(Display{Width: 1920, Height: 1080}, r, "h264_nvenc")
}
func (l *fakeLauncher) Launch(context.Context, Plan) (Source, error) { return l.source, nil }
func (l *fakeLauncher) Cursor(Display) (int, int, bool)              { return 10, 20, true }

func TestManagerOwnershipAndClaim(t *testing.T) {
	now := time.Unix(1000, 0)
	m := NewManager(&fakeLauncher{available: true})
	m.now = func() time.Time { return now }
	a, err := m.Start(1, Request{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Start(2, Request{}); !errors.Is(err, ErrBusy) {
		t.Fatalf("second owner: %v", err)
	}
	// 同一会话重新请求会替换旧流，旧令牌失效。
	b, err := m.Start(1, Request{})
	if err != nil || a.ctx.Err() == nil {
		t.Fatalf("restart: %v", err)
	}
	if _, err = m.Claim(a.ID, a.Token, 1); err == nil {
		t.Fatal("old stream claimable")
	}
	if _, err = m.Claim(b.ID, []byte("wrong"), 1); err == nil {
		t.Fatal("wrong token accepted")
	}
	if _, err = m.Claim(b.ID, b.Token, 2); err == nil {
		t.Fatal("another session claimed the stream")
	}
	if st, err := m.Claim(b.ID, b.Token, 1); err != nil || st != b {
		t.Fatalf("claim: %v", err)
	}
	if _, err = m.Claim(b.ID, b.Token, 1); err == nil {
		t.Fatal("stream claimed twice")
	}
	if m.Owner() != 1 {
		t.Fatal(m.Owner())
	}
	m.Stop(2)
	if m.Owner() != 1 {
		t.Fatal("other owner stopped the stream")
	}
	m.Stop(1)
	if m.Owner() != 0 || b.ctx.Err() == nil {
		t.Fatal("stop did not end stream")
	}
}

func TestManagerUnclaimedStreamExpires(t *testing.T) {
	now := time.Unix(1000, 0)
	m := NewManager(&fakeLauncher{available: true})
	m.now = func() time.Time { return now }
	a, _ := m.Start(1, Request{})
	now = now.Add(attachTTL + time.Second)
	if _, err := m.Claim(a.ID, a.Token, 1); err == nil {
		t.Fatal("expired stream claimed")
	}
	if _, err := m.Start(2, Request{}); err != nil {
		t.Fatalf("expired stream still busy: %v", err)
	}
}

func TestManagerUnavailable(t *testing.T) {
	l := &fakeLauncher{}
	if _, err := NewManager(l).Start(1, Request{}); !errors.Is(err, ErrNoEncoder) || l.prepared != 0 {
		t.Fatal(err)
	}
	var none *Manager
	if none.Available() || none.Owner() != 0 {
		t.Fatal("nil manager")
	}
}

func TestPumpDropsToNextKeyFrame(t *testing.T) {
	src := newFakeSource()
	queue := make(chan []byte, 3)
	st := &Stream{}
	src.frames <- Frame{Config: true, Data: []byte{0x67}}
	src.frames <- Frame{Key: true, Data: []byte{0x65}}
	for i := 0; i < 4; i++ { // 队列满：触发丢弃
		src.frames <- Frame{Data: []byte{0x41, byte(i)}}
	}
	src.frames <- Frame{Data: []byte{0x41, 9}} // 等待关键帧期间被丢弃
	src.frames <- Frame{Key: true, Data: []byte{0x65, 2}}
	close(src.frames)
	if err := pump(src, queue, st); err != io.EOF {
		t.Fatal(err)
	}
	var got [][]byte
	for len(queue) > 0 {
		got = append(got, <-queue)
	}
	if len(got) != 2 || got[0][1] != FlagConfig || got[1][1] != FlagKey || got[1][len(got[1])-1] != 2 {
		t.Fatalf("queue after drop: %x", got)
	}
	if st.Dropped.Load() == 0 {
		t.Fatal("drop not counted")
	}
}

func TestRunWritesFramesAndCursor(t *testing.T) {
	src := newFakeSource()
	l := &fakeLauncher{available: true, source: src}
	m := NewManager(l)
	st, _ := m.Start(7, Request{})
	src.frames <- Frame{Config: true, Data: []byte{0, 0, 0, 1, 0x67}}
	src.frames <- Frame{Key: true, PTS: 16 * time.Millisecond, Data: []byte{0, 0, 0, 1, 0x65}}
	var mu sync.Mutex
	var frames, cursors int
	var keyPTS uint64
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- m.Run(ctx, st, func(_ context.Context, b []byte) error {
			mu.Lock()
			defer mu.Unlock()
			switch b[0] {
			case MessageFrame:
				frames++
				if b[1]&FlagKey != 0 {
					keyPTS = binary.LittleEndian.Uint64(b[2:])
				}
			case MessageCursor:
				cursors++
				if int32(binary.LittleEndian.Uint32(b[1:])) != 10 || int32(binary.LittleEndian.Uint32(b[5:])) != 20 || b[9] != 1 {
					t.Errorf("cursor %x", b)
				}
			}
			return nil
		})
	}()
	time.Sleep(100 * time.Millisecond)
	m.Stop(7)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if frames != 2 || keyPTS != 16000 || cursors != 1 {
		t.Fatalf("frames=%d pts=%d cursors=%d (unchanged cursor must be sent once)", frames, keyPTS, cursors)
	}
	if m.Owner() != 0 {
		t.Fatal("finished stream still active")
	}
}

func TestTimingMessageAndSendStamp(t *testing.T) {
	recv := time.UnixMicro(1_700_000_000_000_000)
	b := TimingMessage(1500*time.Millisecond, 7*time.Millisecond, recv)
	if len(b) != 25 || b[0] != MessageTiming {
		t.Fatalf("timing %x", b)
	}
	stampSend(b, recv.Add(3*time.Millisecond))
	if pts := binary.LittleEndian.Uint64(b[1:]); pts != 1_500_000 {
		t.Fatalf("pts %d", pts)
	}
	if enc := binary.LittleEndian.Uint32(b[9:]); enc != 7000 {
		t.Fatalf("encode %d", enc)
	}
	if wait := binary.LittleEndian.Uint32(b[13:]); wait != 3000 {
		t.Fatalf("queue wait %d", wait)
	}
	if sent := int64(binary.LittleEndian.Uint64(b[17:])); sent != recv.Add(3*time.Millisecond).UnixMicro() {
		t.Fatalf("send time %d", sent)
	}
}

// 截屏起点取“收到时刻 − pts”的最小值：最快一帧为 0，其后的帧报告多出的耗时。
func TestCaptureClockUsesFastestFrameAsOrigin(t *testing.T) {
	var c captureClock
	t0 := time.Unix(100, 0)
	if d := c.encode(t0.Add(20*time.Millisecond), 0); d != 0 {
		t.Fatalf("first %v", d)
	}
	// 第二帧 pts 16ms、收到于 30ms：比第一帧快 6ms，成为新的起点。
	if d := c.encode(t0.Add(30*time.Millisecond), 16*time.Millisecond); d != 0 {
		t.Fatalf("faster frame %v", d)
	}
	if d := c.encode(t0.Add(60*time.Millisecond), 33*time.Millisecond); d != 13*time.Millisecond {
		t.Fatalf("slower frame %v", d)
	}
}

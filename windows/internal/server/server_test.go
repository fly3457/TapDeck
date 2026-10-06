package server

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"net"
	"net/http/httptest"
	"strings"
	"tapdeck/internal/config"
	"tapdeck/internal/protocol"
	"testing"
)

type fakeInput struct{ events []string }

func (f *fakeInput) Move(x, y, sx, sy int32) error {
	if x != 0 || y != 0 {
		f.events = append(f.events, "move")
	}
	return nil
}
func (f *fakeInput) Button(b string, d bool) error {
	if d {
		f.events = append(f.events, "down")
	} else {
		f.events = append(f.events, "up")
	}
	return nil
}
func (f *fakeInput) Chord(s string) error        { f.events = append(f.events, s); return nil }
func (f *fakeInput) Hold(s string, d bool) error { return nil }
func (f *fakeInput) ReleaseAll()                 { f.events = append(f.events, "release") }
func TestCrossChannelDragBarrier(t *testing.T) {
	f := &fakeInput{}
	s := &Server{Input: f, cfg: config.Default()}
	ss := &session{s: s, active: true}
	if e := ss.move(protocol.Movement{Epoch: 1, X: 100 * 1024}); e != nil {
		t.Fatal(e)
	}
	if len(f.events) != 0 {
		t.Fatal("future movement ran before down")
	}
	if e := ss.handle(Message{Type: "mouse_button", Epoch: 0, NextEpoch: 1, X: 40 * 1024, Button: "left", Down: true}); e != nil {
		t.Fatal(e)
	}
	if got := strings.Join(f.events, ","); got != "move,down,move" {
		t.Fatal(got)
	}
	if ss.last.X != 100*1024 {
		t.Fatal("pending cumulative movement lost")
	}
	if e := ss.move(protocol.Movement{Epoch: 0, X: 1}); e != nil {
		t.Fatal(e)
	}
	if ss.last.X != 100*1024 {
		t.Fatal("old epoch changed cursor")
	}
}
func TestCumulativePacketLoss(t *testing.T) {
	f := &fakeInput{}
	s := &Server{Input: f}
	ss := &session{s: s}
	_ = ss.move(protocol.Movement{X: 100 * 1024})
	_ = ss.move(protocol.Movement{X: 300 * 1024})
	if ss.last.X != 300*1024 {
		t.Fatal("loss was not recovered")
	}
}
func TestBootstrapDoesNotExposeCredentials(t *testing.T) {
	s, e := New(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	s.tokens["device"] = "sensitive-token-hash"
	m := httptest.NewRecorder()
	s.pairPage(m, httptest.NewRequest("GET", "http://127.0.0.1/pair", nil))
	if strings.Contains(m.Body.String(), s.secret) || strings.Contains(m.Body.String(), "sensitive-token-hash") {
		t.Fatal("bootstrap exposed credentials")
	}
	b, _ := json.Marshal(s.metadata())
	if strings.Contains(string(b), s.secret) {
		t.Fatal("metadata exposed secret")
	}
}
func TestCodecSessions(t *testing.T) {
	a, _ := protocol.NewCodec(make([]byte, 32), [4]byte{}, 1, protocol.Audio)
	b, _ := protocol.NewCodec(make([]byte, 32), [4]byte{}, 2, protocol.Audio)
	p := a.Seal(0, make([]byte, 976))
	if binary.LittleEndian.Uint64(p[8:]) != 1 {
		t.Fatal("bad session encoding")
	}
	if _, _, e := b.Open(p); e == nil {
		t.Fatal("old connection accepted")
	}
}
func TestCanStartAndStop(t *testing.T) {
	s, e := New(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	getPort := func() int {
		l, e := net.Listen("tcp", "127.0.0.1:0")
		if e != nil {
			t.Fatal(e)
		}
		p := l.Addr().(*net.TCPAddr).Port
		l.Close()
		return p
	}
	s.cfg.HTTPPort = getPort()
	s.cfg.WSSPort = getPort()
	s.cfg.UDPPort = getPort()
	if e = s.Start(); e != nil {
		t.Fatal(e)
	}
	s.Stop()
	if s.Snapshot().Running {
		t.Fatal("still running")
	}
	_ = context.Background()
}

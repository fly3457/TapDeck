package protocol

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestAuthenticatedPackets(t *testing.T) {
	key := make([]byte, 32)
	c, e := NewCodec(key, [4]byte{1, 2, 3, 4}, 42, Mouse)
	if e != nil {
		t.Fatal(e)
	}
	m := Movement{Epoch: 2, X: 1024, Y: -2048, ScrollY: 120 * 1024}
	p := c.Seal(7, m.Bytes())
	seq, b, e := c.Open(p)
	if e != nil || seq != 7 {
		t.Fatal(seq, e)
	}
	got, e := ParseMovement(b)
	if e != nil || got != m {
		t.Fatal(got, e)
	}
	p[len(p)-1] ^= 1
	if _, _, e = c.Open(p); e == nil {
		t.Fatal("tampering accepted")
	}
	wrong, _ := NewCodec(key, [4]byte{1, 2, 3, 4}, 42, Audio)
	if _, _, e = wrong.Open(c.Seal(7, m.Bytes())); e == nil {
		t.Fatal("wrong channel accepted")
	}
}
func TestReplayWindow(t *testing.T) {
	var w ReplayWindow
	for _, v := range []uint64{3, 1, 2, 260, 259} {
		if !w.Accept(v) {
			t.Fatal("reordering rejected", v)
		}
	}
	for _, v := range []uint64{3, 1, 260, 259} {
		if w.Accept(v) {
			t.Fatal("replay accepted", v)
		}
	}
	if !w.Accept(1000) || w.Accept(260) {
		t.Fatal("window did not advance")
	}
}
func TestGolden(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	c, _ := NewCodec(key, [4]byte{0x11, 0x22, 0x33, 0x44}, 0x0102030405060708, Mouse)
	m := Movement{3, 1024, -2048, 0, 122880}
	p := c.Seal(9, m.Bytes())
	const golden = "54444b310100000008070605040302010900000000000000733488da86b890b41259a3809e84f94ad41b8662462290bac51fca3471e832104297707dd4debe4b3dc28aaaf5a7aaf549e21408"
	if hex.EncodeToString(p) != golden {
		t.Fatal("shared Kotlin/Go vector changed")
	}
	seq, b, e := c.Open(p)
	if e != nil || seq != 9 || !bytes.Equal(b, m.Bytes()) {
		t.Fatal(e)
	}
}
func FuzzRejectMalformed(f *testing.F) {
	f.Add([]byte("TDK1"))
	c, _ := NewCodec(make([]byte, 32), [4]byte{}, 1, Mouse)
	f.Add(c.Seal(0, Movement{}.Bytes()))
	f.Fuzz(func(t *testing.T, b []byte) { _, _, _ = c.Open(b) })
}

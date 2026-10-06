// Package protocol is the versioned wire contract shared with the Android client.
package protocol

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"math"
)

const HeaderSize = 24
const Mouse byte = 1
const Audio byte = 2

type Codec struct {
	aead    cipher.AEAD
	Prefix  [4]byte
	Session uint64
	Kind    byte
}

func NewCodec(key []byte, prefix [4]byte, session uint64, kind byte) (*Codec, error) {
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	a, err := cipher.NewGCM(b)
	if err != nil {
		return nil, err
	}
	return &Codec{a, prefix, session, kind}, nil
}
func (c *Codec) Seal(seq uint64, body []byte) []byte {
	h := make([]byte, HeaderSize)
	copy(h, "TDK1")
	h[4] = c.Kind
	binary.LittleEndian.PutUint64(h[8:], c.Session)
	binary.LittleEndian.PutUint64(h[16:], seq)
	var nonce [12]byte
	copy(nonce[:4], c.Prefix[:])
	binary.LittleEndian.PutUint64(nonce[4:], seq)
	return c.aead.Seal(h, nonce[:], body, h)
}
func (c *Codec) Open(packet []byte) (uint64, []byte, error) {
	if len(packet) < HeaderSize+16 || len(packet) > 1200 || string(packet[:4]) != "TDK1" || packet[4] != c.Kind || packet[5] != 0 || packet[6] != 0 || packet[7] != 0 || binary.LittleEndian.Uint64(packet[8:]) != c.Session {
		return 0, nil, errors.New("invalid datagram")
	}
	seq := binary.LittleEndian.Uint64(packet[16:])
	var nonce [12]byte
	copy(nonce[:4], c.Prefix[:])
	binary.LittleEndian.PutUint64(nonce[4:], seq)
	body, err := c.aead.Open(nil, nonce[:], packet[HeaderSize:], packet[:HeaderSize])
	return seq, body, err
}

// ReplayWindow accepts bounded reordering for audio, and authenticates before commit.
type ReplayWindow struct {
	Highest     uint64
	Bits        [4]uint64
	Initialized bool
}

func (w *ReplayWindow) Accept(seq uint64) bool {
	if !w.Initialized {
		w.Initialized = true
		w.Highest = seq
		w.Bits[0] = 1
		return true
	}
	if seq > w.Highest {
		d := seq - w.Highest
		if d >= 256 {
			w.Bits = [4]uint64{}
		} else {
			old := w.Bits
			w.Bits = [4]uint64{}
			for i := uint64(0); i+d < 256; i++ {
				if old[i/64]&(uint64(1)<<(i%64)) != 0 {
					j := i + d
					w.Bits[j/64] |= uint64(1) << (j % 64)
				}
			}
		}
		w.Highest = seq
		w.Bits[0] |= 1
		return true
	}
	d := w.Highest - seq
	if d >= 256 {
		return false
	}
	bit := uint64(1) << (d % 64)
	if w.Bits[d/64]&bit != 0 {
		return false
	}
	w.Bits[d/64] |= bit
	return true
}

type Movement struct {
	Epoch   uint32 `json:"epoch"`
	X       int64  `json:"x"`
	Y       int64  `json:"y"`
	ScrollX int64  `json:"scroll_x"`
	ScrollY int64  `json:"scroll_y"`
}

func ParseMovement(b []byte) (Movement, error) {
	if len(b) != 36 {
		return Movement{}, errors.New("invalid movement length")
	}
	m := Movement{binary.LittleEndian.Uint32(b), int64(binary.LittleEndian.Uint64(b[4:])), int64(binary.LittleEndian.Uint64(b[12:])), int64(binary.LittleEndian.Uint64(b[20:])), int64(binary.LittleEndian.Uint64(b[28:]))}
	for _, n := range []int64{m.X, m.Y, m.ScrollX, m.ScrollY} {
		if math.Abs(float64(n)) > 1e14 {
			return Movement{}, errors.New("movement out of range")
		}
	}
	return m, nil
}
func (m Movement) Bytes() []byte {
	b := make([]byte, 36)
	binary.LittleEndian.PutUint32(b, m.Epoch)
	for i, n := range []int64{m.X, m.Y, m.ScrollX, m.ScrollY} {
		binary.LittleEndian.PutUint64(b[4+i*8:], uint64(n))
	}
	return b
}

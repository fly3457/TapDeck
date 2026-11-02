package video

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
	"time"
)

func tag(kind byte, ts uint32, body []byte) []byte {
	h := []byte{kind, byte(len(body) >> 16), byte(len(body) >> 8), byte(len(body)), byte(ts >> 16), byte(ts >> 8), byte(ts), byte(ts >> 24), 0, 0, 0}
	out := append(h, body...)
	return binary.BigEndian.AppendUint32(out, uint32(len(out)))
}

func flvStream(tags ...[]byte) []byte {
	out := []byte{'F', 'L', 'V', 1, 1, 0, 0, 0, 9, 0, 0, 0, 0}
	for _, t := range tags {
		out = append(out, t...)
	}
	return out
}

func TestFLVReaderConvertsConfigAndFrames(t *testing.T) {
	sps, pps := []byte{0x67, 1, 2, 3}, []byte{0x68, 4}
	config := []byte{0x17, 0, 0, 0, 0, 1, 0x64, 0, 0x28, 0xff, 0xe1, 0, 4}
	config = append(config, sps...)
	config = append(config, 1, 0, 2)
	config = append(config, pps...)
	idr := []byte{0x17, 1, 0, 0, 0x21, 0, 0, 0, 2, 0x65, 9, 0, 0, 0, 1, 0x06}
	inter := []byte{0x27, 1, 0, 0, 0, 0, 0, 0, 3, 0x41, 7, 8}
	data := flvStream(tag(18, 0, []byte("onMetaData")), tag(9, 0, config), tag(8, 0, []byte{1}), tag(9, 16, idr), tag(9, 33, inter))
	r := newFLVReader(bytes.NewReader(data))

	f, err := r.Next()
	if err != nil || !f.Config || !bytes.Equal(f.Data, []byte{0, 0, 0, 1, 0x67, 1, 2, 3, 0, 0, 0, 1, 0x68, 4}) {
		t.Fatalf("config %+v %v", f, err)
	}
	f, err = r.Next()
	if err != nil || !f.Key || f.PTS != 49*time.Millisecond || !bytes.Equal(f.Data, []byte{0, 0, 0, 1, 0x65, 9, 0, 0, 0, 1, 0x06}) {
		t.Fatalf("idr %+v %v", f, err)
	}
	f, err = r.Next()
	if err != nil || f.Key || f.Config || f.PTS != 33*time.Millisecond || !bytes.Equal(f.Data, []byte{0, 0, 0, 1, 0x41, 7, 8}) {
		t.Fatalf("inter %+v %v", f, err)
	}
	if _, err = r.Next(); err != io.EOF {
		t.Fatalf("expected EOF, got %v", err)
	}
}

func TestFLVReaderRejectsBrokenInput(t *testing.T) {
	for name, data := range map[string][]byte{
		"magic":       []byte("FLX\x01\x01\x00\x00\x00\x09\x00\x00\x00\x00"),
		"no config":   flvStream(tag(9, 0, []byte{0x17, 1, 0, 0, 0, 0, 0, 0, 1, 0x65})),
		"not h264":    flvStream(tag(9, 0, []byte{0x12, 1, 0, 0, 0})),
		"bad nal len": flvStream(tag(9, 0, []byte{0x17, 0, 0, 0, 0, 1, 0x64, 0, 0x28, 0xff, 0xe1, 0, 1, 0x67, 0}), tag(9, 0, []byte{0x17, 1, 0, 0, 0, 0, 0, 0, 9, 0x65})),
	} {
		r := newFLVReader(bytes.NewReader(data))
		var err error
		for i := 0; i < 3 && err == nil; i++ {
			_, err = r.Next()
		}
		if err == nil || err == io.EOF {
			t.Errorf("%s: expected parse error, got %v", name, err)
		}
	}
}

func TestToAnnexBShortLengths(t *testing.T) {
	out, err := toAnnexB([]byte{0, 2, 0x65, 1, 0, 1, 0x06}, 2)
	if err != nil || !bytes.Equal(out, []byte{0, 0, 0, 1, 0x65, 1, 0, 0, 0, 1, 0x06}) {
		t.Fatalf("%x %v", out, err)
	}
}

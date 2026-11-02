// Package video 把 PC 主显示器画面编码为 H.264，经独立 WSS 连接发给手机。
//
// 截屏与硬件编码由内嵌的精简 LGPL ffmpeg 子进程完成（ddagrab + NVENC / AMF /
// Media Foundation）。ffmpeg 输出 FLV：每个 tag 头部带长度，收到一帧即可转发，
// 不必等待下一帧的起始码，避免裸 Annex-B 流固有的一帧延迟。
package video

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"
)

// Frame 是一个完整的 H.264 访问单元，统一转换为 Annex-B 起始码格式。
type Frame struct {
	Data []byte
	// Key 表示 IDR 关键帧；Config 表示 SPS/PPS 配置（解码器的 csd）。
	Key, Config bool
	PTS         time.Duration
}

const maxTag = 8 << 20

var startCode = []byte{0, 0, 0, 1}

// flvReader 只解析 ffmpeg flv 复用器输出的视频 tag，脚本与音频 tag 一律跳过。
type flvReader struct {
	r      *bufio.Reader
	nalLen int
	header bool
}

func newFLVReader(r io.Reader) *flvReader {
	return &flvReader{r: bufio.NewReaderSize(r, 256<<10)}
}

func (f *flvReader) readHeader() error {
	var h [9]byte
	if _, err := io.ReadFull(f.r, h[:]); err != nil {
		return err
	}
	if string(h[:3]) != "FLV" {
		return errors.New("不是 FLV 流")
	}
	offset := int(binary.BigEndian.Uint32(h[5:9]))
	if offset < 9 || offset > 1024 {
		return errors.New("FLV 头部长度无效")
	}
	if _, err := f.r.Discard(offset - 9 + 4); err != nil { // 额外头部 + PreviousTagSize0
		return err
	}
	f.header = true
	return nil
}

// Next 返回下一个视频访问单元或配置帧。
func (f *flvReader) Next() (Frame, error) {
	if !f.header {
		if err := f.readHeader(); err != nil {
			return Frame{}, err
		}
	}
	for {
		var h [11]byte
		if _, err := io.ReadFull(f.r, h[:]); err != nil {
			return Frame{}, err
		}
		size := int(h[1])<<16 | int(h[2])<<8 | int(h[3])
		if size > maxTag {
			return Frame{}, fmt.Errorf("FLV tag 过大：%d", size)
		}
		ts := int64(h[7])<<24 | int64(h[4])<<16 | int64(h[5])<<8 | int64(h[6])
		body := make([]byte, size)
		if _, err := io.ReadFull(f.r, body); err != nil {
			return Frame{}, err
		}
		if _, err := f.r.Discard(4); err != nil { // PreviousTagSize
			return Frame{}, err
		}
		if h[0] != 9 {
			continue
		}
		frame, ok, err := f.video(body, ts)
		if err != nil {
			return Frame{}, err
		}
		if ok {
			return frame, nil
		}
	}
}

func (f *flvReader) video(body []byte, ts int64) (Frame, bool, error) {
	if len(body) < 5 {
		return Frame{}, false, errors.New("FLV 视频 tag 过短")
	}
	if body[0]&0x0f != 7 {
		return Frame{}, false, errors.New("FLV 视频不是 H.264")
	}
	key := body[0]>>4 == 1
	cts := int64(int32(uint32(body[2])<<24|uint32(body[3])<<16|uint32(body[4])<<8) >> 8)
	pts := time.Duration(ts+cts) * time.Millisecond
	data := body[5:]
	switch body[1] {
	case 0:
		config, nalLen, err := parseAVCConfig(data)
		if err != nil {
			return Frame{}, false, err
		}
		f.nalLen = nalLen
		return Frame{Data: config, Config: true, PTS: pts}, true, nil
	case 1:
		if f.nalLen == 0 {
			return Frame{}, false, errors.New("缺少 H.264 配置")
		}
		annexB, err := toAnnexB(data, f.nalLen)
		if err != nil {
			return Frame{}, false, err
		}
		return Frame{Data: annexB, Key: key, PTS: pts}, true, nil
	default:
		return Frame{}, false, nil
	}
}

// parseAVCConfig 把 AVCDecoderConfigurationRecord 转为 Annex-B 的 SPS+PPS。
func parseAVCConfig(b []byte) ([]byte, int, error) {
	bad := errors.New("H.264 配置记录无效")
	if len(b) < 6 || b[0] != 1 {
		return nil, 0, bad
	}
	nalLen := int(b[4]&3) + 1
	out := []byte{}
	pos := 5
	for group := 0; group < 2; group++ {
		if pos >= len(b) {
			return nil, 0, bad
		}
		count := int(b[pos])
		if group == 0 {
			count &= 0x1f
		}
		pos++
		for i := 0; i < count; i++ {
			if pos+2 > len(b) {
				return nil, 0, bad
			}
			n := int(binary.BigEndian.Uint16(b[pos:]))
			pos += 2
			if n == 0 || pos+n > len(b) {
				return nil, 0, bad
			}
			out = append(append(out, startCode...), b[pos:pos+n]...)
			pos += n
		}
	}
	if len(out) == 0 {
		return nil, 0, bad
	}
	return out, nalLen, nil
}

// toAnnexB 把长度前缀的 NAL 单元改为 4 字节起始码；4 字节长度时原地替换。
func toAnnexB(b []byte, nalLen int) ([]byte, error) {
	out := b
	if nalLen != 4 {
		out = make([]byte, 0, len(b)+len(b)/8)
	}
	for pos := 0; pos < len(b); {
		if pos+nalLen > len(b) {
			return nil, errors.New("H.264 NAL 长度截断")
		}
		n := 0
		for i := 0; i < nalLen; i++ {
			n = n<<8 | int(b[pos+i])
		}
		start := pos + nalLen
		if n == 0 || start+n > len(b) {
			return nil, errors.New("H.264 NAL 长度无效")
		}
		if nalLen == 4 {
			copy(out[pos:], startCode)
		} else {
			out = append(append(out, startCode...), b[start:start+n]...)
		}
		pos = start + n
	}
	if len(out) == 0 {
		return nil, errors.New("空 H.264 帧")
	}
	return out, nil
}

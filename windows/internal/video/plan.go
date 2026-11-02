package video

import (
	"fmt"
	"strconv"
)

// Quality 是手机端可选的画质档位。
type Quality struct {
	Name                string
	MaxWidth, MaxHeight int
	Kbps                int
}

var qualities = map[string]Quality{
	"smooth": {"smooth", 1920, 1080, 12000},
	"clear":  {"clear", 2560, 1440, 20000},
	"saver":  {"saver", 1280, 720, 6000},
}

// Request 是手机 video_start 的参数；MaxWidth/MaxHeight 为手机解码器上限，0 表示不限制。
type Request struct {
	Quality             string
	MaxWidth, MaxHeight int
	FPS                 int
}

// Display 描述被捕获的显示器：DXGI 适配器与输出序号及桌面坐标（物理像素）。
type Display struct {
	Adapter, Output int
	Left, Top       int
	Width, Height   int
}

// Plan 是一次编码会话的完整参数。
type Plan struct {
	Display
	Encoder       string
	Width, Height int
	FPS, Kbps     int
}

// NewPlan 按档位与手机上限等比缩小画面，从不放大，宽高取偶数。
func NewPlan(d Display, r Request, encoder string) (Plan, error) {
	q, ok := qualities[r.Quality]
	if !ok {
		q = qualities["smooth"]
	}
	if d.Width <= 0 || d.Height <= 0 {
		return Plan{}, fmt.Errorf("显示器尺寸无效")
	}
	maxW, maxH := q.MaxWidth, q.MaxHeight
	if d.Height > d.Width { // 竖向放置的显示器：档位上限也转置
		maxW, maxH = maxH, maxW
	}
	if r.MaxWidth > 0 && r.MaxHeight > 0 {
		// 手机解码器上限与方向无关，取较长边与较短边分别比较。
		long, short := max(r.MaxWidth, r.MaxHeight), min(r.MaxWidth, r.MaxHeight)
		if d.Width >= d.Height {
			maxW, maxH = min(maxW, long), min(maxH, short)
		} else {
			maxW, maxH = min(maxW, short), min(maxH, long)
		}
	}
	scale := min(1, min(float64(maxW)/float64(d.Width), float64(maxH)/float64(d.Height)))
	w, h := int(float64(d.Width)*scale)&^1, int(float64(d.Height)*scale)&^1
	if w < 2 || h < 2 {
		return Plan{}, fmt.Errorf("画面尺寸无效")
	}
	fps := r.FPS
	if fps <= 0 || fps > 60 {
		fps = 60
	}
	if fps < 15 {
		fps = 15
	}
	kbps := q.Kbps * w * h / (q.MaxWidth * q.MaxHeight)
	kbps = max(kbps, q.Kbps/3) * fps / 60
	return Plan{Display: d, Encoder: encoder, Width: w, Height: h, FPS: fps, Kbps: max(kbps, 1000)}, nil
}

// Encoders 是按优先级排列的硬件 H.264 编码器；Media Foundation 会选用厂商硬件 MFT，
// 覆盖 Intel 核显，也是没有独立显卡时的兜底。
var Encoders = []string{"h264_nvenc", "h264_amf", "h264_mf"}

// Args 生成 ffmpeg 参数：ddagrab 在 GPU 上截屏，按编码器选择 GPU 内缩放或下载后缩放，
// 关闭 B 帧与前瞻，GOP 1 秒，输出 FLV 到标准输出。probe 为真时只编码几帧到 null。
func (p Plan) Args(probe bool) []string {
	k := strconv.Itoa(p.Kbps) + "k"
	// VBV 约为两帧大小，限制单帧尖峰，避免网络突发拖慢后续帧。
	vbv := strconv.Itoa(max(p.Kbps*2/p.FPS, 200)) + "k"
	gop := strconv.Itoa(p.FPS)
	chain := fmt.Sprintf("ddagrab=output_idx=%d:framerate=%d:draw_mouse=1:dup_frames=1", p.Output, p.FPS)
	scaled := p.Width != p.Display.Width || p.Height != p.Display.Height
	gpu := p.Encoder != "h264_mf"
	switch {
	case gpu && scaled:
		chain += fmt.Sprintf(",scale_d3d11=width=%d:height=%d", p.Width, p.Height)
	case !gpu:
		chain += ",hwdownload,format=bgra"
		if scaled {
			chain += fmt.Sprintf(",scale=%d:%d:flags=fast_bilinear", p.Width, p.Height)
		}
		chain += ",format=nv12"
	}
	args := []string{"-hide_banner", "-nostdin", "-loglevel", "warning",
		"-init_hw_device", "d3d11va=dda:" + strconv.Itoa(p.Adapter), "-filter_hw_device", "dda",
		"-filter_complex", chain + "[v]", "-map", "[v]", "-an", "-c:v", p.Encoder}
	switch p.Encoder {
	case "h264_nvenc":
		args = append(args, "-preset", "p1", "-tune", "ull", "-zerolatency", "1", "-delay", "0",
			"-rc", "cbr", "-b:v", k, "-maxrate", k, "-bufsize", vbv, "-g", gop, "-bf", "0", "-forced-idr", "1", "-profile:v", "high")
	case "h264_amf":
		args = append(args, "-usage", "ultralowlatency", "-quality", "speed", "-rc", "cbr",
			"-b:v", k, "-maxrate", k, "-bufsize", vbv, "-g", gop, "-bf", "0", "-profile:v", "high")
	default:
		args = append(args, "-hw_encoding", "1", "-scenario", "display_remoting", "-rate_control", "cbr",
			"-b:v", k, "-g", gop, "-bf", "0")
	}
	if probe {
		return append(args, "-frames:v", "5", "-f", "null", "-")
	}
	return append(args, "-flush_packets", "1", "-f", "flv", "-flvflags", "no_duration_filesize", "pipe:1")
}

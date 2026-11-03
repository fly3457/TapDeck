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
	Path          Path
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
	// 码率按像素数折算，基准不超过 1080p：清晰档在小于 1440p 的屏幕上也不会低于流畅档。
	ref := min(q.MaxWidth*q.MaxHeight, 1920*1080)
	kbps := min(q.Kbps, q.Kbps*w*h/ref)
	kbps = max(kbps, q.Kbps/3) * fps / 60
	return Plan{Display: d, Encoder: encoder, Width: w, Height: h, FPS: fps, Kbps: max(kbps, 1000)}, nil
}

// Path 是画面从 ddagrab 到编码器的处理路径。
type Path int

const (
	// GPUScale：scale_d3d11 在显存内缩放并转成 NV12，零拷贝交给编码器。
	GPUScale Path = iota
	// Native：不缩放，编码器直接读取 ddagrab 的 BGRA 显存纹理（零拷贝）。
	// 部分旧驱动不支持 scale_d3d11 创建的编码纹理时使用。
	Native
	// CPUScale：下载到内存缩放并转换格式，最后的兜底。
	CPUScale
)

func (p Path) String() string { return [...]string{"GPU 缩放", "原生分辨率", "CPU 缩放"}[p] }

// Encoders 是按优先级排列的硬件 H.264 编码器；Media Foundation 会选用厂商硬件 MFT，
// 覆盖 Intel 核显，也是没有独立显卡时的兜底。
var Encoders = []string{"h264_nvenc", "h264_amf", "h264_mf"}

// Variants 返回按优先级排列的探测组合：先所有编码器的 GPU 缩放，再原生分辨率零拷贝，
// 最后 CPU 缩放。原生分辨率只在不超过手机解码上限、宽高为偶数且不是省流档时提供；
// 码率按像素数提高，但不超过档位码率的 1.5 倍。
func (p Plan) Variants(r Request) []Plan {
	q, ok := qualities[r.Quality]
	if !ok {
		q = qualities["smooth"]
	}
	var out []Plan
	add := func(path Path) {
		for _, enc := range Encoders {
			v := p
			v.Encoder, v.Path = enc, path
			if path == Native {
				v.Width, v.Height = p.Display.Width, p.Display.Height
				v.Kbps = min(p.Kbps*v.Width*v.Height/max(1, p.Width*p.Height), q.Kbps*3/2*p.FPS/60)
			}
			out = append(out, v)
		}
	}
	add(GPUScale)
	scaled := p.Width != p.Display.Width || p.Height != p.Display.Height
	long, short := max(r.MaxWidth, r.MaxHeight), min(r.MaxWidth, r.MaxHeight)
	fits := r.MaxWidth <= 0 || r.MaxHeight <= 0 || (max(p.Display.Width, p.Display.Height) <= long && min(p.Display.Width, p.Display.Height) <= short)
	even := p.Display.Width%2 == 0 && p.Display.Height%2 == 0
	if even && (!scaled || (r.Quality != "saver" && fits)) {
		add(Native)
	}
	add(CPUScale)
	return out
}

// Args 生成 ffmpeg 参数：ddagrab 在 GPU 上截屏，按 Path 选择显存内缩放、原生零拷贝或 CPU 缩放；
// 关闭 B 帧与前瞻，GOP 1 秒，输出 FLV 到标准输出。probe 为真时只编码几帧到 null。
func (p Plan) Args(probe bool) []string {
	k := strconv.Itoa(p.Kbps) + "k"
	// VBV 约 250ms：关键帧有足够码率，避免过度量化在纯色背景上出现色块闪烁；
	// 局域网 Wi-Fi 的实际吞吐远高于平均码率，突发的关键帧只多占几十毫秒。
	vbv := strconv.Itoa(max(p.Kbps/4, 500)) + "k"
	// GOP 2 秒：减少周期性关键帧带来的画质跳变，丢帧后最多等 2 秒恢复。
	gop := strconv.Itoa(p.FPS * 2)
	chain := fmt.Sprintf("ddagrab=output_idx=%d:framerate=%d:draw_mouse=1:dup_frames=1", p.Output, p.FPS)
	scaled := p.Width != p.Display.Width || p.Height != p.Display.Height
	switch p.Path {
	case GPUScale:
		chain += fmt.Sprintf(",scale_d3d11=width=%d:height=%d:format=nv12", p.Width, p.Height)
	case CPUScale:
		chain += ",hwdownload,format=bgra"
		if scaled {
			chain += fmt.Sprintf(",scale=%d:%d:flags=fast_bilinear", p.Width, p.Height)
		}
		chain += ",format=nv12"
		// NVENC / AMF 从显存读取：重新上传到同一 D3D11 设备；Media Foundation 可直接读内存帧。
		if p.Encoder != "h264_mf" {
			chain += ",hwupload"
		}
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

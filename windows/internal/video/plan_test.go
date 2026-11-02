package video

import (
	"slices"
	"strings"
	"testing"
)

func TestNewPlanScalesWithoutUpscaling(t *testing.T) {
	for _, c := range []struct {
		name          string
		d             Display
		r             Request
		width, height int
	}{
		{"1080p native", Display{Width: 1920, Height: 1080}, Request{Quality: "smooth"}, 1920, 1080},
		{"4K to 1080p", Display{Width: 3840, Height: 2160}, Request{Quality: "smooth"}, 1920, 1080},
		{"1440p clear", Display{Width: 2560, Height: 1440}, Request{Quality: "clear"}, 2560, 1440},
		{"saver", Display{Width: 2560, Height: 1440}, Request{Quality: "saver"}, 1280, 720},
		{"no upscale", Display{Width: 1366, Height: 768}, Request{Quality: "clear"}, 1366, 768},
		{"ultrawide", Display{Width: 3440, Height: 1440}, Request{Quality: "smooth"}, 1920, 802},
		{"portrait monitor", Display{Width: 1080, Height: 1920}, Request{Quality: "saver"}, 720, 1280},
		{"phone decoder limit", Display{Width: 2560, Height: 1440}, Request{Quality: "clear", MaxWidth: 1080, MaxHeight: 1920}, 1920, 1080},
		{"unknown quality", Display{Width: 3840, Height: 2160}, Request{Quality: "x"}, 1920, 1080},
		{"odd size", Display{Width: 1365, Height: 767}, Request{}, 1364, 766},
	} {
		p, err := NewPlan(c.d, c.r, "h264_nvenc")
		if err != nil || p.Width != c.width || p.Height != c.height {
			t.Errorf("%s: %dx%d %v", c.name, p.Width, p.Height, err)
		}
	}
	if _, err := NewPlan(Display{}, Request{}, "h264_nvenc"); err == nil {
		t.Fatal("accepted empty display")
	}
}

func TestNewPlanFrameRateAndBitrate(t *testing.T) {
	p, _ := NewPlan(Display{Width: 1920, Height: 1080}, Request{Quality: "smooth"}, "h264_nvenc")
	if p.FPS != 60 || p.Kbps != 12000 {
		t.Fatalf("%+v", p)
	}
	p, _ = NewPlan(Display{Width: 1920, Height: 1080}, Request{Quality: "smooth", FPS: 30}, "h264_nvenc")
	if p.FPS != 30 || p.Kbps != 6000 {
		t.Fatalf("%+v", p)
	}
	p, _ = NewPlan(Display{Width: 1920, Height: 1080}, Request{FPS: 500}, "h264_nvenc")
	if p.FPS != 60 {
		t.Fatalf("%+v", p)
	}
	// 清晰档在任何屏幕上都不低于流畅档。
	for _, d := range []Display{{Width: 2194, Height: 1234}, {Width: 1366, Height: 768}, {Width: 2560, Height: 1440}, {Width: 3840, Height: 2160}} {
		smooth, _ := NewPlan(d, Request{Quality: "smooth"}, "")
		clear, _ := NewPlan(d, Request{Quality: "clear"}, "")
		saver, _ := NewPlan(d, Request{Quality: "saver"}, "")
		if clear.Kbps < smooth.Kbps || smooth.Kbps < saver.Kbps || clear.Kbps > 20000 {
			t.Errorf("%dx%d: clear %d smooth %d saver %d", d.Width, d.Height, clear.Kbps, smooth.Kbps, saver.Kbps)
		}
	}
}

func TestArgsChooseProcessingPath(t *testing.T) {
	d := Display{Adapter: 1, Output: 2, Width: 3840, Height: 2160}
	nv, _ := NewPlan(d, Request{}, "h264_nvenc")
	a := strings.Join(nv.Args(false), " ")
	for _, want := range []string{"d3d11va=dda:1", "output_idx=2", "scale_d3d11=width=1920:height=1080:format=nv12", "-tune ull", "-bf 0", "-g 60", "-f flv", "pipe:1"} {
		if !strings.Contains(a, want) {
			t.Errorf("nvenc args missing %q: %s", want, a)
		}
	}
	nv.Path = Native
	if a = strings.Join(nv.Args(false), " "); strings.Contains(a, "scale") || strings.Contains(a, "hwdownload") {
		t.Errorf("native must hand ddagrab textures straight to the encoder: %s", a)
	}
	nv.Path = CPUScale
	if a = strings.Join(nv.Args(false), " "); !strings.Contains(a, "hwdownload,format=bgra,scale=1920:1080:flags=fast_bilinear,format=nv12,hwupload") {
		t.Errorf("nvenc cpu args: %s", a)
	}
	mf, _ := NewPlan(d, Request{}, "h264_mf")
	mf.Path = CPUScale
	if a = strings.Join(mf.Args(false), " "); strings.Contains(a, "hwupload") || strings.Contains(a, "scale_d3d11") {
		t.Errorf("mf cpu args: %s", a)
	}
	probe := mf.Args(true)
	if !slices.Contains(probe, "null") || slices.Contains(probe, "pipe:1") {
		t.Errorf("probe args: %v", probe)
	}
}

func TestVariantsOrderAndNativeLimits(t *testing.T) {
	paths := func(vs []Plan) (out []Path) {
		for _, v := range vs {
			if len(out) == 0 || out[len(out)-1] != v.Path {
				out = append(out, v.Path)
			}
		}
		return
	}
	d := Display{Width: 2194, Height: 1234}
	base, _ := NewPlan(d, Request{Quality: "smooth", MaxWidth: 4096, MaxHeight: 2304}, "")
	vs := base.Variants(Request{Quality: "smooth", MaxWidth: 4096, MaxHeight: 2304})
	if !slices.Equal(paths(vs), []Path{GPUScale, Native, CPUScale}) || len(vs) != 9 || vs[0].Encoder != "h264_nvenc" {
		t.Fatalf("order %v", paths(vs))
	}
	native := vs[3]
	if native.Width != 2194 || native.Height != 1234 || native.Kbps <= base.Kbps || native.Kbps > 18000 {
		t.Fatalf("native %+v (base %d kbps)", native, base.Kbps)
	}
	// 省流档、超过手机解码上限或奇数尺寸时不提供原生分辨率。
	for _, c := range []struct {
		d Display
		r Request
	}{
		{d, Request{Quality: "saver"}},
		{Display{Width: 3840, Height: 2160}, Request{Quality: "smooth", MaxWidth: 1920, MaxHeight: 1080}},
		{Display{Width: 1365, Height: 767}, Request{Quality: "smooth"}},
	} {
		p, _ := NewPlan(c.d, c.r, "")
		if slices.Contains(paths(p.Variants(c.r)), Native) {
			t.Errorf("native offered for %+v %+v", c.d, c.r)
		}
	}
	// 已是原生尺寸时，原生零拷贝作为 GPU 转换失败后的首选。
	p, _ := NewPlan(Display{Width: 1280, Height: 720}, Request{Quality: "saver"}, "")
	if !slices.Equal(paths(p.Variants(Request{Quality: "saver"})), []Path{GPUScale, Native, CPUScale}) {
		t.Fatal("unscaled plan must try native before CPU")
	}
}

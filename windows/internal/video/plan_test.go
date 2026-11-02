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
}

func TestArgsChooseScalingPath(t *testing.T) {
	d := Display{Adapter: 1, Output: 2, Width: 3840, Height: 2160}
	nv, _ := NewPlan(d, Request{}, "h264_nvenc")
	a := strings.Join(nv.Args(false), " ")
	for _, want := range []string{"d3d11va=dda:1", "output_idx=2", "scale_d3d11=width=1920:height=1080", "-tune ull", "-bf 0", "-g 60", "-f flv", "pipe:1"} {
		if !strings.Contains(a, want) {
			t.Errorf("nvenc args missing %q: %s", want, a)
		}
	}
	mf, _ := NewPlan(d, Request{}, "h264_mf")
	a = strings.Join(mf.Args(false), " ")
	if !strings.Contains(a, "hwdownload,format=bgra,scale=1920:1080") || strings.Contains(a, "scale_d3d11") {
		t.Errorf("mf args: %s", a)
	}
	native, _ := NewPlan(Display{Width: 1920, Height: 1080}, Request{}, "h264_amf")
	if strings.Contains(strings.Join(native.Args(false), " "), "scale") {
		t.Error("native size must not scale")
	}
	probe := native.Args(true)
	if !slices.Contains(probe, "null") || slices.Contains(probe, "pipe:1") {
		t.Errorf("probe args: %v", probe)
	}
}

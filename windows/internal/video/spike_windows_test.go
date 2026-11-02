//go:build windows

package video

import (
	"context"
	"os"
	"sort"
	"testing"
	"time"
)

// TestLiveEncoder 在本机真实截屏编码 5 秒，统计帧率、帧大小与关键帧间隔。
// 需要真实桌面与内嵌（或 TAPDECK_FFMPEG 指定的）ffmpeg，仅在 TAPDECK_VIDEO_LIVE=1 时运行：
//
//	TAPDECK_VIDEO_LIVE=1 go test -run TestLiveEncoder -v ./internal/video/
func TestLiveEncoder(t *testing.T) {
	if os.Getenv("TAPDECK_VIDEO_LIVE") != "1" {
		t.Skip("设置 TAPDECK_VIDEO_LIVE=1 才实测编码")
	}
	l := NewLauncher()
	if !l.Available() {
		_, err := l.binary()
		t.Fatalf("ffmpeg 不可用：%v", err)
	}
	for _, quality := range []string{"smooth", "clear", "saver"} {
		start := time.Now()
		plan, err := l.Prepare(Request{Quality: quality})
		if err != nil {
			t.Fatalf("%s: %v", quality, err)
		}
		prepared := time.Since(start)
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		launched := time.Now()
		src, err := l.Launch(ctx, plan)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		var first time.Duration
		var frames, keys, bytes, config int
		var sizes []int
		var lastKey time.Duration
		var gaps []time.Duration
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			f, err := src.Next()
			if err != nil {
				t.Fatalf("%s: %v", quality, err)
			}
			if f.Config {
				config++
				continue
			}
			if frames == 0 {
				first = time.Since(launched)
				deadline = time.Now().Add(5 * time.Second)
				if !f.Key {
					t.Errorf("%s: 第一帧不是关键帧", quality)
				}
			}
			frames++
			bytes += len(f.Data)
			sizes = append(sizes, len(f.Data))
			if f.Key {
				if keys > 0 {
					gaps = append(gaps, f.PTS-lastKey)
				}
				keys++
				lastKey = f.PTS
			}
		}
		src.Close()
		cancel()
		if frames == 0 {
			t.Fatalf("%s: 5 秒内没有收到画面（%s %s）", quality, plan.Encoder, plan.Path)
		}
		sort.Ints(sizes)
		// 从首帧开始计时，排除编码进程启动时间。
		elapsed := 5.0
		t.Logf("%s：%s（%s）%dx%d（屏幕 %dx%d）探测 %v，首帧 %v，%.1f fps，%.1f Mbps，帧中位 %d B / 最大 %d B，关键帧 %d 个，间隔 %v，配置帧 %d",
			quality, plan.Encoder, plan.Path, plan.Width, plan.Height, plan.Display.Width, plan.Display.Height, prepared.Round(time.Millisecond), first.Round(time.Millisecond),
			float64(frames)/elapsed, float64(bytes*8)/elapsed/1e6, sizes[len(sizes)/2], sizes[len(sizes)-1], keys, gaps, config)
		if config != 1 {
			t.Errorf("%s: 配置帧 %d 个", quality, config)
		}
	}
}

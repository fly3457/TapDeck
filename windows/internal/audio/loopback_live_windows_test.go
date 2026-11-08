//go:build windows

package audio

import (
	"context"
	"os"
	"testing"
	"time"
)

// 实机测试：TAPDECK_LIVE_LOOPBACK=1 时运行，测试期间需要电脑正在播放声音。
func TestLiveLoopbackCapturesPlayback(t *testing.T) {
	if os.Getenv("TAPDECK_LIVE_LOOPBACK") != "1" {
		t.Skip("设置 TAPDECK_LIVE_LOOPBACK=1 并在电脑上播放声音时运行")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	packets, loud := 0, 0
	err := Loopback(ctx, func(pcm []byte) {
		packets++
		if len(pcm) != LoopbackPacket {
			t.Errorf("包长 %d", len(pcm))
		}
		for i := 0; i+1 < len(pcm); i += 2 {
			if v := int16(pcm[i]) | int16(pcm[i+1])<<8; v > 1000 || v < -1000 {
				loud++
				break
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("3 秒内 %d 包，其中 %d 包有明显声音", packets, loud)
	if loud == 0 {
		t.Fatal("没有采到声音")
	}
}

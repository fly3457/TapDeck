//go:build windows

package notify

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestLiveToastRoundTrip 在真实桌面上弹出一条 Toast 并确认监听器读到它。
// 会在通知中心留下通知，只在设置 TAPDECK_LIVE_NOTIFY=1 时运行。
func TestLiveToastRoundTrip(t *testing.T) {
	if os.Getenv("TAPDECK_LIVE_NOTIFY") != "1" {
		t.Skip("set TAPDECK_LIVE_NOTIFY=1 to show a real toast")
	}
	if err := Register(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	got := make(chan Notification, 16)
	ready := make(chan Access, 4)
	go func() {
		_ = Watch(ctx, 300*time.Millisecond, func(n Notification) { got <- n }, func(a Access) { ready <- a })
	}()
	if a := <-ready; a != AccessAllowed {
		t.Skipf("notification access = %s", a.Text())
	}
	time.Sleep(time.Second) // 等基线建立
	marker := "TapDeck 自测 " + time.Now().Format("150405.000")
	if err := Show(Message{Title: marker, Body: "通知转发往返 😀"}); err != nil {
		t.Fatal(err)
	}
	for {
		select {
		case n := <-got:
			t.Logf("seen: %+v", n)
			if n.Title == marker {
				// 仅在注册表登记的 AppUserModelId 没有 AppInfo，应用名为空，属预期。
				if n.Body != "通知转发往返 😀" {
					t.Fatalf("fields = %+v", n)
				}
				return
			}
		case <-ctx.Done():
			t.Fatal("toast not observed")
		}
	}
}

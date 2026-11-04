package server

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"tapdeck/internal/notify"
)

type testNotifications struct {
	Type  string                `json:"type"`
	Boot  string                `json:"boot"`
	Items []notify.Notification `json:"items"`
}

func decodeNotifications(t *testing.T, m map[string]json.RawMessage) testNotifications {
	t.Helper()
	b, _ := json.Marshal(m)
	var v testNotifications
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestNotificationsBroadcastFilterAndReplay(t *testing.T) {
	s, client := testReceiver(t, func(s *Server) { s.EnableNotifications() })
	conn, ctx := testSocket(t, s, client, "")
	token := testPair(t, s, conn, ctx)
	conn.CloseNow()
	conn, ctx = testSocket(t, s, client, token)
	ready := testRead(t, ctx, conn)
	var features []string
	_ = json.Unmarshal(ready["features"], &features)
	if !slices.Contains(features, NotifyFeature) {
		t.Fatalf("notify not advertised: %v", features)
	}

	c := s.Config()
	c.NotifyExclude = []string{"Spam"}
	if err := s.Update(c); err != nil {
		t.Fatal(err)
	}
	readUntil(t, ctx, conn, "config")
	if s.PublishNotification(notify.Notification{App: "spam", Title: "ignored"}) {
		t.Fatal("excluded app was forwarded")
	}
	if s.PublishNotification(notify.Notification{App: "Chrome"}) {
		t.Fatal("empty notification was forwarded")
	}
	if !s.PublishNotification(notify.Notification{App: "微信", Title: "张三", Body: "晚上吃饭吗"}) {
		t.Fatal("notification not forwarded")
	}
	got := decodeNotifications(t, readUntil(t, ctx, conn, "notifications"))
	if len(got.Items) != 1 || got.Items[0].Seq != 1 || got.Items[0].Title != "张三" || got.Boot == "" {
		t.Fatalf("broadcast = %+v", got)
	}
	s.PublishNotification(notify.Notification{App: "Outlook", Title: "会议"})
	readUntil(t, ctx, conn, "notifications")

	// 重连后按 seq 补发；不同 boot 视为接收端重启，补发全部缓冲。
	if err := write(ctx, conn, Message{Type: "notify_sync", Boot: got.Boot, After: 1}); err != nil {
		t.Fatal(err)
	}
	replay := decodeNotifications(t, readUntil(t, ctx, conn, "notifications"))
	if len(replay.Items) != 1 || replay.Items[0].Title != "会议" {
		t.Fatalf("replay = %+v", replay)
	}
	if err := write(ctx, conn, Message{Type: "notify_sync", Boot: "old", After: 9}); err != nil {
		t.Fatal(err)
	}
	if replay = decodeNotifications(t, readUntil(t, ctx, conn, "notifications")); len(replay.Items) != 2 {
		t.Fatalf("new boot replay = %+v", replay)
	}

	c = s.Config()
	c.NotifyDisabled = true
	if err := s.Update(c); err != nil {
		t.Fatal(err)
	}
	if s.PublishNotification(notify.Notification{App: "微信", Title: "x"}) {
		t.Fatal("disabled forwarding still sent")
	}
}

func TestNotificationsNotAdvertisedWithoutListener(t *testing.T) {
	s, client := testReceiver(t)
	conn, ctx := testSocket(t, s, client, "")
	testPair(t, s, conn, ctx)
	if s.PublishNotification(notify.Notification{Title: "x"}) {
		t.Fatal("published without listener")
	}
	_ = write(ctx, conn, Message{Type: "notify_sync"})
	if m := readUntil(t, ctx, conn, "error"); stringField(m, "reason") == "" {
		t.Fatal("missing error reason")
	}
}

func TestBackgroundLinkModeRelaxesHeartbeat(t *testing.T) {
	oldPing, oldTimeout := backgroundPing, backgroundTimeout
	backgroundPing, backgroundTimeout = 300*time.Millisecond, 2*time.Second
	t.Cleanup(func() { backgroundPing, backgroundTimeout = oldPing, oldTimeout })
	s, client := testReceiver(t, func(s *Server) { s.EnableNotifications() })
	conn, ctx := testSocket(t, s, client, "")
	testPair(t, s, conn, ctx)
	if err := write(ctx, conn, Message{Type: "link_mode", Mode: "background"}); err != nil {
		t.Fatal(err)
	}
	// 超过前台 1s 超时仍保持连接，并收到 PC 发起的 keepalive。
	start := time.Now()
	for time.Since(start) < 1500*time.Millisecond {
		readUntil(t, ctx, conn, "keepalive")
	}
	_ = write(ctx, conn, Message{Type: "keepalive"})
	if err := write(ctx, conn, Message{Type: "link_mode", Mode: "foreground"}); err != nil {
		t.Fatal(err)
	}
	// 回到前台后恢复 1s 超时：沉默即断开。
	deadline := time.Now().Add(1800 * time.Millisecond)
	for {
		if _, _, err := conn.Read(ctx); err != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("foreground timeout not restored")
		}
	}
}

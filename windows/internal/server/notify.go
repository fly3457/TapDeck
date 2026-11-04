package server

import (
	"fmt"
	"time"

	"tapdeck/internal/notify"
)

// NotifyFeature 表示接收端会转发 Windows 系统通知，并支持后台低功耗心跳（link_mode）。
const NotifyFeature = "notify"

const (
	// notifyRingSize 是可补发给重连手机的最近通知数。
	notifyRingSize = 100
	// 前台由手机每 250ms 发心跳，1s 无消息即断开；后台改由 PC 每 20s 发 keepalive，
	// 手机收到后回应，60s 无消息才断开，手机在两次 keepalive 之间可以休眠。
	foregroundTimeout = time.Second
)

// 变量便于测试缩短等待。
var (
	backgroundPing    = 20 * time.Second
	backgroundTimeout = 60 * time.Second
)

// EnableNotifications 开启通知转发能力；由接收端在通知监听器可用时调用。
func (s *Server) EnableNotifications() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.notifications == nil {
		s.notifications = notify.NewRing(notifyRingSize)
	}
}

func (s *Server) notificationRing() *notify.Ring {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.notifications
}

// PublishNotification 按 PC 设置过滤后缓存并广播一条通知；未转发时返回 false。
func (s *Server) PublishNotification(n notify.Notification) bool {
	ring := s.notificationRing()
	if ring == nil {
		return false
	}
	cfg := s.Config()
	n = n.Normalize()
	if cfg.NotifyDisabled || n.Empty() || notify.Excluded(n.App, cfg.NotifyExclude) {
		return false
	}
	n = ring.Add(n)
	s.mu.Lock()
	sessions := make([]*session, 0, len(s.sessions))
	for _, ss := range s.sessions {
		sessions = append(sessions, ss)
	}
	s.mu.Unlock()
	message := notificationMessage(ring.Boot(), []notify.Notification{n})
	for _, ss := range sessions {
		if e := ss.send(message); e != nil {
			ss.close(fmt.Sprintf("通知发送失败：%v", e))
		}
	}
	return true
}

func notificationMessage(boot string, items []notify.Notification) map[string]any {
	return map[string]any{"type": "notifications", "boot": boot, "items": items}
}

// syncNotifications 补发手机尚未收到的通知。boot 与本次启动不同时补发全部缓冲。
func (ss *session) syncNotifications(m Message) error {
	ring := ss.s.notificationRing()
	if ring == nil {
		return fmt.Errorf("接收端未开启通知转发")
	}
	if ss.s.Config().NotifyDisabled {
		return ss.send(notificationMessage(ring.Boot(), []notify.Notification{}))
	}
	return ss.send(notificationMessage(ring.Boot(), ring.Since(m.Boot, m.After)))
}

// setLinkMode 切换前台／后台心跳模式。
func (ss *session) setLinkMode(mode string) error {
	switch mode {
	case "foreground":
		ss.background.Store(false)
	case "background":
		ss.background.Store(true)
	default:
		return fmt.Errorf("未知连接模式 %q", mode)
	}
	ss.lastSeen.Store(time.Now().UnixNano())
	return nil
}

func (ss *session) heartbeatTimeout() time.Duration {
	if ss.background.Load() {
		return backgroundTimeout
	}
	return foregroundTimeout
}

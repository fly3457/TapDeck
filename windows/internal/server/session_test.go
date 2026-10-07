package server

import (
	"context"
	"encoding/binary"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// pairDevice 用独立设备编号完成一次配对（PC 端允许），返回连接、上下文与长期凭据。
func pairDevice(t *testing.T, s *Server, client *http.Client, index int) (*websocket.Conn, context.Context, string) {
	t.Helper()
	deviceID := fmt.Sprintf("integration-device-%d", index)
	conn, ctx := testSocketDevice(t, s, client, "", deviceID)
	challenge := testRead(t, ctx, conn)
	if stringField(challenge, "type") != "pair_challenge" {
		t.Fatalf("设备 %d 未进入配对流程: %v", index, challenge)
	}
	s.Approve(stringField(challenge, "request_id"), true)
	ready := testRead(t, ctx, conn)
	if stringField(ready, "type") != "ready" {
		t.Fatalf("设备 %d 未建立会话: %v", index, ready)
	}
	return conn, ctx, stringField(ready, "token")
}

// readyWith 用已有凭据重连同一台设备。
func readyWith(t *testing.T, s *Server, client *http.Client, index int, token string) (*websocket.Conn, context.Context) {
	t.Helper()
	conn, ctx := testSocketDevice(t, s, client, token, fmt.Sprintf("integration-device-%d", index))
	if got := stringField(testRead(t, ctx, conn), "type"); got != "ready" {
		t.Fatalf("设备 %d 重连收到 %q", index, got)
	}
	return conn, ctx
}

// 一台 PC 最多接受 MaxSessions 个控制端：超过的连接收到明确原因后断开。
func TestMultipleControllersUpToLimit(t *testing.T) {
	s, client := testReceiver(t)
	// 每台设备各自配对一次（凭据与设备编号绑定）。
	for i := 1; i <= MaxSessions; i++ {
		pairDevice(t, s, client, i)
		waitSessions(t, s, i)
	}
	if got := len(s.Snapshot().Devices); got != MaxSessions {
		t.Fatalf("快照里的控制端数量 = %d", got)
	}

	// 第 6 个：收到 too_many_clients 后被拒绝。
	extra, extraCtx := testSocketDevice(t, s, client, "", "integration-device-extra")
	msg := testRead(t, extraCtx, extra)
	if stringField(msg, "type") != "error" || stringField(msg, "code") != "too_many_clients" {
		t.Fatalf("超限连接没有被拒绝: %v", msg)
	}
	extra.CloseNow()

	// 断开一个后名额释放，可以再接回来。
	s.mu.Lock()
	var victim uint64
	for id := range s.sessions {
		victim = id
		break
	}
	victimSession := s.sessions[victim]
	s.mu.Unlock()
	victimSession.close("测试断开一台")
	waitSessions(t, s, MaxSessions-1)
	pairDevice(t, s, client, MaxSessions+10)
	waitSessions(t, s, MaxSessions)
}

// 同一台设备重连时替换旧会话，不占两个名额。
func TestSameDeviceReplacesItsSession(t *testing.T) {
	s, client := testReceiver(t)
	first, ctx := testSocket(t, s, client, "")
	token := testPair(t, s, first, ctx)
	waitSessions(t, s, 1)
	second, ctx2 := testSocket(t, s, client, token)
	if stringField(testRead(t, ctx2, second), "type") != "ready" {
		t.Fatal("重连失败")
	}
	waitSessions(t, s, 1)
	if _, _, err := first.Read(ctx); err == nil {
		t.Fatal("旧会话没有被替换")
	}
}

// 多台控制端同时在线时，UDP 包按会话 id 各自路由。
func TestPacketsRouteToOwningSession(t *testing.T) {
	s, client := testReceiver(t)
	pairDevice(t, s, client, 1)
	pairDevice(t, s, client, 2)
	waitSessions(t, s, 2)
	if s.activeSession(make([]byte, 16)) != nil {
		t.Fatal("未知会话 id 不应匹配到会话")
	}
	s.mu.Lock()
	ids := make([]uint64, 0, len(s.sessions))
	for id := range s.sessions {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	if len(ids) != 2 {
		t.Fatalf("会话数量 = %d", len(ids))
	}
	for _, id := range ids {
		p := make([]byte, 16)
		binary.LittleEndian.PutUint64(p[8:], id)
		if got := s.activeSession(p); got == nil || got.id != id {
			t.Fatalf("会话 %x 未能按 id 路由", id)
		}
	}
}

// 配置更新会推送给所有已连接的控制端。
func TestConfigUpdateReachesEverySession(t *testing.T) {
	s, client := testReceiver(t)
	connA, ctxA, _ := pairDevice(t, s, client, 1)
	connB, ctxB, _ := pairDevice(t, s, client, 2)
	waitSessions(t, s, 2)
	c := s.Config()
	c.Sensitivity = 2.5
	if err := s.Update(c); err != nil {
		t.Fatal(err)
	}
	if got := stringField(testRead(t, ctxA, connA), "type"); got != "config" {
		t.Fatalf("第一个控制端收到 %q", got)
	}
	if got := stringField(testRead(t, ctxB, connB), "type"); got != "config" {
		t.Fatalf("第二个控制端收到 %q", got)
	}
}

// 配对不再需要手机点确认：发出 pair_challenge 后，PC 允许即可拿到 ready。
func TestPairWithoutClientConfirmation(t *testing.T) {
	s, client := testReceiver(t)
	conn, ctx := testSocket(t, s, client, "")
	challenge := testRead(t, ctx, conn)
	if stringField(challenge, "type") != "pair_challenge" {
		t.Fatal("期望 pair_challenge")
	}
	// 手机什么都不发，直接由 PC 端允许。
	s.Approve(stringField(challenge, "request_id"), true)
	ready := testRead(t, ctx, conn)
	if stringField(ready, "type") != "ready" {
		t.Fatalf("PC 允许后未建立会话: %v", ready)
	}
	if len(stringField(ready, "token")) != 43 {
		t.Fatal("缺少长期凭据")
	}
}

// 等待会话数量达到期望值（连接建立与断开都是异步的）。
func waitSessions(t *testing.T, s *Server, want int) {
	t.Helper()
	for i := 0; i < 300; i++ {
		s.mu.Lock()
		n := len(s.sessions)
		s.mu.Unlock()
		if n == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	s.mu.Lock()
	n := len(s.sessions)
	s.mu.Unlock()
	t.Fatalf("会话数量 = %d，期望 %d", n, want)
}

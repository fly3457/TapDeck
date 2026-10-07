//go:build windows

// ctrlprobe 是一个「假的 Android 控制端」诊断工具：用来在只有一台手机的情况下
// 验证一台 PC 能同时接多少个控制端、名额满了如何被拒绝，以及配对流程。
//
//	go run ./cmd/ctrlprobe -port 41443 -name Probe-A -seconds 30
//
// 它会：连上接收端 → 发 hello → 打印收到的 pair_challenge（含校验码）→
// 在 PC 设置窗口点「校验码一致，允许」后打印 ready 与会话编号 → 之后按心跳保活。
package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/coder/websocket"
)

func main() {
	host := flag.String("host", "127.0.0.1", "接收端地址")
	port := flag.Int("port", 41443, "WSS 端口")
	version := flag.Int("version", 2, "控制协议版本")
	name := flag.String("name", "CtrlProbe", "控制端名称")
	device := flag.String("device", "", "设备编号（默认随机；填 probe-N 可配合已配对 token 免配对）")
	token := flag.String("token", "", "长期凭据（与 paired.json 里的哈希对应时免配对）")
	seconds := flag.Int("seconds", 30, "保活时长（秒）；0 表示收到 ready 后立即退出")
	flag.Parse()

	deviceID := *device
	if deviceID == "" {
		deviceID = "probe-" + randomHex(8)
	}
	nonce := randomBytes(32)
	url := fmt.Sprintf("wss://%s:%d/ws", *host, *port)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	dialCtx, dialCancel := context.WithTimeout(ctx, 15*time.Second)
	defer dialCancel()
	// 接收端用自签名证书，诊断工具不校验。
	insecure := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	conn, _, err := websocket.Dial(dialCtx, url, &websocket.DialOptions{HTTPClient: insecure})
	if err != nil {
		fmt.Println("连接失败:", err)
		os.Exit(1)
	}
	defer conn.CloseNow()
	fmt.Printf("已连接 %s，设备编号 %s\n", url, deviceID)

	hello := map[string]any{"type": "hello", "version": *version, "device_id": deviceID, "name": *name, "client_nonce": base64.RawURLEncoding.EncodeToString(nonce)}
	if *token != "" {
		hello["token"] = *token
	}
	if err = writeJSON(dialCtx, conn, hello); err != nil {
		fmt.Println("发送 hello 失败:", err)
		os.Exit(1)
	}

	deadline := time.Now().Add(time.Duration(*seconds) * time.Second)
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	// 读取放在独立 goroutine：coder/websocket 在读取上下文超时后会关闭连接，
	// 所以这里用父 ctx，靠外层 select 控制节奏。
	msgs := make(chan map[string]any, 16)
	go func() {
		defer close(msgs)
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var m map[string]any
			if json.Unmarshal(data, &m) == nil {
				msgs <- m
			}
		}
	}()
	ready := false
	for {
		select {
		case <-ctx.Done():
			return
		case m, ok := <-msgs:
			if !ok {
				fmt.Println("连接已断开")
				return
			}
			switch m["type"] {
			case "pair_challenge":
				fmt.Printf("配对请求：校验码 %v（请在 PC 设置窗口点「校验码一致，允许」）\n", m["code"])
			case "ready":
				ready = true
				fmt.Printf("已建立会话：%v（UDP %v）\n", m["session"], m["udp_port"])
				if *seconds == 0 {
					return
				}
			case "error":
				fmt.Printf("被拒绝：%v\n", m["reason"])
				if *seconds == 0 {
					os.Exit(2)
				}
			case "heartbeat":
			default:
				fmt.Printf("收到 %v\n", m["type"])
			}
		case <-ticker.C:
			if time.Now().After(deadline) {
				fmt.Println("保活结束")
				return
			}
			if ready {
				hbCtx, hbCancel := context.WithTimeout(ctx, time.Second)
				_ = writeJSON(hbCtx, conn, map[string]any{"type": "heartbeat", "tick": time.Now().UnixMilli()})
				hbCancel()
			}
		}
	}
}

func writeJSON(ctx context.Context, conn *websocket.Conn, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, b)
}

func randomHex(n int) string { return hex.EncodeToString(randomBytes(n)) }

func randomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}

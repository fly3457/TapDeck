package server

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/coder/websocket"
	"tapdeck/internal/video"
)

// VideoFeature 在 ready.features 中声明接收端可以提供屏幕画面。
const VideoFeature = "video"

func (ss *session) features() []string {
	f := append(ss.touchpadFeatures(), "voice_profiles")
	if ss.s.Video.Available() {
		f = append(f, VideoFeature)
	}
	return f
}

func videoErrorCode(err error) string {
	switch {
	case errors.Is(err, video.ErrBusy):
		return "busy"
	case errors.Is(err, video.ErrNoEncoder):
		return "no_encoder"
	default:
		return "capture_failed"
	}
}

// startVideo 为本会话分配视频流并回复凭据；画面通过独立的 /video 连接传输，
// 避免大帧阻塞控制连接上的心跳与点击。
func (ss *session) startVideo(m Message) {
	fail := func(code, reason string) {
		_ = ss.send(map[string]any{"type": "video_error", "code": code, "reason": reason})
	}
	if ss.s.Config().VideoDisabled {
		fail("disabled", "电脑已关闭“允许手机查看屏幕”，请在 PC 设置中开启")
		return
	}
	if !ss.s.Video.Available() {
		fail("no_encoder", video.ErrNoEncoder.Error())
		return
	}
	st, err := ss.s.Video.Start(ss.id, video.Request{Quality: m.Quality, MaxWidth: m.MaxWidth, MaxHeight: m.MaxHeight, FPS: m.FPS})
	if err != nil {
		log.Printf("video: %v", err)
		fail(videoErrorCode(err), err.Error())
		return
	}
	ss.mu.Lock()
	active := ss.active
	ss.mu.Unlock()
	if !active {
		ss.s.Video.Stop(ss.id)
		return
	}
	p := st.Plan
	_ = ss.send(map[string]any{"type": "video_ready", "stream": st.ID, "token": base64.RawURLEncoding.EncodeToString(st.Token),
		"width": p.Width, "height": p.Height, "screen_width": p.Display.Width, "screen_height": p.Display.Height,
		"fps": p.FPS, "encoder": p.Encoder})
}

// videoConnect 处理 GET /video：首条 video_hello 携带会话、流 ID 与令牌，
// 校验通过后只向手机发送二进制画面与光标消息。会话结束时视频随之结束。
func (s *Server) videoConnect(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Origin") != "" {
		http.Error(w, "native client required", http.StatusForbidden)
		return
	}
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		return
	}
	defer ws.CloseNow()
	ws.SetReadLimit(4096)
	helloCtx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	hello, err := read(helloCtx, ws)
	cancel()
	if err != nil || hello.Type != "video_hello" {
		return
	}
	reject := func(reason string) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		_ = write(ctx, ws, map[string]any{"type": "video_error", "code": "unauthorized", "reason": reason})
	}
	sid, err := strconv.ParseUint(hello.Session, 16, 64)
	token, tokenErr := base64.RawURLEncoding.DecodeString(hello.Token)
	if err != nil || tokenErr != nil {
		reject("视频凭据无效")
		return
	}
	s.mu.Lock()
	ss := s.sessions[sid]
	s.mu.Unlock()
	if ss == nil {
		reject("控制连接已断开")
		return
	}
	st, err := s.Video.Claim(hello.Stream, token, sid)
	if err != nil {
		reject(err.Error())
		return
	}
	// CloseRead 处理对端的关闭帧：手机断开时 ctx 立即结束。
	ctx := ws.CloseRead(ss.ctx)
	p := st.Plan
	if err = write(ctx, ws, map[string]any{"type": "video_format", "width": p.Width, "height": p.Height,
		"screen_width": p.Display.Width, "screen_height": p.Display.Height, "fps": p.FPS, "encoder": p.Encoder}); err != nil {
		s.Video.Stop(sid)
		return
	}
	log.Printf("video: %s 开始查看屏幕（%s %dx%d@%d %dkbps）", ss.name, p.Encoder, p.Width, p.Height, p.FPS, p.Kbps)
	err = s.Video.Run(ctx, st, func(ctx context.Context, b []byte) error {
		wctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		return ws.Write(wctx, websocket.MessageBinary, b)
	})
	log.Printf("video: %s 结束查看（发送 %d 帧，丢弃 %d 帧）", ss.name, st.Sent.Load(), st.Dropped.Load())
	if err != nil && ctx.Err() == nil {
		log.Printf("video: %v", err)
		notice, cancel := context.WithTimeout(context.Background(), time.Second)
		_ = write(notice, ws, map[string]any{"type": "video_error", "code": "capture_failed", "reason": fmt.Sprint(err)})
		cancel()
		_ = ss.send(map[string]any{"type": "video_error", "code": "capture_failed", "reason": fmt.Sprint(err)})
	}
	ws.Close(websocket.StatusNormalClosure, "")
}

// VideoDevice 返回正在查看屏幕的控制端名称，没有时为空。
func (s *Server) VideoDevice() string {
	owner := s.Video.Owner()
	if owner == 0 {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if ss := s.sessions[owner]; ss != nil {
		return ss.name
	}
	return ""
}

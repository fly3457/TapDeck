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

// VideoAudioFeature 声明视频流可以同时转发电脑声音（接收端 0.5.15 起）。
const VideoAudioFeature = "video_audio"

func (ss *session) features() []string {
	f := append(ss.touchpadFeatures(), "voice_profiles")
	if ss.s.Video.Available() {
		f = append(f, VideoFeature)
		if ss.s.Video.AudioAvailable() {
			f = append(f, VideoAudioFeature)
		}
	}
	if ss.s.notificationRing() != nil {
		f = append(f, NotifyFeature)
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
	st, err := ss.s.Video.Start(ss.id, video.Request{Quality: m.Quality, MaxWidth: m.MaxWidth, MaxHeight: m.MaxHeight, FPS: m.FPS, Audio: m.Audio && ss.s.Video.AudioAvailable()})
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
		"fps": p.FPS, "encoder": p.Encoder, "audio": st.AudioOn()})
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
		"screen_width": p.Display.Width, "screen_height": p.Display.Height, "fps": p.FPS, "encoder": p.Encoder, "audio": st.AudioOn()}); err != nil {
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

// absolutePointer 是支持绝对坐标移动光标的输入后端（视频模式点击位置）。
type absolutePointer interface {
	MoveTo(x, y int32) error
}

// pointer 处理视频模式的点击：把光标移到手机点击的画面位置，再按动作按键。
// 动作：click（左键单击）、right_click（右键单击）。坐标钳制在所捕获显示器内。
// 画面已关闭或后端不支持时只提示、不注入，鼠标分段屏障照常推进。
func (ss *session) pointer(m Message) error {
	notice := func(reason string) error {
		_ = ss.send(map[string]any{"type": "error", "code": "touchpad_error", "reason": reason})
		return nil
	}
	switch m.Action {
	case "click", "right_click", "move", "down", "up":
	default:
		return notice("无效的画面点击")
	}
	d, ok := ss.s.Video.DisplayFor(ss.id)
	if !ok {
		// 画面已关闭时仍要松开拖拽中的左键，避免卡住。
		if m.Action == "up" {
			ss.releasePointer()
		}
		return notice("电脑画面未打开，无法按位置点击")
	}
	px, py := min(max(m.PX, 0), int32(d.Width-1)), min(max(m.PY, 0), int32(d.Height-1))
	if p := ss.s.Pointer; p != nil && !ss.pointerSoftware {
		var err error
		switch m.Action {
		case "click", "right_click":
			err = p.Click(d, px, py, m.Action == "right_click")
		case "move":
			err = p.Move(d, px, py)
		case "down":
			if err = p.Down(d, px, py); err == nil {
				ss.pointerHID.Store(true)
			}
		case "up":
			err = p.Up(d, px, py)
			ss.pointerHID.Store(false)
		}
		if err == nil {
			return nil
		}
		log.Printf("video: 虚拟鼠标失败，改用 SendInput：%v", err)
		// 同一次按住期间不在两种后端之间切换，避免按下与松开落在不同设备上。
		ss.pointerSoftware = m.Action == "down"
	}
	if m.Action == "up" {
		defer func() { ss.pointerSoftware = false }()
	}
	mover, ok := ss.controller().(absolutePointer)
	if !ok {
		return notice("电脑端不支持按位置点击，请升级")
	}
	x, y := int32(d.Left)+px, int32(d.Top)+py
	if err := mover.MoveTo(x, y); err != nil {
		return err
	}
	switch m.Action {
	case "move":
		// 双指滚动前把光标移到手指下方；左键按住时即为拖拽。
		return nil
	case "up":
		return ss.mouseButton("left", false)
	}
	button := "left"
	if m.Action == "right_click" {
		button = "right"
	}
	// 模拟真实点击的节奏：先让目标窗口处理移入（悬停高亮），再按下并按住片刻。
	// 输入法候选框等窗口只接受“先悬停、再按下”的点击，瞬移后立即按下会被忽略。
	pointerPause(pointerHoverDelay)
	if err := mover.MoveTo(x, y); err != nil {
		return err
	}
	if err := ss.mouseButton(button, true); err != nil {
		return err
	}
	if m.Action == "down" {
		return nil
	}
	pointerPause(pointerPressDuration)
	return ss.mouseButton(button, false)
}

// releasePointer 松开本会话通过虚拟鼠标按住的键（拖拽中断开、画面关闭）。
func (ss *session) releasePointer() {
	if ss.pointerHID.Swap(false) && ss.s.Pointer != nil {
		ss.s.Pointer.Release()
	}
}

const (
	pointerHoverDelay    = 25 * time.Millisecond
	pointerPressDuration = 40 * time.Millisecond
)

// pointerPause 可在测试中替换，避免真实等待。
var pointerPause = time.Sleep

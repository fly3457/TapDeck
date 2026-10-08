package server

import (
	"context"
	"time"

	"github.com/coder/websocket"
)

func receiveMessages(ctx context.Context, ws *websocket.Conn, cancel context.CancelFunc) <-chan Message {
	messages := make(chan Message)
	go func() {
		defer close(messages)
		defer cancel()
		for {
			m, err := read(ctx, ws)
			if err != nil {
				return
			}
			select {
			case messages <- m:
			case <-ctx.Done():
				return
			}
		}
	}()
	return messages
}

func awaitPairing(ctx, pairCtx context.Context, ws *websocket.Conn, messages <-chan Message, p *Pending) bool {
	fail := func(code, reason string) bool {
		// A deadline notice must not use the already-expired pairing context.
		notice, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		_ = write(notice, ws, map[string]any{"type": "error", "code": code, "reason": reason})
		return false
	}
	expired := func() bool { return fail("pairing_expired", "配对请求已过期，请重新点击连接") }
	for {
		select {
		case allow := <-p.answer:
			if ctx.Err() != nil {
				return false
			}
			if !time.Now().Before(p.Expires) || pairCtx.Err() != nil {
				return expired()
			}
			if !allow {
				return fail("pairing_rejected", "电脑未允许连接，请确认后重新点击连接")
			}
			return true
		case _, ok := <-messages:
			if !ok {
				return false
			}
			// Ignore legacy pair_confirm and all controls before PC approval.
		case <-pairCtx.Done():
			if ctx.Err() == nil && pairCtx.Err() == context.DeadlineExceeded {
				return expired()
			}
			return false
		}
	}
}

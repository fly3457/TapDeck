package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/coder/websocket"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

type quietInput struct{ releases atomic.Int64 }

func (*quietInput) Move(int32, int32, int32, int32) error { return nil }
func (*quietInput) Button(string, bool) error             { return nil }
func (*quietInput) Chord(string) error                    { return nil }
func (*quietInput) Hold(string, bool) error               { return nil }
func (q *quietInput) ReleaseAll()                         { q.releases.Add(1) }

func testReceiver(t *testing.T, configure ...func(*Server)) (*Server, *http.Client) {
	t.Helper()
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.Input = &quietInput{}
	for _, option := range configure {
		option(s)
	}
	ports := map[int]bool{}
	freePort := func() int {
		for {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			port := listener.Addr().(*net.TCPAddr).Port
			listener.Close()
			if !ports[port] {
				ports[port] = true
				return port
			}
		}
	}
	s.cfg.HTTPPort, s.cfg.WSSPort, s.cfg.UDPPort = freePort(), freePort(), freePort()
	certificate, err := x509.ParseCertificate(s.cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(certificate)
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}}
	if err = s.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Stop)
	return s, client
}
func testSocket(t *testing.T, s *Server, client *http.Client, token string) (*websocket.Conn, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	conn, _, err := websocket.Dial(ctx, fmt.Sprintf("wss://127.0.0.1:%d/ws", s.cfg.WSSPort), &websocket.DialOptions{HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.CloseNow() })
	nonce := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	if err = write(ctx, conn, Message{Type: "hello", Version: ControlVersion, DeviceID: "integration-device", Name: "Test device", Token: token, ClientNonce: nonce}); err != nil {
		t.Fatal(err)
	}
	return conn, ctx
}
func testRead(t *testing.T, ctx context.Context, conn *websocket.Conn) map[string]json.RawMessage {
	t.Helper()
	_, bytes, err := conn.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var message map[string]json.RawMessage
	if err = json.Unmarshal(bytes, &message); err != nil {
		t.Fatal(err)
	}
	return message
}
func stringField(m map[string]json.RawMessage, field string) string {
	var s string
	_ = json.Unmarshal(m[field], &s)
	return s
}
func testPair(t *testing.T, s *Server, conn *websocket.Conn, ctx context.Context) string {
	t.Helper()
	challenge := testRead(t, ctx, conn)
	if stringField(challenge, "type") != "pair_challenge" {
		t.Fatal("manual URL pairing skipped comparison")
	}
	nonce, err := base64.RawURLEncoding.DecodeString(stringField(challenge, "server_nonce"))
	if err != nil {
		t.Fatal(err)
	}
	code := Code(s.pin, make([]byte, 32), nonce)
	if code != stringField(challenge, "code") {
		t.Fatal("comparison code differed")
	}
	if err = write(ctx, conn, Message{Type: "pair_confirm", Code: code}); err != nil {
		t.Fatal(err)
	}
	s.Approve(stringField(challenge, "request_id"), true)
	ready := testRead(t, ctx, conn)
	if stringField(ready, "type") != "ready" {
		t.Fatal("pairing did not establish session")
	}
	token := stringField(ready, "token")
	if len(token) != 43 {
		t.Fatal("missing long-term credential")
	}
	return token
}
func TestPairReconnectAndRevoke(t *testing.T) {
	s, client := testReceiver(t)
	first, ctx := testSocket(t, s, client, "")
	token := testPair(t, s, first, ctx)
	second, ctx2 := testSocket(t, s, client, token)
	ready := testRead(t, ctx2, second)
	if stringField(ready, "type") != "ready" {
		t.Fatal("credential did not reconnect")
	}
	if err := write(ctx2, second, Message{Type: "heartbeat", Tick: 1234}); err != nil {
		t.Fatal(err)
	}
	heartbeat := testRead(t, ctx2, second)
	if stringField(heartbeat, "type") != "heartbeat" || string(heartbeat["tick"]) != "1234" {
		t.Fatal("heartbeat did not echo")
	}
	if err := s.Unpair(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := second.Read(ctx2); err == nil {
		t.Fatal("revoked connection survived")
	}
	third, ctx3 := testSocket(t, s, client, token)
	challenge := testRead(t, ctx3, third)
	if stringField(challenge, "type") != "pair_challenge" {
		t.Fatal("revoked credential accepted")
	}
	_ = write(ctx3, third, Message{Type: "pair_confirm", Code: stringField(challenge, "code")})
	s.Approve(stringField(challenge, "request_id"), false)
	if _, _, err := third.Read(ctx3); err == nil {
		t.Fatal("rejected pair remained open")
	}
}
func TestOldControlVersionGetsUpgradeReason(t *testing.T) {
	s, client := testReceiver(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, fmt.Sprintf("wss://127.0.0.1:%d/ws", s.cfg.WSSPort), &websocket.DialOptions{HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	if err = write(ctx, conn, Message{Type: "hello", Version: 1, DeviceID: "legacy-device"}); err != nil {
		t.Fatal(err)
	}
	m := testRead(t, ctx, conn)
	if stringField(m, "code") != "version_mismatch" || stringField(m, "reason") == "" {
		t.Fatal("missing upgrade guidance")
	}
	if _, _, err := conn.Read(ctx); err == nil {
		t.Fatal("legacy client allowed to continue")
	}
}
func TestStopCancelsPendingPair(t *testing.T) {
	s, client := testReceiver(t)
	conn, ctx := testSocket(t, s, client, "")
	challenge := testRead(t, ctx, conn)
	_ = write(ctx, conn, Message{Type: "pair_confirm", Code: stringField(challenge, "code")})
	s.Stop()
	deadline, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, _, err := conn.Read(deadline); err == nil {
		t.Fatal("stopped receiver retained pending pair")
	}
	if s.Snapshot().Device != "" {
		t.Fatal("pair activated after stop")
	}
}
func TestHeartbeatTimeoutReleasesInput(t *testing.T) {
	s, client := testReceiver(t)
	conn, ctx := testSocket(t, s, client, "")
	_ = testPair(t, s, conn, ctx)
	deadline, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, _, err := conn.Read(deadline); err == nil {
		t.Fatal("silent peer was not disconnected")
	}
	if s.Input.(*quietInput).releases.Load() == 0 {
		deadline := time.Now().Add(100 * time.Millisecond)
		for s.Input.(*quietInput).releases.Load() == 0 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if s.Input.(*quietInput).releases.Load() == 0 {
			t.Fatal("timeout did not release held inputs")
		}
	}
}

type rejectingButtonInput struct{ quietInput }

func (*rejectingButtonInput) Button(string, bool) error {
	return fmt.Errorf("input injection rejected")
}

func TestFailedMouseBarrierDisconnectsAndReleases(t *testing.T) {
	s, client := testReceiver(t)
	fake := &rejectingButtonInput{}
	s.Input = fake
	conn, ctx := testSocket(t, s, client, "")
	_ = testPair(t, s, conn, ctx)
	if err := write(ctx, conn, Message{Type: "mouse_button", Button: "left", Down: true, Epoch: 0, NextEpoch: 1}); err != nil {
		t.Fatal(err)
	}
	response := testRead(t, ctx, conn)
	if stringField(response, "type") != "error" {
		t.Fatal("missing rejection reason")
	}
	if _, _, err := conn.Read(ctx); err == nil {
		t.Fatal("failed barrier left mismatched peers connected")
	}
	deadline := time.Now().Add(100 * time.Millisecond)
	for fake.releases.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if fake.releases.Load() == 0 {
		t.Fatal("failed input did not release held keys")
	}
}

type trackingChordInput struct {
	quietInput
	chords atomic.Int64
}

func (q *trackingChordInput) Chord(string) error { q.chords.Add(1); return nil }

func TestUnavailableMicrophoneKeepsShortcutsWorking(t *testing.T) {
	fake := &trackingChordInput{}
	s, client := testReceiver(t, func(s *Server) { s.Input = fake; s.Audio.Configure("tapdeck-nonexistent-audio-endpoint", 1) })
	conn, ctx := testSocket(t, s, client, "")
	_ = testPair(t, s, conn, ctx)
	if err := write(ctx, conn, Message{Type: "mic_start", Recording: "0000000000000001", Mode: "hold"}); err != nil {
		t.Fatal(err)
	}
	response := testRead(t, ctx, conn)
	if stringField(response, "type") != "mic_error" || stringField(response, "reason") == "" {
		t.Fatal("unavailable microphone did not report its state")
	}
	for slot := 0; slot < 4; slot++ {
		if err := write(ctx, conn, Message{Type: "shortcut", Slot: slot, Revision: s.Config().Revision}); err != nil {
			t.Fatal(err)
		}
	}
	if err := write(ctx, conn, Message{Type: "heartbeat", Tick: 99}); err != nil {
		t.Fatal(err)
	}
	if stringField(testRead(t, ctx, conn), "type") != "heartbeat" || fake.chords.Load() != 4 {
		t.Fatal("audio failure blocked shortcut processing")
	}
}

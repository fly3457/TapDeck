package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/coder/websocket"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"tapdeck/internal/secure"
	"testing"
	"time"
)

func TestLegacyPairedMigrationPreservesBackupAndCredentials(t *testing.T) {
	dir := t.TempDir()
	token := "old credential"
	id := "old-device-12345678"
	original, _ := json.Marshal(map[string]string{id: secure.Hash(token)})
	if err := os.WriteFile(filepath.Join(dir, "paired.json"), original, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(filepath.Join(dir, "paired.v1.bak"))
	if err != nil || !bytes.Equal(backup, original) {
		t.Fatal("migration backup lost", err)
	}
	if got := s.PairedDevices(); len(got) != 1 || got[0].Name != "旧设备（12345678）" || got[0].Online || s.paired[id].Hash != secure.Hash(token) {
		t.Fatal(got)
	}
	if _, err = New(dir); err != nil {
		t.Fatal(err)
	}
	backup2, _ := os.ReadFile(filepath.Join(dir, "paired.v1.bak"))
	if !bytes.Equal(backup2, original) {
		t.Fatal("backup overwritten")
	}
}

func TestLegacyDeviceReconnectionCompletesMetadata(t *testing.T) {
	id, token := "integration-device", "legacy token"
	s, client := testReceiver(t, func(s *Server) { s.paired[id] = pairedRecord{Hash: secure.Hash(token), Name: legacyName(id)} })
	c, ctx := testSocket(t, s, client, token)
	if stringField(testRead(t, ctx, c), "type") != "ready" {
		t.Fatal("legacy credential rejected")
	}
	devices := s.PairedDevices()
	if len(devices) != 1 || devices[0].Name != "Test device" || !devices[0].Online || devices[0].LastConnectedAt.IsZero() {
		t.Fatal(devices)
	}
	records, err := loadPaired(s.dir)
	if err != nil || records[id].Hash != secure.Hash(token) || records[id].Name != "Test device" {
		t.Fatal(records, err)
	}
	encoded, _ := json.Marshal(devices)
	if strings.Contains(string(encoded), secure.Hash(token)) {
		t.Fatal("UI snapshot contains a credential")
	}
}

func TestMigrationFailureDoesNotOverwriteOriginal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "paired.json")
	original := []byte(`{"old-device":"old-hash"}`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "paired.v1.bak"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := loadPaired(dir); err == nil {
		t.Fatal("invalid backup path accepted")
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, original) {
		t.Fatal("failed migration modified original")
	}
}

func TestReconnectRacingWithUnpairCannotRestoreGrant(t *testing.T) {
	s, client := testReceiver(t)
	for i := 0; i < 12; i++ {
		_, _, token := pairDevice(t, s, client, i)
		conn, ctx := testSocketDevice(t, s, client, token, fmt.Sprintf("integration-device-%d", i))
		done := make(chan error, 1)
		go func() { done <- s.UnpairDevice(fmt.Sprintf("integration-device-%d", i)) }()
		_, _, _ = conn.Read(ctx) // ready, revoked, or closed depending on scheduling
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		s.mu.Lock()
		authorized := len(s.paired)
		active := len(s.sessions)
		s.mu.Unlock()
		if authorized != 0 || active != 0 {
			t.Fatal("reconnect restored authorization after unpair", authorized, active)
		}
		conn.CloseNow()
	}
}

func TestOfflineSameNameDevicesUnpairByID(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.paired = map[string]pairedRecord{"device-a": {Hash: "a", Name: "Same phone"}, "device-b": {Hash: "b", Name: "Same phone"}}
	if err := savePaired(s.dir, s.paired); err != nil {
		t.Fatal(err)
	}
	if err := s.UnpairDevice("device-a"); err != nil {
		t.Fatal(err)
	}
	restored, err := New(s.dir)
	if err != nil {
		t.Fatal(err)
	}
	v := restored.PairedDevices()
	if len(v) != 1 || v[0].ID != "device-b" || v[0].Online {
		t.Fatal(v)
	}
}

func TestUnpairSaveFailureRetainsAuthorizationAndSession(t *testing.T) {
	s, client := testReceiver(t)
	conn, ctx := testSocket(t, s, client, "")
	token := testPair(t, s, conn, ctx)
	before := s.paired["integration-device"]
	if err := os.Mkdir(filepath.Join(s.dir, "paired.json.tmp"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := s.UnpairDevice("integration-device"); err == nil {
		t.Fatal("save failure not reported")
	}
	if got := s.PairedDevices(); len(got) != 1 || !got[0].Online {
		t.Fatal("failed save revoked live device", got)
	}
	if s.paired["integration-device"] != before || before.Hash != secure.Hash(token) {
		t.Fatal("in-memory credential changed")
	}
	_ = write(ctx, conn, Message{Type: "heartbeat", Tick: 19})
	if stringField(testRead(t, ctx, conn), "type") != "heartbeat" {
		t.Fatal("failed save disconnected session")
	}
}

func TestUnpairOneDeviceLeavesOtherOnline(t *testing.T) {
	s, client := testReceiver(t)
	a, ca, tokenA := pairDevice(t, s, client, 1)
	b, cb, _ := pairDevice(t, s, client, 2)
	if err := s.UnpairDevice("integration-device-1"); err != nil {
		t.Fatal(err)
	}
	if stringField(testRead(t, ca, a), "code") != "pairing_revoked" {
		t.Fatal("no targeted notice")
	}
	_ = write(cb, b, Message{Type: "heartbeat", Tick: 123})
	if stringField(testRead(t, cb, b), "type") != "heartbeat" {
		t.Fatal("other device disconnected")
	}
	v := s.PairedDevices()
	if len(v) != 1 || v[0].ID != "integration-device-2" || !v[0].Online {
		t.Fatal(v)
	}
	old, co := testSocketDevice(t, s, client, tokenA, "integration-device-1")
	if stringField(testRead(t, co, old), "code") != "pairing_revoked" {
		t.Fatal("old credential reused")
	}
}

func TestUnpairCancelsPendingGrant(t *testing.T) {
	s, client := testReceiver(t)
	a, _, _ := pairDevice(t, s, client, 1)
	a.CloseNow()
	waitSessions(t, s, 0)
	pending, ctx := testSocketDevice(t, s, client, "", "integration-device-1")
	challenge := testRead(t, ctx, pending)
	if err := s.UnpairDevice("integration-device-1"); err != nil {
		t.Fatal(err)
	}
	s.Approve(stringField(challenge, "request_id"), true)
	if stringField(testRead(t, ctx, pending), "code") != "pairing_revoked" {
		t.Fatal("pending device did not receive revocation")
	}
	if _, _, err := pending.Read(ctx); err == nil {
		t.Fatal("late approval restored revoked grant")
	}
	if len(s.PairedDevices()) != 0 || len(s.Snapshot().Pending) != 0 {
		t.Fatal("revoked pairing survived")
	}
}

func TestOldQRSecretAlwaysRequiresPCApproval(t *testing.T) {
	s, client := testReceiver(t)
	if s.QRURI() != s.PairURL() || !strings.HasPrefix(s.QRURI(), "http://") {
		t.Fatal(s.QRURI())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, fmt.Sprintf("wss://127.0.0.1:%d/ws", s.cfg.WSSPort), &websocket.DialOptions{HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	_ = write(ctx, c, Message{Type: "hello", Version: ControlVersion, DeviceID: "old-qr-device", Name: "Old QR", Secret: "previous secret", ClientNonce: base64.RawURLEncoding.EncodeToString(make([]byte, 32))})
	challenge := testRead(t, ctx, c)
	if stringField(challenge, "type") != "pair_challenge" || string(challenge["qr_verified"]) != "false" || len(s.PairedDevices()) != 0 {
		t.Fatal("secret bypassed comparison", challenge)
	}
	s.Approve(stringField(challenge, "request_id"), true)
	if stringField(testRead(t, ctx, c), "type") != "ready" {
		t.Fatal("PC approval failed")
	}
	rec := httptest.NewRecorder()
	s.pairPage(rec, httptest.NewRequest("GET", "http://pc/pair", nil))
	if !strings.Contains(rec.Body.String(), "打开 TapDeck 连接") || strings.Contains(rec.Body.String(), "secret") {
		t.Fatal("web flow contains old QR authorization")
	}
}

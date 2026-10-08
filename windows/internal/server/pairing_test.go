package server

import (
	"context"
	"testing"
	"time"
)

func waitPending(t *testing.T, s *Server, count int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if len(s.Snapshot().Pending) == count {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("pending requests = %d, want %d", len(s.Snapshot().Pending), count)
}

func TestPairingDisconnectRemovesPrompt(t *testing.T) {
	s, client := testReceiver(t)
	ws, ctx := testSocket(t, s, client, "")
	testRead(t, ctx, ws)
	waitPending(t, s, 1)
	ws.CloseNow()
	waitPending(t, s, 0)
	if len(s.PairedDevices()) != 0 {
		t.Fatal("disconnected phone was paired")
	}
}

func TestPairingExpiresWithReason(t *testing.T) {
	s, client := testReceiver(t, func(s *Server) { s.pairTimeout = 120 * time.Millisecond })
	ws, ctx := testSocket(t, s, client, "")
	challenge := testRead(t, ctx, ws)
	if stringField(testRead(t, ctx, ws), "code") != "pairing_expired" {
		t.Fatal("missing expiry reason")
	}
	s.Approve(stringField(challenge, "request_id"), true)
	waitPending(t, s, 0)
	if len(s.PairedDevices()) != 0 {
		t.Fatal("expired phone was paired")
	}
}

func TestPairingApprovalIsBoundToRequestAndOnlyOnce(t *testing.T) {
	s, client := testReceiver(t)
	a, ca := testSocketDevice(t, s, client, "", "pending-device-a")
	idA := stringField(testRead(t, ca, a), "request_id")
	b, cb := testSocketDevice(t, s, client, "", "pending-device-b")
	idB := stringField(testRead(t, cb, b), "request_id")
	waitPending(t, s, 2)
	s.Approve(idA, false)
	s.Approve(idA, true)
	if stringField(testRead(t, ca, a), "code") != "pairing_rejected" {
		t.Fatal("first decision did not win")
	}
	waitPending(t, s, 1)
	s.Approve(idB, true)
	if stringField(testRead(t, cb, b), "type") != "ready" {
		t.Fatal("second request was affected")
	}
	waitPending(t, s, 0)
}

func TestApproveIgnoresExpiredAndCanceledRequests(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := &Server{pending: map[string]*Pending{
		"expired":  {Expires: time.Now().Add(-time.Second), answer: make(chan bool, 1)},
		"canceled": {Expires: time.Now().Add(time.Minute), ctx: ctx, answer: make(chan bool, 1)},
	}}
	for id, p := range s.pending {
		s.Approve(id, true)
		if len(p.answer) != 0 {
			t.Fatalf("approved %s request", id)
		}
	}
}

func TestStopRemovesPendingRequests(t *testing.T) {
	s, client := testReceiver(t)
	ws, ctx := testSocket(t, s, client, "")
	testRead(t, ctx, ws)
	s.Stop()
	waitPending(t, s, 0)
	if _, _, err := ws.Read(ctx); err == nil {
		t.Fatal("pending socket survived stop")
	}
}

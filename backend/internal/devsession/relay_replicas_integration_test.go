// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package devsession_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/devsession"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
)

// sharedLayer is the layer two replicas share: Valkey when
// PLUX_TEST_VALKEY_URL names one, the memory layer otherwise.
func sharedLayer(t *testing.T) devsession.SharedLayer {
	t.Helper()
	url := os.Getenv("PLUX_TEST_VALKEY_URL")
	if url == "" {
		return cache.NewMemory(nil)
	}
	v, err := cache.NewValkey(url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = v.Close() })
	return v
}

// Verifies: DEV-021, DEV-001, DEV-020.
// Two api replicas on one database and one shared layer serve one
// session: the device's socket is on replica B, the watcher and the
// command on replica A, and each sees what the other relays.
func TestRelay_TwoReplicas_DEV_021_DEV_001(t *testing.T) {
	shared := sharedLayer(t)
	var opts devsession.Options
	a := newRelayWith(t, nil, func(o *devsession.Options) { o.Shared = shared; opts = *o })
	t.Cleanup(a.svc.Close)
	svcB, err := devsession.New(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svcB.Close)
	fb := *a.fixture
	fb.svc = svcB
	b := &relay{fixture: &fb, used: map[string]bool{}}
	mux := http.NewServeMux()
	mux.Handle("GET "+devsession.SocketPath, svcB.SocketHandler(b.proof))
	b.srv = httptest.NewServer(mux)
	t.Cleanup(b.srv.Close)

	s, _ := a.open(t)
	d := a.pairDevice(t, s.ID, "device-1")
	events := a.watch(t, s.ID, 0, nil)
	k := b.connect(t, d.Ticket, d.key, &pluxv1.DevDeviceHello{})
	k.event(t, 1, logEvent("one"))
	k.event(t, 2, logEvent("two"))
	k.ackedUpTo(t, 2)
	got := receive(t, events, 2)
	for i, msg := range []string{"one", "two"} {
		if got[i].GetLog().GetMessage() != msg || got[i].GetSequence() != uint64(i+1) {
			t.Errorf("event %d on replica A = %v, want %q", i, got[i], msg)
		}
	}
	sent, err := a.svc.SendCommand(context.Background(), a.Developer, &pluxv1.DevSessionServiceSendCommandRequest{
		SessionId: s.ID, Command: &pluxv1.DevSessionServiceSendCommandRequest_CaptureScreenshot{CaptureScreenshot: &pluxv1.CaptureScreenshotCommand{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if f := k.next(t); f.GetSequence() != 1 || f.GetCommand().GetCommandId() != sent.CommandID {
		t.Errorf("the device on replica B got %v, want the command from replica A", f)
	}
	resumed := b.watch(t, s.ID, 1, nil)
	if ev := receive(t, resumed, 1)[0]; ev.GetLog().GetMessage() != "two" {
		t.Errorf("resuming on replica B after 1 gave %v, want event two", ev)
	}
}

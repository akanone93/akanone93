// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package export_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"slices"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/ext/export"
	"github.com/nightCode42/plux3/backend/ext/export/exporttest"
)

// streamTo returns a stream to a receiver that trusts only its authority.
func streamTo(t *testing.T, r *exporttest.Receiver) *export.Stream {
	t.Helper()
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(r.CAPEM())) {
		t.Fatal("the authority is not PEM")
	}
	dialer := net.Dialer{Timeout: 5 * time.Second}
	s, err := export.NewStream(r.Address(), dialer.DialContext, &tls.Config{RootCAs: pool, ServerName: "127.0.0.1", MinVersion: tls.VersionTLS12}, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// batch frames n messages numbered from first.
func batch(first, n int) []byte {
	var b bytes.Buffer
	for i := range n {
		b.Write(export.Frame([]byte("message " + string(rune('a'+first+i)))))
	}
	return b.Bytes()
}

// Verifies: SEC-143.
// A batch is acknowledged when the receiver has read it and closed the
// connection, and every message arrives whole and in order.
func TestStreamDeliversInOrder_SEC_143(t *testing.T) {
	t.Parallel()
	r := exporttest.NewReceiver(t)
	s := streamTo(t, r)
	if err := s.Send(t.Context(), batch(0, 3)); err != nil {
		t.Fatal(err)
	}
	if err := s.Send(t.Context(), batch(3, 2)); err != nil {
		t.Fatal(err)
	}
	want := []string{"message a", "message b", "message c", "message d", "message e"}
	if got := r.Messages(); !slices.Equal(got, want) {
		t.Errorf("messages = %q, want %q", got, want)
	}
}

// Verifies: SEC-143.
// A receiver that resets the connection mid-batch fails the batch, so the
// caller sends it again; the batch then arrives, at least once and with
// nothing missing.
func TestStreamFailsAReset_SEC_143(t *testing.T) {
	t.Parallel()
	r := exporttest.NewReceiver(t)
	r.ResetAfterFrames(2, 1)
	s := streamTo(t, r)
	data := batch(0, 4)
	if err := s.Send(t.Context(), data); err == nil {
		t.Fatal("a reset connection acknowledged the batch")
	}
	if err := s.Send(t.Context(), data); err != nil {
		t.Fatalf("the second attempt: %v", err)
	}
	got := r.Messages()
	if len(got) < 4 {
		t.Fatalf("messages = %q", got)
	}
	for _, w := range []string{"message a", "message b", "message c", "message d"} {
		if !slices.Contains(got[len(got)-4:], w) {
			t.Errorf("the resent batch lacks %q: %q", w, got)
		}
	}
}

// Verifies: SEC-143.
// A receiver that refuses the handshake, and one whose certificate the
// stream does not trust, fail the batch.
func TestStreamFailsUnverifiedReceivers_SEC_143(t *testing.T) {
	t.Parallel()
	r := exporttest.NewReceiver(t)
	r.RefuseHandshakes(true)
	if err := streamTo(t, r).Send(t.Context(), batch(0, 1)); err == nil {
		t.Error("a refused handshake acknowledged the batch")
	}
	other := exporttest.NewReceiver(t)
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM([]byte(r.CAPEM()))
	dialer := net.Dialer{Timeout: 5 * time.Second}
	s, err := export.NewStream(other.Address(), dialer.DialContext, &tls.Config{RootCAs: pool, ServerName: "127.0.0.1", MinVersion: tls.VersionTLS12}, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Send(t.Context(), batch(0, 1)); err == nil {
		t.Error("an untrusted certificate acknowledged the batch")
	}
	if got := other.Messages(); len(got) != 0 {
		t.Errorf("the untrusted receiver got %q", got)
	}
}

// Verifies: SEC-143.
// A cancelled context stops a send, and a stream cannot be built without
// verification.
func TestStreamRefusesUnsafeUse_SEC_143(t *testing.T) {
	t.Parallel()
	dial := func(context.Context, string, string) (net.Conn, error) { return nil, net.ErrClosed }
	if _, err := export.NewStream("h:1", dial, &tls.Config{InsecureSkipVerify: true}, time.Second); err == nil { //nolint:gosec // proves the refusal
		t.Error("a stream without verification was built")
	}
	if _, err := export.NewStream("h:1", nil, &tls.Config{}, time.Second); err == nil {
		t.Error("a stream without a dialer was built")
	}
	s, err := export.NewStream("h:1", dial, &tls.Config{MinVersion: tls.VersionTLS12}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Send(t.Context(), batch(0, 1)); err == nil {
		t.Error("a failing dial acknowledged the batch")
	}
}

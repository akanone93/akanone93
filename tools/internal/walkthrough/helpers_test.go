// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package walkthrough

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"testing"
	"time"
)

// validSyslog returns five RFC 5424 messages.
func validSyslog() []string {
	var msgs []string
	for i := range 5 {
		msgs = append(msgs, fmt.Sprintf("<14>1 2026-03-01T12:00:0%dZ plux-api plux - audit - entry %d", i, i))
	}
	return msgs
}

// newTestSink starts a sink on the loopback interface.
func newTestSink(t *testing.T) *Sink {
	t.Helper()
	sink, err := NewSink("127.0.0.1:0", []string{"127.0.0.1"}, time.Now, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sink.Close() })
	return sink
}

// dialSink connects to the sink, trusting its CA.
func dialSink(t *testing.T, sink *Sink) (net.Conn, error) {
	t.Helper()
	dialer := &tls.Dialer{Config: tlsClient(t, sink.CAPEM())}
	conn, err := dialer.DialContext(t.Context(), "tcp", sink.Address())
	if err != nil {
		return nil, fmt.Errorf("dial the sink: %w", err)
	}
	return conn, nil
}

// sendSyslog sends octet-counted frames to the sink and waits until it has
// kept them all, so that a step with a fake clock does not race the network.
func sendSyslog(t *testing.T, sink *Sink, msgs []string) {
	t.Helper()
	conn, err := dialSink(t, sink)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	for _, m := range msgs {
		if _, err := fmt.Fprintf(conn, "%d %s", len(m), m); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	for len(sink.Messages()) < len(msgs) {
		select {
		case <-ctx.Done():
			t.Fatalf("the sink kept %d of %d messages", len(sink.Messages()), len(msgs))
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// newTestIDP starts the OIDC test provider on the loopback interface.
func newTestIDP(t *testing.T) *IDP {
	t.Helper()
	port := freePort(t)
	p, err := NewIDP("127.0.0.1:"+port, "127.0.0.1:"+port, []string{"127.0.0.1"}, "plux-walkthrough", "client-secret-abc", time.Now, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}

// signInCaller plays the server's sign-in procedures: it starts a login with
// the provider's PKCE request and completes it with a session once the step
// has fetched a code from the provider.
type signInCaller struct{ idp *IDP }

// Call implements Caller.
func (c *signInCaller) Call(_ context.Context, procedure string, req, resp any) error {
	out, ok := resp.(*map[string]any)
	if !ok {
		return fmt.Errorf("unexpected response type %T", resp)
	}
	switch procedure {
	case identityService + "StartOidcLogin":
		(*out)["url"] = c.idp.Issuer() + "/authorize?" + url.Values{
			"client_id": {c.idp.ClientID()}, "state": {"st-1"}, "nonce": {"n-1"},
			"code_challenge": {"challenge"}, "code_challenge_method": {"S256"},
		}.Encode()
		(*out)["state"] = "st-1"
	case identityService + "CompleteOidcLogin":
		if code, _ := req.(map[string]any)["code"].(string); code == "" {
			return fmt.Errorf("no authorisation code")
		}
		(*out)["session"] = map[string]any{"id": "s1"}
	default:
		return fmt.Errorf("unexpected procedure %s", procedure)
	}
	return nil
}

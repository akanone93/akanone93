// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/dpop"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

// stepUpServer is a server that asks for a step-up the way the real one does:
// it verifies every DPoP proof, binds the grant to the key of the request and
// refuses a proof it has seen.
type stepUpServer struct {
	pluxv1connect.UnimplementedIdentityServiceHandler
	t   *testing.T
	url string
	// public is the base URL the server is configured with and the htu of
	// every proof must name; empty means url.
	public   string
	verifier dpop.Verifier

	mu      sync.Mutex
	seen    map[string]bool
	jkt     string
	polls   int
	refused bool
	revoked []string
	grants  int
}

const (
	testPAT   = "plux_pat_stepup"
	testGrant = "plux_sug_granted"
)

// check verifies the proof of a request to a path and returns its key.
func (s *stepUpServer) check(h http.Header, path string) (string, bool) {
	base := s.url
	if s.public != "" {
		base = s.public
	}
	proof, err := s.verifier.Verify(h.Get("DPoP"), dpop.Expect{Method: http.MethodPost, URL: base + path, AccessToken: testPAT, Window: time.Minute})
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil || s.seen[proof.JTI] {
		s.t.Errorf("proof for %s: %v (replayed %v)", path, err, s.seen[proof.JTI])
		return "", false
	}
	s.seen[proof.JTI] = true
	return proof.JKT, true
}

func (s *stepUpServer) routes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle(pluxv1connect.NewIdentityServiceHandler(s))
	mux.HandleFunc("POST "+stepUpRequestsPath, func(w http.ResponseWriter, r *http.Request) {
		jkt, ok := s.check(r.Header, stepUpRequestsPath)
		if !ok {
			http.Error(w, "proof", http.StatusUnauthorized)
			return
		}
		s.mu.Lock()
		s.jkt = jkt
		s.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"requestId": "r1", "secret": "s3", "url": s.url + "/stepup?request=r1"})
	})
	mux.HandleFunc("POST "+stepUpRequestsPath+"/r1/poll", func(w http.ResponseWriter, r *http.Request) {
		jkt, ok := s.check(r.Header, stepUpRequestsPath+"/r1/poll")
		s.mu.Lock()
		defer s.mu.Unlock()
		if !ok || jkt != s.jkt || r.Header.Get(stepUpProofHeader) != "s3" {
			http.Error(w, "poll", http.StatusUnauthorized)
			return
		}
		if s.refused {
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]string{"code": "PLX-6203", "message": "too many attempts"})
			return
		}
		s.polls++
		if s.polls < 2 {
			_ = json.NewEncoder(w).Encode(map[string]any{"approved": false})
			return
		}
		s.grants++
		_ = json.NewEncoder(w).Encode(map[string]any{"approved": true, "grant": testGrant, "expiresAt": time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339)})
	})
	return mux
}

// needStepUp refuses a call that carries no grant bound to the key of the
// request, as the real server's PLX-6200 does.
func (s *stepUpServer) needStepUp(h http.Header, procedure string) error {
	if h.Get(stepUpGrantHeader) != testGrant {
		return connect.NewError(connect.CodePermissionDenied, errString("PLX-6200: this action needs a step-up with a security key"))
	}
	jkt, ok := s.check(h, procedure)
	s.mu.Lock()
	defer s.mu.Unlock()
	if !ok || jkt != s.jkt {
		return connect.NewError(connect.CodePermissionDenied, errString("PLX-6200: the grant is bound to another key"))
	}
	return nil
}

func (s *stepUpServer) ListSessions(_ context.Context, r *connect.Request[pluxv1.ListSessionsRequest]) (*connect.Response[pluxv1.ListSessionsResponse], error) {
	if err := s.needStepUp(r.Header(), pluxv1connect.IdentityServiceListSessionsProcedure); err != nil {
		return nil, err
	}
	return connect.NewResponse(&pluxv1.ListSessionsResponse{Sessions: []*pluxv1.SessionInfo{{Id: "ses1", ClientKind: "studio", UserId: "u1"}}}), nil
}

func (s *stepUpServer) RevokeSession(_ context.Context, r *connect.Request[pluxv1.RevokeSessionRequest]) (*connect.Response[pluxv1.RevokeSessionResponse], error) {
	if err := s.needStepUp(r.Header(), pluxv1connect.IdentityServiceRevokeSessionProcedure); err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.revoked = append(s.revoked, r.Msg.GetId())
	s.mu.Unlock()
	return connect.NewResponse(&pluxv1.RevokeSessionResponse{}), nil
}

func (s *stepUpServer) RevokeAccessToken(_ context.Context, r *connect.Request[pluxv1.RevokeAccessTokenRequest]) (*connect.Response[pluxv1.RevokeAccessTokenResponse], error) {
	s.mu.Lock()
	s.revoked = append(s.revoked, "token:"+r.Msg.GetId())
	s.mu.Unlock()
	return connect.NewResponse(&pluxv1.RevokeAccessTokenResponse{}), nil
}

// start serves the fake and returns a runner for the CLI against it.
func startStepUpServer(t *testing.T) (*stepUpServer, func(cmd func(env, []string) int, args ...string) (int, string, string)) {
	t.Helper()
	t.Setenv(tokenEnv, testPAT)
	t.Setenv(serverEnv, "")
	t.Setenv(orgEnv, "")
	s := &stepUpServer{t: t, seen: map[string]bool{}}
	srv := httptest.NewServer(s.routes())
	t.Cleanup(srv.Close)
	s.url = srv.URL
	run := func(cmd func(env, []string) int, args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		e := newEnv(&out, &errOut)
		e.creds = credentials{dir: t.TempDir()}
		e.pollInterval = time.Millisecond
		return cmd(e, append([]string{args[0], "--server", srv.URL, "--org", "org-1"}, args[1:]...)), out.String(), errOut.String()
	}
	return s, run
}

// Verifies: GOV-005, SEC-103.
// A command the server refuses with PLX-6200 runs the ceremony, polls with
// DPoP proofs of one key, attaches the grant bound to that key and repeats the
// call, and the revocation commands reach the server.
func TestStepUpIsObtainedAndRetried_SEC_103(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	s, run := startStepUpServer(t)

	code, out, stderr := run(env.sessions, "list")
	if code != exitOK || !strings.Contains(out, "ses1\tstudio\tu1") {
		t.Fatalf("sessions list: %d %q %s", code, out, stderr)
	}
	if !strings.Contains(stderr, "/stepup?request=r1") || s.grants != 1 {
		t.Errorf("the person is told where to confirm (%q) and one grant is issued (%d)", stderr, s.grants)
	}
	if code, out, stderr = run(env.sessions, "revoke", "ses1"); code != exitOK || !strings.Contains(out, "Revoked ses1") {
		t.Fatalf("sessions revoke: %d %q %s", code, out, stderr)
	}
	if code, out, stderr = run(env.tokens, "revoke", "tok1"); code != exitOK || !strings.Contains(out, "Revoked tok1") {
		t.Fatalf("tokens revoke: %d %q %s", code, out, stderr)
	}
	if len(s.revoked) != 2 || s.revoked[0] != "ses1" || s.revoked[1] != "token:tok1" {
		t.Errorf("revoked = %v", s.revoked)
	}
}

// Verifies: GOV-005, SEC-103.
// A ceremony the server closes (too many attempts) fails the command with
// both the original refusal and the reason, and the call is not repeated.
func TestStepUpRefusalFailsTheCommand_SEC_103(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	s, run := startStepUpServer(t)
	s.refused = true
	code, _, stderr := run(env.sessions, "revoke", "ses1")
	if code != exitFailed || !strings.Contains(stderr, "PLX-6203") || !strings.Contains(stderr, "PLX-6200") {
		t.Errorf("a refused ceremony: %d %s", code, stderr)
	}
	if len(s.revoked) != 0 {
		t.Errorf("revoked = %v", s.revoked)
	}
}

// Verifies: SEC-103, SEC-022.
// A server reached by another address than its public base URL (a
// port-forward, an internal service) compares the htu of a proof with the
// public one, so a CLI told that URL builds every proof of the ceremony for
// it, and the whole step-up succeeds.
func TestStepUpProofNamesThePublicURL_SEC_103(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	s, run := startStepUpServer(t)
	s.public = "https://plux.example.com"
	t.Setenv(publicURLEnv, s.public+"/")

	code, out, stderr := run(env.sessions, "list")
	if code != exitOK || !strings.Contains(out, "ses1\tstudio\tu1") || s.grants != 1 {
		t.Fatalf("sessions list: %d %q %s (grants %d)", code, out, stderr, s.grants)
	}
}

// Verifies: SEC-103, SEC-022.
// Without a public URL the proofs name the address the CLI calls, and the
// server's check is not loosened for the rest: a proof for that address is
// refused by a server whose public base URL is another.
func TestStepUpProofDefaultsToTheServerAddress_SEC_103(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	t.Setenv(publicURLEnv, "")
	const server, public = "http://127.0.0.1:18081", "https://plux.example.com"
	su := newEnv(&bytes.Buffer{}, &bytes.Buffer{}).newStepUpper(server, testPAT)
	proof, err := su.proof(http.MethodPost, su.proofBase+stepUpRequestsPath)
	if err != nil {
		t.Fatal(err)
	}
	var v dpop.Verifier
	expect := dpop.Expect{Method: http.MethodPost, AccessToken: testPAT, Window: time.Minute, URL: server + stepUpRequestsPath}
	if _, err := v.Verify(proof, expect); err != nil {
		t.Errorf("a proof for the address the CLI calls: %v", err)
	}
	expect.URL = public + stepUpRequestsPath
	if _, err := v.Verify(proof, expect); err == nil {
		t.Error("a proof for another address verified")
	}
}

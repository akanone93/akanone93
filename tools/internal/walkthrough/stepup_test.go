// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package walkthrough

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// stepUpServer is a fake API that refuses a call until it carries a step-up
// grant with a valid DPoP proof, as the server does (SEC-103).
type stepUpServer struct {
	t         *testing.T
	publicURL string
	token     string
	retried   bool
}

func (s *stepUpServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case stepUpRequestsPath:
		s.checkProof(r, stepUpRequestsPath)
		_ = json.NewEncoder(w).Encode(map[string]any{"requestId": "rq-1", "secret": "s-1", "url": "https://plux.test/stepup/rq-1"})
	case stepUpRequestsPath + "/rq-1/poll":
		s.checkProof(r, r.URL.Path)
		if r.Header.Get(stepUpProofHeader) != "s-1" {
			s.t.Errorf("poll without the request's secret")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"approved": true, "grant": "g-1", "expiresAt": time.Now().Add(time.Minute).Format(time.RFC3339)})
	case "/plux.v1.FederationService/CreateScimToken":
		if r.Header.Get(stepUpGrantHeader) != "g-1" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"code":"permission_denied","message":"PLX-6200: this action needs a step-up with a security key"}`))
			return
		}
		s.checkProof(r, r.URL.Path)
		s.retried = true
		_, _ = w.Write([]byte(`{"token":"scim-1"}`))
	default:
		http.NotFound(w, r)
	}
}

// checkProof verifies a request's DPoP proof: ES256 over its own key, the
// method, the public URL of the path and the hash of the bearer token.
func (s *stepUpServer) checkProof(r *http.Request, path string) {
	s.t.Helper()
	parts := strings.Split(r.Header.Get(dpopHeader), ".")
	if len(parts) != 3 {
		s.t.Fatalf("no DPoP proof on %s", path)
	}
	dec := base64.RawURLEncoding
	var header struct {
		Alg, Typ string
		JWK      struct{ X, Y string }
	}
	var claims struct{ Htm, Htu, Ath string }
	h, _ := dec.DecodeString(parts[0])
	c, _ := dec.DecodeString(parts[1])
	sig, _ := dec.DecodeString(parts[2])
	if json.Unmarshal(h, &header) != nil || json.Unmarshal(c, &claims) != nil || len(sig) != 64 {
		s.t.Fatalf("malformed proof on %s", path)
	}
	x, _ := dec.DecodeString(header.JWK.X)
	y, _ := dec.DecodeString(header.JWK.Y)
	pub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), append(append([]byte{4}, x...), y...))
	if err != nil {
		s.t.Fatalf("proof key: %v", err)
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(pub, digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		s.t.Errorf("the proof on %s does not verify", path)
	}
	ath := sha256.Sum256([]byte(s.token))
	if header.Alg != "ES256" || header.Typ != "dpop+jwt" || claims.Htm != http.MethodPost ||
		claims.Htu != s.publicURL+path || claims.Ath != dec.EncodeToString(ath[:]) {
		s.t.Errorf("proof on %s: header %+v, claims %+v", path, header, claims)
	}
}

// Verifies: SEC-103, GOV-004.
// A call the server refuses for want of a step-up is stepped up through the
// hook (as the named actor, at the server's URL) and repeated once with the
// grant and a DPoP proof naming the public URL.
func TestConnectClientStepsUp(t *testing.T) {
	t.Parallel()
	fake := &stepUpServer{t: t, publicURL: "https://plux.example", token: "pat-1"}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	var confirmed []string
	c := &ConnectClient{
		BaseURL: srv.URL, Token: "pat-1", HTTP: srv.Client(), PublicURL: fake.publicURL, Actor: "admin",
		StepUp: func(_ context.Context, actor, url string) error {
			confirmed = append(confirmed, actor+" "+url)
			return nil
		},
	}
	var out struct{ Token string }
	if err := c.Call(t.Context(), "/plux.v1.FederationService/CreateScimToken", map[string]any{}, &out); err != nil {
		t.Fatal(err)
	}
	if out.Token != "scim-1" || !fake.retried || len(confirmed) != 1 || confirmed[0] != "admin https://plux.test/stepup/rq-1" {
		t.Errorf("token %q, retried %v, confirmed %v", out.Token, fake.retried, confirmed)
	}
}

// Verifies: SEC-103.
// Without a StepUp hook the refusal is returned unchanged, and a failed
// ceremony is reported rather than retried.
func TestConnectClientStepUpRefusals(t *testing.T) {
	t.Parallel()
	fake := &stepUpServer{t: t, publicURL: "", token: "pat-1"}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	fake.publicURL = srv.URL
	plain := &ConnectClient{BaseURL: srv.URL, Token: "pat-1", HTTP: srv.Client()}
	if err := plain.Call(t.Context(), "/plux.v1.FederationService/CreateScimToken", map[string]any{}, nil); !needsStepUp(err) {
		t.Errorf("without a hook = %v, want the step-up refusal", err)
	}
	failing := &ConnectClient{
		BaseURL: srv.URL, Token: "pat-1", HTTP: srv.Client(),
		StepUp: func(context.Context, string, string) error { return errors.New("no authenticator") },
	}
	if err := failing.Call(t.Context(), "/plux.v1.FederationService/CreateScimToken", map[string]any{}, nil); err == nil || !strings.Contains(err.Error(), "no authenticator") {
		t.Errorf("failed ceremony = %v", err)
	}
	if fake.retried {
		t.Error("a call was retried without a grant")
	}
}

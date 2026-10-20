// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/go-jose/go-jose/v4"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// The wire names of the CLI's step-up (SEC-103, ADR-0075). They are the
// server's routes and headers; the CLI keeps its own copy because it does not
// import the server's API package.
const (
	stepUpRequestsPath = "/stepup/requests"
	stepUpGrantHeader  = "Plux-Step-Up-Grant"
	stepUpSecretHeader = "Plux-Step-Up-Secret"
	dpopHeader         = "DPoP"
	dpopType           = "dpop+jwt"
)

// stepUpper obtains the step-up grant of a command and attaches it to the
// calls that follow (SEC-103, GOV-005). A person proves presence with a
// security key in the browser; the server binds the resulting grant to the
// CLI's token and to the thumbprint of a DPoP key, so the grant is useless
// without both. The key is generated for the command and never stored: it
// lives as long as the grant is useful.
type stepUpper struct {
	http     *http.Client
	server   string
	token    string
	interval time.Duration
	out      io.Writer
	now      func() time.Time

	mu      sync.Mutex
	key     *ecdsa.PrivateKey
	grant   string
	expires time.Time
}

// newStepUpper returns the step-up of one command against a server.
func (e env) newStepUpper(server, token string) *stepUpper {
	return &stepUpper{http: e.httpClient, server: server, token: token, interval: e.pollInterval, out: e.stderr, now: time.Now}
}

// needsStepUp reports whether the server refused a call for want of a
// step-up (PLX-6200) or for one that is too old (PLX-6201).
func needsStepUp(err error) bool {
	var ce *connect.Error
	if !errors.As(err, &ce) {
		return false
	}
	return strings.Contains(ce.Message(), plxerr.StepUpRequired.String()) ||
		strings.Contains(ce.Message(), plxerr.StepUpExpired.String())
}

// attach adds the grant, when one is held and still valid, and a fresh DPoP
// proof of the key it is bound to, to a call of the procedure.
func (s *stepUpper) attach(h http.Header, procedure string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.grant == "" || !s.now().Before(s.expires) {
		return nil
	}
	proof, err := s.proof(http.MethodPost, s.server+procedure)
	if err != nil {
		return err
	}
	h.Set(stepUpGrantHeader, s.grant)
	h.Set(dpopHeader, proof)
	return nil
}

// obtain runs the ceremony: it asks the server for a request, tells the
// person where to complete it and polls until the grant is issued.
func (s *stepUpper) obtain(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	created, err := s.post(ctx, stepUpRequestsPath, "")
	if err != nil {
		return fmt.Errorf("step-up: %w", err)
	}
	_, _ = fmt.Fprintf(s.out, "This action needs a step-up. Open %s and confirm with your security key.\n", created.URL)
	path := stepUpRequestsPath + "/" + url.PathEscape(created.RequestID) + "/poll"
	for {
		polled, err := s.post(ctx, path, created.Secret)
		if err != nil {
			return fmt.Errorf("step-up: %w", err)
		}
		if polled.Approved {
			return s.keep(polled)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("step-up: %w", ctx.Err())
		case <-time.After(s.interval):
		}
	}
}

// keep remembers the grant of an approved request.
func (s *stepUpper) keep(r stepUpReply) error {
	expires, err := time.Parse(time.RFC3339, r.ExpiresAt)
	if err != nil || r.Grant == "" {
		return errors.New("step-up: the server issued no usable grant")
	}
	s.grant, s.expires = r.Grant, expires
	return nil
}

// stepUpReply is what the step-up routes answer.
type stepUpReply struct {
	RequestID string `json:"requestId"`
	Secret    string `json:"secret"`
	URL       string `json:"url"`
	Approved  bool   `json:"approved"`
	Grant     string `json:"grant"`
	ExpiresAt string `json:"expiresAt"`
}

// post calls one of the step-up routes with the CLI's token and a DPoP
// proof; a poll also carries the request's secret.
func (s *stepUpper) post(ctx context.Context, path, secret string) (stepUpReply, error) {
	proof, err := s.proof(http.MethodPost, s.server+path)
	if err != nil {
		return stepUpReply{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.server+path, http.NoBody)
	if err != nil {
		return stepUpReply{}, fmt.Errorf("build the request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set(dpopHeader, proof)
	if secret != "" {
		req.Header.Set(stepUpSecretHeader, secret)
	}
	res, err := s.http.Do(req)
	if err != nil {
		return stepUpReply{}, fmt.Errorf("call the server: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<16))
	if err != nil {
		return stepUpReply{}, fmt.Errorf("read the answer: %w", err)
	}
	if res.StatusCode/100 != 2 {
		return stepUpReply{}, refusal(res.StatusCode, body)
	}
	var out stepUpReply
	if err := json.Unmarshal(body, &out); err != nil {
		return stepUpReply{}, fmt.Errorf("read the answer: %w", err)
	}
	return out, nil
}

// refusal turns an error answer of the step-up routes into an error.
func refusal(status int, body []byte) error {
	var e struct{ Code, Message string }
	if json.Unmarshal(body, &e) != nil || e.Code == "" {
		return fmt.Errorf("the server answered %d", status)
	}
	return fmt.Errorf("%s: %s", e.Code, e.Message)
}

// proof builds a DPoP proof for a request, signed with the command's key and
// bound to the token (RFC 9449). Each proof is used once.
func (s *stepUpper) proof(method, target string) (string, error) {
	if s.key == nil {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return "", fmt.Errorf("generate a DPoP key: %w", err)
		}
		s.key = key
	}
	jti := make([]byte, 16)
	if _, err := rand.Read(jti); err != nil {
		return "", fmt.Errorf("generate a proof identifier: %w", err)
	}
	sum := sha256.Sum256([]byte(s.token))
	claims, err := json.Marshal(map[string]any{
		"htm": method, "htu": target, "iat": s.now().Unix(),
		"jti": base64.RawURLEncoding.EncodeToString(jti),
		"ath": base64.RawURLEncoding.EncodeToString(sum[:]),
	})
	if err != nil {
		return "", fmt.Errorf("encode the proof: %w", err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.ES256, Key: s.key},
		(&jose.SignerOptions{}).WithType(dpopType).WithHeader("jwk", jose.JSONWebKey{Key: &s.key.PublicKey}))
	if err != nil {
		return "", fmt.Errorf("sign the proof: %w", err)
	}
	jws, err := signer.Sign(claims)
	if err != nil {
		return "", fmt.Errorf("sign the proof: %w", err)
	}
	compact, err := jws.CompactSerialize()
	if err != nil {
		return "", fmt.Errorf("sign the proof: %w", err)
	}
	return compact, nil
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package walkthrough

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
)

// The wire names of the server's step-up for API clients (SEC-103,
// ADR-0075); the same routes and headers the plux CLI uses.
const (
	stepUpRequestsPath = "/stepup/requests"
	stepUpGrantHeader  = "Plux-Step-Up-Grant"
	stepUpProofHeader  = "Plux-Step-Up-Secret"
	dpopHeader         = "DPoP"
	stepUpPoll         = 500 * time.Millisecond
)

// stepUpCodes are the refusals a step-up answers: required and expired.
var stepUpCodes = []string{"PLX-6200", "PLX-6201"}

// stepUpper obtains a step-up grant for a ConnectClient and attaches it to
// later calls, as the CLI does: the person confirms with a security key (the
// walkthrough's StepUp hook drives a software authenticator), and the server
// binds the grant to the bearer token and to the thumbprint of a DPoP key
// generated for this client and never stored.
type stepUpper struct {
	mu      sync.Mutex
	key     *ecdsa.PrivateKey
	grant   string
	expires time.Time
}

// needsStepUp reports whether a refusal asks for a step-up.
func needsStepUp(err error) bool {
	if err == nil {
		return false
	}
	for _, code := range stepUpCodes {
		if strings.Contains(err.Error(), code) {
			return true
		}
	}
	return false
}

// attach adds the grant and a DPoP proof for the call when a grant is held.
func (s *stepUpper) attach(h http.Header, token, target string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.grant == "" || !time.Now().Before(s.expires) {
		return nil
	}
	proof, err := s.proof(http.MethodPost, target, token)
	if err != nil {
		return err
	}
	h.Set(stepUpGrantHeader, s.grant)
	h.Set(dpopHeader, proof)
	return nil
}

// stepUpReply is the server's answer to a step-up request or poll.
type stepUpReply struct {
	RequestID string `json:"requestId"`
	Secret    string `json:"secret"`
	URL       string `json:"url"`
	Approved  bool   `json:"approved"`
	Grant     string `json:"grant"`
	ExpiresAt string `json:"expiresAt"`
}

// obtain asks for a step-up, has confirm complete the ceremony at its URL, and
// polls until the server issues the grant.
func (s *stepUpper) obtain(ctx context.Context, c *ConnectClient) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	created, err := s.post(ctx, c, stepUpRequestsPath, "")
	if err != nil {
		return fmt.Errorf("walkthrough.stepUp: %w", err)
	}
	if err := c.StepUp(ctx, c.Actor, created.URL); err != nil {
		return fmt.Errorf("walkthrough.stepUp: confirm: %w", err)
	}
	path := stepUpRequestsPath + "/" + url.PathEscape(created.RequestID) + "/poll"
	for {
		polled, err := s.post(ctx, c, path, created.Secret)
		if err != nil {
			return fmt.Errorf("walkthrough.stepUp: %w", err)
		}
		if polled.Approved {
			expires, err := time.Parse(time.RFC3339, polled.ExpiresAt)
			if err != nil || polled.Grant == "" {
				return errors.New("walkthrough.stepUp: the server issued no usable grant")
			}
			s.grant, s.expires = polled.Grant, expires
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("walkthrough.stepUp: %w", ctx.Err())
		case <-time.After(stepUpPoll):
		}
	}
}

// post calls a step-up route with a DPoP proof and, when polling, the
// request's secret.
func (s *stepUpper) post(ctx context.Context, c *ConnectClient, path, secret string) (stepUpReply, error) {
	proof, err := s.proof(http.MethodPost, c.proofBase()+path, c.Token)
	if err != nil {
		return stepUpReply{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(c.BaseURL, "/")+path, http.NoBody)
	if err != nil {
		return stepUpReply{}, fmt.Errorf("build the request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set(dpopHeader, proof)
	if secret != "" {
		req.Header.Set(stepUpProofHeader, secret)
	}
	res, err := c.client().Do(req)
	if err != nil {
		return stepUpReply{}, fmt.Errorf("call the server: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(res.Body, maxReply))
	if err != nil {
		return stepUpReply{}, fmt.Errorf("read the answer: %w", err)
	}
	if res.StatusCode/100 != 2 {
		return stepUpReply{}, connectError(path, res.StatusCode, body)
	}
	var out stepUpReply
	if err := json.Unmarshal(body, &out); err != nil {
		return stepUpReply{}, fmt.Errorf("read the answer: %w", err)
	}
	return out, nil
}

// proof signs a DPoP proof (RFC 9449) for a request with the client's key,
// bound to the bearer token by its hash. The caller holds mu.
func (s *stepUpper) proof(method, target, token string) (string, error) {
	if s.key == nil {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return "", fmt.Errorf("walkthrough.stepUp: generate a DPoP key: %w", err)
		}
		s.key = key
	}
	point, err := s.key.PublicKey.Bytes()
	if err != nil {
		return "", fmt.Errorf("walkthrough.stepUp: encode the DPoP key: %w", err)
	}
	enc := base64.RawURLEncoding
	header, err := json.Marshal(map[string]any{
		"alg": "ES256", "typ": "dpop+jwt",
		"jwk": map[string]string{"kty": "EC", "crv": "P-256", "x": enc.EncodeToString(point[1:33]), "y": enc.EncodeToString(point[33:65])},
	})
	if err != nil {
		return "", fmt.Errorf("walkthrough.stepUp: encode the proof: %w", err)
	}
	jti := make([]byte, 16)
	if _, err := rand.Read(jti); err != nil {
		return "", fmt.Errorf("walkthrough.stepUp: generate a proof identifier: %w", err)
	}
	ath := sha256.Sum256([]byte(token))
	claims, err := json.Marshal(map[string]any{
		"htm": method, "htu": target, "iat": time.Now().Unix(),
		"jti": enc.EncodeToString(jti), "ath": enc.EncodeToString(ath[:]),
	})
	if err != nil {
		return "", fmt.Errorf("walkthrough.stepUp: encode the proof: %w", err)
	}
	signing := enc.EncodeToString(header) + "." + enc.EncodeToString(claims)
	digest := sha256.Sum256([]byte(signing))
	r, sig, err := ecdsa.Sign(rand.Reader, s.key, digest[:])
	if err != nil {
		return "", fmt.Errorf("walkthrough.stepUp: sign the proof: %w", err)
	}
	raw := make([]byte, 64)
	r.FillBytes(raw[:32])
	sig.FillBytes(raw[32:])
	return signing + "." + enc.EncodeToString(raw), nil
}

// firstNonEmpty returns the first of values that is not empty.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

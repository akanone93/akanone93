// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package loadgen

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
	"net/http"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/go-jose/go-jose/v4"

	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

// session is a device's credentials on the wire. As a connect interceptor
// it signs each unary call with a DPoP proof bound to the access token and
// the server's latest nonce, and retries once when the server asks for a
// new nonce, the way the runtime does (SEC-020).
type session struct {
	key     *ecdsa.PrivateKey
	signer  jose.Signer
	baseURL string
	address string
	now     func() time.Time

	mu           sync.Mutex
	token, nonce string
}

// newSession creates a session with a fresh P-256 key. address, when not
// empty, is sent as X-Forwarded-For.
func newSession(baseURL, address string, now func() time.Time) (*session, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("loadgen.newSession: %w", err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.ES256, Key: key}, (&jose.SignerOptions{}).
		WithType("dpop+jwt").WithHeader("jwk", jose.JSONWebKey{Key: &key.PublicKey}))
	if err != nil {
		return nil, fmt.Errorf("loadgen.newSession: %w", err)
	}
	return &session{key: key, signer: signer, baseURL: strings.TrimSuffix(baseURL, "/"), address: address, now: now}, nil
}

// publicJWK is the device's public key as a JSON Web Key.
func (s *session) publicJWK() ([]byte, error) {
	raw, err := json.Marshal(jose.JSONWebKey{Key: &s.key.PublicKey})
	if err != nil {
		return nil, fmt.Errorf("loadgen.publicJWK: %w", err)
	}
	return raw, nil
}

// setToken stores the access token that later calls are bound to.
func (s *session) setToken(token string) {
	s.mu.Lock()
	s.token = token
	s.mu.Unlock()
}

// proof builds a DPoP proof for a procedure (RFC 9449 section 4.2). The
// token endpoint is called without the access token.
func (s *session) proof(procedure string) (proof, authorization string, err error) {
	s.mu.Lock()
	token, nonce := s.token, s.nonce
	s.mu.Unlock()
	id := make([]byte, 12)
	if _, err := rand.Read(id); err != nil {
		return "", "", fmt.Errorf("loadgen.proof: %w", err)
	}
	claims := map[string]any{
		"htm": http.MethodPost, "htu": s.baseURL + procedure,
		"iat": s.now().Unix(), "jti": base64.RawURLEncoding.EncodeToString(id),
	}
	if token != "" && procedure != pluxv1connect.TokenServiceRefreshDeviceTokenProcedure {
		sum := sha256.Sum256([]byte(token))
		claims["ath"] = base64.RawURLEncoding.EncodeToString(sum[:])
		authorization = "DPoP " + token
	}
	if nonce != "" {
		claims["nonce"] = nonce
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", "", fmt.Errorf("loadgen.proof: %w", err)
	}
	jws, err := s.signer.Sign(payload)
	if err != nil {
		return "", "", fmt.Errorf("loadgen.proof: %w", err)
	}
	compact, err := jws.CompactSerialize()
	if err != nil {
		return "", "", fmt.Errorf("loadgen.proof: %w", err)
	}
	return compact, authorization, nil
}

// remember keeps the nonce a response or a refusal carried and reports
// whether there was one.
func (s *session) remember(h http.Header) bool {
	nonce := h.Get("DPoP-Nonce")
	if nonce == "" {
		return false
	}
	s.mu.Lock()
	s.nonce = nonce
	s.mu.Unlock()
	return true
}

// WrapUnary signs each unary call and retries it once with a new nonce.
func (s *session) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, r connect.AnyRequest) (connect.AnyResponse, error) {
		for attempt := 0; ; attempt++ {
			proof, authorization, err := s.proof(r.Spec().Procedure)
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, err)
			}
			r.Header().Set("DPoP", proof)
			if authorization != "" {
				r.Header().Set("Authorization", authorization)
			}
			if s.address != "" {
				r.Header().Set("X-Forwarded-For", s.address)
			}
			res, err := next(ctx, r)
			if err == nil {
				s.remember(res.Header())
				return res, nil
			}
			var refusal *connect.Error
			if errors.As(err, &refusal) && s.remember(refusal.Meta()) && attempt == 0 &&
				strings.Contains(refusal.Meta().Get("WWW-Authenticate"), "use_dpop_nonce") {
				continue
			}
			return res, err
		}
	}
}

// WrapStreamingClient leaves streaming calls alone: devices make none.
func (*session) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

// WrapStreamingHandler is part of the interceptor interface; a client has
// no handlers.
func (*session) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}

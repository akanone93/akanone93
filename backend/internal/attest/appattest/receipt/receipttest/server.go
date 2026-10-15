// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package receipttest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// Path is the endpoint the simulator serves.
const Path = "/v1/attestationData"

// tokenSkew is how far a token's iat may be from the server's clock.
const tokenSkew = 5 * time.Minute

// Mode selects how the server answers an authorised request.
type Mode int

const (
	// ModeOK answers 200 with a valid receipt.
	ModeOK Mode = iota
	// ModeNotModified answers 304 without a body.
	ModeNotModified
	// ModeUnauthorized answers 401.
	ModeUnauthorized
	// ModeTooManyRequests answers 429.
	ModeTooManyRequests
	// ModeServerError answers 500.
	ModeServerError
	// ModeTimeout never answers.
	ModeTimeout
	// ModeTampered answers 200 with a receipt whose signature is wrong.
	ModeTampered
	// ModeTamperedContent answers 200 with a payload changed after signing.
	ModeTamperedContent
	// ModeExpired answers 200 with a receipt whose validity ended.
	ModeExpired
	// ModeWrongApp answers 200 with a receipt for another app.
	ModeWrongApp
	// ModeWrongKey answers 200 with a receipt for another key.
	ModeWrongKey
	// ModeUntrusted answers 200 with a receipt from an unknown authority.
	ModeUntrusted
)

// Config configures a Server.
type Config struct {
	// Authority issues the receipts; nil makes a new one.
	Authority *Authority
	// JWTKey verifies the request token.
	JWTKey *ecdsa.PublicKey
	// KeyID and TeamID are the kid and iss the token must carry.
	KeyID, TeamID string
	// AppID and Key are what the receipts name.
	AppID string
	Key   *ecdsa.PublicKey
	// Now is the server's clock.
	Now func() time.Time
}

// Server is the simulator. Create it with New.
type Server struct {
	// URL is the base URL of the simulator.
	URL string

	cfg   Config
	auth  *Authority
	srv   *httptest.Server
	other *Authority
	done  chan struct{}
	log   testing.TB

	mu       sync.Mutex
	mode     Mode
	encoding Encoding
	risk     int
	rawBody  bool
	requests int
	received []byte
}

// New starts a simulator and stops it when the test ends.
func New(t testing.TB, cfg Config) *Server {
	t.Helper()
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Authority == nil {
		cfg.Authority = NewAuthority(t, cfg.Now())
	}
	s := &Server{cfg: cfg, auth: cfg.Authority, other: NewAuthority(t, cfg.Now()), done: make(chan struct{}), log: t}
	s.srv = httptest.NewServer(http.HandlerFunc(s.handle))
	s.URL = s.srv.URL
	t.Cleanup(func() {
		close(s.done)
		s.srv.Close()
	})
	return s
}

// Authority returns the authority that issues the receipts.
func (s *Server) Authority() *Authority { return s.auth }

// SetMode sets how the server answers.
func (s *Server) SetMode(m Mode) { s.set(func() { s.mode = m }) }

// SetEncoding sets the CMS encoding of the receipts.
func (s *Server) SetEncoding(e Encoding) { s.set(func() { s.encoding = e }) }

// SetRisk sets the risk metric of the receipts.
func (s *Server) SetRisk(n int) { s.set(func() { s.risk = n }) }

// SetRawBody makes the server answer with the receipt bytes instead of
// their base64 form.
func (s *Server) SetRawBody(raw bool) { s.set(func() { s.rawBody = raw }) }

func (s *Server) set(f func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f()
}

// Requests returns how many authorised requests reached the server.
func (s *Server) Requests() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requests
}

// Received returns the receipt the last authorised request carried.
func (s *Server) Received() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.received...)
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != Path {
		http.NotFound(w, r)
		return
	}
	if !s.authorised(r) {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.requests++
	if raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(body))); err == nil {
		s.received = raw
	} else {
		s.received = body
	}
	mode, enc, risk, rawBody := s.mode, s.encoding, s.risk, s.rawBody
	s.mu.Unlock()
	s.answer(w, r, mode, enc, risk, rawBody)
}

func (s *Server) answer(w http.ResponseWriter, r *http.Request, mode Mode, enc Encoding, risk int, rawBody bool) {
	switch mode {
	case ModeNotModified:
		w.WriteHeader(http.StatusNotModified)
	case ModeUnauthorized:
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
	case ModeTooManyRequests:
		w.Header().Set("Retry-After", "60")
		http.Error(w, "too many requests", http.StatusTooManyRequests)
	case ModeServerError:
		http.Error(w, "internal error", http.StatusInternalServerError)
	case ModeTimeout:
		select {
		case <-r.Context().Done():
		case <-s.done:
		}
	default:
		receipt, err := s.issue(mode, enc, risk)
		if err != nil {
			s.log.Errorf("receipttest: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if rawBody {
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(receipt)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(base64.StdEncoding.EncodeToString(receipt)))
	}
}

// issue builds the receipt a mode asks for.
func (s *Server) issue(mode Mode, enc Encoding, risk int) ([]byte, error) {
	o := IssueOptions{AppID: s.cfg.AppID, Key: s.cfg.Key, Risk: risk, Encoding: enc}
	auth := s.auth
	now := s.cfg.Now()
	switch mode {
	case ModeTampered:
		o.TamperSignature = true
	case ModeTamperedContent:
		o.TamperContent = true
	case ModeExpired:
		o.NotBefore, o.Expiration = now.Add(-48*time.Hour), now.Add(-time.Hour)
	case ModeWrongApp:
		o.AppID = "TEAMID.other.app"
	case ModeWrongKey:
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, err //nolint:wrapcheck // a test helper
		}
		o.Key = &k.PublicKey
	case ModeUntrusted:
		auth = s.other
	}
	return auth.Build(o)
}

// authorised checks the request token: ES256 under the configured key,
// with the configured kid and iss, and an iat close to the clock.
func (s *Server) authorised(r *http.Request) bool {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		return false
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return false
	}
	var header struct{ Alg, Kid string }
	var claims struct {
		Iss string
		Iat int64
		Exp int64
	}
	if !decodeJSON(parts[0], &header) || !decodeJSON(parts[1], &claims) {
		return false
	}
	now := s.cfg.Now()
	iat := time.Unix(claims.Iat, 0)
	if header.Alg != "ES256" || header.Kid != s.cfg.KeyID || claims.Iss != s.cfg.TeamID ||
		iat.Before(now.Add(-tokenSkew)) || iat.After(now.Add(tokenSkew)) ||
		(claims.Exp != 0 && !now.Before(time.Unix(claims.Exp, 0))) {
		return false
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(sig) != 64 {
		return false
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	return ecdsa.Verify(s.cfg.JWTKey, digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:]))
}

// decodeJSON decodes a base64url JSON segment; field names match
// case-insensitively.
func decodeJSON(segment string, out any) bool {
	raw, err := base64.RawURLEncoding.DecodeString(segment)
	return err == nil && json.Unmarshal(raw, out) == nil
}

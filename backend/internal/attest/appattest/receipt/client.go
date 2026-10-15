// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package receipt

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nightCode42/plux3/backend/internal/attest/appattest"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Role is the name of the imported signing key that signs Apple requests
// (ADR-0065, SEC-123).
const Role = "apple-appattest-receipt"

// The hosts of Apple's App Attest data service and the one path used.
const (
	// ProductionHost serves attestations made in the production
	// environment.
	ProductionHost = "https://data.appattest.apple.com"
	// DevelopmentHost serves attestations made in the sandbox.
	DevelopmentHost = "https://data-development.appattest.apple.com"
	// Path is the endpoint that returns a refreshed receipt.
	Path = "/v1/attestationData"

	// defaultTimeout bounds one refresh, the signature included.
	defaultTimeout = 10 * time.Second
	// tokenLifetime is how long the request token is valid. It is short
	// because a fresh one is signed for every request.
	tokenLifetime = 5 * time.Minute
	// digestLen and signatureLen are those of SHA-256 and of an ES256 JWS
	// signature, r‖s of two 32-byte integers.
	digestLen    = sha256.Size
	signatureLen = 64
)

// ES256Signer signs a SHA-256 digest with an imported P-256 key and returns
// the JWS form r‖s (64 bytes) (SEC-123). *signing.Imported is one; the key
// never leaves it.
type ES256Signer interface {
	SignES256(ctx context.Context, role string, digest []byte) ([]byte, error)
}

// Hosts are the base URLs of the data service. Only tests override them.
type Hosts struct {
	// Production and Development default to Apple's hosts when empty.
	Production, Development string
}

// Config configures a Client.
type Config struct {
	// KeyID is the ID of the App Store Connect key (the JWT's kid).
	KeyID string
	// TeamID is the Apple Team ID (the JWT's iss).
	TeamID string
	// Signer signs the request token; it holds the imported key.
	Signer ES256Signer
	// HTTP sends the requests; use a client from package httpx.
	HTTP *http.Client
	// Now is the clock; nil means time.Now.
	Now func() time.Time
	// Hosts override Apple's hosts, for tests.
	Hosts Hosts
	// Roots anchors the receipt's signer chain; nil means the embedded
	// Apple App Attestation root.
	Roots *x509.CertPool
	// Timeout bounds one refresh; zero means ten seconds.
	Timeout time.Duration
	// MaxBytes bounds a response; zero means the registry default,
	// attest.appAttestObjectBytes (LIM-001).
	MaxBytes int64
}

// Client refreshes receipts at Apple. It is safe for concurrent use.
type Client struct {
	cfg      Config
	verifier Verifier
}

// New returns a client, or an error when the configuration is incomplete.
func New(cfg Config) (*Client, error) {
	if cfg.KeyID == "" || cfg.TeamID == "" || cfg.Signer == nil || cfg.HTTP == nil {
		return nil, errors.New("receipt: a key ID, a team ID, a signer and an HTTP client are required")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	if cfg.Hosts.Production == "" {
		cfg.Hosts.Production = ProductionHost
	}
	if cfg.Hosts.Development == "" {
		cfg.Hosts.Development = DevelopmentHost
	}
	if cfg.Roots == nil {
		roots, err := appattest.DefaultRoots()
		if err != nil {
			return nil, errors.New("receipt: the embedded Apple root does not load")
		}
		cfg.Roots = roots
	}
	return &Client{cfg: cfg, verifier: Verifier{Roots: cfg.Roots, Now: cfg.Now, MaxBytes: cfg.MaxBytes}}, nil
}

// Verify opens a receipt without contacting Apple.
func (c *Client) Verify(raw []byte, want Expect) (Receipt, error) {
	return c.verifier.Verify(raw, want)
}

// Refresh asks Apple for the current receipt of a device's attested key
// and verifies the answer against want. env selects Apple's production or
// development host. A 304 keeps the given receipt, which is verified too.
// Every failure is PLX-6080 and carries no key, token or receipt bytes.
func (c *Client) Refresh(ctx context.Context, receipt []byte, env appattest.Environment, want Expect) (Receipt, error) {
	if len(receipt) == 0 {
		return Receipt{}, unavailable("there is no receipt to refresh")
	}
	host := c.cfg.Hosts.Development
	switch env {
	case appattest.Development:
	case appattest.Production:
		host = c.cfg.Hosts.Production
	default:
		return Receipt{}, unavailable("the App Attest environment is unknown")
	}
	ctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	token, err := c.token(ctx)
	if err != nil {
		return Receipt{}, err
	}
	status, body, err := c.post(ctx, strings.TrimSuffix(host, "/")+Path, token, receipt)
	if err != nil {
		return Receipt{}, err
	}
	switch status {
	case http.StatusOK:
		return c.verifier.Verify(decodeBody(body), want)
	case http.StatusNotModified:
		return c.verifier.Verify(receipt, want)
	default:
		return Receipt{}, unavailable("Apple answered with status %d", status)
	}
}

// post sends the receipt, base64 encoded as Apple documents, and reads a
// bounded response.
func (c *Client) post(ctx context.Context, url, token string, receipt []byte) (int, []byte, error) {
	body := base64.StdEncoding.EncodeToString(receipt)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return 0, nil, unavailable("the request cannot be built")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "text/plain")
	resp, err := c.cfg.HTTP.Do(req)
	if err != nil {
		return 0, nil, plxerr.Wrap(plxerr.FraudReceiptUnavailable, errors.Unwrap(err), "fraud receipt: Apple cannot be reached")
	}
	defer func() { _ = resp.Body.Close() }()
	limit := maxBytes(c.cfg.MaxBytes)
	// The base64 form is a third longer than the receipt it carries.
	out, err := io.ReadAll(io.LimitReader(resp.Body, limit*2+1))
	if err != nil {
		return 0, nil, unavailable("the response cannot be read")
	}
	if int64(len(out)) > limit*2 {
		return 0, nil, unavailable("the response is too large")
	}
	return resp.StatusCode, out, nil
}

// decodeBody returns the receipt bytes of a 200 response. Apple documents
// the body as base64, so that is tried first; a body that is not base64,
// or does not decode to a DER SEQUENCE, is taken as the raw receipt.
func decodeBody(body []byte) []byte {
	trimmed := bytes.TrimSpace(body)
	if raw, err := base64.StdEncoding.DecodeString(string(trimmed)); err == nil && len(raw) > 0 && raw[0] == 0x30 {
		return raw
	}
	return body
}

// token builds and signs the request JWT: ES256, kid the key ID, iss the
// Team ID, iat now and a short exp. The private key stays in the signer.
func (c *Client) token(ctx context.Context) (string, error) {
	now := c.cfg.Now()
	header, err := json.Marshal(map[string]string{"alg": "ES256", "kid": c.cfg.KeyID, "typ": "JWT"})
	if err != nil {
		return "", unavailable("the request token cannot be built")
	}
	claims, err := json.Marshal(map[string]any{"iss": c.cfg.TeamID, "iat": now.Unix(), "exp": now.Add(tokenLifetime).Unix()})
	if err != nil {
		return "", unavailable("the request token cannot be built")
	}
	input := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(input))
	sig, err := c.cfg.Signer.SignES256(ctx, Role, digest[:])
	if err != nil {
		return "", plxerr.Wrap(plxerr.FraudReceiptUnavailable, err, "fraud receipt: the request token cannot be signed")
	}
	if len(sig) != signatureLen {
		return "", unavailable("the signer returned a signature of the wrong length")
	}
	return input + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

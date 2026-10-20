// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package push

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

// apnsTokenLifetime is how long a provider token is reused. Apple refuses
// one older than an hour and one renewed more often than every twenty
// minutes, so a token is kept for 50.
const apnsTokenLifetime = 50 * time.Minute

// APNsCredentials are the operator's APNs token-based authentication
// settings, the secret PUSH_APNS: the key's identifier and the team's, the
// app's bundle identifier as the topic, the .p8 key and whether the
// sandbox is used.
type APNsCredentials struct {
	KeyID   string `json:"keyId"`
	TeamID  string `json:"teamId"`
	Topic   string `json:"topic"`
	Key     string `json:"key"`
	Sandbox bool   `json:"sandbox"`
}

// apnsSender sends background notifications over HTTP/2 with a provider
// token signed ES256 (token-based authentication).
type apnsSender struct {
	creds    APNsCredentials
	key      *ecdsa.PrivateKey
	client   *http.Client
	endpoint string
	now      func() time.Time

	mu      sync.Mutex
	token   string
	created time.Time
}

// newAPNsSender checks the credentials and returns a sender for them. The
// sandbox flag chooses between the two endpoints unless an endpoint is
// given.
func newAPNsSender(raw []byte, client *http.Client, production, sandbox string, now func() time.Time) (*apnsSender, error) {
	var c APNsCredentials
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, errors.New("push: the APNs credentials are not JSON")
	}
	if c.KeyID == "" || c.TeamID == "" || c.Topic == "" || c.Key == "" {
		return nil, errors.New("push: the APNs credentials lack keyId, teamId, topic or key")
	}
	block, _ := pem.Decode([]byte(c.Key))
	if block == nil {
		return nil, errors.New("push: the APNs key is not PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("push: the APNs key is not PKCS #8")
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok || key.Curve != elliptic.P256() {
		return nil, errors.New("push: the APNs key is not an ECDSA P-256 key")
	}
	endpoint := production
	if c.Sandbox {
		endpoint = sandbox
	}
	return &apnsSender{creds: c, key: key, client: client, endpoint: strings.TrimSuffix(endpoint, "/"), now: now}, nil
}

// apnsPayload is the notification body: a background notification with the
// marker and the app, and no alert, sound or badge.
type apnsPayload struct {
	APS  map[string]int `json:"aps"`
	Plux string         `json:"plux"`
	App  string         `json:"app"`
}

// Send delivers one message.
func (a *apnsSender) Send(ctx context.Context, token string, m Message) error {
	body, err := json.Marshal(apnsPayload{APS: map[string]int{"content-available": 1}, Plux: Marker, App: m.AppID})
	if err != nil {
		return fmt.Errorf("push: %w", err)
	}
	bearer, err := a.providerToken()
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpoint+"/3/device/"+url.PathEscape(token), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("push: %w", err)
	}
	req.Header.Set("Authorization", "bearer "+bearer)
	req.Header.Set("apns-topic", a.creds.Topic)
	req.Header.Set("apns-push-type", "background")
	req.Header.Set("apns-priority", "5")
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: APNs: %w", ErrTransient, err)
	}
	defer func() { _ = resp.Body.Close() }()
	return a.interpret(resp)
}

// interpret classifies APNs's answer.
func (a *apnsSender) interpret(resp *http.Response) error {
	if resp.StatusCode == http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponse))
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
	var out struct {
		Reason string `json:"reason"`
	}
	_ = json.Unmarshal(body, &out)
	switch {
	case resp.StatusCode == http.StatusGone, out.Reason == "BadDeviceToken", out.Reason == "Unregistered", out.Reason == "DeviceTokenNotForTopic":
		return ErrTokenInvalid
	case out.Reason == "ExpiredProviderToken":
		a.forget()
		return fmt.Errorf("%w: APNs answered ExpiredProviderToken; the provider token was dropped", ErrTransient)
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return statusError("APNs", resp)
}

// forget drops the cached provider token.
func (a *apnsSender) forget() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.token, a.created = "", time.Time{}
}

// providerToken returns the signed provider token, renewing it when it is
// older than apnsTokenLifetime.
func (a *apnsSender) providerToken() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	if a.token != "" && now.Sub(a.created) < apnsTokenLifetime {
		return a.token, nil
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.ES256, Key: a.key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", a.creds.KeyID))
	if err != nil {
		return "", fmt.Errorf("push: %w", err)
	}
	raw, err := jwt.Signed(signer).Claims(jwt.Claims{Issuer: a.creds.TeamID, IssuedAt: jwt.NewNumericDate(now)}).Serialize()
	if err != nil {
		return "", fmt.Errorf("push: %w", err)
	}
	a.token, a.created = raw, now
	return raw, nil
}

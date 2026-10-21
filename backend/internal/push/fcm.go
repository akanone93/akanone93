// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package push

import (
	"bytes"
	"context"
	"crypto/rsa"
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

// fcmScope is the OAuth scope that lets a service account send messages.
const fcmScope = "https://www.googleapis.com/auth/firebase.messaging"

// fcmAssertionLifetime is how long the JWT assertion that is exchanged
// for an access token is valid; Google accepts at most an hour.
const fcmAssertionLifetime = time.Hour

// fcmTokenMargin is how long before expiry a cached access token is
// replaced.
const fcmTokenMargin = time.Minute

// ServiceAccount is the part of a Google service account key file the
// sender uses. The key file is the operator's secret PUSH_FCM_SERVICE_ACCOUNT.
type ServiceAccount struct {
	ProjectID   string `json:"project_id"`
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
}

// fcmSender sends data-only messages through the FCM HTTP v1 API. It
// exchanges a JWT signed with the service account's key for an OAuth
// access token, caches it until shortly before expiry, and uses it as a
// bearer.
type fcmSender struct {
	account  ServiceAccount
	key      *rsa.PrivateKey
	client   *http.Client
	sendBase string
	tokenURL string
	now      func() time.Time

	mu      sync.Mutex
	access  string
	expires time.Time
}

// newFCMSender checks a service account and returns a sender for it.
func newFCMSender(raw []byte, client *http.Client, sendBase, tokenURL string, now func() time.Time) (*fcmSender, error) {
	var acct ServiceAccount
	if err := json.Unmarshal(raw, &acct); err != nil {
		return nil, errors.New("push: the FCM service account is not JSON")
	}
	if acct.ProjectID == "" || acct.ClientEmail == "" || acct.PrivateKey == "" {
		return nil, errors.New("push: the FCM service account lacks project_id, client_email or private_key")
	}
	block, _ := pem.Decode([]byte(acct.PrivateKey))
	if block == nil {
		return nil, errors.New("push: the FCM service account's private key is not PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("push: the FCM service account's private key is not PKCS #8")
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("push: the FCM service account's private key is not RSA")
	}
	return &fcmSender{account: acct, key: key, client: client, sendBase: strings.TrimSuffix(sendBase, "/"), tokenURL: tokenURL, now: now}, nil
}

// fcmMessage is the body of a send request. It has a data member and no
// notification member, so the message shows nothing and carries no content;
// the apns member makes the same message a silent background push for an
// iOS device that registered through FCM.
type fcmMessage struct {
	Message struct {
		Token   string            `json:"token"`
		Data    map[string]string `json:"data"`
		Android struct {
			Priority string `json:"priority"`
		} `json:"android"`
		APNS struct {
			Headers map[string]string `json:"headers"`
			Payload struct {
				APS struct {
					ContentAvailable int `json:"content-available"`
				} `json:"aps"`
			} `json:"payload"`
		} `json:"apns"`
	} `json:"message"`
}

// body renders the message for one token.
func fcmBody(token string, m Message) ([]byte, error) {
	var out fcmMessage
	out.Message.Token = token
	out.Message.Data = map[string]string{"plux": Marker, "app": m.AppID}
	out.Message.Android.Priority = "high"
	out.Message.APNS.Headers = map[string]string{"apns-push-type": "background", "apns-priority": "5"}
	out.Message.APNS.Payload.APS.ContentAvailable = 1
	b, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("push: %w", err)
	}
	return b, nil
}

// Send delivers one message.
func (f *fcmSender) Send(ctx context.Context, token string, m Message) error {
	body, err := fcmBody(token, m)
	if err != nil {
		return err
	}
	access, err := f.accessToken(ctx)
	if err != nil {
		return err
	}
	endpoint := f.sendBase + "/v1/projects/" + url.PathEscape(f.account.ProjectID) + "/messages:send"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("push: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+access)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := f.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: FCM: %w", ErrTransient, err)
	}
	defer func() { _ = resp.Body.Close() }()
	return f.interpret(resp)
}

// interpret classifies FCM's answer to a send.
func (f *fcmSender) interpret(resp *http.Response) error {
	if resp.StatusCode == http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponse))
		return nil
	}
	if resp.StatusCode == http.StatusUnauthorized {
		f.forget()
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
	if resp.StatusCode == http.StatusNotFound || bytes.Contains(body, []byte("UNREGISTERED")) {
		return ErrTokenInvalid
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("%w: FCM answered 401; the access token was dropped", ErrTransient)
	}
	return statusError("FCM", resp)
}

// forget drops the cached access token.
func (f *fcmSender) forget() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.access, f.expires = "", time.Time{}
}

// accessToken returns a valid OAuth access token, exchanging a signed
// assertion for a new one when the cached one is about to expire.
func (f *fcmSender) accessToken(ctx context.Context) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := f.now()
	if f.access != "" && now.Add(fcmTokenMargin).Before(f.expires) {
		return f.access, nil
	}
	assertion, err := f.assertion(now)
	if err != nil {
		return "", err
	}
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {assertion}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("push: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := f.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: Google OAuth: %w", ErrTransient, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", statusError("Google OAuth", resp)
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponse)).Decode(&out); err != nil || out.AccessToken == "" || out.ExpiresIn <= 0 {
		return "", fmt.Errorf("%w: Google OAuth answered without an access token", ErrRejected)
	}
	f.access, f.expires = out.AccessToken, now.Add(time.Duration(out.ExpiresIn)*time.Second)
	return f.access, nil
}

// assertion signs the JWT that is exchanged for an access token.
func (f *fcmSender) assertion(now time.Time) (string, error) {
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: f.key}, (&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		return "", fmt.Errorf("push: %w", err)
	}
	claims := jwt.Claims{
		Issuer: f.account.ClientEmail, Audience: jwt.Audience{f.tokenURL},
		IssuedAt: jwt.NewNumericDate(now), Expiry: jwt.NewNumericDate(now.Add(fcmAssertionLifetime)),
	}
	raw, err := compactJWT(signer, struct {
		jwt.Claims
		Scope string `json:"scope"`
	}{Claims: claims, Scope: fcmScope})
	if err != nil {
		return "", fmt.Errorf("push: %w", err)
	}
	return raw, nil
}

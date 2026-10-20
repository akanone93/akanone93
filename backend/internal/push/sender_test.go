// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package push

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

// clock is a settable time source.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newClock() *clock { return &clock{t: time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)} }

// pkcs8PEM encodes a private key as PEM at test time, so that no key is
// ever committed.
func pkcs8PEM(t *testing.T, key any) string {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

// fcmFake is a stand-in for Google's token endpoint and FCM.
type fcmFake struct {
	t       *testing.T
	public  *rsa.PublicKey
	tokenAt string
	mu      sync.Mutex
	exchang int
	bodies  []map[string]any
	status  int
	reply   string
	srv     *httptest.Server
}

func newFCMFake(t *testing.T, public *rsa.PublicKey) *fcmFake {
	t.Helper()
	f := &fcmFake{t: t, public: public, status: http.StatusOK}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	f.tokenAt = f.srv.URL + "/token"
	return f
}

func (f *fcmFake) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.URL.Path == "/token" {
		f.exchang++
		if err := r.ParseForm(); err != nil {
			f.t.Errorf("token form: %v", err)
		}
		f.checkAssertion(r.PostForm.Get("assertion"))
		_, _ = io.WriteString(w, `{"access_token":"access-`+strconv.Itoa(f.exchang)+`","expires_in":3600}`)
		return
	}
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer access-") {
		f.t.Errorf("send without the access token: %q", r.Header.Get("Authorization"))
	}
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		f.t.Errorf("send body: %v", err)
	}
	f.bodies = append(f.bodies, body)
	w.WriteHeader(f.status)
	_, _ = io.WriteString(w, f.reply)
}

// checkAssertion verifies the JWT bearer assertion with the account's
// public key and checks its claims.
func (f *fcmFake) checkAssertion(raw string) {
	f.t.Helper()
	parsed, err := jwt.ParseSigned(raw, []jose.SignatureAlgorithm{jose.RS256})
	if err != nil {
		f.t.Errorf("assertion: %v", err)
		return
	}
	var claims jwt.Claims
	var extra struct {
		Scope string `json:"scope"`
	}
	if err := parsed.Claims(f.public, &claims, &extra); err != nil {
		f.t.Errorf("assertion signature: %v", err)
		return
	}
	if claims.Issuer != "push@example.iam" || !claims.Audience.Contains(f.tokenAt) || extra.Scope != fcmScope {
		f.t.Errorf("assertion claims = %+v %+v", claims, extra)
	}
	if got := claims.Expiry.Time().Sub(claims.IssuedAt.Time()); got != fcmAssertionLifetime {
		f.t.Errorf("assertion lifetime = %s", got)
	}
}

func fcmAccount(t *testing.T) ([]byte, *rsa.PublicKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(ServiceAccount{ProjectID: "demo", ClientEmail: "push@example.iam", PrivateKey: pkcs8PEM(t, key)})
	if err != nil {
		t.Fatal(err)
	}
	return raw, &key.PublicKey
}

// Verifies: SYN-060.
// The FCM sender exchanges a JWT signed with the service account's key for
// an access token, reuses it until shortly before it expires, and sends a
// data message whose only content is the marker and the app.
func TestFCMSenderSendsOnlyTheMarker_SYN_060(t *testing.T) {
	t.Parallel()
	raw, public := fcmAccount(t)
	fake := newFCMFake(t, public)
	clk := newClock()
	sender, err := newFCMSender(raw, fake.srv.Client(), fake.srv.URL, fake.tokenAt, clk.now)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	for range 2 {
		if err := sender.Send(ctx, "tok", Message{AppID: "app-1"}); err != nil {
			t.Fatalf("Send: %v", err)
		}
	}
	if fake.exchang != 1 {
		t.Errorf("%d token exchanges for two sends, want the access token reused", fake.exchang)
	}
	msg, _ := fake.bodies[0]["message"].(map[string]any)
	data, _ := msg["data"].(map[string]any)
	if _, shows := msg["notification"]; shows || len(data) != 2 || data["plux"] != Marker || data["app"] != "app-1" || msg["token"] != "tok" {
		t.Errorf("message = %v", msg)
	}
	clk.advance(2 * time.Hour)
	if err := sender.Send(ctx, "tok", Message{AppID: "app-1"}); err != nil || fake.exchang != 2 {
		t.Errorf("after the access token expired: %v, %d exchanges", err, fake.exchang)
	}
}

// Verifies: SYN-060.
// FCM's answers are classified: an unregistered token is dead, a throttled
// or failing service is transient, a refusal is not retried, and a 401
// drops the cached access token.
func TestFCMSenderClassifiesAnswers_SYN_060(t *testing.T) {
	t.Parallel()
	raw, public := fcmAccount(t)
	fake := newFCMFake(t, public)
	sender, err := newFCMSender(raw, fake.srv.Client(), fake.srv.URL, fake.tokenAt, newClock().now)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		status int
		body   string
		want   error
	}{
		{http.StatusNotFound, `{"error":{"status":"NOT_FOUND"}}`, ErrTokenInvalid},
		{http.StatusBadRequest, `{"error":{"details":[{"errorCode":"UNREGISTERED"}]}}`, ErrTokenInvalid},
		{http.StatusTooManyRequests, ``, ErrTransient},
		{http.StatusServiceUnavailable, ``, ErrTransient},
		{http.StatusForbidden, `{"error":"sender id mismatch"}`, ErrRejected},
		{http.StatusUnauthorized, ``, ErrTransient},
	} {
		fake.status, fake.reply = c.status, c.body
		if err := sender.Send(t.Context(), "tok", Message{AppID: "a"}); !errors.Is(err, c.want) {
			t.Errorf("%d: %v, want %v", c.status, err, c.want)
		}
	}
	fake.status, fake.reply = http.StatusOK, ""
	if err := sender.Send(t.Context(), "tok", Message{AppID: "a"}); err != nil || fake.exchang != 2 {
		t.Errorf("after the 401: %v, %d exchanges, want a second one because the access token was dropped", err, fake.exchang)
	}
}

// Verifies: SYN-060, SEC-106.
// A credential that is not a usable service account or APNs key is refused
// when the sender is built, and the error never repeats the credential.
func TestSendersRefuseBadCredentials_SYN_060(t *testing.T) {
	t.Parallel()
	ecKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := newClock().now
	secret := "super-secret-value"
	for name, raw := range map[string]string{
		"not json":      secret,
		"no project":    `{"client_email":"a","private_key":"` + secret + `"}`,
		"not pem":       `{"project_id":"p","client_email":"a","private_key":"` + secret + `"}`,
		"not rsa (EC)":  mustJSON(t, ServiceAccount{ProjectID: "p", ClientEmail: "a", PrivateKey: pkcs8PEM(t, ecKey)}),
		"empty account": `{}`,
	} {
		if _, err := newFCMSender([]byte(raw), http.DefaultClient, "https://x", "https://y", now); err == nil || strings.Contains(err.Error(), secret) {
			t.Errorf("fcm %s: %v", name, err)
		}
	}
	for name, raw := range map[string]string{
		"not json":          secret,
		"no team":           `{"keyId":"k","topic":"t","key":"` + secret + `"}`,
		"not pem":           `{"keyId":"k","teamId":"t","topic":"t","key":"` + secret + `"}`,
		"wrong P-384 curve": mustJSON(t, APNsCredentials{KeyID: "k", TeamID: "t", Topic: "t", Key: pkcs8PEM(t, ecKey)}),
	} {
		if _, err := newAPNsSender([]byte(raw), http.DefaultClient, "https://x", "https://y", now); err == nil || strings.Contains(err.Error(), secret) {
			t.Errorf("apns %s: %v", name, err)
		}
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// apnsFake is a stand-in for APNs.
type apnsFake struct {
	t      *testing.T
	public *ecdsa.PublicKey
	mu     sync.Mutex
	tokens []string
	bodies []map[string]any
	status int
	reply  string
	srv    *httptest.Server
}

func newAPNsFake(t *testing.T, public *ecdsa.PublicKey) *apnsFake {
	t.Helper()
	f := &apnsFake{t: t, public: public, status: http.StatusOK}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *apnsFake) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "bearer ")
	f.tokens = append(f.tokens, bearer)
	f.checkHeaders(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		f.t.Errorf("APNs body: %v", err)
	}
	f.bodies = append(f.bodies, body)
	w.WriteHeader(f.status)
	_, _ = io.WriteString(w, f.reply)
}

func (f *apnsFake) checkHeaders(r *http.Request) {
	f.t.Helper()
	if r.URL.Path != "/3/device/device-token" || r.Header.Get("apns-topic") != "com.example.app" ||
		r.Header.Get("apns-push-type") != "background" || r.Header.Get("apns-priority") != "5" {
		f.t.Errorf("request = %s %v", r.URL.Path, r.Header)
	}
}

// providerClaims verifies a provider token with the key's public half.
func (f *apnsFake) providerClaims(raw string) (jwt.Claims, string) {
	f.t.Helper()
	parsed, err := jwt.ParseSigned(raw, []jose.SignatureAlgorithm{jose.ES256})
	if err != nil {
		f.t.Fatalf("provider token: %v", err)
	}
	var claims jwt.Claims
	if err := parsed.Claims(f.public, &claims); err != nil {
		f.t.Fatalf("provider token signature: %v", err)
	}
	return claims, parsed.Headers[0].KeyID
}

func apnsSenderFor(t *testing.T, clk *clock, sandbox bool) (*apnsSender, *apnsFake) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	fake := newAPNsFake(t, &key.PublicKey)
	raw := mustJSON(t, APNsCredentials{KeyID: "KEY123", TeamID: "TEAM456", Topic: "com.example.app", Key: pkcs8PEM(t, key), Sandbox: sandbox})
	production, sandboxURL := "https://not-used.invalid", fake.srv.URL
	if !sandbox {
		production, sandboxURL = fake.srv.URL, "https://not-used.invalid"
	}
	sender, err := newAPNsSender([]byte(raw), fake.srv.Client(), production, sandboxURL, clk.now)
	if err != nil {
		t.Fatal(err)
	}
	return sender, fake
}

// Verifies: SYN-060.
// The APNs sender signs an ES256 provider token with the team and key
// identifiers, reuses it for 50 minutes, renews it afterwards, picks the
// sandbox endpoint from its credentials, and sends a background
// notification with no alert, sound or badge.
func TestAPNsSenderSendsOnlyTheMarker_SYN_060(t *testing.T) {
	t.Parallel()
	for _, sandbox := range []bool{false, true} {
		clk := newClock()
		sender, fake := apnsSenderFor(t, clk, sandbox)
		for range 2 {
			if err := sender.Send(t.Context(), "device-token", Message{AppID: "app-1"}); err != nil {
				t.Fatalf("sandbox=%t Send: %v", sandbox, err)
			}
		}
		if fake.tokens[0] != fake.tokens[1] {
			t.Errorf("sandbox=%t: the provider token was not reused", sandbox)
		}
		claims, kid := fake.providerClaims(fake.tokens[0])
		if claims.Issuer != "TEAM456" || kid != "KEY123" || !claims.IssuedAt.Time().Equal(clk.now()) {
			t.Errorf("sandbox=%t: claims = %+v kid = %q", sandbox, claims, kid)
		}
		body := fake.bodies[0]
		aps, _ := body["aps"].(map[string]any)
		if len(body) != 3 || body["plux"] != Marker || body["app"] != "app-1" || len(aps) != 1 || aps["content-available"] != float64(1) {
			t.Errorf("sandbox=%t: payload = %v", sandbox, body)
		}
		clk.advance(apnsTokenLifetime + time.Second)
		if err := sender.Send(t.Context(), "device-token", Message{AppID: "app-1"}); err != nil || fake.tokens[2] == fake.tokens[0] {
			t.Errorf("sandbox=%t: the provider token was not renewed: %v", sandbox, err)
		}
	}
}

// Verifies: SYN-060.
// APNs's answers are classified: a gone or unknown device token is dead, an
// expired provider token is dropped and retried later, throttling is
// transient and any other refusal is final.
func TestAPNsSenderClassifiesAnswers_SYN_060(t *testing.T) {
	t.Parallel()
	sender, fake := apnsSenderFor(t, newClock(), false)
	for _, c := range []struct {
		status int
		body   string
		want   error
	}{
		{http.StatusGone, `{"reason":"Unregistered"}`, ErrTokenInvalid},
		{http.StatusBadRequest, `{"reason":"BadDeviceToken"}`, ErrTokenInvalid},
		{http.StatusBadRequest, `{"reason":"DeviceTokenNotForTopic"}`, ErrTokenInvalid},
		{http.StatusTooManyRequests, `{"reason":"TooManyRequests"}`, ErrTransient},
		{http.StatusServiceUnavailable, `{"reason":"ServiceUnavailable"}`, ErrTransient},
		{http.StatusForbidden, `{"reason":"InvalidProviderToken"}`, ErrRejected},
		{http.StatusForbidden, `{"reason":"ExpiredProviderToken"}`, ErrTransient},
	} {
		fake.status, fake.reply = c.status, c.body
		if err := sender.Send(t.Context(), "device-token", Message{AppID: "a"}); !errors.Is(err, c.want) {
			t.Errorf("%d %s: %v, want %v", c.status, c.body, err, c.want)
		}
	}
	if sender.token != "" {
		t.Errorf("the provider token survived ExpiredProviderToken")
	}
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package webauthndriver

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// The names the server uses for a browser session (SEC-101). They are the
// values of api.SessionCookie and api.CSRFHeader, which a separate module
// cannot import.
const (
	sessionCookie = "__Host-plux_session"
	csrfHeader    = "X-CSRF-Token"
)

// maxReply bounds what the driver reads of one answer.
const maxReply = 4 << 20

// session is a browser session as the driver holds it: the secret of the
// cookie and the CSRF token that state-changing calls must echo.
type session struct {
	cookie string
	csrf   string
}

// connectClient makes unary ConnectRPC calls with JSON messages, as the
// Studio browser does: with the session cookie and its CSRF token.
type connectClient struct {
	baseURL string
	http    *http.Client
}

// call posts req to the procedure, such as "/plux.v1.IdentityService/Logout",
// as the session (nil for an unauthenticated call) and decodes the answer into
// resp. It returns the session cookie the answer set, if any.
func (c connectClient) call(ctx context.Context, s *session, procedure string, req, resp any) (string, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("webauthndriver.call %s: %w", procedure, err)
	}
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(c.baseURL, "/")+procedure, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("webauthndriver.call %s: %w", procedure, err)
	}
	hr.Header.Set("Content-Type", "application/json")
	hr.Header.Set("Connect-Protocol-Version", "1")
	if s != nil {
		hr.AddCookie(&http.Cookie{Name: sessionCookie, Value: s.cookie, Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
		hr.Header.Set(csrfHeader, s.csrf)
	}
	res, err := c.http.Do(hr)
	if err != nil {
		return "", fmt.Errorf("webauthndriver.call %s: %w", procedure, err)
	}
	defer func() { _ = res.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(res.Body, maxReply))
	if err != nil {
		return "", fmt.Errorf("webauthndriver.call %s: %w", procedure, err)
	}
	if res.StatusCode != http.StatusOK {
		return "", connectError(procedure, res.StatusCode, data)
	}
	if err := json.Unmarshal(data, resp); err != nil {
		return "", fmt.Errorf("webauthndriver.call %s: %w", procedure, err)
	}
	return setCookie(res), nil
}

// setCookie returns the value of the session cookie a response sets.
func setCookie(res *http.Response) string {
	for _, c := range res.Cookies() {
		if c.Name == sessionCookie {
			return c.Value
		}
	}
	return ""
}

// connectError renders a failed call from the Connect error body, which has a
// code and a message. The message is the server's, which never carries a
// secret.
func connectError(procedure string, status int, data []byte) error {
	var e struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if json.Unmarshal(data, &e) != nil || e.Code == "" {
		return fmt.Errorf("webauthndriver.call %s: HTTP %d", procedure, status)
	}
	return fmt.Errorf("webauthndriver.call %s: %s: %s", procedure, e.Code, e.Message)
}

// decodeBytes reads a protobuf JSON bytes field, which a server writes in
// standard base64 and a reader accepts in any of the four base64 forms.
func decodeBytes(s string) ([]byte, error) {
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(s); err == nil {
			return b, nil
		}
	}
	return nil, fmt.Errorf("webauthndriver.decodeBytes: %q is not base64", s)
}

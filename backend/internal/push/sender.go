// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package push

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// Marker is the value of the "plux" key every message carries; the host
// hands the payload to Plux.handleSilentPush, which recognises it.
const Marker = "control"

// Provider names the service that delivers to a token.
type Provider string

// The providers (Q22).
const (
	FCM  Provider = "fcm"
	APNs Provider = "apns"
)

// Message is what a silent push says: that this app's control document
// has changed. It has no other content by design.
type Message struct {
	// AppID is the app's identifier.
	AppID string
}

// Sender delivers a message to one device token.
type Sender interface {
	Send(ctx context.Context, token string, m Message) error
}

var (
	// ErrTokenInvalid means the provider no longer knows the token; the
	// device uninstalled the app or the token was replaced. The token is
	// deleted.
	ErrTokenInvalid = errors.New("push: the token is no longer valid")
	// ErrTransient means a failure worth retrying: the provider or the
	// network is unavailable or throttling.
	ErrTransient = errors.New("push: a temporary failure")
	// ErrRejected means the provider refused the message for a reason a
	// retry will not change, such as a revoked credential.
	ErrRejected = errors.New("push: the provider rejected the message")
)

// maxResponse bounds how much of a provider's answer is read.
const maxResponse = 64 << 10

// statusError classifies an unsuccessful answer. The body is read only to
// name the provider's reason in the error; it never holds a secret.
func statusError(provider string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
	switch {
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError:
		return fmt.Errorf("%w: %s answered %d", ErrTransient, provider, resp.StatusCode)
	}
	return fmt.Errorf("%w: %s answered %d: %s", ErrRejected, provider, resp.StatusCode, reason(body))
}

// reason shortens a provider's error body for an error message.
func reason(body []byte) string {
	const most = 200
	if len(body) > most {
		body = body[:most]
	}
	return string(body)
}

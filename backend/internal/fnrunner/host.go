// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package fnrunner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/egress"
	"github.com/nightCode42/plux3/backend/internal/fnhost"
	"github.com/nightCode42/plux3/backend/internal/fnpb"
	"github.com/nightCode42/plux3/backend/internal/fnpb/fnpbconnect"
)

// Egress opens the HTTP transport through which one function reaches the
// internet. *egress.Dialer satisfies it.
type Egress interface {
	// Transport returns a transport whose every connection is a tunnel for
	// the function, limited to the allowlist (FN-017).
	Transport(allow []string, function string) *http.Transport
}

var _ Egress = (*egress.Dialer)(nil)

// fallbackHTTPTimeout bounds an outbound request whose context carries no
// deadline. The engine always gives a call a deadline, so it is only a
// backstop against a caller bug.
const fallbackHTTPTimeout = 30 * time.Second

// remoteHost is the fnhost.Host of one invocation. Key-value, secrets and
// nested invocations go to the api role through FunctionHostService, each
// call presenting the invocation's grant; HTTP goes through the egress role
// (FN-012, FN-017, ADR-0064). It is used by one invocation and is safe for
// concurrent use.
type remoteHost struct {
	client      fnpbconnect.FunctionHostServiceClient
	egress      Egress
	grant       string
	traceparent string
	allow       []string
	label       string
	maxBody     int64

	transport *http.Transport
}

var _ fnhost.Host = (*remoteHost)(nil)

// header sets the trace context of the invocation on a host call.
func (h *remoteHost) header(hdr http.Header) {
	if h.traceparent != "" {
		hdr.Set("traceparent", h.traceparent)
	}
}

// KVGet implements fnhost.Host.
func (h *remoteHost) KVGet(ctx context.Context, key string) ([]byte, error) {
	req := connect.NewRequest(&fnpb.KvGetRequest{Grant: h.grant, Key: key})
	h.header(req.Header())
	resp, err := h.client.KvGet(ctx, req)
	if err != nil {
		return nil, mapRemote(err)
	}
	if !resp.Msg.GetFound() {
		return nil, fnhost.ErrNotFound
	}
	return resp.Msg.GetValue(), nil
}

// KVPut implements fnhost.Host.
func (h *remoteHost) KVPut(ctx context.Context, key string, value []byte) error {
	req := connect.NewRequest(&fnpb.KvPutRequest{Grant: h.grant, Key: key, Value: value})
	h.header(req.Header())
	if _, err := h.client.KvPut(ctx, req); err != nil {
		return mapRemote(err)
	}
	return nil
}

// KVDelete implements fnhost.Host.
func (h *remoteHost) KVDelete(ctx context.Context, key string) error {
	req := connect.NewRequest(&fnpb.KvDeleteRequest{Grant: h.grant, Key: key})
	h.header(req.Header())
	resp, err := h.client.KvDelete(ctx, req)
	if err != nil {
		return mapRemote(err)
	}
	if !resp.Msg.GetDeleted() {
		return fnhost.ErrNotFound
	}
	return nil
}

// Secret implements fnhost.Host. The value is returned to the engine and
// kept nowhere else (SEC-092).
func (h *remoteHost) Secret(ctx context.Context, name string) ([]byte, error) {
	req := connect.NewRequest(&fnpb.GetSecretRequest{Grant: h.grant, Name: name})
	h.header(req.Header())
	resp, err := h.client.GetSecret(ctx, req)
	if err != nil {
		return nil, mapRemote(err)
	}
	if !resp.Msg.GetFound() {
		return nil, fnhost.ErrNotFound
	}
	return resp.Msg.GetValue(), nil
}

// Invoke implements fnhost.Host. depth is the depth at which the nested
// function runs, the caller's plus one (FN-011).
func (h *remoteHost) Invoke(ctx context.Context, name string, input []byte, depth int) (fnhost.Outcome, error) {
	req := connect.NewRequest(&fnpb.FunctionHostServiceInvokeRequest{
		Grant: h.grant, Function: name, Input: input,
		Depth: uint32(depth), Traceparent: h.traceparent, //nolint:gosec // G115: depth is non-negative and bounded by the call-depth limit
	})
	h.header(req.Header())
	resp, err := h.client.Invoke(ctx, req)
	if err != nil {
		return fnhost.Outcome{}, mapRemote(err)
	}
	result := resp.Msg.GetResult()
	if raw := result.GetBusinessError(); raw != nil {
		var be fnhost.BusinessError
		if err := json.Unmarshal(raw, &be); err != nil {
			return fnhost.Outcome{}, fmt.Errorf("%w: malformed business error", fnhost.ErrUpstream)
		}
		return fnhost.Outcome{Business: &be}, nil
	}
	return fnhost.Outcome{Output: json.RawMessage(result.GetOutput())}, nil
}

// HTTP implements fnhost.Host: one request through the egress role. A
// redirect is not followed; the function sees the 3xx answer as it is. The
// request lives no longer than the invocation, and a body above the
// configured bound is a limit failure rather than a truncated success.
func (h *remoteHost) HTTP(ctx context.Context, r fnhost.HTTPRequest) (fnhost.HTTPResponse, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, fallbackHTTPTimeout)
		defer cancel()
	}
	var body io.Reader
	if len(r.Body) > 0 {
		body = bytes.NewReader(r.Body)
	}
	req, err := http.NewRequestWithContext(ctx, r.Method, r.URL, body)
	if err != nil {
		return fnhost.HTTPResponse{}, fmt.Errorf("%w: %w", fnhost.ErrUpstream, err)
	}
	for k, v := range r.Headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{
		Transport:     h.httpTransport(),
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		return fnhost.HTTPResponse{}, mapEgress(err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, h.maxBody+1))
	if err != nil {
		return fnhost.HTTPResponse{}, mapEgress(err)
	}
	if int64(len(data)) > h.maxBody {
		return fnhost.HTTPResponse{}, fnhost.ErrLimit
	}
	return fnhost.HTTPResponse{Status: resp.StatusCode, Headers: flatten(resp.Header), Body: data}, nil
}

// httpTransport builds the invocation's transport on first use.
func (h *remoteHost) httpTransport() *http.Transport {
	if h.transport == nil {
		h.transport = h.egress.Transport(h.allow, h.label)
	}
	return h.transport
}

// close releases the connections of the invocation's transport.
func (h *remoteHost) close() {
	if h.transport != nil {
		h.transport.CloseIdleConnections()
	}
}

// flatten renders response headers in the single-value form of the ABI;
// repeated headers are joined with a comma.
func flatten(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, v := range h {
		out[k] = strings.Join(v, ", ")
	}
	return out
}

// mapRemote maps an error from FunctionHostService to the sentinel the
// engine turns into a status.
func mapRemote(err error) error {
	switch connect.CodeOf(err) {
	case connect.CodeNotFound:
		return fnhost.ErrNotFound
	case connect.CodeResourceExhausted:
		return fnhost.ErrLimit
	case connect.CodeUnavailable:
		return fnhost.ErrUnavailable
	}
	return fnhost.ErrUpstream
}

// mapEgress maps an error of the outbound request: what the egress role
// refused or could not reach, and the transport's own failures.
func mapEgress(err error) error {
	switch {
	case errors.Is(err, egress.ErrRefused):
		return fnhost.ErrEgressRefused
	case errors.Is(err, egress.ErrLimited):
		return fnhost.ErrUnavailable
	}
	return fnhost.ErrUpstream
}

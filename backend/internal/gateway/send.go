// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package gateway

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"strings"
	"sync"
	"time"

	"github.com/nightCode42/plux3/backend/internal/httpx"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Response is an upstream's answer to a call whose body fits in one
// message.
type Response struct {
	Status int
	Header http.Header
	Body   []byte
}

// Do sends a plan and reads the whole answer, up to gateway.responseBytes.
// It follows no redirect: the assertion must not travel to a host the
// upstream's operator did not register.
func (g *Gateway) Do(ctx context.Context, p *Plan) (Response, error) {
	start := time.Now()
	status := 0
	defer func() { g.Finished(p.Upstream, status, time.Since(start)) }()

	ctx, cancel := context.WithTimeout(ctx, p.Timeout)
	defer cancel()
	var body io.Reader
	if p.Body != nil {
		body = bytes.NewReader(p.Body)
	}
	req, err := http.NewRequestWithContext(ctx, p.Method, p.URL.String(), body)
	if err != nil {
		return Response{}, plxerr.New(plxerr.GatewayDestinationRefused, "the call cannot be sent")
	}
	req.Header = p.Header.Clone()
	client := &http.Client{Transport: p.Transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		status = http.StatusBadGateway
		return Response{}, transportError(err, p.Upstream)
	}
	defer resp.Body.Close()
	raw, err := httpx.ReadAll(resp.Body, p.MaxResponse)
	if err != nil {
		status = http.StatusBadGateway
		if errors.Is(err, httpx.ErrTooLarge) {
			return Response{}, plxerr.New(plxerr.GatewayUpstreamFailed, "the response of upstream %s is larger than gateway.responseBytes = %d", p.Upstream, p.MaxResponse)
		}
		return Response{}, transportError(err, p.Upstream)
	}
	status = resp.StatusCode
	return Response{Status: resp.StatusCode, Header: filterResponse(resp.Header), Body: raw}, nil
}

// transportError turns a failure to reach an upstream into the refusal a
// device sees. It names the upstream and the kind of failure, never the
// address or an error text that might quote one.
func transportError(err error, upstream string) error {
	switch {
	case Blocked(err):
		return plxerr.New(plxerr.GatewayDestinationRefused, "upstream %s resolves to an address the gateway may not reach", upstream)
	case errors.Is(err, context.DeadlineExceeded):
		return plxerr.New(plxerr.GatewayUpstreamFailed, "upstream %s did not answer in time", upstream)
	case errors.Is(err, context.Canceled):
		return plxerr.New(plxerr.GatewayUpstreamFailed, "the call to upstream %s was cancelled", upstream)
	}
	return plxerr.New(plxerr.GatewayUpstreamFailed, "upstream %s could not be reached", upstream)
}

// filterResponse keeps the response headers a device receives.
func filterResponse(in http.Header) http.Header {
	out := http.Header{}
	for _, name := range forwardResponseHeaders {
		if v := in.Values(name); len(v) > 0 {
			out[http.CanonicalHeaderKey(name)] = append([]string(nil), v...)
		}
	}
	return out
}

// streaming reports whether a response is open-ended: server-sent events,
// or the switch to another protocol. Its length is not bounded by
// gateway.responseBytes; the device's rate and the connection's own end
// bound it.
func streaming(resp *http.Response) bool {
	return resp.StatusCode == http.StatusSwitchingProtocols ||
		strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream")
}

// Proxy sends a plan for a request whose body streams, and streams the
// answer back, without buffering either; it carries WebSocket and
// server-sent events too. The caller has authenticated the device and
// passed Begin and Finish. Errors become the HTTP error written to w.
func (g *Gateway) Proxy(w http.ResponseWriter, r *http.Request, p *Plan, fail func(http.ResponseWriter, error)) {
	start := time.Now()
	status := 0
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	// The timeout bounds the wait for the answer's headers and, for an
	// answer that is not a stream, its body; a stream lives as long as
	// its connection.
	timer := time.AfterFunc(p.Timeout, cancel)
	defer timer.Stop()

	var once sync.Once
	rp := &httputil.ReverseProxy{
		Transport:     p.Transport,
		FlushInterval: -1,
		// The proxy's own log would quote errors of the connection; the
		// gateway records a finished call in its metrics and nothing else.
		ErrorLog: log.New(io.Discard, "", 0),
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL = p.URL
			pr.Out.Host = p.URL.Host
			upgrade := pr.In.Header.Get("Upgrade")
			pr.Out.Header = p.Header.Clone()
			if upgrade != "" {
				pr.Out.Header.Set("Connection", "Upgrade")
				pr.Out.Header.Set("Upgrade", upgrade)
			}
			if p.Body != nil {
				pr.Out.Body = io.NopCloser(bytes.NewReader(p.Body))
				pr.Out.ContentLength = int64(len(p.Body))
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			status = resp.StatusCode
			if streaming(resp) {
				timer.Stop()
			} else {
				timer.Reset(p.Timeout)
				resp.Body = &capped{ReadCloser: resp.Body, left: p.MaxResponse}
			}
			header := filterResponse(resp.Header)
			upgrade := resp.StatusCode == http.StatusSwitchingProtocols
			var keepUpgrade []string
			if upgrade {
				keepUpgrade = []string{resp.Header.Get("Connection"), resp.Header.Get("Upgrade")}
			}
			resp.Header = header
			if upgrade {
				resp.Header.Set("Connection", keepUpgrade[0])
				resp.Header.Set("Upgrade", keepUpgrade[1])
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			once.Do(func() {
				status = http.StatusBadGateway
				fail(w, transportError(err, p.Upstream))
			})
		},
	}
	req := r
	if r.Body != nil && r.Body != http.NoBody && p.Body == nil {
		req = r.Clone(ctx)
		req.Body = http.MaxBytesReader(w, r.Body, p.MaxRequest)
	} else {
		req = r.Clone(ctx)
	}
	rp.ServeHTTP(w, req)
	g.Finished(p.Upstream, status, time.Since(start))
}

// capped fails a read once more than left bytes have been read, so an
// answer over gateway.responseBytes ends in a cut connection rather than
// in unbounded memory or bandwidth.
type capped struct {
	io.ReadCloser
	left int64
}

// Read reads at most the bytes that remain.
func (c *capped) Read(p []byte) (int, error) {
	if c.left <= 0 {
		return 0, httpx.ErrTooLarge
	}
	if int64(len(p)) > c.left {
		p = p[:c.left]
	}
	n, err := c.ReadCloser.Read(p)
	c.left -= int64(n)
	if err == io.EOF {
		return n, io.EOF
	}
	if err != nil {
		return n, err //nolint:wrapcheck // the proxy reports it
	}
	if c.left <= 0 {
		// Exactly at the limit: another byte decides.
		var one [1]byte
		m, probeErr := c.ReadCloser.Read(one[:])
		if m > 0 {
			return n, httpx.ErrTooLarge
		}
		if probeErr != nil && !errors.Is(probeErr, io.EOF) {
			return n, probeErr //nolint:wrapcheck // the proxy reports it
		}
		return n, io.EOF
	}
	return n, nil
}

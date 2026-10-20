// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package push

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"

	"github.com/nightCode42/plux3/backend/internal/httpx"
)

// The production endpoints of the push services.
const (
	fcmSendEndpoint     = "https://fcm.googleapis.com"
	fcmTokenEndpoint    = "https://oauth2.googleapis.com/token"
	apnsEndpoint        = "https://api.push.apple.com"
	apnsSandboxEndpoint = "https://api.sandbox.push.apple.com"
)

// egressDestinations lists the hosts this package talks to: those of the
// push destination in the egress registry (SEC-163, ADR-0082). Nothing
// else is reachable: the client refuses any other host, whatever a
// credential or a response names. An unregistered kind lists none, which
// refuses everything.
func egressDestinations() []string {
	d, err := httpx.Lookup(httpx.KindPush)
	if err != nil {
		return nil
	}
	return slices.Clone(d.Hosts)
}

// errHostNotListed is returned for a request to a host the senders do not
// list.
var errHostNotListed = errors.New("push: the host is not on the egress list")

// hostGuard refuses a request to a host outside its list before it is
// sent, so a malformed credential or a hostile response cannot steer a
// sender elsewhere.
type hostGuard struct {
	next  http.RoundTripper
	hosts []string
}

// RoundTrip sends the request when its host is listed.
func (g hostGuard) RoundTrip(req *http.Request) (*http.Response, error) {
	if !slices.Contains(g.hosts, req.URL.Hostname()) {
		return nil, fmt.Errorf("%w: %s", errHostNotListed, req.URL.Hostname())
	}
	return g.next.RoundTrip(req) //nolint:wrapcheck // a transparent guard
}

// guardedClient returns a client that reaches only the listed hosts and
// follows no redirect: the push services never redirect.
func guardedClient(base *http.Client, extra ...string) *http.Client {
	hosts := egressDestinations()
	for _, raw := range extra {
		if u, err := url.Parse(raw); err == nil && u.Hostname() != "" {
			hosts = append(hosts, u.Hostname())
		}
	}
	next := base.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	return &http.Client{
		Transport:     hostGuard{next: next, hosts: hosts},
		Timeout:       base.Timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

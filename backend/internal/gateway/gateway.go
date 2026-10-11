// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package gateway

import (
	"context"
	"crypto/sha256"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nightCode42/plux3/backend/internal/device"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// rateWindow is the window gateway.requestsPerMinutePerDevice counts in.
const rateWindow = time.Minute

// Upstreams resolves the upstream a device named; *Registry is one.
type Upstreams interface {
	Resolve(ctx context.Context, organizationID, environmentID, name string) (Upstream, error)
}

// Options configures a Gateway.
type Options struct {
	Upstreams  Upstreams
	Egress     *Egress
	Assertions *Assertions
	// Users verifies the user tokens devices present; nil refuses them.
	Users *UserTokens
	// SCA verifies transaction signatures; nil refuses transactions.
	SCA *SCA
	// Limits returns the limits of the device's app (LIM-001): the
	// gateway's sizes, timeout, rate and challenge lifetime.
	Limits func(ctx context.Context, d device.Identity) (limits.Set, error)
	// Count increments a counter in the shared cache and returns its new
	// value; nil leaves the rate unlimited.
	Count func(ctx context.Context, key string, window time.Duration) (int64, error)
	// Production says whether an environment is a production one.
	Production func(ctx context.Context, environmentID string) (bool, error)
	// Observe receives every finished call: the upstream, the status the
	// device saw and the time the whole call took. It never receives a
	// header or a body (SEC-031).
	Observe func(upstream string, status int, d time.Duration)
	// Now is the clock; nil uses time.Now.
	Now func() time.Time
}

// Gateway authorises device calls to upstreams and forwards them.
type Gateway struct{ o Options }

// New returns a gateway.
func New(o Options) (*Gateway, error) {
	switch {
	case o.Upstreams == nil || o.Egress == nil || o.Assertions == nil:
		return nil, errors.New("gateway: a gateway needs upstreams, an egress and an assertion issuer")
	case o.Limits == nil || o.Production == nil:
		return nil, errors.New("gateway: a gateway needs limits and the environment's class")
	}
	return &Gateway{o: o}, nil
}

// Request is one call a device asks the gateway to make.
type Request struct {
	// Upstream and Operation name what to call.
	Upstream, Operation string
	// Plugin is the plugin that makes the call (SEC-080).
	Plugin string
	// Method and Target are the HTTP method and the path with its query,
	// relative to the upstream's base URL.
	Method, Target string
	// Header is what the device offers; only an allowlist is forwarded.
	Header http.Header
	// UserToken is the token ExchangeUserToken returned, when the call
	// acts for a user.
	UserToken string
	// ChallengeID and Signature answer a transaction challenge.
	ChallengeID string
	Signature   []byte
}

// forwardRequestHeaders are the request headers an upstream may receive
// from a device; every other header is dropped, device credentials and
// DPoP among them (SEC-031).
var forwardRequestHeaders = []string{
	"Accept", "Accept-Language", "Cache-Control", "Content-Type", "Idempotency-Key", "If-Match", "If-None-Match",
	"If-Modified-Since", "If-Unmodified-Since", "If-Range", "Range", "Last-Event-ID", "Traceparent", "Tracestate",
	"Sec-Websocket-Protocol", "Sec-Websocket-Version", "Sec-Websocket-Key", "Sec-Websocket-Extensions",
}

// forwardResponseHeaders are the response headers a device receives.
var forwardResponseHeaders = []string{
	"Content-Type", "Content-Language", "Content-Disposition", "Content-Range", "Accept-Ranges", "Cache-Control",
	"Etag", "Last-Modified", "Expires", "Retry-After", "Location", "Sec-Websocket-Accept", "Sec-Websocket-Protocol",
	"Sec-Websocket-Extensions",
}

// Pending is a call that passed every check that needs no body.
type Pending struct {
	g    *Gateway
	d    device.Identity
	req  Request
	up   Upstream
	op   Operation
	lim  limits.Set
	url  *url.URL
	user *User
	prod bool
}

// Plan is an authorised call, ready to be sent.
type Plan struct {
	// Upstream names the upstream, for metrics.
	Upstream string
	Method   string
	URL      *url.URL
	// Header is what is sent: the allowed headers of the device, the
	// assertion and the upstream's credential.
	Header http.Header
	// Body is set when the body was read, as a transaction's must be.
	Body []byte
	// Transport is the upstream's pooled connection.
	Transport http.RoundTripper
	// Timeout bounds the call; MaxRequest and MaxResponse the bytes.
	Timeout                 time.Duration
	MaxRequest, MaxResponse int64
}

func refused(format string, args ...any) error {
	return plxerr.New(plxerr.GatewayDestinationRefused, format, args...)
}

// Begin runs every check of a call that does not need its body, in this
// order: the device's rate, the upstream, the method and path, the
// operation and the plugin's capability, the assurance level, the user
// token, and the shape of a transaction. The cheapest and most general
// checks come first, so a refused call costs little.
func (g *Gateway) Begin(ctx context.Context, d device.Identity, r Request) (*Pending, error) {
	lim, err := g.o.Limits(ctx, d)
	if err != nil {
		return nil, err
	}
	if g.o.Count != nil {
		if n, err := g.o.Count(ctx, "rate:gateway:"+d.DeviceID, rateWindow); err == nil && n > lim.Get(limits.GatewayRequestsPerMinutePerDevice) {
			return nil, plxerr.New(plxerr.GatewayRateLimited, "more than %d gateway requests in %s", lim.Get(limits.GatewayRequestsPerMinutePerDevice), rateWindow)
		}
	}
	up, err := g.o.Upstreams.Resolve(ctx, d.OrganizationID, d.EnvironmentID, r.Upstream)
	if err != nil {
		return nil, err //nolint:wrapcheck // a domain error
	}
	target, err := checkTarget(up, r)
	if err != nil {
		return nil, err
	}
	op, err := g.operation(up, r)
	if err != nil {
		return nil, err
	}
	if err := checkAssurance(d, up, op); err != nil {
		return nil, err
	}
	if err := checkHeaders(r.Header); err != nil {
		return nil, err
	}
	if err := checkTransactionShape(op, r); err != nil {
		return nil, err
	}
	prod, err := g.o.Production(ctx, d.EnvironmentID)
	if err != nil {
		return nil, err
	}
	p := &Pending{g: g, d: d, req: r, up: up, op: op, lim: lim, url: target, prod: prod}
	if r.UserToken != "" {
		if g.o.Users == nil {
			return nil, userInvalid("unsupported")
		}
		u, err := g.o.Users.Verify(ctx, r.UserToken, BoundTo{
			DeviceID: d.DeviceID, AppID: d.AppID, EnvironmentID: d.EnvironmentID, Upstream: up.Name, Production: prod,
		})
		if err != nil {
			return nil, err
		}
		p.user = &u
	}
	return p, nil
}

// checkTarget validates the method and the path and joins the path to the
// upstream's base URL.
func checkTarget(up Upstream, r Request) (*url.URL, error) {
	if !contains(up.Methods, r.Method) {
		return nil, refused("the method %s is not one the upstream allows", methodForMessage(r.Method))
	}
	if !strings.HasPrefix(r.Target, "/") || strings.HasPrefix(r.Target, "//") || strings.ContainsAny(r.Target, "#\\") {
		return nil, refused("the path must start with a single slash and have no fragment or backslash")
	}
	for _, c := range r.Target {
		if c < 0x20 || c == 0x7f {
			return nil, refused("the path has a control character")
		}
	}
	rel, err := url.ParseRequestURI(r.Target)
	if err != nil || rel.Scheme != "" || rel.Host != "" {
		return nil, refused("the path is not a path")
	}
	if err := cleanPath(rel.EscapedPath()); err != nil {
		return nil, err
	}
	base, err := url.Parse(up.BaseURL)
	if err != nil {
		return nil, plxerr.New(plxerr.GatewayUpstreamFailed, "the upstream's base URL is not usable")
	}
	out := *base
	prefix := strings.TrimSuffix(base.EscapedPath(), "/")
	out.Path = strings.TrimSuffix(base.Path, "/") + rel.Path
	out.RawPath = prefix + rel.EscapedPath()
	out.RawQuery = rel.RawQuery
	out.Fragment, out.User = "", nil
	return &out, nil
}

// methodForMessage keeps an arbitrary method out of an error message.
func methodForMessage(m string) string {
	if contains(httpMethods, m) {
		return m
	}
	return "sent"
}

// operation finds the operation a call names and applies its method, path
// and plugin rules. An upstream with operations accepts only them.
func (g *Gateway) operation(up Upstream, r Request) (Operation, error) {
	if len(up.Operations) == 0 {
		if r.Operation != "" && len(r.Operation) > 128 {
			return Operation{}, refused("the operation name is too long")
		}
		return Operation{Name: r.Operation}, nil
	}
	op, ok := up.Operation(r.Operation)
	if !ok {
		return Operation{}, refused("the upstream has no such operation")
	}
	if len(op.Methods) > 0 && !contains(op.Methods, r.Method) {
		return Operation{}, refused("the method %s is not one the operation allows", methodForMessage(r.Method))
	}
	if op.PathPrefix != "" {
		rel, _ := url.ParseRequestURI(r.Target)
		if rel == nil || !underPrefix(rel.Path, op.PathPrefix) {
			return Operation{}, refused("the path is not under the operation's path")
		}
	}
	if len(op.Plugins) > 0 && !contains(op.Plugins, r.Plugin) {
		return Operation{}, refused("the plugin has not declared the capability to call this operation")
	}
	return op, nil
}

// underPrefix reports whether path is the prefix or below it, by whole
// segments.
func underPrefix(path, prefix string) bool {
	if strings.HasSuffix(prefix, "/") {
		return strings.HasPrefix(path, prefix)
	}
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

// checkAssurance requires the highest level the upstream and the
// operation ask for (SEC-007).
func checkAssurance(d device.Identity, up Upstream, op Operation) error {
	need := up.Assurance
	if AssuranceRank(op.Assurance) > AssuranceRank(need) {
		need = op.Assurance
	}
	if need == "" || AssuranceRank(need) <= 0 {
		return nil
	}
	if AssuranceRank(d.Assurance) < AssuranceRank(need) {
		return plxerr.New(plxerr.AssuranceInsufficient, "this call needs assurance level %s", need)
	}
	return nil
}

// checkHeaders refuses headers a device may not set. The names of Plux's
// own headers are reserved: a device cannot forge an assertion, and it
// cannot present a transaction signature in a header of a call that is
// not the typed one (SEC-032).
func checkHeaders(h http.Header) error {
	for name := range h {
		lower := strings.ToLower(name)
		if !strings.HasPrefix(lower, "plux-") {
			continue
		}
		if strings.Contains(lower, "transaction") || strings.Contains(lower, "challenge") || strings.Contains(lower, "signature") {
			return plxerr.New(plxerr.TransactionRequiresGateway, "a transaction signature travels in the typed fields of the gateway call, not in a header")
		}
		return refused("headers beginning with Plux- belong to Plux")
	}
	return nil
}

// checkTransactionShape requires a transaction to carry a challenge and a
// signature, and anything else to carry none: a signature sent for an
// operation the registry does not know as a transaction would leave the
// user believing a confirmation protected it.
func checkTransactionShape(op Operation, r Request) error {
	switch {
	case op.Transaction != nil && r.ChallengeID == "":
		return plxerr.New(plxerr.SCAChallengeInvalid, "this operation is a transaction and needs a challenge")
	case op.Transaction != nil && len(r.Signature) == 0:
		return plxerr.New(plxerr.SCASignatureInvalid, "this operation is a transaction and needs a signature")
	case op.Transaction == nil && (r.ChallengeID != "" || len(r.Signature) > 0):
		return plxerr.New(plxerr.PreconditionFailed, "this operation is not registered as a transaction")
	}
	return nil
}

// NeedsBody reports whether Finish needs the request body: only a
// transaction's is read, to check its dynamic linking.
func (p *Pending) NeedsBody() bool { return p.op.Transaction != nil }

// MaxRequest is the most bytes of request body the gateway forwards
// (gateway.requestBytes).
func (p *Pending) MaxRequest() int64 { return p.lim.Get(limits.GatewayRequestBytes) }

// Finish completes the checks, with the body when NeedsBody asked for it,
// and returns the plan to send. It verifies a transaction's signature and
// linking and signs the assertion.
func (p *Pending) Finish(ctx context.Context, body []byte) (*Plan, error) {
	g := p.g
	if int64(len(body)) > p.MaxRequest() {
		return nil, plxerr.New(plxerr.LimitExceeded, "the request body exceeds gateway.requestBytes = %d", p.MaxRequest())
	}
	claims := AssertionClaims{
		Audience: p.up.AudienceOrDefault(), DeviceID: p.d.DeviceID, AppID: p.d.AppID, EnvironmentID: p.d.EnvironmentID,
		OrganizationID: p.d.OrganizationID, Assurance: p.d.Assurance, Production: p.prod, User: p.user,
	}
	var bodySum []byte
	if body != nil {
		sum := sha256.Sum256(body)
		bodySum = sum[:]
	}
	if p.op.Transaction != nil {
		if g.o.SCA == nil {
			return nil, plxerr.New(plxerr.TransactionRequiresGateway, "this server cannot verify transactions")
		}
		proof, err := g.o.SCA.Verify(ctx, p.d, p.up, p.op, p.req.ChallengeID, p.req.Signature, body, p.challengeLifetime())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		claims.SCA = &proof
	}
	claims.RequestHash = RequestHash(p.req.Method, p.url.RequestURI(), bodySum)
	assertion, err := g.o.Assertions.Issue(ctx, claims)
	if err != nil {
		return nil, err
	}
	transport, err := g.o.Egress.Transport(ctx, p.up)
	if err != nil {
		return nil, err
	}
	header := allowedHeaders(p.req.Header)
	header.Set(AssertionHeader, assertion)
	if name, value, ok, err := g.o.Egress.Credential(ctx, p.up); err != nil {
		return nil, err
	} else if ok {
		header.Set(name, value)
	}
	timeout := p.up.Timeout
	if timeout <= 0 {
		timeout = time.Duration(p.lim.Get(limits.GatewayTimeout)) * time.Millisecond
	}
	return &Plan{
		Upstream: p.up.Name, Method: p.req.Method, URL: p.url, Header: header, Body: body, Transport: transport,
		Timeout: timeout, MaxRequest: p.MaxRequest(), MaxResponse: p.lim.Get(limits.GatewayResponseBytes),
	}, nil
}

// challengeLifetime is sca.challengeLifetime for the device's app.
func (p *Pending) challengeLifetime() time.Duration {
	return time.Duration(p.lim.Get(limits.ScaChallengeLifetime)) * time.Millisecond
}

// allowedHeaders keeps the request headers an upstream may receive.
func allowedHeaders(in http.Header) http.Header {
	out := http.Header{}
	for _, name := range forwardRequestHeaders {
		if v := in.Values(name); len(v) > 0 {
			out[http.CanonicalHeaderKey(name)] = append([]string(nil), v...)
		}
	}
	return out
}

// CreateChallenge issues the challenge of a transaction (SEC-027): the
// device sends the summary the user is shown, and signs what the server
// returns together with it.
func (g *Gateway) CreateChallenge(ctx context.Context, d device.Identity, upstream, operation string, summary []byte) (Challenge, error) {
	if g.o.SCA == nil {
		return Challenge{}, plxerr.New(plxerr.TransactionRequiresGateway, "this server cannot verify transactions")
	}
	lim, err := g.o.Limits(ctx, d)
	if err != nil {
		return Challenge{}, err
	}
	up, err := g.o.Upstreams.Resolve(ctx, d.OrganizationID, d.EnvironmentID, upstream)
	if err != nil {
		return Challenge{}, err //nolint:wrapcheck // a domain error
	}
	op, ok := up.Operation(operation)
	if !ok || op.Transaction == nil {
		return Challenge{}, refused("the upstream has no such transaction")
	}
	if err := checkAssurance(d, up, op); err != nil {
		return Challenge{}, err
	}
	life := time.Duration(lim.Get(limits.ScaChallengeLifetime)) * time.Millisecond
	return g.o.SCA.Create(ctx, d, up, op, summary, life)
}

// Finished records a finished call for the metrics.
func (g *Gateway) Finished(upstream string, status int, d time.Duration) {
	if g.o.Observe != nil {
		g.o.Observe(upstream, status, d)
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package gateway

import (
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/nightCode42/plux3/backend/internal/httpx"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// AuthKind is how the gateway authenticates to an upstream (SEC-031).
type AuthKind string

// The ways to authenticate to an upstream. With none, the Plux assertion
// is the only credential.
const (
	AuthNone   AuthKind = "none"
	AuthMTLS   AuthKind = "mtls"
	AuthOAuth  AuthKind = "oauth_client_credentials"
	AuthAPIKey AuthKind = "api_key"
)

// Auth is an upstream's authentication. It names secrets, never holds
// them: each reference is the key of an environment secret, which is
// sealed at rest (SEC-106).
type Auth struct {
	Kind AuthKind
	// MTLSSecret names the secret that holds the client certificate chain
	// and its private key, both PEM.
	MTLSSecret string
	// OAuth client credentials (RFC 6749 section 4.4).
	TokenURL string
	ClientID string
	// ClientSecretRef is the key of the environment secret that holds the
	// client secret, which is sealed at rest (SEC-106) and never stored or
	// serialised here.
	ClientSecretRef string
	Scopes          []string
	// Header and KeySecret are the header that carries a static key and
	// the secret that holds it.
	Header    string
	KeySecret string
}

// Transaction maps the fields dynamic linking signs to positions in the
// request body, as JSON Pointers (RFC 6901). It makes an operation a
// transaction (SEC-028).
type Transaction struct {
	Amount, Currency, Payee string
	// Summary is optional.
	Summary string
}

// Operation is one named operation of an upstream and the policy the
// gateway enforces for it.
type Operation struct {
	Name string
	// Methods narrow the upstream's; empty means all of them.
	Methods []string
	// PathPrefix is the path every call must be under; empty means any.
	PathPrefix string
	// Assurance is the lowest assurance level the operation needs; empty
	// means the upstream's.
	Assurance string
	// Transaction, when set, requires strong customer authentication.
	Transaction *Transaction
	// Plugins limits the operation to these plugins (SEC-080); empty means
	// any plugin of the app.
	Plugins []string
}

// Upstream is an API the gateway may reach for the devices of one
// environment (SEC-031).
type Upstream struct {
	ID, OrganizationID, AppID, EnvironmentID string
	Name                                     string
	BaseURL                                  string
	Auth                                     Auth
	// AllowPrivate lets the upstream resolve to a private or loopback
	// address (SEC-105). Link-local and metadata addresses stay refused.
	AllowPrivate bool
	// Timeout bounds a call; zero uses gateway.timeout.
	Timeout time.Duration
	Methods []string
	// Assurance is the lowest assurance level any call needs; empty
	// means "AL0".
	Assurance  string
	Operations []Operation
	// Audience is the `aud` of the assertion; empty means BaseURL.
	Audience string
	// Version rises with every change.
	Version int64
}

// AudienceOrDefault returns the assertion's audience.
func (u Upstream) AudienceOrDefault() string {
	if u.Audience != "" {
		return u.Audience
	}
	return u.BaseURL
}

// Operation finds an operation by name.
func (u Upstream) Operation(name string) (Operation, bool) {
	for _, op := range u.Operations {
		if op.Name == name {
			return op, true
		}
	}
	return Operation{}, false
}

// httpMethods are the methods an upstream may allow. CONNECT, TRACE and
// OPTIONS are never forwarded.
var httpMethods = []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE"}

// bodyMethods are the methods a transaction may use.
var bodyMethods = []string{"POST", "PUT", "PATCH"}

var (
	upstreamName = regexp.MustCompile(`^[a-z][a-zA-Z0-9]{0,63}$`)
	operationKey = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,127}$`)
	secretKey    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)
	headerName   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]{0,63}$`)
	pluginKey    = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	pointerForm  = regexp.MustCompile(`^(/([^/~]|~[01])*)*$`)
)

// assuranceLevels are the levels a registration may name.
var assuranceLevels = []string{"AL0", "AL1", "AL2", "AL3"}

// AssuranceRank orders assurance levels; an unknown level ranks below AL0.
func AssuranceRank(level string) int {
	return slices.Index(assuranceLevels, level)
}

// reserved headers are never an upstream's api key header: they frame or
// route the request, or belong to Plux.
func reservedHeader(name string) bool {
	switch strings.ToLower(name) {
	case "host", "content-length", "transfer-encoding", "connection", "upgrade", "te", "trailer", "dpop",
		"cookie", "proxy-authorization":
		return true
	}
	return strings.HasPrefix(strings.ToLower(name), "plux-")
}

func invalid(format string, args ...any) error {
	return plxerr.New(plxerr.InvalidFormat, format, args...)
}

// Validate checks an upstream's registration. It returns the first
// problem as an error that names the field and never echoes a secret.
func (u Upstream) Validate() error {
	if !upstreamName.MatchString(u.Name) {
		return invalid("the name of an upstream is the name of its data source: a lower-case letter followed by up to 63 letters and digits")
	}
	if err := u.validateBaseURL(); err != nil {
		return err
	}
	if err := u.Auth.validate(u.AllowPrivate); err != nil {
		return err
	}
	if u.Timeout < 0 {
		return invalid("the timeout is not negative")
	}
	if len(u.Methods) == 0 {
		return plxerr.New(plxerr.MissingProperty, "an upstream needs at least one allowed method")
	}
	if err := checkMethods(u.Methods, httpMethods); err != nil {
		return err
	}
	if u.Assurance != "" && AssuranceRank(u.Assurance) < 0 {
		return invalid("%q is not an assurance level from AL0 to AL3", u.Assurance)
	}
	if u.Audience != "" && len(u.Audience) > 256 {
		return invalid("the audience is longer than 256 characters")
	}
	seen := map[string]bool{}
	for _, op := range u.Operations {
		if seen[op.Name] {
			return invalid("the operation %q is listed twice", op.Name)
		}
		seen[op.Name] = true
		if err := op.validate(u); err != nil {
			return err
		}
	}
	return nil
}

// validateBaseURL checks the base URL: HTTPS, a host, no credentials,
// query or fragment, and no private literal address unless allowed.
func (u Upstream) validateBaseURL() error {
	parsed, err := url.Parse(u.BaseURL)
	if err != nil || parsed.Host == "" || parsed.Opaque != "" {
		return invalid("the base URL is not an absolute URL")
	}
	switch {
	case parsed.Scheme == "https":
	case parsed.Scheme == "http" && u.AllowPrivate:
	default:
		return invalid("the base URL must be https; http is accepted only for an upstream that allows private addresses")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.ForceQuery {
		return invalid("the base URL has no credentials, query or fragment")
	}
	if err := cleanPath(parsed.EscapedPath()); err != nil {
		return err
	}
	if addr, err := netip.ParseAddr(parsed.Hostname()); err == nil && !u.AllowPrivate {
		if err := (httpx.Policy{}).Check(addr); err != nil {
			return plxerr.New(plxerr.GatewayDestinationRefused, "the base URL is a private or reserved address; the upstream must allow private addresses")
		}
	}
	return nil
}

// validate checks the authentication settings.
func (a Auth) validate(allowPrivate bool) error {
	switch a.Kind {
	case AuthNone:
		return nil
	case AuthMTLS:
		if !secretKey.MatchString(a.MTLSSecret) {
			return invalid("mutual TLS names the secret that holds the client certificate")
		}
	case AuthOAuth:
		if !secretKey.MatchString(a.ClientSecretRef) || a.ClientID == "" || len(a.ClientID) > 256 {
			return invalid("OAuth client credentials need a client ID and the secret that holds the client secret")
		}
		if err := a.validateOAuth(allowPrivate); err != nil {
			return err
		}
	case AuthAPIKey:
		if !headerName.MatchString(a.Header) || reservedHeader(a.Header) || !secretKey.MatchString(a.KeySecret) {
			return invalid("an API key names a header it may use and the secret that holds the key")
		}
	default:
		return invalid("%q is not an authentication kind", a.Kind)
	}
	return nil
}

// validateOAuth checks the token endpoint and the scopes of client
// credentials.
func (a Auth) validateOAuth(allowPrivate bool) error {
	token, err := url.Parse(a.TokenURL)
	secure := err == nil && (token.Scheme == "https" || (token.Scheme == "http" && allowPrivate))
	if !secure || token.Host == "" || token.User != nil || token.Fragment != "" {
		return invalid("the token URL must be an https URL without credentials")
	}
	for _, s := range a.Scopes {
		if s == "" || strings.ContainsAny(s, " \t\r\n\"\\") {
			return invalid("a scope is not empty and has no spaces")
		}
	}
	return nil
}

// validate checks one operation against its upstream.
func (op Operation) validate(u Upstream) error {
	if !operationKey.MatchString(op.Name) {
		return invalid("the name of an operation is a letter followed by up to 127 letters, digits, dots, underscores and hyphens")
	}
	if err := checkMethods(op.Methods, u.Methods); err != nil {
		return err
	}
	if err := op.validatePolicy(); err != nil {
		return err
	}
	if op.Transaction == nil {
		return nil
	}
	return op.validateTransaction(u)
}

// validatePolicy checks an operation's path, assurance and plugins.
func (op Operation) validatePolicy() error {
	if op.PathPrefix != "" {
		if !strings.HasPrefix(op.PathPrefix, "/") {
			return invalid("the path prefix of operation %q starts with /", op.Name)
		}
		if err := cleanPath(op.PathPrefix); err != nil {
			return err
		}
	}
	if op.Assurance != "" && AssuranceRank(op.Assurance) < 0 {
		return invalid("%q is not an assurance level from AL0 to AL3", op.Assurance)
	}
	for _, p := range op.Plugins {
		if !pluginKey.MatchString(p) {
			return invalid("%q is not a plugin key", p)
		}
	}
	return nil
}

// validateTransaction checks a transaction's pointers, and that it can
// carry a body.
func (op Operation) validateTransaction(u Upstream) error {
	t := op.Transaction
	for _, ptr := range []string{t.Amount, t.Currency, t.Payee} {
		if ptr == "" || !pointerForm.MatchString(ptr) {
			return invalid("the transaction of operation %q maps amount, currency and payee to JSON Pointers", op.Name)
		}
	}
	if t.Summary != "" && !pointerForm.MatchString(t.Summary) {
		return invalid("the summary of operation %q is not a JSON Pointer", op.Name)
	}
	allowed := u.Methods
	if len(op.Methods) > 0 {
		allowed = op.Methods
	}
	for _, m := range allowed {
		if !slices.Contains(bodyMethods, m) {
			return invalid("operation %q is a transaction and so allows only POST, PUT and PATCH", op.Name)
		}
	}
	return nil
}

// checkMethods requires every method to be an upper-case HTTP method from
// the allowed list, listed once.
func checkMethods(methods, allowed []string) error {
	seen := map[string]bool{}
	for _, m := range methods {
		if !slices.Contains(allowed, m) {
			return invalid("%q is not a method this upstream allows", m)
		}
		if seen[m] {
			return invalid("the method %q is listed twice", m)
		}
		seen[m] = true
	}
	return nil
}

// cleanPath refuses a path that could walk out of the base it is joined
// to: dot segments, encoded separators and dots, backslashes and control
// characters. The path is as sent, still escaped.
func cleanPath(escaped string) error {
	lower := strings.ToLower(escaped)
	if strings.Contains(escaped, `\`) || strings.Contains(lower, "%2f") || strings.Contains(lower, "%5c") ||
		strings.Contains(lower, "%2e") || strings.Contains(escaped, "//") {
		return plxerr.New(plxerr.GatewayDestinationRefused, "the path has an encoded separator, a backslash, an empty segment or an encoded dot")
	}
	for _, r := range escaped {
		if r < 0x20 || r == 0x7f {
			return plxerr.New(plxerr.GatewayDestinationRefused, "the path has a control character")
		}
	}
	for _, seg := range strings.Split(escaped, "/") {
		if seg == "." || seg == ".." {
			return plxerr.New(plxerr.GatewayDestinationRefused, "the path has a dot segment")
		}
	}
	return nil
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package egress

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/nightCode42/plux3/backend/internal/httpx"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Header names of the protocol between fnrunner and egress.
const (
	// HeaderAllow carries the function's domain allowlist, a
	// comma-separated list of exact names and "*.name" patterns.
	HeaderAllow = "Plux-Egress-Allow"
	// HeaderFunction names the calling function as
	// "<app>/<environment>/<name>@<version>". It is used for logs only.
	HeaderFunction = "Plux-Function"
	// HeaderError carries the PLX code of a refusal.
	HeaderError = "Plux-Error"
)

// Resolver looks up the addresses of a name. *net.Resolver satisfies it.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// ContextDialer opens a connection to a resolved address. *net.Dialer
// satisfies it.
type ContextDialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

// Options configures a Server. Zero values take the secure defaults of
// FN-017.
type Options struct {
	// Listen is the address Start listens on.
	Listen string
	// TLS is the listener's configuration; it must require and verify a
	// client certificate (httpx.InternalServerTLS).
	TLS *tls.Config
	// Policy decides which resolved addresses are reachable. AllowLoopback
	// is for tests and development only.
	Policy httpx.Policy
	// Ports are the destination ports a tunnel may target; empty means 443
	// and 80.
	Ports []int
	// ConnectTimeout bounds the name resolution and the connection to the
	// destination, each; zero means 10 s.
	ConnectTimeout time.Duration
	// IdleTimeout closes a tunnel without traffic; zero means 60 s.
	IdleTimeout time.Duration
	// MaxTunnelDuration closes a tunnel however active; zero means 5 min.
	MaxTunnelDuration time.Duration
	// MaxTunnelBytes closes a tunnel when a direction has carried this
	// many bytes; zero means 64 MiB.
	MaxTunnelBytes int64
	// MaxTunnels bounds the tunnels open at once; zero means 1024.
	MaxTunnels int
	// Resolver looks up names; nil uses the system resolver.
	Resolver Resolver
	// Dialer connects to a resolved address; nil uses a dialer that checks
	// the policy again on the address it connects to.
	Dialer ContextDialer
	// Now is the clock; nil means time.Now.
	Now func() time.Time
	// Log receives one line per refusal; it never receives payload bytes.
	Log *slog.Logger
	// Registerer receives the metrics; nil uses a private registry.
	Registerer prometheus.Registerer
}

// Server is the egress proxy. It is safe for concurrent use. It owns the
// goroutines of its listener and tunnels; Stop ends them.
type Server struct {
	opts    Options
	log     *slog.Logger
	metrics *metrics
	ports   []int
	http    *http.Server
	// slots holds one token per open tunnel.
	slots chan struct{}

	// quit is closed by Stop to end the tunnels still open.
	quit     chan struct{}
	quitOnce sync.Once

	mu      sync.Mutex
	closing bool
	active  sync.WaitGroup
	ln      net.Listener
}

// New builds a Server. It opens no socket; call Start.
func New(opts Options) (*Server, error) {
	if opts.TLS == nil || opts.TLS.ClientAuth != tls.RequireAndVerifyClientCert {
		return nil, errors.New("egress.New: the listener must require and verify a client certificate (SEC-043)")
	}
	opts = withDefaults(opts)
	reg := opts.Registerer
	if reg == nil {
		reg = prometheus.NewRegistry()
	}
	m, err := newMetrics(reg)
	if err != nil {
		return nil, fmt.Errorf("egress.New: %w", err)
	}
	tlsConfig := opts.TLS.Clone()
	tlsConfig.MinVersion = tls.VersionTLS13
	// CONNECT is an HTTP/1.1 mechanism: the connection is hijacked.
	tlsConfig.NextProtos = []string{"http/1.1"}
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	s := &Server{
		opts: opts, log: opts.Log, metrics: m, ports: slices.Clone(opts.Ports),
		slots: make(chan struct{}, opts.MaxTunnels), quit: make(chan struct{}),
	}
	s.http = &http.Server{
		Handler:           s,
		Protocols:         protocols,
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: opts.ConnectTimeout,
		MaxHeaderBytes:    32 << 10,
		ErrorLog:          slog.NewLogLogger(opts.Log.Handler(), slog.LevelDebug),
	}
	return s, nil
}

// withDefaults fills the zero values of FN-017.
func withDefaults(o Options) Options {
	if len(o.Ports) == 0 {
		o.Ports = []int{443, 80}
	}
	if o.ConnectTimeout <= 0 {
		o.ConnectTimeout = 10 * time.Second
	}
	if o.IdleTimeout <= 0 {
		o.IdleTimeout = time.Minute
	}
	if o.MaxTunnelDuration <= 0 {
		o.MaxTunnelDuration = 5 * time.Minute
	}
	if o.MaxTunnelBytes <= 0 {
		o.MaxTunnelBytes = 64 << 20
	}
	if o.MaxTunnels <= 0 {
		o.MaxTunnels = 1024
	}
	if o.Resolver == nil {
		o.Resolver = net.DefaultResolver
	}
	if o.Dialer == nil {
		o.Dialer = guardedDialer(o.Policy, o.ConnectTimeout)
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Log == nil {
		o.Log = slog.New(slog.DiscardHandler)
	}
	return o
}

// guardedDialer connects to an address and checks the policy again on the
// address the socket is about to use, so that even a caller of the dialer
// that skipped the check cannot reach a reserved range.
func guardedDialer(policy httpx.Policy, timeout time.Duration) ContextDialer {
	return &net.Dialer{
		Timeout: timeout,
		Control: func(_, address string, _ syscall.RawConn) error {
			ap, err := netip.ParseAddrPort(address)
			if err != nil {
				return fmt.Errorf("%w: %s", httpx.ErrBlocked, address)
			}
			return policy.Check(ap.Addr())
		},
	}
}

// Name identifies the server as a component of the process.
func (*Server) Name() string { return "egress" }

// Start listens and serves in the background. It returns once the socket
// is open.
func (s *Server) Start(ctx context.Context) error {
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", s.opts.Listen)
	if err != nil {
		return fmt.Errorf("egress.Start: %w", err)
	}
	return s.Serve(tls.NewListener(ln, s.http.TLSConfig))
}

// Serve serves on a listener that already terminates TLS with the
// server's configuration, in the background. Tests and Start use it.
func (s *Server) Serve(ln net.Listener) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing || s.ln != nil {
		_ = ln.Close()
		return errors.New("egress.Serve: the server is stopped or already serving")
	}
	s.ln = ln
	s.active.Add(1)
	go func() {
		defer s.active.Done()
		if err := s.http.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.log.Error("the egress listener failed", slog.String("error", err.Error()))
		}
	}()
	return nil
}

// Addr is the address the server listens on, or nil before Start.
func (s *Server) Addr() net.Addr {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln == nil {
		return nil
	}
	return s.ln.Addr()
}

// Stop closes the listener, lets open tunnels finish until the context
// ends, then closes the rest and waits for every goroutine.
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	s.closing = true
	s.mu.Unlock()
	err := s.http.Shutdown(ctx)
	done := make(chan struct{})
	go func() {
		s.active.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		s.endTunnels()
		<-done
	}
	s.endTunnels()
	if err != nil {
		return fmt.Errorf("egress.Stop: %w", err)
	}
	return nil
}

// endTunnels tells every open tunnel to close.
func (s *Server) endTunnels() { s.quitOnce.Do(func() { close(s.quit) }) }

// begin registers an in-flight request, or reports false once Stop has
// begun.
func (s *Server) begin() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing {
		return false
	}
	s.active.Add(1)
	return true
}

// target is a validated CONNECT request.
type target struct {
	host     string
	port     int
	allow    []string
	function string
	// literal is set when the host is an IP address in any spelling.
	literal bool
}

// ServeHTTP answers one CONNECT request. Every other method is refused.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodConnect {
		w.Header().Set("Allow", http.MethodConnect)
		s.reply(w, http.StatusMethodNotAllowed, "only CONNECT is served")
		s.metrics.result(resultBadRequest)
		return
	}
	if !s.begin() {
		s.reply(w, http.StatusServiceUnavailable, "shutting down")
		s.metrics.result(resultLimited)
		return
	}
	defer s.active.Done()

	t, ok := s.parse(w, r)
	if !ok || !s.admit(w, t) {
		return
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		s.reply(w, http.StatusServiceUnavailable, "too many tunnels")
		s.metrics.result(resultLimited)
		return
	}
	upstream, ok := s.connect(r.Context(), w, t)
	if !ok {
		return
	}
	s.tunnel(w, t, upstream)
}

// parse validates the target and the two headers. It answers 400 itself.
func (s *Server) parse(w http.ResponseWriter, r *http.Request) (target, bool) {
	host, portText, err := net.SplitHostPort(r.Host)
	port, perr := strconv.Atoi(portText)
	if err != nil || perr != nil || port < 1 || port > 65535 || host == "" {
		s.bad(w, "", "the target must be host:port")
		return target{}, false
	}
	function := r.Header.Get(HeaderFunction)
	if !validFunction(function) {
		s.bad(w, host, "the "+HeaderFunction+" header is missing or malformed")
		return target{}, false
	}
	allow := parseAllowlist(r.Header.Values(HeaderAllow))
	if len(allow) == 0 {
		s.bad(w, host, "the "+HeaderAllow+" header is missing or empty")
		return target{}, false
	}
	// An IP literal is recognised in every spelling: a parsed address,
	// which covers IPv6 and zones, and a name that only a legacy parser
	// would read as a number. It is refused by admit, after the port.
	if _, err := netip.ParseAddr(host); err == nil {
		return target{host: host, port: port, function: function, literal: true}, true
	}
	name, ok := normalizeHost(host)
	if !ok {
		s.bad(w, host, "the target is not a valid host name")
		return target{}, false
	}
	literal := looksLikeAddress(name)
	return target{host: name, port: port, allow: allow, function: function, literal: literal}, true
}

// admit applies the port and allowlist rules, answering 403 itself.
func (s *Server) admit(w http.ResponseWriter, t target) bool {
	if !slices.Contains(s.ports, t.port) {
		s.refuse(w, resultRefusedPort, t.function, t.host, "the port is not allowed")
		return false
	}
	if t.literal {
		s.refuse(w, resultRefusedIPLit, t.function, t.host, "the target must be a host name")
		return false
	}
	if !matchAllowlist(t.allow, t.host) {
		s.refuse(w, resultRefusedDomain, t.function, t.host, "the domain is not allowed")
		return false
	}
	return true
}

// connect resolves the name, checks every address and dials one of them.
// It answers 403 or 502 itself.
func (s *Server) connect(ctx context.Context, w http.ResponseWriter, t target) (net.Conn, bool) {
	rctx, cancel := context.WithTimeout(ctx, s.opts.ConnectTimeout)
	addrs, err := s.opts.Resolver.LookupNetIP(rctx, "ip", t.host)
	cancel()
	if err != nil || len(addrs) == 0 {
		s.upstreamFailed(w, t, "the name did not resolve")
		return nil, false
	}
	// Every address must be reachable: a name with one private address is
	// a name that can be steered there (FN-017, ADR-0064).
	for _, a := range addrs {
		if err := s.opts.Policy.Check(a); err != nil {
			s.refuse(w, resultRefusedAddress, t.function, t.host, "the address is not allowed")
			return nil, false
		}
	}
	dctx, cancel := context.WithTimeout(ctx, s.opts.ConnectTimeout)
	defer cancel()
	for _, a := range addrs {
		// The validated address is dialled, never the name again.
		addr := net.JoinHostPort(a.Unmap().String(), strconv.Itoa(t.port))
		if conn, err := s.opts.Dialer.DialContext(dctx, "tcp", addr); err == nil {
			return conn, true
		}
	}
	s.upstreamFailed(w, t, "the destination did not accept the connection")
	return nil, false
}

// tunnel answers 200, takes over the client connection and splices.
func (s *Server) tunnel(w http.ResponseWriter, t target, upstream net.Conn) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		_ = upstream.Close()
		s.reply(w, http.StatusInternalServerError, "the connection cannot be taken over")
		return
	}
	conn, rw, err := hj.Hijack()
	if err != nil {
		_ = upstream.Close()
		return
	}
	_ = conn.SetDeadline(s.opts.Now().Add(s.opts.ConnectTimeout))
	if _, err := conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		_ = conn.Close()
		_ = upstream.Close()
		return
	}
	_ = conn.SetDeadline(time.Time{})
	s.metrics.result(resultOK)
	end := splice(s.quit, &bufferedConn{Conn: conn, r: rw.Reader}, upstream, &tunnel{
		limits: spliceLimits{Idle: s.opts.IdleTimeout, Duration: s.opts.MaxTunnelDuration, MaxBytes: s.opts.MaxTunnelBytes},
		now:    s.opts.Now,
		count:  s.metrics.carried,
	})
	if end == endBytes || end == endIdle || end == endDuration {
		s.log.Info("egress tunnel ended by a limit",
			slog.String("function", t.function), slog.String("host", t.host), slog.String("limit", end))
	}
}

// reply writes a short plain-text answer.
func (*Server) reply(w http.ResponseWriter, status int, reason string) {
	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("Connection", "close")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(reason + "\n"))
}

// bad answers 400.
func (s *Server) bad(w http.ResponseWriter, host, reason string) {
	s.metrics.result(resultBadRequest)
	s.log.Info("egress request malformed", slog.String("host", clip(host)), slog.String("reason", reason))
	s.reply(w, http.StatusBadRequest, reason)
}

// refuse answers 403 PLX-7010 and logs the refusal. The answer names the
// rule that refused and nothing about the destination (FN-017).
func (s *Server) refuse(w http.ResponseWriter, result, function, host, reason string) {
	s.metrics.result(result)
	s.log.Warn("egress refused",
		slog.String("function", function), slog.String("host", clip(host)),
		slog.String("result", result), slog.String("reason", reason))
	w.Header().Set(HeaderError, plxerr.FunctionEgressRefused.String())
	s.reply(w, http.StatusForbidden, reason)
}

// upstreamFailed answers 502.
func (s *Server) upstreamFailed(w http.ResponseWriter, t target, reason string) {
	s.metrics.result(resultUpstreamFailed)
	s.log.Info("egress upstream failed",
		slog.String("function", t.function), slog.String("host", t.host), slog.String("reason", reason))
	s.reply(w, http.StatusBadGateway, reason)
}

// clip bounds a client-supplied string before it is logged.
func clip(s string) string {
	if len(s) > maxHostLength {
		return s[:maxHostLength]
	}
	return s
}

// validFunction reports whether v looks like "<app>/<env>/<name>@<version>".
// The value is only logged, but it is client-supplied, so it is held to a
// narrow alphabet.
func validFunction(v string) bool {
	if v == "" || len(v) > 256 || strings.Count(v, "/") != 2 || strings.Count(v, "@") != 1 {
		return false
	}
	for i := range len(v) {
		b := v[i]
		letterOrDigit := b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
		if !letterOrDigit && strings.IndexByte("-_./@+:", b) < 0 {
			return false
		}
	}
	return true
}

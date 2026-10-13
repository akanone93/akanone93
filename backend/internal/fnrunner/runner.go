// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package fnrunner

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/nightCode42/plux3/backend/internal/fnhost"
	"github.com/nightCode42/plux3/backend/internal/fnpb"
	"github.com/nightCode42/plux3/backend/internal/fnpb/fnpbconnect"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Options configures a Runner. Every field without a stated default is
// required.
type Options struct {
	// Keys are the public keys that may have signed an artifact, by key ID.
	// They come from the role's configuration, never from the api role
	// (FN-009).
	Keys map[string]ed25519.PublicKey
	// Host is the client of the api role's FunctionHostService.
	Host fnpbconnect.FunctionHostServiceClient
	// Egress opens the transports for outbound HTTP.
	Egress Egress
	// Clock reports the time guests see and measures durations.
	Clock func() time.Time
	// Random is the source of the guests' random_get; production passes
	// crypto/rand.Reader.
	Random io.Reader
	// MaxMemoryBytes is the largest memory any invocation may be given.
	MaxMemoryBytes uint64
	// CacheDir persists compiled code between restarts; empty keeps it in
	// memory.
	CacheDir string
	// MaxModules and MaxModuleBytes bound the compiled-module cache.
	MaxModules     int
	MaxModuleBytes int64
	// MaxConcurrent bounds the invocations running at once.
	MaxConcurrent int
	// HTTPResponseBytes bounds the body of an outbound response.
	HTTPResponseBytes int64
	// Log receives one line per system error; nil discards.
	Log *slog.Logger
	// Registerer receives the metrics; nil uses a private registry.
	Registerer prometheus.Registerer
}

// Runner implements the internal FunctionRunnerService (ADR-0064). It is
// safe for concurrent use; Close releases the engine and every compiled
// module.
type Runner struct {
	opts    Options
	engine  *fnhost.Engine
	cache   *moduleCache
	slots   chan struct{}
	metrics *metrics
	log     *slog.Logger
}

var _ fnpbconnect.FunctionRunnerServiceHandler = (*Runner)(nil)

// New builds a Runner and its engine.
func New(ctx context.Context, opts Options) (*Runner, error) {
	if err := opts.check(); err != nil {
		return nil, fmt.Errorf("fnrunner.New: %w", err)
	}
	reg := opts.Registerer
	if reg == nil {
		reg = prometheus.NewRegistry()
	}
	m, err := newMetrics(reg)
	if err != nil {
		return nil, fmt.Errorf("fnrunner.New: %w", err)
	}
	engine, err := fnhost.NewEngine(ctx, fnhost.Config{
		Clock: opts.Clock, Random: opts.Random, MaxMemoryBytes: opts.MaxMemoryBytes, CacheDir: opts.CacheDir,
	})
	if err != nil {
		return nil, fmt.Errorf("fnrunner.New: %w", err)
	}
	log := opts.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Runner{
		opts: opts, engine: engine, cache: newModuleCache(opts.MaxModules, opts.MaxModuleBytes),
		slots: make(chan struct{}, opts.MaxConcurrent), metrics: m, log: log,
	}, nil
}

// check rejects options that would leave the runner unable to refuse
// something it must refuse.
func (o *Options) check() error {
	switch {
	case len(o.Keys) == 0:
		return errors.New("at least one artifact key is required (FN-009)")
	case o.Host == nil || o.Egress == nil || o.Clock == nil || o.Random == nil:
		return errors.New("the host client, egress, clock and randomness are required")
	case o.MaxMemoryBytes == 0 || o.MaxModules < 1 || o.MaxModuleBytes < 1 || o.MaxConcurrent < 1 || o.HTTPResponseBytes < 1:
		return errors.New("the memory, module, concurrency and response bounds must be positive")
	}
	for id, key := range o.Keys {
		if len(key) != ed25519.PublicKeySize {
			return fmt.Errorf("artifact key %q is not an Ed25519 public key", id)
		}
	}
	return nil
}

// Close releases every compiled module and the engine. Call it after the
// server stopped delivering requests.
func (r *Runner) Close(ctx context.Context) error {
	r.cache.close(ctx)
	if err := r.engine.Close(ctx); err != nil {
		return fmt.Errorf("fnrunner.Close: %w", err)
	}
	return nil
}

// Prepare implements FunctionRunnerService: it fetches, verifies and
// compiles a module and keeps the compiled form (FN-010, FN-023).
func (r *Runner) Prepare(ctx context.Context, req *connect.Request[fnpb.PrepareRequest]) (*connect.Response[fnpb.PrepareResponse], error) {
	e, err := r.module(ctx, req.Msg.GetModuleSha256(), req.Msg.GetGrant())
	if err != nil {
		r.log.WarnContext(ctx, "preparing a function module failed", slog.String("code", codeOf(err).String()))
		return nil, toConnect(err)
	}
	r.cache.release(ctx, e)
	return connect.NewResponse(&fnpb.PrepareResponse{}), nil
}

// Invoke implements FunctionRunnerService: one invocation in a fresh
// instance under the request's limits and capabilities (FN-010 to FN-012).
// Above the configured number of concurrent invocations the call is
// refused with PLX-8040 instead of queueing.
func (r *Runner) Invoke(ctx context.Context, req *connect.Request[fnpb.FunctionRunnerServiceInvokeRequest]) (*connect.Response[fnpb.FunctionRunnerServiceInvokeResponse], error) {
	return r.invoke(ctx, req.Msg, serverFunctionIndex)
}

// serverFunctionIndex is the dispatch entry of a server function: a version
// is a module of one function (ADR-0062).
const serverFunctionIndex = 0

// invoke runs entry index of the module. Production always passes
// serverFunctionIndex; the parameter lets the tests reach the other entries
// of the conformance probe through the same path.
func (r *Runner) invoke(ctx context.Context, msg *fnpb.FunctionRunnerServiceInvokeRequest, index uint32) (*connect.Response[fnpb.FunctionRunnerServiceInvokeResponse], error) {
	start := r.opts.Clock()
	select {
	case r.slots <- struct{}{}:
		defer func() { <-r.slots }()
	default:
		err := plxerr.New(plxerr.RateLimited, "the function runner is at its concurrency limit")
		r.metrics.invocations.WithLabelValues(resultOf(err)).Inc()
		return nil, toConnect(err)
	}
	out, stats, err := r.run(ctx, msg, index)
	r.metrics.duration.Observe(r.opts.Clock().Sub(start).Seconds())
	if err != nil {
		r.metrics.invocations.WithLabelValues(resultOf(err)).Inc()
		if stats != nil {
			r.metrics.memoryPeak.Observe(float64(stats.PeakMemoryBytes))
		}
		r.logFailure(ctx, msg, err)
		return nil, toConnect(err)
	}
	r.metrics.memoryPeak.Observe(float64(stats.PeakMemoryBytes))
	resp := &fnpb.FunctionRunnerServiceInvokeResponse{
		MemoryPeakBytes: stats.PeakMemoryBytes,
		HttpCalls:       uint32(stats.HTTPCalls), //nolint:gosec // G115: bounded by the http-calls limit
	}
	if out.Business != nil {
		raw, merr := json.Marshal(out.Business)
		if merr != nil {
			err = plxerr.Wrap(plxerr.FunctionOutputInvalid, merr, "the business error cannot be encoded")
			r.metrics.invocations.WithLabelValues(resultOf(err)).Inc()
			return nil, toConnect(err)
		}
		r.metrics.invocations.WithLabelValues(resultBusinessError).Inc()
		resp.Result = &fnpb.FunctionRunnerServiceInvokeResponse_BusinessError{BusinessError: raw}
		return connect.NewResponse(resp), nil
	}
	r.metrics.invocations.WithLabelValues(resultOK).Inc()
	resp.Result = &fnpb.FunctionRunnerServiceInvokeResponse_Output{Output: out.Output}
	return connect.NewResponse(resp), nil
}

// logFailure writes the one line of a system error: identifiers and the
// code, never input, output, claims, secrets or the error text, which can
// quote the guest (SEC-092).
func (r *Runner) logFailure(ctx context.Context, msg *fnpb.FunctionRunnerServiceInvokeRequest, err error) {
	r.log.WarnContext(ctx, "function invocation failed",
		slog.String("app", msg.GetApp()), slog.String("environment", msg.GetEnvironment()),
		slog.String("function", msg.GetFunction()), slog.Uint64("version", uint64(msg.GetVersion())),
		slog.String("code", codeOf(err).String()))
}

// run resolves the module and runs the call. The stats are nil when no
// instance ran.
func (r *Runner) run(ctx context.Context, msg *fnpb.FunctionRunnerServiceInvokeRequest, index uint32) (fnhost.Outcome, *fnhost.Stats, error) {
	call, err := r.buildCall(msg, index)
	if err != nil {
		return fnhost.Outcome{}, nil, err
	}
	e, err := r.module(ctx, msg.GetModuleSha256(), msg.GetGrant())
	if err != nil {
		return fnhost.Outcome{}, nil, err
	}
	defer r.cache.release(ctx, e)
	host := &remoteHost{
		client: r.opts.Host, egress: r.opts.Egress, grant: msg.GetGrant(), traceparent: msg.GetTraceparent(),
		allow: call.Grants.HTTPDomains, label: egressLabel(msg), maxBody: r.opts.HTTPResponseBytes,
	}
	defer host.close()
	call.Host = host
	out, stats, err := r.engine.Invoke(ctx, e.module, call)
	if err != nil {
		return out, &stats, fmt.Errorf("fnrunner.Invoke: %w", err)
	}
	return out, &stats, nil
}

// buildCall turns the request into a call of the function host.
func (r *Runner) buildCall(msg *fnpb.FunctionRunnerServiceInvokeRequest, index uint32) (fnhost.Call, error) {
	l, caps := msg.GetLimits(), msg.GetCapabilities()
	if l == nil || l.GetTimeoutMs() == 0 {
		return fnhost.Call{}, plxerr.New(plxerr.InvalidStructure, "the request carries no resolved limits")
	}
	memory := min(l.GetMemoryBytes(), r.opts.MaxMemoryBytes)
	call := fnhost.Call{
		Index: index,
		Input: msg.GetInput(),
		Limits: fnhost.Limits{
			Timeout:     time.Duration(l.GetTimeoutMs()) * time.Millisecond,
			MemoryBytes: memory,
			InputBytes:  clampInt(l.GetInputBytes()),
			OutputBytes: clampInt(l.GetOutputBytes()),
			LogBytes:    clampInt(l.GetLogBytes()),
			HTTPCalls:   clampInt(uint64(l.GetHttpCalls())),
			CallDepth:   clampInt(uint64(l.GetCallDepth())),
		},
		Grants: fnhost.Grants{
			HTTPDomains: caps.GetHttpDomains(), KV: caps.GetKv(),
			Secrets: caps.GetSecrets(), Functions: caps.GetFunctions(), Claims: caps.GetClaims(),
		},
		Context: fnhost.ContextInfo{
			App: msg.GetApp(), Environment: msg.GetEnvironment(), AssuranceLevel: msg.GetAssuranceLevel(),
		},
		Depth: clampInt(uint64(msg.GetDepth())),
	}
	if caps.GetClaims() && len(msg.GetClaims()) > 0 {
		call.Claims = json.RawMessage(msg.GetClaims())
	}
	return call, nil
}

// clampInt converts a wire value to an int without overflow.
func clampInt(v uint64) int {
	const maxInt = int(^uint(0) >> 1)
	if v > uint64(maxInt) {
		return maxInt
	}
	return int(v)
}

// egressLabel names the function for the egress role: "<app>/<env>/<name>@<version>".
// Each part is held to the alphabet the egress role accepts; anything else
// becomes an underscore.
func egressLabel(msg *fnpb.FunctionRunnerServiceInvokeRequest) string {
	clean := func(s string) string {
		return strings.Map(func(r rune) rune {
			switch {
			case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.', r == '+', r == ':':
				return r
			}
			return '_'
		}, s)
	}
	return fmt.Sprintf("%s/%s/%s@%d", clean(msg.GetApp()), clean(msg.GetEnvironment()), clean(msg.GetFunction()), msg.GetVersion())
}

// module returns the compiled module for a requested SHA-256 with one
// reference the caller must release. A cache miss fetches the artifact
// through FunctionHostService, verifies it (FN-009) and compiles it;
// concurrent callers for the same module share one load.
func (r *Runner) module(ctx context.Context, digest, grant string) (*entry, error) {
	want, ok := parseDigest(digest)
	if !ok {
		return nil, plxerr.New(plxerr.InvalidStructure, "module_sha256 is not a lower-case hex SHA-256")
	}
	key := hex.EncodeToString(want[:])
	for {
		if e := r.cache.acquire(key); e != nil {
			r.metrics.cache.WithLabelValues(cacheHit).Inc()
			return e, nil
		}
		fl, leader := r.cache.begin(key)
		if leader {
			return r.load(ctx, key, want, grant, fl)
		}
		select {
		case <-fl.done:
		case <-ctx.Done():
			return nil, plxerr.Wrap(plxerr.FunctionCancelled, ctx.Err(), "the request ended while the module was loading")
		}
		if fl.err != nil && !isContextError(fl.err) {
			return nil, fl.err
		}
	}
}

// isContextError reports whether the leader of a load failed only because
// its own request ended.
func isContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, plxerr.Sentinel(plxerr.FunctionCancelled))
}

// load fetches, verifies and compiles a module as the leader of its flight.
func (r *Runner) load(ctx context.Context, key string, want [sha256.Size]byte, grant string, fl *flight) (e *entry, err error) {
	defer func() { r.cache.finish(key, fl, err) }()
	r.metrics.cache.WithLabelValues(cacheMiss).Inc()
	resp, err := r.opts.Host.GetModule(ctx, connect.NewRequest(&fnpb.GetModuleRequest{Grant: grant, ModuleSha256: key}))
	if err != nil {
		return nil, fetchError(ctx, err)
	}
	if int64(len(resp.Msg.GetModule())) > r.opts.MaxModuleBytes {
		return nil, plxerr.New(plxerr.FunctionModuleInvalid, "the module exceeds the runner's module size bound")
	}
	if err := verifyArtifact(r.opts.Keys, want, resp.Msg); err != nil {
		r.metrics.cache.WithLabelValues(cacheRefused).Inc()
		return nil, err
	}
	m, err := r.engine.Compile(ctx, resp.Msg.GetModule(), fnhost.PlacementServer)
	if err != nil {
		return nil, fmt.Errorf("fnrunner.module: %w", err)
	}
	return r.cache.add(ctx, key, m, int64(len(resp.Msg.GetModule()))), nil
}

// fetchError maps a failed module fetch to a Plux error.
func fetchError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return plxerr.Wrap(plxerr.FunctionCancelled, ctx.Err(), "the request ended while the module was fetched")
	}
	switch connect.CodeOf(err) {
	case connect.CodeNotFound:
		return plxerr.New(plxerr.FunctionNotFound, "the module is not stored")
	case connect.CodePermissionDenied, connect.CodeUnauthenticated:
		return plxerr.New(plxerr.PermissionDenied, "the grant does not allow the module")
	}
	return plxerr.New(plxerr.UpstreamUnavailable, "the module could not be fetched")
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/nightCode42/plux3/backend/internal/attest/binding"
	"github.com/nightCode42/plux3/backend/internal/device"
	"github.com/nightCode42/plux3/backend/internal/fnpb"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage/idempotency"
)

// rateWindow is the window the invocation limits count in.
const rateWindow = time.Minute

// Declarations loads a version with its declaration; *Service is one.
type Declarations interface {
	LoadInvocable(ctx context.Context, organizationID, appID, environmentID, name string, version int32) (Invocable, error)
}

// Resolver finds the version a name stands for; *Service is one.
type Resolver interface {
	ResolveVersion(ctx context.Context, organizationID, appID, environmentID, name, alias string) (Resolved, error)
}

// Environment reads the environment variables a function declares;
// *Service is one.
type Environment interface {
	EnvironmentVariables(ctx context.Context, organizationID, appID, environmentID string, names []string) (map[string]string, error)
}

// Runner runs one invocation on the function runner role (ADR-0064). An
// error is a *plxerr.Error carrying the runner's code.
type Runner interface {
	Run(ctx context.Context, req *fnpb.FunctionRunnerServiceInvokeRequest) (*fnpb.FunctionRunnerServiceInvokeResponse, error)
}

// Attestations verifies the per-request platform evidence of an
// invocation that declares `attestation` (FN-020, SEC-003, SEC-004);
// *device.Service is one.
type Attestations interface {
	VerifyInvocationEvidence(ctx context.Context, d device.Identity, e device.InvocationEvidence) error
}

// SCAProof is what a device presents to confirm an invocation that
// declares `sca` (FN-020, SEC-027).
type SCAProof struct {
	Function    string
	Version     int32
	Input       []byte
	ChallengeID string
	Signature   []byte
}

// SCAChallenge is a challenge for the device to sign (SEC-027): it signs
// Bytes followed by Summary.
type SCAChallenge struct {
	ID        string
	Bytes     []byte
	ExpiresAt time.Time
	// Summary is the canonical JSON the challenge is bound to: what the
	// user is shown, with the function, the version and the input hash.
	Summary []byte
}

// SCAVerifier issues and verifies the strong customer authentication of
// an invocation. A function invocation is a transaction whose operation is
// the function and its version and whose bound body is the input
// (SEC-027, SEC-028); *gateway.SCA backs it through the server package.
type SCAVerifier interface {
	// CreateInvocationChallenge issues the challenge for an input, given
	// by its SHA-256, and the summary the device shows the user.
	CreateInvocationChallenge(ctx context.Context, d device.Identity, function string, version int32, inputSHA256, summary []byte, lifetime time.Duration) (SCAChallenge, error)
	// VerifyInvocation spends the challenge a call names and checks the
	// signature and that the input is the one the user confirmed.
	VerifyInvocation(ctx context.Context, d device.Identity, p SCAProof, lifetime time.Duration) error
}

// Users verifies the user token a device presents and returns the user's
// claims as JSON (SEC-026).
type Users interface {
	VerifyUser(ctx context.Context, token string, d device.Identity, function string) ([]byte, error)
}

// Keys remembers the result of a keyed invocation; *idempotency.Store is
// one (FN-021).
type Keys interface {
	Begin(ctx context.Context, c idempotency.Call) ([]byte, bool, error)
	Complete(ctx context.Context, c idempotency.Call, response []byte) error
	Abandon(ctx context.Context, c idempotency.Call) error
}

// LogMeta says which invocation produced logs.
type LogMeta struct {
	OrganizationID, AppID, EnvironmentID string
	Function                             string
	Version                              int32
	Depth                                uint32
	Traceparent                          string
}

// LogLine is one line a function logged.
type LogLine struct {
	Level   uint32
	Message string
	Time    time.Time
}

// FunctionLogSink receives the logs of every finished invocation; the
// observability milestone stores them (FN-032).
type FunctionLogSink interface { //nolint:revive // the name the observability milestone persists logs under
	Record(ctx context.Context, meta LogMeta, logs []LogLine)
}

// InvokerOptions configures an Invoker.
type InvokerOptions struct {
	Declarations Declarations
	Resolver     Resolver
	Pins         PinChecker
	Environment  Environment
	Grants       *Grants
	Runner       Runner
	Keys         Keys
	// Limits returns the limits of an app (LIM-001).
	Limits func(ctx context.Context, organizationID, appID string) (limits.Set, error)
	// Count increments a counter in the shared cache and returns its new
	// value; nil leaves the rates unlimited.
	Count func(ctx context.Context, key string, window time.Duration) (int64, error)
	// DefaultAssurance returns the environment's defaultFunctionAssurance
	// setting (SEC-007).
	DefaultAssurance func(ctx context.Context, appID, environmentID string) (string, error)
	// Attestations, SCA and Users may be nil; a function that needs what
	// is missing is then refused, never run without it.
	Attestations Attestations
	SCA          SCAVerifier
	Users        Users
	// Logs may be nil, which drops the logs.
	Logs FunctionLogSink
	// Observer may be nil, which drops the invocation records (FN-032).
	Observer InvocationObserver
	// Metrics may be nil, which counts nothing (FN-032).
	Metrics InvokeMetrics
	// Now is the clock of the durations; nil uses time.Now.
	Now func() time.Time
}

// Invoker runs server functions for devices and for other functions. It
// is safe for concurrent use: it holds no state beyond its options.
type Invoker struct{ o InvokerOptions }

// NewInvoker returns an invoker.
func NewInvoker(o InvokerOptions) (*Invoker, error) {
	if o.Declarations == nil || o.Resolver == nil || o.Pins == nil || o.Environment == nil || o.Grants == nil || o.Runner == nil ||
		o.Keys == nil || o.Limits == nil || o.DefaultAssurance == nil {
		return nil, errors.New("function: an invoker needs declarations, a resolver, pins, environment variables, grants, a runner, idempotency keys, limits and the default assurance")
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return &Invoker{o: o}, nil
}

// Caller is the device that invokes, as authenticated for the request.
type Caller struct {
	Device device.Identity
	// UserToken is the token ExchangeUserToken returned, when the call
	// acts for a user (SEC-026).
	UserToken string
	// Traceparent is the W3C trace context of the request (FN-032).
	Traceparent string
	// ProofID is the jti of the request's DPoP proof, which the replay
	// check has made single-use; the platform evidence of an invocation
	// is bound to it (FN-020).
	ProofID string
}

// Call is one invocation a device asks for (FN-020).
type Call struct {
	Function       string
	Version        int32
	Input          []byte
	IdempotencyKey string
	ChallengeID    string
	Signature      []byte
	// AppAttestAssertion is an iOS device's assertion over the invocation's
	// binding, and PlayIntegrityToken an Android device's token bound to
	// it; the function declares `attestation` to require one (FN-020,
	// SEC-003, SEC-004).
	AppAttestAssertion []byte
	PlayIntegrityToken string
}

// BusinessError is an error the function declared itself (FN-022).
type BusinessError struct {
	Code       string          `json:"code"`
	MessageKey string          `json:"messageKey"`
	Params     json.RawMessage `json:"params,omitempty"`
}

// Result is what a function answered: an output or a business error.
type Result struct {
	Output   []byte
	Business *BusinessError
	Version  int32
	// Replayed is true when the answer is the stored one (FN-021).
	Replayed bool
}

// Invoke runs a server function for a device (FN-020). Every check runs
// before the module does, cheapest first: the device's rate, the pin by an
// active release, the placement, the assurance, the platform evidence of
// the request, the idempotency key, then SCA. A device-placed version is refused, and no
// call accepts a result from a device (FN-054).
func (i *Invoker) Invoke(ctx context.Context, c Caller, call Call) (Result, error) {
	start := i.o.Now()
	res, err := i.invoke(ctx, c, call)
	if i.o.Metrics != nil {
		i.o.Metrics.Invoked(resultName(res, err), i.o.Now().Sub(start), err == nil && res.Replayed)
	}
	return res, err
}

// invoke is Invoke without the bookkeeping.
func (i *Invoker) invoke(ctx context.Context, c Caller, call Call) (Result, error) {
	d := c.Device
	lim, err := i.o.Limits(ctx, d.OrganizationID, d.AppID)
	if err != nil {
		return Result{}, err
	}
	if most := lim.Get(limits.FunctionInputSize); int64(len(call.Input)) > most {
		return Result{}, plxerr.New(plxerr.FunctionInputTooLarge, "the input is at most function.inputSize = %d bytes", most)
	}
	if err := i.rate(ctx, "rate:fn:device:"+d.DeviceID, lim.Get(limits.FunctionInvocationsPerMinutePerDevice)); err != nil {
		return Result{}, err
	}
	inv, err := i.authorised(ctx, d, call)
	if err != nil {
		return Result{}, err
	}
	if err := i.attested(ctx, c, call, inv.Declaration); err != nil {
		return Result{}, err
	}
	for _, r := range []struct {
		key   string
		limit int64
	}{
		{"rate:fn:app:" + d.AppID, lim.Get(limits.FunctionInvocationsPerMinutePerApp)},
		{"rate:fn:" + d.EnvironmentID + ":" + call.Function, lim.Get(limits.FunctionInvocationsPerMinute)},
	} {
		if err := i.rate(ctx, r.key, r.limit); err != nil {
			return Result{}, err
		}
	}
	return i.keyed(ctx, c, call, inv, lim)
}

// rate counts one use of a key and refuses past the limit (FN-024). While
// the counter is unavailable the limit is not enforced, as everywhere
// else (ADR-0007).
func (i *Invoker) rate(ctx context.Context, key string, limit int64) error {
	if i.o.Count == nil {
		return nil
	}
	n, err := i.o.Count(ctx, key, rateWindow)
	if err == nil && n > limit {
		return plxerr.New(plxerr.FunctionRateLimited, "more than %d invocations in %s", limit, rateWindow)
	}
	return nil
}

// authorised checks everything that needs neither the idempotency record
// nor the signature: the pin, the placement and the assurance.
func (i *Invoker) authorised(ctx context.Context, d device.Identity, call Call) (Invocable, error) {
	if call.Version <= 0 || ValidateName(call.Function) != nil {
		return Invocable{}, plxerr.New(plxerr.FunctionNotFound, "no such function version")
	}
	notPinned := plxerr.New(plxerr.FunctionNotFound, "no active release of this app pins version %d of the function %s", call.Version, call.Function)
	pin, found, err := i.o.Pins.Pinned(ctx, d.OrganizationID, d.AppID, d.EnvironmentID, call.Function, call.Version)
	if err != nil {
		return Invocable{}, err //nolint:wrapcheck // a domain error
	}
	if !found || pin.Placement != PlacementServer {
		return Invocable{}, notPinned
	}
	inv, err := i.o.Declarations.LoadInvocable(ctx, d.OrganizationID, d.AppID, d.EnvironmentID, call.Function, call.Version)
	if err != nil {
		return Invocable{}, err //nolint:wrapcheck // a domain error
	}
	if inv.Version.Placement != PlacementServer || !slices.Equal(pin.ModuleSHA256, inv.Version.ModuleSHA256) {
		return Invocable{}, notPinned
	}
	if err := i.checkAssurance(ctx, d, inv.Declaration); err != nil {
		return Invocable{}, err
	}
	return inv, nil
}

// attested requires a function that declares `attestation` to carry fresh
// platform evidence bound to this request: an App Attest assertion or a
// Play Integrity token over the function, the version, the input and the
// DPoP proof's identifier (FN-020, SEC-003, SEC-004). A function that does
// not declare it needs none.
func (i *Invoker) attested(ctx context.Context, c Caller, call Call, decl Declaration) error {
	if !decl.Attestation {
		return nil
	}
	if i.o.Attestations == nil {
		return plxerr.New(plxerr.AttestationUnavailable, "this server cannot verify the attestation of a request")
	}
	if c.ProofID == "" {
		return plxerr.New(plxerr.AttestationFailed, "the request carries no DPoP proof to bind the attestation to")
	}
	return i.o.Attestations.VerifyInvocationEvidence(ctx, c.Device, device.InvocationEvidence{ //nolint:wrapcheck // a domain error
		Binding:            binding.InvocationHash(call.Function, uint32(call.Version), call.Input, c.ProofID), //nolint:gosec // versions are positive
		AppAttestAssertion: call.AppAttestAssertion,
		PlayIntegrityToken: call.PlayIntegrityToken,
	})
}

// checkAssurance requires the assurance level the function declares, or
// the environment's defaultFunctionAssurance when it declares none
// (FN-020, SEC-007).
func (i *Invoker) checkAssurance(ctx context.Context, d device.Identity, decl Declaration) error {
	need := decl.Assurance
	if need == "" {
		var err error
		if need, err = i.o.DefaultAssurance(ctx, d.AppID, d.EnvironmentID); err != nil {
			return err
		}
	}
	want := device.Level(need).Rank()
	if want < 0 {
		return fmt.Errorf("function: %q is not an assurance level", need)
	}
	if device.Level(d.Assurance).Rank() < want {
		return plxerr.New(plxerr.AssuranceInsufficient, "this function needs assurance level %s", need)
	}
	return nil
}

// keyed runs the call under its idempotency key, when it has one: a repeat
// of the same input returns the stored result without running again for
// function.idempotencyWindow, and the same key with another input is
// PLX-7011 (FN-021).
func (i *Invoker) keyed(ctx context.Context, c Caller, call Call, inv Invocable, lim limits.Set) (Result, error) {
	if call.IdempotencyKey == "" {
		return i.confirmed(ctx, c, call, inv, lim)
	}
	sum := sha256.Sum256(call.Input)
	key := idempotency.Call{
		Subject: "function:" + c.Device.DeviceID + ":" + call.Function, Key: call.IdempotencyKey, Procedure: "FunctionService/Invoke",
		Request: append([]byte(fmt.Sprintf("%d:", call.Version)), sum[:]...),
		Window:  time.Duration(lim.Get(limits.FunctionIdempotencyWindow)) * time.Millisecond,
	}
	stored, done, err := i.o.Keys.Begin(ctx, key)
	if err != nil {
		if got, _ := plxerr.CodeOf(err); got == plxerr.IdempotencyConflict {
			return Result{}, plxerr.New(plxerr.IdempotencyKeyConflict, "this idempotency key was used for another input, or a call with it is still running")
		}
		return Result{}, err //nolint:wrapcheck // a domain error
	}
	if done {
		res, err := decodeStored(stored)
		res.Version, res.Replayed = call.Version, true
		if err == nil {
			i.observe(ctx, runSpec{org: c.Device.OrganizationID, app: c.Device.AppID, env: c.Device.EnvironmentID, inv: inv}, resultName(res, nil), 0, 0, true)
		}
		return res, err
	}
	res, err := i.confirmed(ctx, c, call, inv, lim)
	if err != nil {
		_ = i.o.Keys.Abandon(ctx, key)
		return Result{}, err
	}
	// The function ran; failing the call now would invite a retry that
	// runs it again. The key stays claimed, so a retry is refused instead.
	if stored, eerr := encodeStored(res); eerr == nil {
		_ = i.o.Keys.Complete(ctx, key, stored)
	}
	return res, nil
}

// confirmed checks the SCA signature when the function declares sca, then
// runs the function (FN-020, SEC-027).
func (i *Invoker) confirmed(ctx context.Context, c Caller, call Call, inv Invocable, lim limits.Set) (Result, error) {
	if inv.Declaration.SCA {
		if call.ChallengeID == "" {
			return Result{}, plxerr.New(plxerr.SCAChallengeInvalid, "this function needs a confirmed transaction")
		}
		if i.o.SCA == nil {
			return Result{}, plxerr.New(plxerr.TransactionRequiresGateway, "this server cannot verify transactions")
		}
		if err := i.o.SCA.VerifyInvocation(ctx, c.Device, SCAProof{
			Function: call.Function, Version: call.Version, Input: call.Input, ChallengeID: call.ChallengeID, Signature: call.Signature,
		}, challengeLifetime(lim)); err != nil {
			return Result{}, err //nolint:wrapcheck // a domain error
		}
	}
	claims, err := i.userClaims(ctx, c, call.Function, inv.Declaration)
	if err != nil {
		return Result{}, err
	}
	spec := runSpec{
		org: c.Device.OrganizationID, app: c.Device.AppID, env: c.Device.EnvironmentID, inv: inv, input: call.Input,
		assurance: c.Device.Assurance, claims: claims, traceparent: c.Traceparent,
	}
	start := i.o.Now()
	resp, err := i.run(ctx, lim, spec)
	took := i.o.Now().Sub(start)
	if err != nil {
		i.observe(ctx, spec, resultName(Result{}, err), took, 0, false)
		return Result{}, err
	}
	res, err := resultOf(resp, call.Version)
	i.observe(ctx, spec, resultName(res, err), took, resp.GetMemoryPeakBytes(), false)
	return res, err
}

// observe hands a finished run, or a replay, to the observer (FN-032).
func (i *Invoker) observe(ctx context.Context, s runSpec, result string, took time.Duration, memoryPeak uint64, replayed bool) {
	if i.o.Observer == nil {
		return
	}
	v := s.inv.Version
	i.o.Observer.Observe(ctx, Observation{
		OrganizationID: s.org, AppID: s.app, EnvironmentID: s.env, Function: v.Name, Version: v.Version,
		Result: result, Duration: took, MemoryPeakBytes: memoryPeak, Replayed: replayed,
	})
}

// resultName is the label an invocation is counted under: ok, business_error
// or the lower-case reason of the system error (FN-032).
func resultName(r Result, err error) string {
	switch {
	case err != nil:
		code, _ := plxerr.CodeOf(err)
		if def, ok := plxerr.Lookup(code); ok {
			return strings.ToLower(string(def.Reason))
		}
		return "internal_server_error"
	case r.Business != nil:
		return ResultBusinessError
	}
	return ResultOK
}

// challengeLifetime is sca.challengeLifetime of an app (LIM-001).
func challengeLifetime(lim limits.Set) time.Duration {
	return time.Duration(lim.Get(limits.ScaChallengeLifetime)) * time.Millisecond
}

// ChallengeRequest asks for the challenge of a function invocation.
type ChallengeRequest struct {
	Function string
	Version  int32
	// InputSHA256 is the SHA-256 of the input the user confirms.
	InputSHA256 []byte
	// Summary is the canonical JSON object of strings the device shows the
	// user.
	Summary []byte
}

// CreateChallenge issues the SCA challenge of a function invocation
// (FN-020, SEC-027). The function must be pinned, server-placed, within
// the device's assurance and declare `sca`; a call that confirms an
// invocation the function does not ask to confirm is a mistake the device
// learns of here rather than by a challenge nobody checks.
func (i *Invoker) CreateChallenge(ctx context.Context, d device.Identity, r ChallengeRequest) (SCAChallenge, error) {
	inv, err := i.authorised(ctx, d, Call{Function: r.Function, Version: r.Version})
	if err != nil {
		return SCAChallenge{}, err
	}
	if !inv.Declaration.SCA {
		return SCAChallenge{}, plxerr.New(plxerr.PreconditionFailed, "the function %s does not declare sca", r.Function)
	}
	if i.o.SCA == nil {
		return SCAChallenge{}, plxerr.New(plxerr.TransactionRequiresGateway, "this server cannot verify transactions")
	}
	lim, err := i.o.Limits(ctx, d.OrganizationID, d.AppID)
	if err != nil {
		return SCAChallenge{}, err
	}
	c, err := i.o.SCA.CreateInvocationChallenge(ctx, d, r.Function, r.Version, r.InputSHA256, r.Summary, challengeLifetime(lim))
	if err != nil {
		return SCAChallenge{}, err //nolint:wrapcheck // a domain error
	}
	return c, nil
}

// userClaims verifies the user token and returns the claims the function
// may read: none unless it declares the claims capability, in which case
// a token that does not verify refuses the call (SEC-026).
func (i *Invoker) userClaims(ctx context.Context, c Caller, name string, decl Declaration) ([]byte, error) {
	if !decl.Capabilities.Claims || c.UserToken == "" {
		return nil, nil
	}
	if i.o.Users == nil {
		return nil, plxerr.New(plxerr.UserTokenInvalid, "this server cannot verify user tokens")
	}
	claims, err := i.o.Users.VerifyUser(ctx, c.UserToken, c.Device, name)
	if err != nil {
		return nil, err //nolint:wrapcheck // a domain error
	}
	return claims, nil
}

// runSpec is one invocation on the runner.
type runSpec struct {
	org, app, env string
	inv           Invocable
	input         []byte
	depth         uint32
	assurance     string
	claims        []byte
	traceparent   string
}

// run issues the invocation's grant and calls the runner with the limits
// the declaration and the registry allow (FN-011, ADR-0064).
func (i *Invoker) run(ctx context.Context, lim limits.Set, s runSpec) (*fnpb.FunctionRunnerServiceInvokeResponse, error) {
	decl, v := s.inv.Declaration, s.inv.Version
	lm := invocationLimits(lim, decl.Limits)
	timeout := time.Duration(lm.TimeoutMs) * time.Millisecond
	env, err := i.o.Environment.EnvironmentVariables(ctx, s.org, s.app, s.env, decl.Env)
	if err != nil {
		return nil, err //nolint:wrapcheck // a domain error
	}
	if !decl.Capabilities.Claims {
		s.claims = nil
	}
	grant, _, err := i.o.Grants.Issue(GrantClaims{
		OrganizationID: s.org, AppID: s.app, EnvironmentID: s.env, Function: v.Name, Version: v.Version,
		ModuleSHA256: HexSHA256(v.ModuleSHA256), Depth: s.depth, Assurance: s.assurance, UserClaims: s.claims,
	}, timeout)
	if err != nil {
		return nil, fmt.Errorf("function: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout+GrantMargin)
	defer cancel()
	resp, err := i.o.Runner.Run(ctx, &fnpb.FunctionRunnerServiceInvokeRequest{
		Function: v.Name, Version: uint32(v.Version), ModuleSha256: HexSHA256(v.ModuleSHA256), Input: s.input, //nolint:gosec // versions are positive
		Limits: lm.proto(), Capabilities: capabilitiesOf(decl.Capabilities), Claims: s.claims, Grant: string(grant), Depth: s.depth,
		Traceparent: s.traceparent, App: s.app, Environment: s.env, AssuranceLevel: s.assurance, Env: env,
	})
	if err != nil {
		return nil, err //nolint:wrapcheck // a domain error
	}
	i.record(ctx, s, resp)
	return resp, nil
}

// record hands the function's logs to the sink.
func (i *Invoker) record(ctx context.Context, s runSpec, resp *fnpb.FunctionRunnerServiceInvokeResponse) {
	if i.o.Logs == nil || len(resp.GetLogs()) == 0 {
		return
	}
	logs := make([]LogLine, len(resp.GetLogs()))
	for n, l := range resp.GetLogs() {
		logs[n] = LogLine{Level: l.GetLevel(), Message: l.GetMessage(), Time: time.Unix(0, l.GetTimeUnixNano()).UTC()}
	}
	v := s.inv.Version
	i.o.Logs.Record(ctx, LogMeta{
		OrganizationID: s.org, AppID: s.app, EnvironmentID: s.env, Function: v.Name, Version: v.Version, Depth: s.depth, Traceparent: s.traceparent,
	}, logs)
}

// limitSet is the limits one invocation runs under.
type limitSet struct {
	TimeoutMs, MemoryBytes, OutputBytes, HTTPCalls, CallDepth, LogBytes, InputBytes int64
}

// invocationLimits takes the registry's values and lets the declaration
// tighten them, never loosen them (FN-011, LIM-002).
func invocationLimits(lim limits.Set, d DeclaredLimits) limitSet {
	tighten := func(registry, declared int64) int64 {
		if declared > 0 {
			return min(registry, declared)
		}
		return registry
	}
	return limitSet{
		TimeoutMs:   tighten(lim.Get(limits.FunctionTimeout), d.TimeoutMs),
		MemoryBytes: tighten(lim.Get(limits.FunctionMemory), d.MemoryBytes),
		OutputBytes: tighten(lim.Get(limits.FunctionOutputSize), d.OutputBytes),
		HTTPCalls:   tighten(lim.Get(limits.FunctionHTTPCalls), d.HTTPCalls),
		CallDepth:   lim.Get(limits.FunctionCallDepth),
		LogBytes:    lim.Get(limits.FunctionLogBytes),
		InputBytes:  lim.Get(limits.FunctionInputSize),
	}
}

// proto is the limits on the internal protocol.
func (l limitSet) proto() *fnpb.InvocationLimits {
	return &fnpb.InvocationLimits{
		TimeoutMs: uint32(l.TimeoutMs), MemoryBytes: uint64(l.MemoryBytes), OutputBytes: uint64(l.OutputBytes), //nolint:gosec // registry values are positive and bounded
		HttpCalls: uint32(l.HTTPCalls), CallDepth: uint32(l.CallDepth), LogBytes: uint64(l.LogBytes), InputBytes: uint64(l.InputBytes), //nolint:gosec // as above
	}
}

// capabilitiesOf is the declaration's capabilities on the internal
// protocol: what the runner enforces, and nothing else (FN-012).
func capabilitiesOf(c Capabilities) *fnpb.Capabilities {
	return &fnpb.Capabilities{HttpDomains: c.HTTP, Kv: c.KV, Secrets: c.Secrets, Functions: c.Functions, Claims: c.Claims}
}

// resultOf reads the runner's answer.
func resultOf(resp *fnpb.FunctionRunnerServiceInvokeResponse, version int32) (Result, error) {
	if raw := resp.GetBusinessError(); raw != nil {
		var be BusinessError
		if err := json.Unmarshal(raw, &be); err != nil || be.Code == "" || be.MessageKey == "" {
			return Result{}, plxerr.New(plxerr.FunctionOutputInvalid, "the function's business error is malformed")
		}
		return Result{Business: &be, Version: version}, nil
	}
	return Result{Output: resp.GetOutput(), Version: version}, nil
}

// encodeStored is the form of a result in the idempotency record.
func encodeStored(r Result) ([]byte, error) {
	resp := &fnpb.FunctionRunnerServiceInvokeResponse{}
	if r.Business != nil {
		raw, err := json.Marshal(r.Business)
		if err != nil {
			return nil, fmt.Errorf("function: %w", err)
		}
		resp.Result = &fnpb.FunctionRunnerServiceInvokeResponse_BusinessError{BusinessError: raw}
	} else {
		resp.Result = &fnpb.FunctionRunnerServiceInvokeResponse_Output{Output: r.Output}
	}
	return proto.MarshalOptions{Deterministic: true}.Marshal(resp) //nolint:wrapcheck // proto errors are descriptive
}

// decodeStored reads a result from the idempotency record.
func decodeStored(b []byte) (Result, error) {
	resp := &fnpb.FunctionRunnerServiceInvokeResponse{}
	if err := proto.Unmarshal(b, resp); err != nil {
		return Result{}, fmt.Errorf("function: read a stored result: %w", err)
	}
	return resultOf(resp, 0)
}

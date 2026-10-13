// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/genproto/googleapis/rpc/errdetails"

	"github.com/nightCode42/plux3/backend/internal/device"
	"github.com/nightCode42/plux3/backend/internal/fnpb"
	"github.com/nightCode42/plux3/backend/internal/fnpb/fnpbconnect"
	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage/idempotency"
)

// fakeRunner stands in for the function runner role.
type fakeRunner struct {
	mu    sync.Mutex
	reqs  []*fnpb.FunctionRunnerServiceInvokeRequest
	reply func(*fnpb.FunctionRunnerServiceInvokeRequest) (*fnpb.FunctionRunnerServiceInvokeResponse, error)
}

func (f *fakeRunner) Run(_ context.Context, r *fnpb.FunctionRunnerServiceInvokeRequest) (*fnpb.FunctionRunnerServiceInvokeResponse, error) {
	f.mu.Lock()
	f.reqs = append(f.reqs, r)
	reply := f.reply
	f.mu.Unlock()
	if reply != nil {
		return reply(r)
	}
	return &fnpb.FunctionRunnerServiceInvokeResponse{Result: &fnpb.FunctionRunnerServiceInvokeResponse_Output{Output: []byte(`{"ok":true}`)}}, nil
}

func (f *fakeRunner) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.reqs)
}

func (f *fakeRunner) last() *fnpb.FunctionRunnerServiceInvokeRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reqs[len(f.reqs)-1]
}

// fakeCatalogue is the declarations, pins and resolver of a test.
type fakeCatalogue struct {
	versions map[string]function.Invocable
	pins     map[string]function.Pin
	env      map[string]string
	envAsked []string
}

func key(name string, version int32) string { return fmt.Sprintf("%s@%d", name, version) }

func (c *fakeCatalogue) LoadInvocable(_ context.Context, _, _, _, name string, version int32) (function.Invocable, error) {
	inv, ok := c.versions[key(name, version)]
	if !ok {
		return function.Invocable{}, plxerr.New(plxerr.FunctionNotFound, "no such version")
	}
	return inv, nil
}

func (c *fakeCatalogue) Pinned(_ context.Context, _, _, _, name string, version int32) (function.Pin, bool, error) {
	p, ok := c.pins[key(name, version)]
	return p, ok, nil
}

func (c *fakeCatalogue) ResolveVersion(_ context.Context, _, _, _, name, _ string) (function.Resolved, error) {
	var best function.Invocable
	for _, inv := range c.versions {
		if inv.Version.Name == name && inv.Version.Version > best.Version.Version {
			best = inv
		}
	}
	if best.Version.Name == "" {
		return function.Resolved{}, plxerr.New(plxerr.FunctionNotFound, "no such function")
	}
	return function.Resolved{Name: name, Version: best.Version.Version, ModuleSHA256: best.Version.ModuleSHA256, Placement: best.Version.Placement}, nil
}

func (c *fakeCatalogue) EnvironmentVariables(_ context.Context, _, _, _ string, names []string) (map[string]string, error) {
	c.envAsked = names
	return c.env, nil
}

// add registers a pinned server version with a declaration.
func (c *fakeCatalogue) add(name string, version int32, placement function.Placement, decl function.Declaration) {
	sum := sha256.Sum256([]byte(name + fmt.Sprint(version)))
	decl.Name = name
	c.versions[key(name, version)] = function.Invocable{
		Version:     function.Version{Name: name, Version: version, Placement: placement, ModuleSHA256: sum[:]},
		Declaration: decl,
	}
	c.pins[key(name, version)] = function.Pin{ModuleSHA256: sum[:], Placement: placement}
}

// memKeys is an in-memory idempotency store with the real one's rules.
type memKeys struct {
	mu      sync.Mutex
	records map[string]*memRecord
	windows []time.Duration
}

type memRecord struct {
	hash string
	done bool
	resp []byte
}

func (m *memKeys) Begin(_ context.Context, c idempotency.Call) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.records == nil {
		m.records = map[string]*memRecord{}
	}
	m.windows = append(m.windows, c.Window)
	id := c.Subject + "|" + c.Key
	hash := c.Procedure + "|" + string(c.Request)
	r, ok := m.records[id]
	if !ok {
		m.records[id] = &memRecord{hash: hash}
		return nil, false, nil
	}
	if r.hash != hash {
		return nil, false, plxerr.New(plxerr.IdempotencyConflict, "this idempotency key was used for a different request")
	}
	if !r.done {
		return nil, false, plxerr.New(plxerr.IdempotencyConflict, "a call with this idempotency key is still in progress")
	}
	return r.resp, true, nil
}

func (m *memKeys) Complete(_ context.Context, c idempotency.Call, resp []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.records[c.Subject+"|"+c.Key]
	r.done, r.resp = true, resp
	return nil
}

func (m *memKeys) Abandon(_ context.Context, c idempotency.Call) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.records, c.Subject+"|"+c.Key)
	return nil
}

// counter counts uses of a key in the current test window.
type counter struct {
	mu sync.Mutex
	n  map[string]int64
}

func (c *counter) count(_ context.Context, key string, _ time.Duration) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.n == nil {
		c.n = map[string]int64{}
	}
	c.n[key]++
	return c.n[key], nil
}

type fakeAttest struct{ err error }

func (f fakeAttest) RequireFreshAttestation(context.Context, device.Identity) error { return f.err }

type fakeSCA struct {
	err    error
	proofs []function.SCAProof
}

func (f *fakeSCA) VerifyInvocation(_ context.Context, _ device.Identity, p function.SCAProof) error {
	f.proofs = append(f.proofs, p)
	return f.err
}

type fakeUsers struct {
	claims []byte
	err    error
	asked  []string
}

func (f *fakeUsers) VerifyUser(_ context.Context, token string, _ device.Identity, name string) ([]byte, error) {
	f.asked = append(f.asked, token+"@"+name)
	return f.claims, f.err
}

type logSink struct {
	metas []function.LogMeta
	logs  [][]function.LogLine
}

func (s *logSink) Record(_ context.Context, m function.LogMeta, l []function.LogLine) {
	s.metas, s.logs = append(s.metas, m), append(s.logs, l)
}

// rig is an invoker with fakes.
type rig struct {
	inv     *function.Invoker
	cat     *fakeCatalogue
	runner  *fakeRunner
	keys    *memKeys
	grants  *function.Grants
	count   *counter
	sca     *fakeSCA
	users   *fakeUsers
	logs    *logSink
	limits  limits.Set
	dev     device.Identity
	now     time.Time
	assured string
}

func newRig(t *testing.T, mutate func(*rig, *function.InvokerOptions)) *rig {
	t.Helper()
	r := &rig{
		cat:    &fakeCatalogue{versions: map[string]function.Invocable{}, pins: map[string]function.Pin{}, env: map[string]string{"REGION": "eu"}},
		runner: &fakeRunner{}, keys: &memKeys{}, count: &counter{}, sca: &fakeSCA{}, users: &fakeUsers{claims: []byte(`{"sub":"u1"}`)}, logs: &logSink{},
		limits: limits.Defaults(), now: time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC), assured: "AL1",
		dev: device.Identity{DeviceID: "dev1", OrganizationID: "org", AppID: "app", EnvironmentID: "env", Assurance: "AL2"},
	}
	var err error
	if r.grants, err = function.NewGrants(make([]byte, 32), func() time.Time { return r.now }, nil); err != nil {
		t.Fatal(err)
	}
	o := function.InvokerOptions{
		Declarations: r.cat, Resolver: r.cat, Pins: r.cat, Environment: r.cat, Grants: r.grants, Runner: r.runner, Keys: r.keys,
		Limits: func(context.Context, string, string) (limits.Set, error) { return r.limits, nil },
		Count:  r.count.count, SCA: r.sca, Users: r.users, Logs: r.logs, Attestations: fakeAttest{},
		DefaultAssurance: func(context.Context, string, string) (string, error) { return r.assured, nil },
	}
	if mutate != nil {
		mutate(r, &o)
	}
	if r.inv, err = function.NewInvoker(o); err != nil {
		t.Fatal(err)
	}
	return r
}

func (r *rig) tighten(t *testing.T, k limits.Key, v int64) {
	t.Helper()
	l, err := r.limits.Tighten(k, limits.ScopeApp, v)
	if err != nil {
		t.Fatal(err)
	}
	r.limits = l
}

func (r *rig) call(name string, version int32, input string) (function.Result, error) {
	return r.inv.Invoke(context.Background(), function.Caller{Device: r.dev, Traceparent: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"},
		function.Call{Function: name, Version: version, Input: []byte(input)})
}

const charge = "payments.charge"

// Verifies: FN-020, FN-006.
// The module runs only for a version an active release of the device's app
// and environment pins; every other version, whatever its state, is
// PLX-7012 and reaches no runner.
func TestInvokeNeedsAPin_FN_020(t *testing.T) {
	t.Parallel()
	r := newRig(t, nil)
	r.cat.add(charge, 1, function.PlacementServer, function.Declaration{})
	r.cat.add(charge, 2, function.PlacementServer, function.Declaration{})
	delete(r.cat.pins, key(charge, 2))
	// A pin of another module than the version has.
	r.cat.add("payments.refund", 1, function.PlacementServer, function.Declaration{})
	r.cat.pins[key("payments.refund", 1)] = function.Pin{ModuleSHA256: make([]byte, 32), Placement: function.PlacementServer}

	if res, err := r.call(charge, 1, `{}`); err != nil || string(res.Output) != `{"ok":true}` || res.Version != 1 || res.Replayed {
		t.Fatalf("a pinned version: %+v %v", res, err)
	}
	for name, c := range map[string]struct {
		fn      string
		version int32
	}{"unpinned": {charge, 2}, "unknown": {"payments.nope", 1}, "version zero": {charge, 0}, "bad name": {"Bad", 1}, "another module": {"payments.refund", 1}} {
		before := r.runner.calls()
		if _, err := r.call(c.fn, c.version, `{}`); code(err) != plxerr.FunctionNotFound {
			t.Errorf("%s: %v", name, err)
		}
		if r.runner.calls() != before {
			t.Errorf("%s reached the runner", name)
		}
	}
}

// Verifies: FN-020, FN-054.
// A device-placed version is never run by the server for a device, and the
// API has no way to hand the server a result computed on a device.
func TestInvokeServerPlacedOnly_FN_054(t *testing.T) {
	t.Parallel()
	r := newRig(t, nil)
	r.cat.add("device.calc", 1, function.PlacementDevice, function.Declaration{})
	if _, err := r.call("device.calc", 1, `{}`); code(err) != plxerr.FunctionNotFound {
		t.Errorf("a device-placed version: %v", err)
	}
	// Placement disagreeing between the pin and the version is refused too.
	r.cat.add("mixed.calc", 1, function.PlacementServer, function.Declaration{})
	r.cat.pins[key("mixed.calc", 1)] = function.Pin{ModuleSHA256: r.cat.versions[key("mixed.calc", 1)].Version.ModuleSHA256, Placement: function.PlacementDevice}
	if _, err := r.call("mixed.calc", 1, `{}`); code(err) != plxerr.FunctionNotFound {
		t.Errorf("a pin that says device: %v", err)
	}
	if r.runner.calls() != 0 {
		t.Error("the runner ran a device function")
	}
}

// Verifies: FN-020, SEC-007.
// The function's declared assurance, or the environment's default when it
// declares none, must be held by the device (PLX-6002).
func TestInvokeAssurance_FN_020(t *testing.T) {
	t.Parallel()
	r := newRig(t, nil)
	r.cat.add("declared.high", 1, function.PlacementServer, function.Declaration{Assurance: "AL3"})
	r.cat.add("declared.low", 1, function.PlacementServer, function.Declaration{Assurance: "AL0"})
	r.cat.add("default.level", 1, function.PlacementServer, function.Declaration{})
	r.dev.Assurance = "AL2"
	if _, err := r.call("declared.high", 1, `{}`); code(err) != plxerr.AssuranceInsufficient {
		t.Errorf("AL2 for AL3: %v", err)
	}
	if _, err := r.call("declared.low", 1, `{}`); err != nil {
		t.Errorf("AL2 for AL0: %v", err)
	}
	if _, err := r.call("default.level", 1, `{}`); err != nil {
		t.Errorf("AL2 for the default AL1: %v", err)
	}
	r.assured = "AL3"
	if _, err := r.call("default.level", 1, `{}`); code(err) != plxerr.AssuranceInsufficient {
		t.Errorf("AL2 for a default of AL3: %v", err)
	}
	r.assured = "ALx"
	if _, err := r.call("default.level", 1, `{}`); err == nil {
		t.Error("an unknown default level let the call through")
	}
	r.dev.Assurance = ""
	r.assured = "AL0"
	if _, err := r.call("default.level", 1, `{}`); code(err) != plxerr.AssuranceInsufficient {
		t.Errorf("a device with no level: %v", err)
	}
}

// Verifies: FN-020, SEC-004.
// A function that declares attestation runs only for a device that
// attested recently; with no way to check, it is refused.
func TestInvokeAttestation_FN_020(t *testing.T) {
	t.Parallel()
	stale := plxerr.New(plxerr.ReattestationRequired, "stale")
	r := newRig(t, func(_ *rig, o *function.InvokerOptions) { o.Attestations = fakeAttest{err: stale} })
	r.cat.add("attested.fn", 1, function.PlacementServer, function.Declaration{Attestation: true})
	r.cat.add("plain.fn", 1, function.PlacementServer, function.Declaration{})
	if _, err := r.call("attested.fn", 1, `{}`); code(err) != plxerr.ReattestationRequired {
		t.Errorf("a stale attestation: %v", err)
	}
	if _, err := r.call("plain.fn", 1, `{}`); err != nil {
		t.Errorf("a function without the flag: %v", err)
	}
	none := newRig(t, func(_ *rig, o *function.InvokerOptions) { o.Attestations = nil })
	none.cat.add("attested.fn", 1, function.PlacementServer, function.Declaration{Attestation: true})
	if _, err := none.call("attested.fn", 1, `{}`); code(err) != plxerr.ReattestationRequired {
		t.Errorf("no checker: %v", err)
	}
	ok := newRig(t, nil)
	ok.cat.add("attested.fn", 1, function.PlacementServer, function.Declaration{Attestation: true})
	if _, err := ok.call("attested.fn", 1, `{}`); err != nil {
		t.Errorf("a fresh attestation: %v", err)
	}
}

// Verifies: FN-020, SEC-027.
// A function that declares sca runs only with a signature the verifier
// accepts, over the function, version and input the call names.
func TestInvokeSCA_FN_020(t *testing.T) {
	t.Parallel()
	r := newRig(t, nil)
	r.cat.add("sca.fn", 1, function.PlacementServer, function.Declaration{SCA: true})
	ctx := context.Background()
	c := function.Caller{Device: r.dev}
	if _, err := r.inv.Invoke(ctx, c, function.Call{Function: "sca.fn", Version: 1, Input: []byte(`{}`)}); code(err) != plxerr.SCAChallengeInvalid {
		t.Errorf("no challenge: %v", err)
	}
	r.sca.err = plxerr.New(plxerr.SCASignatureInvalid, "bad")
	if _, err := r.inv.Invoke(ctx, c, function.Call{Function: "sca.fn", Version: 1, Input: []byte(`{}`), ChallengeID: "c1", Signature: []byte("s")}); code(err) != plxerr.SCASignatureInvalid {
		t.Errorf("a bad signature: %v", err)
	}
	if r.runner.calls() != 0 {
		t.Error("the runner ran without SCA")
	}
	r.sca.err = nil
	if _, err := r.inv.Invoke(ctx, c, function.Call{Function: "sca.fn", Version: 1, Input: []byte(`{"a":1}`), ChallengeID: "c2", Signature: []byte("s")}); err != nil {
		t.Fatalf("a good signature: %v", err)
	}
	if p := r.sca.proofs[len(r.sca.proofs)-1]; p.Function != "sca.fn" || p.Version != 1 || string(p.Input) != `{"a":1}` || p.ChallengeID != "c2" {
		t.Errorf("proof = %+v", p)
	}
	none := newRig(t, func(_ *rig, o *function.InvokerOptions) { o.SCA = nil })
	none.cat.add("sca.fn", 1, function.PlacementServer, function.Declaration{SCA: true})
	if _, err := none.inv.Invoke(ctx, function.Caller{Device: none.dev}, function.Call{Function: "sca.fn", Version: 1, ChallengeID: "c", Signature: []byte("s")}); code(err) != plxerr.TransactionRequiresGateway {
		t.Errorf("no verifier: %v", err)
	}
}

// Verifies: FN-020.
// The input is bounded by function.inputSize (PLX-7022) before anything
// else is read.
func TestInvokeInputSize_FN_020(t *testing.T) {
	t.Parallel()
	r := newRig(t, nil)
	r.tighten(t, limits.FunctionInputSize, 7)
	r.cat.add(charge, 1, function.PlacementServer, function.Declaration{})
	if _, err := r.call(charge, 1, `{"a":1}`); err != nil {
		t.Errorf("an input at the limit: %v", err)
	}
	if _, err := r.call(charge, 1, `{"a":12}`); code(err) != plxerr.FunctionInputTooLarge {
		t.Errorf("an input above the limit: %v", err)
	}
}

func (r *rig) keyed(name, input, k string) (function.Result, error) {
	return r.inv.Invoke(context.Background(), function.Caller{Device: r.dev}, function.Call{Function: name, Version: 1, Input: []byte(input), IdempotencyKey: k})
}

// Verifies: FN-021.
// A retry with the same key and input returns the stored result without
// running the function again, for function.idempotencyWindow; another input
// under the key is PLX-7011; keys belong to a device and a function; a
// failed run releases its key.
func TestInvokeIdempotency_FN_021(t *testing.T) {
	t.Parallel()
	r := newRig(t, nil)
	r.cat.add(charge, 1, function.PlacementServer, function.Declaration{})
	r.cat.add("payments.refund", 1, function.PlacementServer, function.Declaration{})
	n := 0
	r.runner.reply = func(*fnpb.FunctionRunnerServiceInvokeRequest) (*fnpb.FunctionRunnerServiceInvokeResponse, error) {
		n++
		return &fnpb.FunctionRunnerServiceInvokeResponse{Result: &fnpb.FunctionRunnerServiceInvokeResponse_Output{Output: []byte(fmt.Sprintf(`{"run":%d}`, n))}}, nil
	}
	first, err := r.keyed(charge, `{"a":1}`, "k1")
	if err != nil || string(first.Output) != `{"run":1}` || first.Replayed {
		t.Fatalf("first = %+v %v", first, err)
	}
	again, err := r.keyed(charge, `{"a":1}`, "k1")
	if err != nil || string(again.Output) != `{"run":1}` || !again.Replayed || again.Version != 1 || r.runner.calls() != 1 {
		t.Fatalf("replay = %+v %v, runner calls %d", again, err, r.runner.calls())
	}
	if _, err := r.keyed(charge, `{"a":2}`, "k1"); code(err) != plxerr.IdempotencyKeyConflict {
		t.Errorf("another input under the key: %v", err)
	}
	if r.runner.calls() != 1 {
		t.Error("a conflicting call ran")
	}
	if other, err := r.keyed("payments.refund", `{"a":1}`, "k1"); err != nil || other.Replayed || r.runner.calls() != 2 {
		t.Errorf("the same key for another function: %+v %v", other, err)
	}
	r.dev.DeviceID = "dev2"
	if other, err := r.keyed(charge, `{"a":1}`, "k1"); err != nil || other.Replayed || r.runner.calls() != 3 {
		t.Errorf("the same key from another device: %+v %v", other, err)
	}
	for _, w := range r.keys.windows {
		if w != 24*time.Hour {
			t.Errorf("window = %v, want function.idempotencyWindow", w)
		}
	}

	failing := errors.New("runner down")
	r.runner.reply = func(*fnpb.FunctionRunnerServiceInvokeRequest) (*fnpb.FunctionRunnerServiceInvokeResponse, error) {
		return nil, failing
	}
	if _, err := r.keyed(charge, `{"a":9}`, "k2"); err == nil {
		t.Fatal("a failing run succeeded")
	}
	r.runner.reply = nil
	if res, err := r.keyed(charge, `{"a":9}`, "k2"); err != nil || res.Replayed {
		t.Errorf("a retry after a failure should run: %+v %v", res, err)
	}
}

// Verifies: FN-021, FN-022.
// A business error is a result: it is returned in the response and
// replayed like an output.
func TestInvokeBusinessErrorIsAResult_FN_022(t *testing.T) {
	t.Parallel()
	r := newRig(t, nil)
	r.cat.add(charge, 1, function.PlacementServer, function.Declaration{})
	r.runner.reply = func(*fnpb.FunctionRunnerServiceInvokeRequest) (*fnpb.FunctionRunnerServiceInvokeResponse, error) {
		return &fnpb.FunctionRunnerServiceInvokeResponse{Result: &fnpb.FunctionRunnerServiceInvokeResponse_BusinessError{
			BusinessError: []byte(`{"code":"insufficient_funds","messageKey":"pay.funds","params":{"missing":"4.20"}}`),
		}}, nil
	}
	for i, wantReplay := range []bool{false, true} {
		res, err := r.keyed(charge, `{}`, "kb")
		if err != nil || res.Business == nil || res.Output != nil || res.Replayed != wantReplay {
			t.Fatalf("attempt %d = %+v %v", i, res, err)
		}
		if b := res.Business; b.Code != "insufficient_funds" || b.MessageKey != "pay.funds" || string(b.Params) != `{"missing":"4.20"}` {
			t.Errorf("business error = %+v", b)
		}
	}
	r.runner.reply = func(*fnpb.FunctionRunnerServiceInvokeRequest) (*fnpb.FunctionRunnerServiceInvokeResponse, error) {
		return &fnpb.FunctionRunnerServiceInvokeResponse{Result: &fnpb.FunctionRunnerServiceInvokeResponse_BusinessError{BusinessError: []byte(`{"code":"x"}`)}}, nil
	}
	if _, err := r.call(charge, 1, `{}`); code(err) != plxerr.FunctionOutputInvalid {
		t.Errorf("a malformed business error: %v", err)
	}
}

// runnerFailure is a client whose Invoke fails as the runner's would.
type runnerFailure struct {
	fnpbconnect.FunctionRunnerServiceClient
	err error
}

func (f runnerFailure) Invoke(context.Context, *connect.Request[fnpb.FunctionRunnerServiceInvokeRequest]) (*connect.Response[fnpb.FunctionRunnerServiceInvokeResponse], error) {
	return nil, f.err
}

// Verifies: FN-022.
// A system error carries the runner's PLX code and the registry's generic
// title; the text the runner put in its error, which may quote what the
// function trapped on, does not survive.
func TestRunnerSystemErrorsAreGeneric_FN_022(t *testing.T) {
	t.Parallel()
	trap := connect.NewError(connect.CodeAborted, errors.New("PLX-7007: wasm trap at secret_key=hunter2 in fn.rs:42"))
	detail, err := connect.NewErrorDetail(&errdetails.ErrorInfo{Reason: "FUNCTION_TRAPPED", Domain: "plux.dev", Metadata: map[string]string{"code": "PLX-7007"}})
	if err != nil {
		t.Fatal(err)
	}
	trap.AddDetail(detail)
	forged, _ := connect.NewErrorDetail(&errdetails.ErrorInfo{Metadata: map[string]string{"code": "PLX-8030"}})
	forgedErr := connect.NewError(connect.CodeAborted, errors.New("hunter2"))
	forgedErr.AddDetail(forged)

	for name, tc := range map[string]struct {
		err  error
		want plxerr.Code
	}{
		"trap":        {trap, plxerr.FunctionTrapped},
		"foreign":     {forgedErr, plxerr.InternalServerError},
		"unreachable": {connect.NewError(connect.CodeUnavailable, errors.New("dial hunter2")), plxerr.UpstreamUnavailable},
		"plain":       {errors.New("hunter2"), plxerr.UpstreamUnavailable},
	} {
		_, err := function.ConnectRunner{Client: runnerFailure{err: tc.err}}.Run(context.Background(), &fnpb.FunctionRunnerServiceInvokeRequest{})
		var plx *plxerr.Error
		if !errors.As(err, &plx) || plx.Code != tc.want {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if strings.Contains(plx.Message, "hunter2") || strings.Contains(plx.Message, "trap at") {
			t.Errorf("%s: the message leaks the runner's text: %q", name, plx.Message)
		}
	}
	timedOut, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	<-timedOut.Done()
	if _, err := (function.ConnectRunner{Client: runnerFailure{err: errors.New("x")}}).Run(timedOut, &fnpb.FunctionRunnerServiceInvokeRequest{}); code(err) != plxerr.FunctionTimeout {
		t.Errorf("a deadline: %v", err)
	}
}

// Verifies: FN-024.
// Invocations are counted per device, per app and per function; the call
// past a limit is PLX-7015, and a refused call does not reach the runner.
func TestInvokeRateLimits_FN_024(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		key  limits.Key
	}{
		{"per device", limits.FunctionInvocationsPerMinutePerDevice},
		{"per app", limits.FunctionInvocationsPerMinutePerApp},
		{"per function", limits.FunctionInvocationsPerMinute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := newRig(t, nil)
			r.tighten(t, tc.key, 2)
			r.cat.add(charge, 1, function.PlacementServer, function.Declaration{})
			for i := range 2 {
				if _, err := r.call(charge, 1, `{}`); err != nil {
					t.Fatalf("call %d: %v", i, err)
				}
			}
			if _, err := r.call(charge, 1, `{}`); code(err) != plxerr.FunctionRateLimited {
				t.Errorf("the third call: %v", err)
			}
			if r.runner.calls() != 2 {
				t.Errorf("runner calls = %d", r.runner.calls())
			}
		})
	}
	t.Run("another function is another count", func(t *testing.T) {
		t.Parallel()
		r := newRig(t, nil)
		r.tighten(t, limits.FunctionInvocationsPerMinute, 1)
		r.cat.add(charge, 1, function.PlacementServer, function.Declaration{})
		r.cat.add("payments.refund", 1, function.PlacementServer, function.Declaration{})
		for _, name := range []string{charge, "payments.refund"} {
			if _, err := r.call(name, 1, `{}`); err != nil {
				t.Errorf("%s: %v", name, err)
			}
		}
	})
}

// Verifies: FN-020, FN-011, FN-012, FN-032.
// The runner gets the declared capabilities, the limits the declaration may
// tighten but not loosen, the app, environment and assurance, the
// environment's values for the declared variables, the request's
// traceparent, depth 0 and a grant scoped to the version.
func TestInvokeRunnerRequest_FN_020(t *testing.T) {
	t.Parallel()
	r := newRig(t, nil)
	r.cat.add(charge, 3, function.PlacementServer, function.Declaration{
		Capabilities: function.Capabilities{HTTP: []string{"api.example.com"}, KV: true, Secrets: []string{"API_KEY"}, Functions: []string{"payments.fx"}},
		Limits:       function.DeclaredLimits{TimeoutMs: 500, MemoryBytes: 1 << 40, OutputBytes: 1024, HTTPCalls: 3},
		Env:          []string{"REGION"},
	})
	if _, err := r.call(charge, 3, `{"amount":5}`); err != nil {
		t.Fatal(err)
	}
	got := r.runner.last()
	reg := limits.Defaults()
	if got.GetFunction() != charge || got.GetVersion() != 3 || string(got.GetInput()) != `{"amount":5}` || got.GetDepth() != 0 ||
		got.GetApp() != "app" || got.GetEnvironment() != "env" || got.GetAssuranceLevel() != "AL2" || got.GetEnv()["REGION"] != "eu" ||
		got.GetTraceparent() != "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01" {
		t.Errorf("request = %v", got)
	}
	if r.cat.envAsked[0] != "REGION" {
		t.Errorf("environment variables asked for %v", r.cat.envAsked)
	}
	l := got.GetLimits()
	if l.GetTimeoutMs() != 500 || l.GetMemoryBytes() != uint64(reg.Get(limits.FunctionMemory)) || l.GetOutputBytes() != 1024 || l.GetHttpCalls() != 3 || //nolint:gosec // registry values are positive
		l.GetCallDepth() != uint32(reg.Get(limits.FunctionCallDepth)) || l.GetLogBytes() != uint64(reg.Get(limits.FunctionLogBytes)) || l.GetInputBytes() != uint64(reg.Get(limits.FunctionInputSize)) { //nolint:gosec // registry values are positive
		t.Errorf("limits = %v", l)
	}
	c := got.GetCapabilities()
	if len(c.GetHttpDomains()) != 1 || !c.GetKv() || c.GetSecrets()[0] != "API_KEY" || c.GetFunctions()[0] != "payments.fx" || c.GetClaims() {
		t.Errorf("capabilities = %v", c)
	}
	claims, err := r.grants.Verify(function.Grant(got.GetGrant()))
	if err != nil || claims.Function != charge || claims.Version != 3 || claims.AppID != "app" || claims.EnvironmentID != "env" || claims.OrganizationID != "org" ||
		claims.Depth != 0 || claims.ModuleSHA256 != function.HexSHA256(r.cat.versions[key(charge, 3)].Version.ModuleSHA256) || claims.ExpiresAt.Sub(r.now) != 500*time.Millisecond+function.GrantMargin {
		t.Errorf("grant = %+v %v", claims, err)
	}
}

// Verifies: SEC-026, FN-012.
// Verified user claims reach a function that declares the claims
// capability and no other; a token that does not verify refuses the call.
func TestInvokeClaims_SEC_026(t *testing.T) {
	t.Parallel()
	r := newRig(t, nil)
	r.cat.add("needs.claims", 1, function.PlacementServer, function.Declaration{Capabilities: function.Capabilities{Claims: true}})
	r.cat.add("no.claims", 1, function.PlacementServer, function.Declaration{})
	ctx := context.Background()
	c := function.Caller{Device: r.dev, UserToken: "tok"}

	if _, err := r.inv.Invoke(ctx, c, function.Call{Function: "needs.claims", Version: 1}); err != nil {
		t.Fatal(err)
	}
	if got := r.runner.last(); string(got.GetClaims()) != `{"sub":"u1"}` || !got.GetCapabilities().GetClaims() {
		t.Errorf("claims = %q, capability %v", got.GetClaims(), got.GetCapabilities().GetClaims())
	}
	if g, err := r.grants.Verify(function.Grant(r.runner.last().GetGrant())); err != nil || string(g.UserClaims) != `{"sub":"u1"}` {
		t.Errorf("the grant does not carry the claims for nested calls: %+v %v", g, err)
	}
	if r.users.asked[0] != "tok@needs.claims" {
		t.Errorf("verified %v", r.users.asked)
	}
	if _, err := r.inv.Invoke(ctx, c, function.Call{Function: "no.claims", Version: 1}); err != nil {
		t.Fatal(err)
	}
	if got := r.runner.last(); got.GetClaims() != nil || len(r.users.asked) != 1 {
		t.Errorf("a function without the capability got claims %q (verifications: %v)", got.GetClaims(), r.users.asked)
	}
	if _, err := r.inv.Invoke(ctx, function.Caller{Device: r.dev}, function.Call{Function: "needs.claims", Version: 1}); err != nil || r.runner.last().GetClaims() != nil {
		t.Errorf("no user token: %v", err)
	}
	r.users.err = plxerr.New(plxerr.UserTokenInvalid, "bad token")
	before := r.runner.calls()
	if _, err := r.inv.Invoke(ctx, c, function.Call{Function: "needs.claims", Version: 1}); code(err) != plxerr.UserTokenInvalid || r.runner.calls() != before {
		t.Errorf("an invalid user token: %v", err)
	}
}

// Verifies: FN-032.
// The logs of an invocation go to the sink with the invocation's identity.
func TestInvokeLogsGoToTheSink_FN_032(t *testing.T) {
	t.Parallel()
	r := newRig(t, nil)
	r.cat.add(charge, 1, function.PlacementServer, function.Declaration{})
	r.runner.reply = func(*fnpb.FunctionRunnerServiceInvokeRequest) (*fnpb.FunctionRunnerServiceInvokeResponse, error) {
		return &fnpb.FunctionRunnerServiceInvokeResponse{
			Result: &fnpb.FunctionRunnerServiceInvokeResponse_Output{Output: []byte(`1`)},
			Logs:   []*fnpb.FunctionLog{{Level: 1, Message: "hello", TimeUnixNano: 1_700_000_000_000_000_000}},
		}, nil
	}
	if _, err := r.call(charge, 1, `{}`); err != nil {
		t.Fatal(err)
	}
	if len(r.logs.metas) != 1 || r.logs.metas[0].Function != charge || r.logs.metas[0].AppID != "app" || r.logs.metas[0].Traceparent == "" ||
		len(r.logs.logs[0]) != 1 || r.logs.logs[0][0].Message != "hello" || r.logs.logs[0][0].Level != 1 || r.logs.logs[0][0].Time.Unix() != 1_700_000_000 {
		t.Errorf("sink = %+v %+v", r.logs.metas, r.logs.logs)
	}
}

// nestedGrant issues a grant for a running version.
func (r *rig) nestedGrant(t *testing.T, name string, version int32, depth uint32, claims []byte) function.Grant {
	t.Helper()
	g, _, err := r.grants.Issue(function.GrantClaims{
		OrganizationID: "org", AppID: "app", EnvironmentID: "env", Function: name, Version: version, Depth: depth,
		ModuleSHA256: function.HexSHA256(r.cat.versions[key(name, version)].Version.ModuleSHA256), Assurance: "AL2", UserClaims: claims,
	}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// Verifies: FN-012, FN-011.
// A nested call needs the callee in the caller's declared functions
// (PLX-7003), a depth that is the caller's plus one (FN-012) and within
// function.callDepth (PLX-7005); the callee runs with the caller's
// assurance and, if it declares the capability, the caller's claims.
func TestNestedInvoke_FN_012(t *testing.T) {
	t.Parallel()
	r := newRig(t, nil)
	r.cat.add("outer.fn", 1, function.PlacementServer, function.Declaration{Capabilities: function.Capabilities{Functions: []string{"inner.fn", "claims.fn", "device.fn"}}})
	r.cat.add("inner.fn", 1, function.PlacementServer, function.Declaration{})
	r.cat.add("inner.fn", 2, function.PlacementServer, function.Declaration{Limits: function.DeclaredLimits{TimeoutMs: 100}})
	r.cat.add("claims.fn", 1, function.PlacementServer, function.Declaration{Capabilities: function.Capabilities{Claims: true}})
	r.cat.add("device.fn", 1, function.PlacementDevice, function.Declaration{})
	r.cat.add("hidden.fn", 1, function.PlacementServer, function.Declaration{})
	ctx := context.Background()
	user := []byte(`{"sub":"u1"}`)
	outer := r.nestedGrant(t, "outer.fn", 1, 0, user)

	res, err := r.inv.Nested(ctx, outer, "inner.fn", []byte(`{"x":1}`), 1, "tp")
	if err != nil || string(res.GetOutput()) != `{"ok":true}` {
		t.Fatalf("a declared callee: %v %v", res, err)
	}
	got := r.runner.last()
	if got.GetFunction() != "inner.fn" || got.GetVersion() != 2 || got.GetDepth() != 1 || got.GetAssuranceLevel() != "AL2" || got.GetTraceparent() != "tp" || got.GetClaims() != nil || got.GetLimits().GetTimeoutMs() != 100 {
		t.Errorf("callee request = %v", got)
	}
	if g, err := r.grants.Verify(function.Grant(got.GetGrant())); err != nil || g.Function != "inner.fn" || g.Version != 2 || g.Depth != 1 {
		t.Errorf("callee grant = %+v %v", g, err)
	}
	if _, err := r.inv.Nested(ctx, outer, "claims.fn", nil, 1, ""); err != nil || string(r.runner.last().GetClaims()) != string(user) {
		t.Errorf("the caller's claims for a callee that declares them: %v %q", err, r.runner.last().GetClaims())
	}
	if _, err := r.inv.Nested(ctx, outer, "hidden.fn", nil, 1, ""); code(err) != plxerr.FunctionCapabilityDenied {
		t.Errorf("an undeclared callee: %v", err)
	}
	if _, err := r.inv.Nested(ctx, outer, "device.fn", nil, 1, ""); code(err) != plxerr.FunctionNotFound {
		t.Errorf("a device-placed callee: %v", err)
	}
	if _, err := r.inv.Nested(ctx, outer, "inner.fn", nil, 3, ""); code(err) != plxerr.InvalidStructure {
		t.Errorf("a depth that skips levels: %v", err)
	}
	if _, err := r.inv.Nested(ctx, outer+"x", "inner.fn", nil, 1, ""); code(err) != plxerr.AuthenticationRequired {
		t.Errorf("a tampered grant: %v", err)
	}

	r.tighten(t, limits.FunctionCallDepth, 2)
	deep := r.nestedGrant(t, "outer.fn", 1, 1, nil)
	if _, err := r.inv.Nested(ctx, deep, "inner.fn", nil, 2, ""); err != nil {
		t.Errorf("depth 2 within callDepth 2: %v", err)
	}
	last := r.nestedGrant(t, "outer.fn", 1, 2, nil)
	before := r.runner.calls()
	if _, err := r.inv.Nested(ctx, last, "inner.fn", nil, 3, ""); code(err) != plxerr.FunctionCallDepthExceeded || r.runner.calls() != before {
		t.Errorf("depth 3 above callDepth 2: %v", err)
	}
}

// Verifies: FN-020.
// An invoker needs every collaborator; none is guessed.
func TestNewInvokerRefusesAGap_FN_020(t *testing.T) {
	t.Parallel()
	if _, err := function.NewInvoker(function.InvokerOptions{}); err == nil {
		t.Error("an empty invoker was built")
	}
}

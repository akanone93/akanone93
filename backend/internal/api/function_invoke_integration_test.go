// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/device"
	"github.com/nightCode42/plux3/backend/internal/fnpb"
	"github.com/nightCode42/plux3/backend/internal/fnpb/fnpbconnect"
	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/gateway"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/idempotency"
)

// apiRunner stands in for the function runner role.
type apiRunner struct {
	mu    sync.Mutex
	reqs  []*fnpb.FunctionRunnerServiceInvokeRequest
	reply func(*fnpb.FunctionRunnerServiceInvokeRequest) (*fnpb.FunctionRunnerServiceInvokeResponse, error)
}

func (r *apiRunner) Run(_ context.Context, req *fnpb.FunctionRunnerServiceInvokeRequest) (*fnpb.FunctionRunnerServiceInvokeResponse, error) {
	r.mu.Lock()
	r.reqs = append(r.reqs, req)
	reply := r.reply
	r.mu.Unlock()
	if reply != nil {
		return reply(req)
	}
	return &fnpb.FunctionRunnerServiceInvokeResponse{Result: &fnpb.FunctionRunnerServiceInvokeResponse_Output{Output: []byte(`{"total":"12.50"}`)}}, nil
}

func (r *apiRunner) calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.reqs)
}

func (r *apiRunner) last() *fnpb.FunctionRunnerServiceInvokeRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.reqs[len(r.reqs)-1]
}

// apiSCA is the gateway's strong customer authentication behind the
// invoker, as the server assembles it, with an SCA key the test holds.
type apiSCA struct {
	sca *gateway.SCA
	key *ecdsa.PrivateKey
}

// apiSCAKeys answers with the one SCA key of the test for every device.
type apiSCAKeys struct{ key *ecdsa.PrivateKey }

func (k apiSCAKeys) SCAKey(context.Context, device.Identity) (device.SCAKey, error) {
	return device.SCAKey{Public: &k.key.PublicKey, JKT: "sca-jkt", Storage: device.KeyStorageStrongBox}, nil
}

func newAPISCA(t testing.TB) *apiSCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &apiSCA{sca: &gateway.SCA{Cache: cache.NewMemory(time.Now), Keys: apiSCAKeys{key: key}}, key: key}
}

func (s *apiSCA) CreateInvocationChallenge(ctx context.Context, d device.Identity, name string, version int32, inputSHA256, summary []byte, lifetime time.Duration) (function.SCAChallenge, error) {
	c, err := s.sca.CreateInvocation(ctx, d, gateway.FunctionTransaction{Name: name, Version: version}, inputSHA256, summary, lifetime)
	if err != nil {
		return function.SCAChallenge{}, err //nolint:wrapcheck // the code is what the test reads
	}
	return function.SCAChallenge{ID: c.ID, Bytes: c.Bytes, ExpiresAt: c.ExpiresAt, Summary: c.Summary}, nil
}

func (s *apiSCA) VerifyInvocation(ctx context.Context, d device.Identity, p function.SCAProof, lifetime time.Duration) error {
	return s.sca.VerifyInvocation(ctx, d, gateway.FunctionTransaction{Name: p.Function, Version: p.Version}, p.Input, p.ChallengeID, p.Signature, lifetime) //nolint:wrapcheck // the code is what the test reads
}

// sign answers a challenge as the device's SCA key would: ES256 over the
// challenge followed by the summary, r then s.
func (s *apiSCA) sign(t testing.TB, challenge, summary []byte) []byte {
	t.Helper()
	digest := sha256.Sum256(gateway.SigningInput(challenge, summary))
	r, sv, err := ecdsa.Sign(rand.Reader, s.key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	sig := make([]byte, gateway.SignatureSize)
	r.FillBytes(sig[:32])
	sv.FillBytes(sig[32:])
	return sig
}

// apiUsers verifies the token "good" and returns claims.
type apiUsers struct{}

func (apiUsers) VerifyUser(_ context.Context, token string, _ device.Identity, _ string) ([]byte, error) {
	if token != "good" {
		return nil, plxerr.New(plxerr.UserTokenInvalid, "the user token is not valid")
	}
	return []byte(`{"sub":"user-1"}`), nil
}

// apiSecrets are the environment's server-held values.
type apiSecrets map[string]string

func (s apiSecrets) Secret(_ context.Context, _, _, _, name string) (string, bool, error) {
	v, ok := s[name]
	return v, ok, nil
}

// fnRuntime is the api role's side of server functions, with the fakes of
// the roles around it.
type fnRuntime struct {
	invoker *function.Invoker
	host    *function.Host
	grants  *function.Grants
	runner  *apiRunner
	sca     *apiSCA

	mu     sync.Mutex
	limits limits.Set
	now    time.Time
	moved  bool
}

type fnRuntimeDeps struct {
	db               *storage.DB
	backend          signing.Crypter
	service          *function.Service
	keys             *idempotency.Store
	devices          *device.Service
	resolver         *gateway.ConfigResolver
	set              limits.Set
	count            func(context.Context, string, time.Duration) (int64, error)
	defaultAssurance func(context.Context, string, string) (string, error)
}

// clock is the grants' clock: real time until a test moves it.
func (f *fnRuntime) clock() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.moved {
		return f.now
	}
	return time.Now()
}

// advance moves the grants' clock forward.
func (f *fnRuntime) advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.moved {
		f.now, f.moved = time.Now(), true
	}
	f.now = f.now.Add(d)
}

// tighten lowers a limit for the invocations that follow.
func (f *fnRuntime) tighten(t testing.TB, k limits.Key, v int64) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	l, err := f.limits.Tighten(k, limits.ScopeApp, v)
	if err != nil {
		t.Fatal(err)
	}
	f.limits = l
}

func newFnRuntime(t *testing.T, d fnRuntimeDeps) *fnRuntime {
	t.Helper()
	f := &fnRuntime{runner: &apiRunner{}, sca: newAPISCA(t), limits: d.set}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	var err error
	if f.grants, err = function.NewGrants(key, f.clock, nil); err != nil {
		t.Fatal(err)
	}
	if f.invoker, err = function.NewInvoker(function.InvokerOptions{
		Declarations: d.service, Resolver: d.service, Pins: d.service, Environment: d.service, Grants: f.grants, Runner: f.runner, Keys: d.keys,
		Limits: func(context.Context, string, string) (limits.Set, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			return f.limits, nil
		},
		Count: d.count, DefaultAssurance: d.defaultAssurance, Attestations: d.devices, SCA: f.sca, Users: apiUsers{},
	}); err != nil {
		t.Fatal(err)
	}
	if f.host, err = function.NewHost(function.HostOptions{
		Service: d.service, Grants: f.grants, Invoker: f.invoker, Secrets: apiSecrets{"API_KEY": "s3cret", "OTHER": "x"},
	}); err != nil {
		t.Fatal(err)
	}
	return f
}

// invokeFixture is a released app with a development device and the
// functions a test deploys and pins.
type invokeFixture struct {
	w     *world
	admin caller
	app   string
	envs  map[string]string
	dev   *testDevice
	fn    pluxv1connect.FunctionServiceClient
	host  fnpbconnect.FunctionHostServiceClient
}

func newInvokeFixture(t *testing.T) *invokeFixture {
	t.Helper()
	w := newWorld(t)
	admin, app, envs := w.releasedApp(t)
	dev := w.registerDevelopment(t, app)
	dev.refresh(t)
	return &invokeFixture{
		w: w, admin: admin, app: app, envs: envs, dev: dev,
		fn:   pluxv1connect.NewFunctionServiceClient(http.DefaultClient, w.base),
		host: fnpbconnect.NewFunctionHostServiceClient(http.DefaultClient, w.base),
	}
}

func (f *invokeFixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	err := f.w.db.InTx(context.Background(), storage.Tenant{OrganizationID: f.admin.org}, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, sql, args...)
		return err
	})
	if err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

// deploy builds a function, stores its declaration and deploys it as the
// next version of the development environment.
func (f *invokeFixture) deploy(t *testing.T, name, declaration string) *pluxv1.FunctionVersion {
	t.Helper()
	id := f.w.builtBy(t, f.admin, f.app, f.envs["development"], name)
	f.exec(t, `INSERT INTO function_declarations (build_id, organization_id, declaration) VALUES ($1, $2, $3::jsonb)`, id, f.admin.org, declaration)
	return must(f.w.functionAdmin.Deploy(context.Background(), req(f.admin, &pluxv1.DeployRequest{BuildId: id})))(t).GetVersion()
}

// pin records a version in release 1, which the development channel
// points at.
func (f *invokeFixture) pin(t *testing.T, v *pluxv1.FunctionVersion) {
	t.Helper()
	sum, err := hex.DecodeString(v.GetModuleSha256())
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, `UPDATE channels SET release_sequence = 1 WHERE environment_id = $1`, f.envs["development"])
	f.exec(t, `INSERT INTO release_functions (release_id, organization_id, app_id, environment_id, name, version, module_sha256, placement)
	           SELECT id, organization_id, app_id, $3, $4, $5, $6, 'server' FROM releases WHERE app_id = $1 AND sequence = $2`,
		f.app, 1, f.envs["development"], v.GetName(), v.GetVersion(), sum)
}

// deployPinned deploys and pins a function.
func (f *invokeFixture) deployPinned(t *testing.T, name, declaration string) *pluxv1.FunctionVersion {
	t.Helper()
	v := f.deploy(t, name, declaration)
	f.pin(t, v)
	return v
}

// invoke calls FunctionService.Invoke as the device.
func (f *invokeFixture) invoke(t *testing.T, m *pluxv1.FunctionServiceInvokeRequest) (*pluxv1.FunctionServiceInvokeResponse, error) {
	t.Helper()
	res, err := f.fn.Invoke(context.Background(), bound(t, f.dev, pluxv1connect.FunctionServiceInvokeProcedure, m))
	if err != nil {
		return nil, err //nolint:wrapcheck // the code is what the test reads
	}
	return res.Msg, nil
}

func invokeOf(v *pluxv1.FunctionVersion, input string) *pluxv1.FunctionServiceInvokeRequest {
	return &pluxv1.FunctionServiceInvokeRequest{Function: v.GetName(), Version: v.GetVersion(), Input: []byte(input)}
}

const (
	fnCharge = "payments.charge"
	// declAL0 lets the development device, which holds AL0, call.
	declAL0 = `{"name":"payments.charge","placement":"server","assurance":"AL0"}`
)

// Verifies: FN-020, FN-054, FN-006.
// A device invokes a pinned, server-placed version with its token and
// proof; an unpinned version, a person's session and a call without a token
// are refused before a module runs.
func TestFunctionInvoke_FN_020(t *testing.T) {
	t.Parallel()
	f := newInvokeFixture(t)
	v := f.deployPinned(t, fnCharge, declAL0)
	unpinned := f.deploy(t, "payments.refund", declAL0)

	res, err := f.invoke(t, invokeOf(v, `{"amount":"12.50"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if string(res.GetOutput()) != `{"total":"12.50"}` || res.GetVersion() != v.GetVersion() || res.GetReplayed() {
		t.Errorf("response = %v", res)
	}
	got := f.w.fnrt.runner.last()
	if got.GetFunction() != fnCharge || got.GetApp() != f.app || got.GetEnvironment() != f.envs["development"] || got.GetAssuranceLevel() != "AL0" || got.GetDepth() != 0 {
		t.Errorf("runner request = %v", got)
	}

	before := f.w.fnrt.runner.calls()
	if _, err := f.invoke(t, invokeOf(unpinned, `{}`)); codeOf(err) != connect.CodeNotFound || reasonOf(t, err) != "FUNCTION_NOT_FOUND" {
		t.Errorf("an unpinned version: %v", err)
	}
	missing := invokeOf(v, `{}`)
	missing.Version = 99
	if _, err := f.invoke(t, missing); reasonOf(t, err) != "FUNCTION_NOT_FOUND" {
		t.Errorf("a version that does not exist: %v", err)
	}
	if _, err := f.fn.Invoke(context.Background(), connect.NewRequest(invokeOf(v, `{}`))); codeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("a call without a token: %v", err)
	}
	if _, err := f.fn.Invoke(context.Background(), req(f.admin, invokeOf(v, `{}`))); codeOf(err) == 0 {
		t.Error("a person's session invoked a device function")
	}
	if f.w.fnrt.runner.calls() != before {
		t.Error("a refused call reached the runner")
	}
}

// Verifies: FN-054.
// A device-placed version cannot be run by the server for a device, even
// when a release pins it, and the request has no field that carries a
// result computed on a device.
func TestFunctionInvokeRefusesDevicePlacement_FN_054(t *testing.T) {
	t.Parallel()
	f := newInvokeFixture(t)
	v := f.deploy(t, "device.calc", declAL0)
	sum, _ := hex.DecodeString(v.GetModuleSha256())
	f.exec(t, `UPDATE channels SET release_sequence = 1 WHERE environment_id = $1`, f.envs["development"])
	f.exec(t, `INSERT INTO release_functions (release_id, organization_id, app_id, environment_id, name, version, module_sha256, placement)
	           SELECT id, organization_id, app_id, $3, $4, $5, $6, 'device' FROM releases WHERE app_id = $1 AND sequence = $2`,
		f.app, 1, f.envs["development"], v.GetName(), v.GetVersion(), sum)
	if _, err := f.invoke(t, invokeOf(v, `{}`)); reasonOf(t, err) != "FUNCTION_NOT_FOUND" {
		t.Errorf("a device-pinned version: %v", err)
	}
	fields := new(pluxv1.FunctionServiceInvokeRequest).ProtoReflect().Descriptor().Fields()
	for i := range fields.Len() {
		if n := string(fields.Get(i).Name()); strings.Contains(n, "result") || strings.Contains(n, "output") || strings.Contains(n, "evidence") {
			t.Errorf("the request has a field %q that could carry a device's result", n)
		}
	}
	if f.w.fnrt.runner.calls() != 0 {
		t.Error("the runner ran")
	}
}

// Verifies: FN-020, SEC-007.
// The declared assurance is required, and the profile's default applies to
// a function that declares none. Attestation and SCA have their own tests.
func TestFunctionInvokeRequirements_FN_020(t *testing.T) {
	t.Parallel()
	f := newInvokeFixture(t)
	high := f.deployPinned(t, "needs.assurance", `{"name":"needs.assurance","placement":"server","assurance":"AL2"}`)
	standard := f.deployPinned(t, "default.assurance", `{"name":"default.assurance","placement":"server"}`)
	for name, v := range map[string]*pluxv1.FunctionVersion{"declared AL2": high, "the default of the profile": standard} {
		_, err := f.invoke(t, invokeOf(v, `{}`))
		if codeOf(err) != connect.CodePermissionDenied || reasonOf(t, err) != "ASSURANCE_INSUFFICIENT" {
			t.Errorf("%s for an AL0 device: %v", name, err)
		}
	}
}

// Verifies: FN-021.
// A retry with the same idempotency key returns the stored result without
// running the function again; the same key with another input is PLX-7011.
func TestFunctionInvokeIdempotency_FN_021(t *testing.T) {
	t.Parallel()
	f := newInvokeFixture(t)
	v := f.deployPinned(t, fnCharge, declAL0)
	n := 0
	f.w.fnrt.runner.reply = func(*fnpb.FunctionRunnerServiceInvokeRequest) (*fnpb.FunctionRunnerServiceInvokeResponse, error) {
		n++
		return &fnpb.FunctionRunnerServiceInvokeResponse{Result: &fnpb.FunctionRunnerServiceInvokeResponse_Output{Output: []byte(fmt.Sprintf(`{"run":%d}`, n))}}, nil
	}
	m := invokeOf(v, `{"amount":"1"}`)
	m.IdempotencyKey = "pay-123"
	first, err := f.invoke(t, m)
	if err != nil || string(first.GetOutput()) != `{"run":1}` || first.GetReplayed() {
		t.Fatalf("first = %v %v", first, err)
	}
	again, err := f.invoke(t, m)
	if err != nil || string(again.GetOutput()) != `{"run":1}` || !again.GetReplayed() || again.GetVersion() != v.GetVersion() {
		t.Fatalf("replay = %v %v", again, err)
	}
	if f.w.fnrt.runner.calls() != 1 {
		t.Errorf("the function ran %d times", f.w.fnrt.runner.calls())
	}
	other := invokeOf(v, `{"amount":"2"}`)
	other.IdempotencyKey = "pay-123"
	if _, err := f.invoke(t, other); reasonOf(t, err) != "IDEMPOTENCY_KEY_CONFLICT" || !strings.Contains(err.Error(), "PLX-7011") {
		t.Errorf("another input under the key: %v", err)
	}
	if f.w.fnrt.runner.calls() != 1 {
		t.Error("a conflicting call ran")
	}
	// A refused call releases its key, so the retry that follows runs.
	f.w.fnrt.runner.reply = func(*fnpb.FunctionRunnerServiceInvokeRequest) (*fnpb.FunctionRunnerServiceInvokeResponse, error) {
		return nil, plxerr.New(plxerr.FunctionTimeout, "the function exceeded its time limit")
	}
	failing := invokeOf(v, `{"amount":"3"}`)
	failing.IdempotencyKey = "pay-456"
	if _, err := f.invoke(t, failing); reasonOf(t, err) != "FUNCTION_TIMEOUT" {
		t.Fatalf("a timeout: %v", err)
	}
	f.w.fnrt.runner.reply = nil
	if res, err := f.invoke(t, failing); err != nil || res.GetReplayed() {
		t.Errorf("the retry after a failure: %v %v", res, err)
	}
}

// Verifies: FN-022.
// A business error comes back in the response, with its code, message key
// and params; a system error comes back as a Connect error with its PLX
// code.
func TestFunctionInvokeErrors_FN_022(t *testing.T) {
	t.Parallel()
	f := newInvokeFixture(t)
	v := f.deployPinned(t, fnCharge, declAL0)
	f.w.fnrt.runner.reply = func(*fnpb.FunctionRunnerServiceInvokeRequest) (*fnpb.FunctionRunnerServiceInvokeResponse, error) {
		return &fnpb.FunctionRunnerServiceInvokeResponse{Result: &fnpb.FunctionRunnerServiceInvokeResponse_BusinessError{
			BusinessError: []byte(`{"code":"insufficient_funds","messageKey":"pay.funds","params":{"missing":"4.20"}}`),
		}}, nil
	}
	res, err := f.invoke(t, invokeOf(v, `{}`))
	if err != nil {
		t.Fatalf("a business error is not a failure: %v", err)
	}
	be := res.GetBusinessError()
	if be.GetCode() != "insufficient_funds" || be.GetMessageKey() != "pay.funds" || string(be.GetParams()) != `{"missing":"4.20"}` || res.GetOutput() != nil {
		t.Errorf("response = %v", res)
	}

	f.w.fnrt.runner.reply = func(*fnpb.FunctionRunnerServiceInvokeRequest) (*fnpb.FunctionRunnerServiceInvokeResponse, error) {
		return nil, plxerr.New(plxerr.FunctionTrapped, "the function trapped")
	}
	if _, err := f.invoke(t, invokeOf(v, `{}`)); reasonOf(t, err) != "FUNCTION_TRAPPED" || !strings.Contains(err.Error(), "PLX-7007") {
		t.Errorf("a system error: %v", err)
	}
	f.w.fnrt.runner.reply = func(*fnpb.FunctionRunnerServiceInvokeRequest) (*fnpb.FunctionRunnerServiceInvokeResponse, error) {
		return nil, fmt.Errorf("dial tcp 10.0.0.7:9443: connection refused")
	}
	_, err = f.invoke(t, invokeOf(v, `{}`))
	if codeOf(err) != connect.CodeInternal || strings.Contains(err.Error(), "10.0.0.7") {
		t.Errorf("a failure the domain did not name must not leak its text: %v", err)
	}
}

// Verifies: FN-024, FN-020.
// The device's allowance of invocations is a registry limit; the call past
// it is PLX-7015, and the input is bounded by function.inputSize
// (PLX-7022).
func TestFunctionInvokeLimits_FN_024(t *testing.T) {
	t.Parallel()
	f := newInvokeFixture(t)
	v := f.deployPinned(t, fnCharge, declAL0)
	f.w.fnrt.tighten(t, limits.FunctionInvocationsPerMinutePerDevice, 2)
	f.w.fnrt.tighten(t, limits.FunctionInputSize, 4)
	for i := range 2 {
		if _, err := f.invoke(t, invokeOf(v, `{}`)); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if _, err := f.invoke(t, invokeOf(v, `{}`)); codeOf(err) != connect.CodeResourceExhausted || reasonOf(t, err) != "FUNCTION_RATE_LIMITED" {
		t.Errorf("the third call: %v", err)
	}
	if _, err := f.invoke(t, invokeOf(v, `{"a":1}`)); reasonOf(t, err) != "FUNCTION_INPUT_TOO_LARGE" {
		t.Errorf("an input above function.inputSize: %v", err)
	}
	if f.w.fnrt.runner.calls() != 2 {
		t.Errorf("runner calls = %d", f.w.fnrt.runner.calls())
	}
}

// Verifies: SEC-026.
// The user's token reaches a function that declares the claims capability
// as verified claims, and an invalid token refuses the call.
func TestFunctionInvokeClaims_SEC_026(t *testing.T) {
	t.Parallel()
	f := newInvokeFixture(t)
	v := f.deployPinned(t, "needs.claims", `{"name":"needs.claims","placement":"server","assurance":"AL0","capabilities":{"claims":true}}`)
	invoke := func(token string) error {
		r := bound(t, f.dev, pluxv1connect.FunctionServiceInvokeProcedure, invokeOf(v, `{}`))
		r.Header().Set("Plux-User-Token", token)
		_, err := f.fn.Invoke(context.Background(), r)
		return err //nolint:wrapcheck // the code is what the test reads
	}
	if err := invoke("good"); err != nil {
		t.Fatal(err)
	}
	if got := f.w.fnrt.runner.last(); string(got.GetClaims()) != `{"sub":"user-1"}` || !got.GetCapabilities().GetClaims() {
		t.Errorf("claims = %q", got.GetClaims())
	}
	before := f.w.fnrt.runner.calls()
	if err := invoke("forged"); reasonOf(t, err) != "USER_TOKEN_INVALID" || f.w.fnrt.runner.calls() != before {
		t.Errorf("an invalid user token: %v", err)
	}
}

// grantFor issues a grant for a deployed version, as the invocation path
// would.
func (f *invokeFixture) grantFor(t *testing.T, v *pluxv1.FunctionVersion, depth uint32) string {
	t.Helper()
	g, _, err := f.w.fnrt.grants.Issue(function.GrantClaims{
		OrganizationID: f.admin.org, AppID: f.app, EnvironmentID: f.envs["development"], Function: v.GetName(), Version: int32(v.GetVersion()), //nolint:gosec // a test version
		ModuleSHA256: v.GetModuleSha256(), Depth: depth, Assurance: "AL0",
	}, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return string(g)
}

// Verifies: FN-012, FN-018, FN-009.
// The host service serves the grant's module with its signed statement,
// scopes the key-value store to the grant, refuses a secret the function
// does not declare, and refuses an expired or tampered grant.
func TestFunctionHostService_FN_012(t *testing.T) {
	t.Parallel()
	f := newInvokeFixture(t)
	ctx := context.Background()
	mine := f.deploy(t, fnCharge, `{"name":"payments.charge","placement":"server","capabilities":{"kv":true,"secrets":["API_KEY"]}}`)
	sibling := f.deploy(t, "payments.refund", `{"name":"payments.refund","placement":"server","capabilities":{"kv":true}}`)
	closed := f.deploy(t, "payments.audit", `{"name":"payments.audit","placement":"server"}`)
	g, sg, cg := f.grantFor(t, mine, 0), f.grantFor(t, sibling, 0), f.grantFor(t, closed, 0)

	mod := must(f.host.GetModule(ctx, connect.NewRequest(&fnpb.GetModuleRequest{Grant: g, ModuleSha256: mine.GetModuleSha256()})))(t)
	if string(mod.GetModule()) != "\x00asm "+fnCharge || len(mod.GetStatement()) == 0 || len(mod.GetSignature()) == 0 || mod.GetKeyId() != "function-artifact-test" {
		t.Errorf("module = %q statement %d signature %d key %q", mod.GetModule(), len(mod.GetStatement()), len(mod.GetSignature()), mod.GetKeyId())
	}
	if _, err := f.host.GetModule(ctx, connect.NewRequest(&fnpb.GetModuleRequest{Grant: g, ModuleSha256: sibling.GetModuleSha256()})); codeOf(err) != connect.CodePermissionDenied {
		t.Errorf("another function's module: %v", err)
	}

	must(f.host.KvPut(ctx, connect.NewRequest(&fnpb.KvPutRequest{Grant: g, Key: "counter", Value: []byte("1")})))(t)
	if r := must(f.host.KvGet(ctx, connect.NewRequest(&fnpb.KvGetRequest{Grant: g, Key: "counter"})))(t); !r.GetFound() || string(r.GetValue()) != "1" {
		t.Errorf("own key = %v", r)
	}
	if r := must(f.host.KvGet(ctx, connect.NewRequest(&fnpb.KvGetRequest{Grant: sg, Key: "counter"})))(t); r.GetFound() {
		t.Error("another function's grant reads the key")
	}
	if _, err := f.host.KvGet(ctx, connect.NewRequest(&fnpb.KvGetRequest{Grant: cg, Key: "counter"})); reasonOf(t, err) != "FUNCTION_CAPABILITY_DENIED" {
		t.Errorf("a function without kv: %v", err)
	}

	if r := must(f.host.GetSecret(ctx, connect.NewRequest(&fnpb.GetSecretRequest{Grant: g, Name: "API_KEY"})))(t); !r.GetFound() || string(r.GetValue()) != "s3cret" {
		t.Errorf("declared secret = %v", r)
	}
	if _, err := f.host.GetSecret(ctx, connect.NewRequest(&fnpb.GetSecretRequest{Grant: g, Name: "OTHER"})); codeOf(err) != connect.CodePermissionDenied || reasonOf(t, err) != "FUNCTION_CAPABILITY_DENIED" {
		t.Errorf("an undeclared secret: %v", err)
	}

	if _, err := f.host.KvGet(ctx, connect.NewRequest(&fnpb.KvGetRequest{Grant: g + "x", Key: "counter"})); codeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("a tampered grant: %v", err)
	}
	f.w.fnrt.advance(time.Minute)
	if _, err := f.host.KvGet(ctx, connect.NewRequest(&fnpb.KvGetRequest{Grant: g, Key: "counter"})); codeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("an expired grant: %v", err)
	}
}

// Verifies: FN-012, FN-011.
// A nested call needs the callee in the caller's declaration and a depth
// within function.callDepth.
func TestFunctionHostNestedInvoke_FN_012(t *testing.T) {
	t.Parallel()
	f := newInvokeFixture(t)
	ctx := context.Background()
	outer := f.deploy(t, "outer.fn", `{"name":"outer.fn","placement":"server","capabilities":{"functions":["inner.fn"]}}`)
	f.deploy(t, "inner.fn", `{"name":"inner.fn","placement":"server"}`)
	f.deploy(t, "hidden.fn", `{"name":"hidden.fn","placement":"server"}`)
	nested := func(grant, callee string, depth uint32) (*fnpb.FunctionHostServiceInvokeResponse, error) {
		res, err := f.host.Invoke(ctx, connect.NewRequest(&fnpb.FunctionHostServiceInvokeRequest{Grant: grant, Function: callee, Input: []byte(`{}`), Depth: depth}))
		if err != nil {
			return nil, err //nolint:wrapcheck // the code is what the test reads
		}
		return res.Msg, nil
	}

	res, err := nested(f.grantFor(t, outer, 0), "inner.fn", 1)
	if err != nil || string(res.GetResult().GetOutput()) != `{"total":"12.50"}` {
		t.Fatalf("a declared callee: %v %v", res, err)
	}
	if got := f.w.fnrt.runner.last(); got.GetFunction() != "inner.fn" || got.GetDepth() != 1 {
		t.Errorf("callee request = %v", got)
	}
	if _, err := nested(f.grantFor(t, outer, 0), "hidden.fn", 1); reasonOf(t, err) != "FUNCTION_CAPABILITY_DENIED" {
		t.Errorf("an undeclared callee: %v", err)
	}
	before := f.w.fnrt.runner.calls()
	if _, err := nested(f.grantFor(t, outer, 3), "inner.fn", 4); reasonOf(t, err) != "FUNCTION_CALL_DEPTH_EXCEEDED" || !strings.Contains(err.Error(), "PLX-7005") {
		t.Errorf("a call nested too deep: %v", err)
	}
	if f.w.fnrt.runner.calls() != before {
		t.Error("a call past the depth reached the runner")
	}
}

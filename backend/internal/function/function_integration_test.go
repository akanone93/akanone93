// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/jobs"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/schema/uuid7"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/objects"
	"github.com/nightCode42/plux3/backend/internal/storage/storagetest"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
)

type ids struct{ g *uuid7.Generator }

func (i ids) New() (string, error) {
	u, err := i.g.New()
	return u.String(), err
}

func code(err error) plxerr.Code {
	c, _ := plxerr.CodeOf(err)
	return c
}

// fakeBuilder returns the output a test sets, and counts its calls.
type fakeBuilder struct {
	mu    sync.Mutex
	out   function.BuildOutput
	err   error
	calls []function.BuildInput
	// delegate, when set, answers instead of the canned output: the
	// end-to-end test puts the real builder client here.
	delegate function.Builder
}

func (f *fakeBuilder) Build(ctx context.Context, in function.BuildInput) (function.BuildOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, in)
	if f.delegate != nil {
		return f.delegate.Build(ctx, in)
	}
	return f.out, f.err
}

func (f *fakeBuilder) set(out function.BuildOutput, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.out, f.err = out, err
}

// fakeSigner signs with the SHA-256 of the statement and keeps what it saw.
type fakeSigner struct {
	mu         sync.Mutex
	statements [][]byte
	err        error
	// delegate, when set, signs instead of the SHA-256 stand-in.
	delegate function.ArtifactSigner
}

func (f *fakeSigner) SignStatement(ctx context.Context, statement []byte) ([]byte, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, "", f.err
	}
	f.statements = append(f.statements, bytes.Clone(statement))
	if f.delegate != nil {
		return f.delegate.SignStatement(ctx, statement)
	}
	sum := sha256.Sum256(statement)
	return sum[:], "function-artifact-1", nil
}

// recorder keeps the jobs the service enqueued instead of running them.
type recorder struct {
	mu   sync.Mutex
	jobs []function.BuildJob
}

func (r *recorder) Enqueue(_ context.Context, _ pgx.Tx, job function.BuildJob) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.jobs = append(r.jobs, job)
	return nil
}

func (r *recorder) last(t *testing.T) function.BuildJob {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.jobs) == 0 {
		t.Fatal("no build was enqueued")
	}
	return r.jobs[len(r.jobs)-1]
}

// world is an organisation with an owner, a developer, a viewer and two
// apps, against a fresh schema.
type world struct {
	db        *storage.DB
	store     objects.Store
	svc       *function.Service
	tenancy   *tenancy.Service
	auth      *auth.Service
	builder   *fakeBuilder
	signer    *fakeSigner
	queue     *recorder
	limits    limits.Set
	org       string
	owner     auth.Principal
	developer auth.Principal
	viewer    auth.Principal
	adminID   auth.Identity
	app       string
	otherApp  string
	envs      map[string]string
	otherEnvs map[string]string
	enqueuer  function.Enqueuer
	linker    *fakeLinker
	opts      function.Options
	// deviceFunctions is the security setting deviceFunctions.
	deviceFunctions atomic.Bool
}

// newWorld builds the world; enqueue is the service's job queue, or nil
// for a recorder.
func newWorld(t *testing.T, enqueue func(*world) function.Enqueuer, mutate func(*limits.Set)) *world {
	t.Helper()
	db := storagetest.Open(t)
	backend, err := signing.NewFile(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	gen := ids{g: uuid7.NewGenerator(time.Now, rand.Reader)}
	log := audit.NewLog(gen, nil)
	w := &world{db: db, envs: map[string]string{}, otherEnvs: map[string]string{}, builder: &fakeBuilder{}, linker: &fakeLinker{}, signer: &fakeSigner{}, queue: &recorder{}, limits: limits.Defaults()}
	if mutate != nil {
		mutate(&w.limits)
	}
	if w.auth, err = auth.NewService(auth.Options{DB: db, Audit: log, Cache: cache.NewMemory(nil), Crypter: backend, IDs: gen, VerificationURI: "https://p.example/device"}); err != nil {
		t.Fatal(err)
	}
	if w.tenancy, err = tenancy.NewService(tenancy.Options{DB: db, Audit: log, Auth: w.auth, Crypter: backend, IDs: gen, SigningKeyPrefix: "targets"}); err != nil {
		t.Fatal(err)
	}
	store, err := objects.NewFilesystem(t.TempDir(), "https://p.example")
	if err != nil {
		t.Fatal(err)
	}
	w.store = store
	w.enqueuer = w.queue
	if enqueue != nil {
		w.enqueuer = enqueue(w)
	}
	w.opts = function.Options{
		DB: db, Audit: log, Objects: store, IDs: gen, Jobs: w.enqueuer, Builder: w.builder, Signer: w.signer, Linker: w.linker,
		DeviceFunctions: func(context.Context, string, string) (bool, error) { return w.deviceFunctions.Load(), nil },
		Limits:          func(context.Context, pgx.Tx, string, string) (limits.Set, error) { return w.limits, nil },
	}
	w.svc = w.service(t, nil)
	w.populate(t)
	return w
}

// service returns a function service over the world's database and store,
// with its options changed by mutate, which may be nil.
func (w *world) service(t *testing.T, mutate func(*function.Options)) *function.Service {
	t.Helper()
	o := w.opts
	if mutate != nil {
		mutate(&o)
	}
	svc, err := function.NewService(o)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

// populate creates the organisation, its people and its apps.
func (w *world) populate(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	admin, invitation, err := w.auth.Bootstrap(ctx, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.auth.AcceptInvitation(ctx, invitation, "Admin", "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	w.adminID = auth.Identity{Kind: auth.KindUser, ID: admin.ID, UserID: admin.ID, Display: "Admin", SecondFactor: true, InstallationAdmin: true}
	o, err := w.tenancy.CreateOrganization(ctx, w.adminID, "acme", "Acme")
	if err != nil {
		t.Fatal(err)
	}
	w.org = o.ID
	if w.owner, err = w.auth.Resolve(ctx, w.adminID, w.org); err != nil {
		t.Fatal(err)
	}
	w.app, w.envs = w.makeApp(t, "demo")
	w.otherApp, w.otherEnvs = w.makeApp(t, "other")
	w.developer = w.member(t, "dev@example.com", "developer", true)
	w.viewer = w.member(t, "viewer@example.com", "viewer", true)
}

func (w *world) makeApp(t *testing.T, key string) (string, map[string]string) {
	t.Helper()
	ctx := context.Background()
	a, err := w.tenancy.CreateApp(ctx, w.owner, key, key)
	if err != nil {
		t.Fatal(err)
	}
	envs, err := w.tenancy.ListEnvironments(ctx, w.owner, a.ID, tenancy.Page{Size: 10})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, e := range envs {
		out[e.Key] = e.ID
	}
	return a.ID, out
}

func (w *world) member(t *testing.T, email, role string, secondFactor bool) auth.Principal {
	t.Helper()
	m, _, err := w.tenancy.AddMember(context.Background(), w.owner, "", email, role)
	if err != nil {
		t.Fatal(err)
	}
	p, err := w.auth.Resolve(context.Background(), auth.Identity{Kind: auth.KindUser, ID: m.UserID, UserID: m.UserID, Display: role, SecondFactor: secondFactor}, w.org)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// goodOutput is a module the builder built, with a passing test.
func goodOutput(name string) function.BuildOutput {
	return function.BuildOutput{
		Module: []byte("\x00asm module of " + name), ABIVersion: 1, Placement: function.PlacementServer,
		Functions:   []function.Export{{Name: name, InputSchema: `{"type":"object"}`, OutputSchema: `{"type":"string"}`}},
		Provenance:  function.Provenance{Toolchain: "1.97.0", WasmOptVersion: "119", BuilderIdentity: "builder-1", BuildTime: time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)},
		TestResults: []function.TestResult{{Name: "adds", Passed: true}},
	}
}

// built uploads a source, builds it with the fake builder and returns the
// build the worker finished.
func (w *world) built(t *testing.T, env, name string) function.Build {
	t.Helper()
	ctx := context.Background()
	src, err := w.svc.UploadSource(ctx, w.developer, w.app, w.envs[env], name, strings.NewReader("source of "+name))
	if err != nil {
		t.Fatalf("UploadSource: %v", err)
	}
	b, err := w.svc.RequestBuild(ctx, w.developer, src.ID, "")
	if err != nil || b.State != function.BuildQueued {
		t.Fatalf("RequestBuild: %v %+v", err, b)
	}
	if err := w.svc.RunBuild(ctx, w.queue.last(t)); err != nil {
		t.Fatalf("RunBuild: %v", err)
	}
	done, err := w.svc.GetBuild(ctx, w.developer, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	return done
}

const fnName = "payments.charge"

// Verifies: FN-006, FN-009, FN-016, FN-031.
// A source is uploaded, built (the worker asks the builder and the
// signer), deployed as version 1, 2, 3 with aliases that move, and
// resolved for a release; every mutation is audited.
func TestLifecycle_FN_006_FN_009(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t, nil, nil)
	w.builder.set(goodOutput(fnName), nil)

	b := w.built(t, "development", fnName)
	if b.State != function.BuildSucceeded || !b.Signed || b.KeyID != "function-artifact-1" || b.Placement != function.PlacementServer {
		t.Fatalf("build = %+v", b)
	}
	if len(w.builder.calls) != 1 || w.builder.calls[0].SourceObjectKey == "" || w.builder.calls[0].ModuleBytes != w.limits.Get(limits.FunctionModuleSize) {
		t.Errorf("builder input = %+v", w.builder.calls)
	}
	if len(w.signer.statements) != 1 || !bytes.Contains(w.signer.statements[0], []byte(`"moduleSha256":"`)) || !bytes.Contains(w.signer.statements[0], []byte(`"sourceSha256":"`)) {
		t.Errorf("signed statement = %s", w.signer.statements)
	}

	v1, err := w.svc.Deploy(ctx, w.developer, b.ID, "stable")
	if err != nil || v1.Version != 1 || !v1.TestsPassed || len(v1.Signature) == 0 || v1.InputSchema != `{"type":"object"}` {
		t.Fatalf("Deploy: %v %+v", err, v1)
	}
	v2, err := w.svc.Deploy(ctx, w.developer, b.ID, "")
	if err != nil || v2.Version != 2 {
		t.Fatalf("second Deploy: %v %+v", err, v2)
	}
	w.builder.set(goodOutput("payments.refund"), nil)
	other, err := w.svc.Deploy(ctx, w.developer, w.built(t, "development", "payments.refund").ID, "")
	if err != nil || other.Version != 1 {
		t.Errorf("another function starts at 1: %v %+v", err, other)
	}

	if _, err := w.svc.SetAlias(ctx, w.developer, w.app, w.envs["development"], fnName, "prod", 2); err != nil {
		t.Fatal(err)
	}
	aliases, err := w.svc.SetAlias(ctx, w.developer, w.app, w.envs["development"], fnName, "stable", 2)
	if err != nil || len(aliases) != 2 || aliases[0] != (function.Alias{Name: "prod", Version: 2}) || aliases[1] != (function.Alias{Name: "stable", Version: 2}) {
		t.Fatalf("SetAlias: %v %+v", err, aliases)
	}
	if _, err := w.svc.SetAlias(ctx, w.developer, w.app, w.envs["development"], fnName, "stable", 9); code(err) != plxerr.FunctionNotFound {
		t.Errorf("an alias at a missing version: %v", err)
	}
	if _, err := w.svc.SetAlias(ctx, w.developer, w.app, w.envs["development"], fnName, "Bad", 1); code(err) != plxerr.InvalidFormat {
		t.Errorf("a bad alias: %v", err)
	}

	got, err := w.svc.GetFunction(ctx, w.viewer, w.app, w.envs["development"], fnName)
	if err != nil || got.Latest.Version != 2 || len(got.Aliases) != 2 {
		t.Fatalf("GetFunction: %v %+v", err, got)
	}
	latest, err := w.svc.ListFunctions(ctx, w.viewer, w.app, w.envs["development"], "", 10)
	if err != nil || len(latest) != 2 || latest[0].Name != fnName || latest[0].Version != 2 || latest[1].Name != "payments.refund" {
		t.Fatalf("ListFunctions: %v (%d functions)", err, len(latest))
	}
	if rest, _ := w.svc.ListFunctions(ctx, w.viewer, w.app, w.envs["development"], fnName, 10); len(rest) != 1 {
		t.Errorf("the page after %s: %+v", fnName, rest)
	}
	versions, err := w.svc.ListVersions(ctx, w.viewer, w.app, w.envs["development"], fnName, 0, 10)
	if err != nil || len(versions) != 2 || versions[0].Version != 2 || versions[1].Version != 1 {
		t.Fatalf("ListVersions: %v %+v", err, versions)
	}
	if older, _ := w.svc.ListVersions(ctx, w.viewer, w.app, w.envs["development"], fnName, 2, 10); len(older) != 1 || older[0].Version != 1 {
		t.Errorf("the page below 2: %+v", older)
	}

	assertResolved(t, w, "prod", 2)
	assertResolved(t, w, "", 2)
	if _, err := w.svc.ResolveVersion(ctx, w.org, w.app, w.envs["development"], fnName, "canary"); code(err) != plxerr.FunctionAliasNotFound {
		t.Errorf("a missing alias: %v", err)
	}
	if _, err := w.svc.ResolveVersion(ctx, w.org, w.app, w.envs["development"], "no.such", ""); code(err) != plxerr.FunctionNotFound {
		t.Errorf("a missing function: %v", err)
	}

	if _, err := w.svc.GetFunction(ctx, w.viewer, w.app, w.envs["development"], "no.such"); code(err) != plxerr.FunctionNotFound {
		t.Errorf("GetFunction of a missing function: %v", err)
	}
	if got := w.auditCount(t, audit.FunctionDeployed); got != 3 {
		t.Errorf("deploys audited = %d, want 3", got)
	}
	for _, a := range []audit.Action{audit.FunctionSourceUploaded, audit.FunctionBuildRequested, audit.FunctionBuildFinished, audit.FunctionAliasSet} {
		if w.auditCount(t, a) == 0 {
			t.Errorf("no %s entry was audited", a)
		}
	}
}

func assertResolved(t *testing.T, w *world, alias string, version int32) {
	t.Helper()
	r, err := w.svc.ResolveVersion(context.Background(), w.org, w.app, w.envs["development"], fnName, alias)
	want := sha256.Sum256(goodOutput(fnName).Module)
	if err != nil || r.Version != version || !bytes.Equal(r.ModuleSHA256, want[:]) || r.Placement != function.PlacementServer {
		t.Errorf("ResolveVersion(%q) = %+v, %v", alias, r, err)
	}
}

func (w *world) auditCount(t *testing.T, a audit.Action) int {
	t.Helper()
	var n int
	err := w.db.InTx(context.Background(), storage.Tenant{OrganizationID: w.org}, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action = $1`, string(a)).Scan(&n)
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// Verifies: FN-031, FN-009, FN-016.
// A build whose tests fail, one the builder refused and one the signer
// could not sign are not deployable; a refused module is a failed build
// with its diagnostics, a builder fault leaves the build for a retry.
func TestDeployRefusals_FN_031(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t, nil, nil)

	failing := goodOutput(fnName)
	failing.TestResults = append(failing.TestResults, function.TestResult{Name: "subtracts", Passed: false, Message: "got 3"})
	w.builder.set(failing, nil)
	b := w.built(t, "development", fnName)
	if b.State != function.BuildSucceeded || len(b.TestResults) != 2 {
		t.Fatalf("build = %+v", b)
	}
	if _, err := w.svc.Deploy(ctx, w.developer, b.ID, ""); code(err) != plxerr.FunctionTestsFailed {
		t.Errorf("deploying a build with a failing test: %v", err)
	}

	broken := function.BuildOutput{Diagnostics: plxerr.Diagnostics{plxerr.NewDiagnostic(plxerr.FunctionBuildFailed, plxerr.Location{}, "error[E0308]")}}
	w.builder.set(broken, nil)
	b = w.built(t, "development", fnName)
	if b.State != function.BuildFailed || len(b.Diagnostics) != 1 || b.Signed {
		t.Fatalf("failed build = %+v", b)
	}
	if _, err := w.svc.Deploy(ctx, w.developer, b.ID, ""); code(err) != plxerr.FunctionBuildFailed {
		t.Errorf("deploying a failed build: %v", err)
	}

	w.builder.set(function.BuildOutput{}, plxerr.New(plxerr.FunctionBuildLimitExceeded, "the build ran out of time"))
	if b = w.built(t, "development", fnName); b.State != function.BuildFailed || b.Diagnostics[0].Code != plxerr.FunctionBuildLimitExceeded {
		t.Errorf("a refused build = %+v", b)
	}

	mismatch := goodOutput(fnName)
	mismatch.Functions = []function.Export{{Name: "other.name"}}
	w.builder.set(mismatch, nil)
	if b = w.built(t, "development", fnName); b.State != function.BuildFailed || b.Diagnostics[len(b.Diagnostics)-1].Code != plxerr.FunctionDeclarationMismatch {
		t.Errorf("a module without the function = %+v", b)
	}

	w.builder.set(goodOutput(fnName), nil)
	src, _ := w.svc.UploadSource(ctx, w.developer, w.app, w.envs["development"], fnName, strings.NewReader("x"))
	queued, err := w.svc.RequestBuild(ctx, w.developer, src.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.svc.Deploy(ctx, w.developer, queued.ID, ""); code(err) != plxerr.PreconditionFailed {
		t.Errorf("deploying a queued build: %v", err)
	}
	w.signer.err = errors.New("kms unavailable")
	if err := w.svc.RunBuild(ctx, w.queue.last(t)); err == nil {
		t.Error("a signer fault did not return an error for the retry")
	}
	if b, _ := w.svc.GetBuild(ctx, w.developer, queued.ID); b.State != function.BuildRunning {
		t.Errorf("after a signer fault the build is %s, want running", b.State)
	}
	w.signer.err = nil
	if err := w.svc.RunBuild(ctx, w.queue.last(t)); err != nil {
		t.Fatalf("the retry: %v", err)
	}
	if b, _ := w.svc.GetBuild(ctx, w.developer, queued.ID); b.State != function.BuildSucceeded {
		t.Errorf("after the retry the build is %s", b.State)
	}
	calls := len(w.builder.calls)
	if err := w.svc.RunBuild(ctx, w.queue.last(t)); err != nil || len(w.builder.calls) != calls {
		t.Errorf("a finished build ran again: %v", err)
	}
	if _, err := w.svc.RequestBuild(ctx, w.developer, src.ID, "edge"); code(err) != plxerr.InvalidEnumValue {
		t.Errorf("an unknown placement: %v", err)
	}
}

// Verifies: FN-007.
// A production deploy needs function.deploy.production, and with it a
// second factor; a developer, who holds only function.deploy, deploys
// outside production alone. Moving an alias in production needs the same.
func TestProductionDeploy_FN_007(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t, nil, nil)
	w.builder.set(goodOutput(fnName), nil)

	b := w.built(t, "production", fnName)
	if _, err := w.svc.Deploy(ctx, w.developer, b.ID, ""); code(err) != plxerr.PermissionDenied {
		t.Errorf("a developer deploying to production: %v", err)
	}
	if _, err := w.svc.Deploy(ctx, w.viewer, b.ID, ""); code(err) != plxerr.PermissionDenied {
		t.Errorf("a viewer deploying: %v", err)
	}
	owner := w.owner
	owner.SecondFactor = false
	if _, err := w.svc.Deploy(ctx, owner, b.ID, ""); code(err) != plxerr.MultiFactorRequired {
		t.Errorf("deploying to production without a second factor: %v", err)
	}
	v, err := w.svc.Deploy(ctx, w.owner, b.ID, "prod")
	if err != nil || v.Version != 1 {
		t.Fatalf("the owner deploying to production: %v %+v", err, v)
	}
	if _, err := w.svc.SetAlias(ctx, w.developer, w.app, w.envs["production"], fnName, "stable", 1); code(err) != plxerr.PermissionDenied {
		t.Errorf("a developer moving a production alias: %v", err)
	}
	if _, err := w.svc.SetAlias(ctx, owner, w.app, w.envs["production"], fnName, "stable", 1); code(err) != plxerr.MultiFactorRequired {
		t.Errorf("moving a production alias without a second factor: %v", err)
	}
	if _, err := w.svc.SetAlias(ctx, w.owner, w.app, w.envs["production"], fnName, "stable", 1); err != nil {
		t.Errorf("the owner moving a production alias: %v", err)
	}
	dev := w.built(t, "development", fnName)
	if _, err := w.svc.Deploy(ctx, w.developer, dev.ID, ""); err != nil {
		t.Errorf("a developer deploying to development: %v", err)
	}
	if _, err := w.svc.UploadSource(ctx, w.viewer, w.app, w.envs["development"], fnName, strings.NewReader("x")); code(err) != plxerr.PermissionDenied {
		t.Errorf("a viewer uploading a source: %v", err)
	}
	if _, err := w.svc.GetBuild(ctx, w.viewer, b.ID); err != nil {
		t.Errorf("a viewer reading a build: %v", err)
	}
}

// Verifies: FN-006.
// The database refuses to change a version, whoever asks.
func TestVersionsAreImmutable_FN_006(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t, nil, nil)
	w.builder.set(goodOutput(fnName), nil)
	if _, err := w.svc.Deploy(ctx, w.developer, w.built(t, "development", fnName).ID, ""); err != nil {
		t.Fatal(err)
	}
	err := w.db.InTx(ctx, storage.Tenant{OrganizationID: w.org}, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE function_versions SET module_sha256 = $1`, bytes.Repeat([]byte{1}, 32))
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("an update of a version: %v", err)
	}
	var sum []byte
	err = w.db.InTx(ctx, storage.Tenant{OrganizationID: w.org}, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT module_sha256 FROM function_versions`).Scan(&sum)
	})
	want := sha256.Sum256(goodOutput(fnName).Module)
	if err != nil || !bytes.Equal(sum, want[:]) {
		t.Errorf("the version changed: %x %v", sum, err)
	}
}

// Verifies: FN-018.
// Versions and key-value entries are bound to their organisation by
// row-level security, and a namespace is one function in one app and
// environment: another app's function of the same name has its own keys.
func TestIsolation_FN_018(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t, nil, nil)
	w.builder.set(goodOutput(fnName), nil)
	if _, err := w.svc.Deploy(ctx, w.developer, w.built(t, "development", fnName).ID, ""); err != nil {
		t.Fatal(err)
	}
	mine := function.KVScope{OrganizationID: w.org, AppID: w.app, EnvironmentID: w.envs["development"], Function: fnName}
	theirs := function.KVScope{OrganizationID: w.org, AppID: w.otherApp, EnvironmentID: w.otherEnvs["development"], Function: fnName}
	if err := w.svc.KVPut(ctx, mine, "counter", []byte("1")); err != nil {
		t.Fatal(err)
	}
	if _, found, err := w.svc.KVGet(ctx, theirs, "counter"); err != nil || found {
		t.Errorf("another app's namespace sees the key: %v %v", found, err)
	}
	if err := w.svc.KVPut(ctx, theirs, "counter", []byte("2")); err != nil {
		t.Fatal(err)
	}
	if v, found, err := w.svc.KVGet(ctx, mine, "counter"); err != nil || !found || string(v) != "1" {
		t.Errorf("my key = %q %v %v", v, found, err)
	}
	if _, found, _ := w.svc.KVGet(ctx, function.KVScope{OrganizationID: w.org, AppID: w.app, EnvironmentID: w.envs["production"], Function: fnName}, "counter"); found {
		t.Error("another environment's namespace sees the key")
	}
	if _, err := w.svc.ResolveVersion(ctx, w.org, w.otherApp, w.otherEnvs["development"], fnName, ""); code(err) != plxerr.FunctionNotFound {
		t.Errorf("another app resolves the function: %v", err)
	}

	// A second organisation sees none of the rows and cannot name the
	// environment.
	o2, err := w.tenancy.CreateOrganization(ctx, w.adminID, "beta", "Beta")
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"function_sources", "function_builds", "function_versions", "function_aliases", "function_kv"} {
		var n int
		err := w.db.InTx(ctx, storage.Tenant{OrganizationID: o2.ID}, func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&n)
		})
		if err != nil || n != 0 {
			t.Errorf("%s: another organisation sees %d rows (%v)", table, n, err)
		}
	}
	foreign := function.KVScope{OrganizationID: o2.ID, AppID: w.app, EnvironmentID: w.envs["development"], Function: fnName}
	if _, _, err := w.svc.KVGet(ctx, foreign, "counter"); code(err) != plxerr.ResourceNotFound {
		t.Errorf("another organisation reading by identifier: %v", err)
	}
	if err := w.svc.KVPut(ctx, foreign, "counter", []byte("x")); code(err) != plxerr.ResourceNotFound {
		t.Errorf("another organisation writing by identifier: %v", err)
	}
	err = w.db.InTx(ctx, storage.Tenant{OrganizationID: o2.ID}, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO function_kv (organization_id, app_id, environment_id, function_name, key, value)
			VALUES ($1, $2, $3, 'x.y', 'k', '\x00')`, w.org, w.app, w.envs["development"])
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "row-level security") {
		t.Errorf("a row written for another organisation: %v", err)
	}
}

// Verifies: FN-018.
// The key-value store keeps what was written, deletes it, and refuses a
// write past function.kvValueBytes, function.kvKeys or function.kvBytes in
// the transaction that writes.
func TestKeyValueStore_FN_018(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t, nil, func(l *limits.Set) {
		for k, v := range map[limits.Key]int64{limits.FunctionKVValueBytes: 8, limits.FunctionKVKeys: 3, limits.FunctionKVBytes: 24} {
			tight, err := l.Tighten(k, limits.ScopeApp, v)
			if err != nil {
				t.Fatal(err)
			}
			*l = tight
		}
	})
	sc := function.KVScope{OrganizationID: w.org, AppID: w.app, EnvironmentID: w.envs["development"], Function: fnName}
	if _, found, err := w.svc.KVGet(ctx, sc, "a"); err != nil || found {
		t.Fatalf("a missing key: %v %v", found, err)
	}
	if err := w.svc.KVPut(ctx, sc, "a", []byte("12345678")); err != nil {
		t.Fatalf("a value at the limit: %v", err)
	}
	if err := w.svc.KVPut(ctx, sc, "b", []byte("123456789")); code(err) != plxerr.FunctionKVLimitExceeded {
		t.Errorf("a value above kvValueBytes: %v", err)
	}
	if err := w.svc.KVPut(ctx, sc, "b", []byte("12345678")); err != nil {
		t.Fatal(err)
	}
	// 18 bytes held; "c"+8 bytes would make 27 > 24.
	if err := w.svc.KVPut(ctx, sc, "c", []byte("12345678")); code(err) != plxerr.FunctionKVLimitExceeded {
		t.Errorf("a write above kvBytes: %v", err)
	}
	if err := w.svc.KVPut(ctx, sc, "c", []byte("1")); err != nil {
		t.Fatal(err)
	}
	if err := w.svc.KVPut(ctx, sc, "d", nil); code(err) != plxerr.FunctionKVLimitExceeded {
		t.Errorf("a fourth key: %v", err)
	}
	if err := w.svc.KVPut(ctx, sc, "a", []byte("1")); err != nil {
		t.Errorf("an overwrite at the key limit: %v", err)
	}
	if v, found, err := w.svc.KVGet(ctx, sc, "a"); err != nil || !found || string(v) != "1" {
		t.Errorf("a = %q %v %v", v, found, err)
	}
	if existed, err := w.svc.KVDelete(ctx, sc, "a"); err != nil || !existed {
		t.Errorf("Delete: %v %v", existed, err)
	}
	if existed, _ := w.svc.KVDelete(ctx, sc, "a"); existed {
		t.Error("a deleted key was deleted again")
	}
	if err := w.svc.KVPut(ctx, sc, "", []byte("1")); code(err) != plxerr.InvalidFormat {
		t.Errorf("an empty key: %v", err)
	}
	// Concurrent writes of new keys cannot together pass kvKeys = 3: two
	// keys are held, so exactly one of many writers succeeds.
	var wg sync.WaitGroup
	results := make(chan error, 6)
	for i := range 6 {
		wg.Go(func() { results <- w.svc.KVPut(ctx, sc, fmt.Sprintf("new%d", i), []byte("1")) })
	}
	wg.Wait()
	close(results)
	ok := 0
	for err := range results {
		if err == nil {
			ok++
		} else if code(err) != plxerr.FunctionKVLimitExceeded {
			t.Errorf("a concurrent write: %v", err)
		}
	}
	if ok != 1 {
		t.Errorf("%d concurrent writers succeeded, want 1", ok)
	}
}

// Verifies: FN-016.
// A source archive is bounded by function.sourceArchiveSize, whatever the
// size the client announced; the refusal is PLX-7203 and stores nothing.
func TestSourceArchiveLimit_FN_016(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t, nil, func(l *limits.Set) {
		tight, err := l.Tighten(limits.FunctionSourceArchiveSize, limits.ScopeApp, 16)
		if err != nil {
			t.Fatal(err)
		}
		*l = tight
	})
	if _, err := w.svc.UploadSource(ctx, w.developer, w.app, w.envs["development"], fnName, strings.NewReader(strings.Repeat("x", 17))); code(err) != plxerr.FunctionBuildLimitExceeded {
		t.Errorf("an archive above the limit: %v", err)
	}
	src, err := w.svc.UploadSource(ctx, w.developer, w.app, w.envs["development"], fnName, strings.NewReader(strings.Repeat("x", 16)))
	if err != nil || src.Size != 16 || len(src.SHA256) != 32 {
		t.Fatalf("an archive at the limit: %v %+v", err, src)
	}
	if _, err := w.svc.UploadSource(ctx, w.developer, w.app, w.envs["development"], "Bad", strings.NewReader("x")); code(err) != plxerr.InvalidFormat {
		t.Errorf("a bad name: %v", err)
	}
	if _, err := w.svc.UploadSource(ctx, w.developer, w.app, w.envs["production"], fnName, strings.NewReader("x")); err != nil {
		t.Errorf("an upload to production needs only function.deploy: %v", err)
	}
	if _, err := w.svc.UploadSource(ctx, w.developer, w.app, w.otherEnvs["development"], fnName, strings.NewReader("x")); code(err) != plxerr.ResourceNotFound {
		t.Errorf("an environment of another app: %v", err)
	}
}

// riverEnqueuer enqueues on a River client set after the service exists.
type riverEnqueuer struct{ client *jobs.Client }

func (r *riverEnqueuer) Enqueue(ctx context.Context, tx pgx.Tx, job function.BuildJob) error {
	_, err := r.client.InsertTx(ctx, tx, job, &river.InsertOpts{Queue: jobs.QueueFunction, MaxAttempts: 3})
	return err //nolint:wrapcheck // a test
}

// buildWorker runs the build job of the service.
type buildWorker struct {
	river.WorkerDefaults[function.BuildJob]
	svc *function.Service
}

func (w *buildWorker) Work(ctx context.Context, job *river.Job[function.BuildJob]) error {
	return w.svc.RunBuild(ctx, job.Args) //nolint:wrapcheck // a test
}

// Verifies: FN-016, FN-009, SRV-024.
// RequestBuild enqueues the build job in its own transaction; the worker
// runs it through the Builder and the ArtifactSigner, and the build ends
// succeeded and signed.
func TestBuildJobPath_FN_016(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	enq := &riverEnqueuer{}
	w := newWorld(t, func(*world) function.Enqueuer { return enq }, nil)
	w.builder.set(goodOutput(fnName), nil)
	if err := jobs.Migrate(ctx, w.db.Pool()); err != nil {
		t.Fatal(err)
	}
	workers := jobs.NewWorkers()
	jobs.AddWorker(workers, &buildWorker{svc: w.svc})
	client, err := jobs.New(jobs.Options{Pool: w.db.Pool(), Workers: workers, Run: true})
	if err != nil {
		t.Fatal(err)
	}
	enq.client = client
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stop, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := client.Stop(stop); err != nil {
			t.Errorf("Stop: %v", err)
		}
	})

	src, err := w.svc.UploadSource(ctx, w.developer, w.app, w.envs["development"], fnName, strings.NewReader("source"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := w.svc.RequestBuild(ctx, w.developer, src.ID, function.PlacementServer)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		got, err := w.svc.GetBuild(ctx, w.developer, b.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.State == function.BuildSucceeded {
			if !got.Signed || got.ModuleSHA256 == nil || got.Provenance == nil || got.Provenance.SourceSHA256 == "" {
				t.Errorf("build = %+v", got)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the build is still %s", got.State)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, err := w.svc.Deploy(ctx, w.developer, b.ID, "prod"); err != nil {
		t.Errorf("deploying the job's build: %v", err)
	}
}

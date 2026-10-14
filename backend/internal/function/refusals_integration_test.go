// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// failureLog is a log handler that keeps what it was given.
type failureLog struct {
	mu      sync.Mutex
	records []string
}

func (l *failureLog) Enabled(context.Context, slog.Level) bool { return true }

func (l *failureLog) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	b.WriteString(r.Message)
	r.Attrs(func(a slog.Attr) bool {
		b.WriteString(" " + a.Key + "=" + a.Value.String())
		return true
	})
	l.mu.Lock()
	l.records = append(l.records, b.String())
	l.mu.Unlock()
	return nil
}

func (l *failureLog) WithAttrs([]slog.Attr) slog.Handler { return l }
func (l *failureLog) WithGroup(string) slog.Handler      { return l }

func (l *failureLog) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.records...)
}

// Verifies: FN-012.
func TestHostRefusesWhatIsNotWiredOrNotDeclared_FN_012(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t, nil, nil)
	if _, err := function.NewHost(function.HostOptions{Service: w.svc}); err == nil {
		t.Error("a host without grants was accepted")
	}
	if _, err := function.NewHost(function.HostOptions{}); err == nil {
		t.Error("a host without a service was accepted")
	}

	host, grants := newHost(t, w, nil, nil)
	g := w.grantFor(t, grants, "development", w.deployed(t, "development", fnName, declaredKV))
	if _, err := host.Invoke(ctx, g, "payments.refund", []byte(`{}`), 1, ""); code(err) != plxerr.FunctionCapabilityDenied {
		t.Errorf("a nested call on a host without an invoker: %v", err)
	}
	if _, found, err := host.Secret(ctx, g, "API_KEY"); code(err) != plxerr.ConfigStoreUnavailable || found {
		t.Errorf("a declared secret on a host without secrets: %v %v", found, err)
	}
}

// Verifies: FN-012, FN-006.
func TestHostRefusesAGrantForAVersionThatIsGone_FN_012(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t, nil, nil)
	host, grants := newHost(t, w, nil, nil)
	v := w.deployed(t, "development", fnName, declaredKV)
	missing := v
	missing.Version = 99
	g := w.grantFor(t, grants, "development", missing)

	if _, _, err := host.KVGet(ctx, g, "k"); code(err) != plxerr.FunctionNotFound {
		t.Errorf("KVGet: %v", err)
	}
	if _, _, err := host.Secret(ctx, g, "API_KEY"); code(err) != plxerr.FunctionNotFound {
		t.Errorf("Secret: %v", err)
	}
	if _, err := host.GetModule(ctx, g, function.HexSHA256(v.ModuleSHA256)); code(err) != plxerr.FunctionNotFound {
		t.Errorf("GetModule: %v", err)
	}
}

// Verifies: FN-005.
func TestModuleBytesThatAreNotTheBuildsAreRefused_FN_005(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t, nil, nil)
	host, grants := newHost(t, w, nil, nil)
	v := w.deployed(t, "development", fnName, declaredKV)
	g := w.grantFor(t, grants, "development", v)

	if m, err := w.svc.Module(ctx, v); err != nil || !bytes.Equal(m, []byte("\x00asm module of "+fnName)) {
		t.Fatalf("the stored module: %q %v", m, err)
	}
	// The store is not trusted: replace the object with other bytes.
	if err := w.store.Delete(ctx, v.ModuleKey); err != nil {
		t.Fatal(err)
	}
	if _, err := w.store.Put(ctx, v.ModuleKey, []byte("\x00asm substituted"), "application/wasm"); err != nil {
		t.Fatal(err)
	}
	if _, err := w.svc.Module(ctx, v); err == nil || !strings.Contains(err.Error(), "not the one that was built") {
		t.Errorf("a substituted module was served: %v", err)
	}
	if _, err := host.GetModule(ctx, g, function.HexSHA256(v.ModuleSHA256)); err == nil {
		t.Error("the host served a substituted module")
	}

	if err := w.store.Delete(ctx, v.ModuleKey); err != nil {
		t.Fatal(err)
	}
	if _, err := w.svc.Module(ctx, v); err == nil || !strings.Contains(err.Error(), "read the module") {
		t.Errorf("a missing module: %v", err)
	}
}

// Verifies: FN-018.
func TestKeyValueRefusesAnEmptyKeyAndAnInvalidScope_FN_018(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t, nil, nil)
	scope := function.KVScope{OrganizationID: w.org, AppID: w.app, EnvironmentID: w.envs["development"], Function: fnName}

	if err := w.svc.KVPut(ctx, scope, "", []byte("v")); code(err) != plxerr.InvalidFormat {
		t.Errorf("an empty key: %v", err)
	}
	bad := scope
	bad.Function = "NOT A NAME"
	if _, _, err := w.svc.KVGet(ctx, bad, "k"); err == nil {
		t.Error("a function name that is not a name reached the store")
	}
	foreign := scope
	foreign.EnvironmentID = w.otherEnvs["development"]
	if err := w.svc.KVPut(ctx, foreign, "k", []byte("v")); code(err) != plxerr.ResourceNotFound {
		t.Errorf("an environment of another app: %v", err)
	}
	if _, err := w.svc.KVDelete(ctx, foreign, "k"); code(err) != plxerr.ResourceNotFound {
		t.Errorf("a delete in an environment of another app: %v", err)
	}
	malformed := scope
	malformed.EnvironmentID = "not-a-uuid"
	if _, _, err := w.svc.KVGet(ctx, malformed, "k"); code(err) != plxerr.InvalidFormat {
		t.Errorf("a malformed environment: %v", err)
	}
}

// Verifies: FN-006.
func TestPinnedIsFalseWithoutAnActiveReleaseAndRefusesBadIdentifiers_FN_006(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t, nil, nil)
	v := w.deployed(t, "development", fnName, declaredKV)

	pin, found, err := w.svc.Pinned(ctx, w.org, w.app, w.envs["development"], fnName, v.Version)
	if err != nil || found || pin.ModuleSHA256 != nil {
		t.Errorf("a deployed version no release pins: %+v %v %v", pin, found, err)
	}
	if _, _, err := w.svc.Pinned(ctx, w.org, "nope", w.envs["development"], fnName, 1); code(err) != plxerr.InvalidFormat {
		t.Errorf("a malformed app: %v", err)
	}
	if _, _, err := w.svc.Pinned(ctx, w.org, w.app, "nope", fnName, 1); code(err) != plxerr.InvalidFormat {
		t.Errorf("a malformed environment: %v", err)
	}
}

// Verifies: FN-006, FN-012.
func TestInvocableIsReadFromTheEnvironmentOfTheApp_FN_006(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t, nil, nil)
	v := w.deployed(t, "development", fnName, declaredKV)

	inv, err := w.svc.LoadInvocable(ctx, w.org, w.app, w.envs["development"], fnName, v.Version)
	if err != nil || !inv.Declaration.Capabilities.KV || inv.Version.Version != v.Version {
		t.Fatalf("LoadInvocable: %+v %v", inv, err)
	}
	if _, err := w.svc.LoadInvocable(ctx, w.org, w.app, w.envs["development"], fnName, 99); code(err) != plxerr.FunctionNotFound {
		t.Errorf("a version that does not exist: %v", err)
	}
	if _, err := w.svc.LoadInvocable(ctx, w.org, w.app, w.otherEnvs["development"], fnName, v.Version); code(err) != plxerr.ResourceNotFound {
		t.Errorf("an environment of another app: %v", err)
	}
	if _, err := w.svc.LoadInvocable(ctx, w.org, w.app, "nope", fnName, v.Version); code(err) != plxerr.InvalidFormat {
		t.Errorf("a malformed environment: %v", err)
	}
	// A version whose build has no declaration cannot be decided on.
	w.builder.set(goodOutput("payments.bare"), nil)
	bare := w.built(t, "development", "payments.bare")
	if _, err := w.svc.Deploy(ctx, w.developer, bare.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := w.svc.LoadInvocable(ctx, w.org, w.app, w.envs["development"], "payments.bare", 1); err == nil {
		t.Error("a version without a declaration was loaded")
	}
}

// Verifies: FN-020.
func TestEnvironmentVariablesAreOnlyThoseNamedAndHeld_FN_020(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t, nil, nil)
	env := w.envs["development"]
	for k, v := range map[string]string{"REGION": "eu", "TIER": "gold", "UNUSED": "x"} {
		if _, err := w.tenancy.SetVariable(ctx, w.owner, env, k, v); err != nil {
			t.Fatal(err)
		}
	}

	got, err := w.svc.EnvironmentVariables(ctx, w.org, w.app, env, []string{"REGION", "TIER", "ABSENT"})
	if err != nil || len(got) != 2 || got["REGION"] != "eu" || got["TIER"] != "gold" {
		t.Errorf("variables = %v %v; want the two held names only", got, err)
	}
	if none, err := w.svc.EnvironmentVariables(ctx, w.org, w.app, env, nil); err != nil || len(none) != 0 {
		t.Errorf("no names: %v %v", none, err)
	}
	if _, err := w.svc.EnvironmentVariables(ctx, w.org, w.app, w.otherEnvs["development"], []string{"REGION"}); code(err) != plxerr.ResourceNotFound {
		t.Errorf("an environment of another app: %v", err)
	}
	other, err := w.svc.EnvironmentVariables(ctx, w.org, w.otherApp, w.otherEnvs["development"], []string{"REGION"})
	if err != nil || len(other) != 0 {
		t.Errorf("another app's environment sees %v (%v)", other, err)
	}
}

// Verifies: FN-012, SEC-120.
func TestGrantSecretIsSharedByEveryReplica_FN_012(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t, nil, nil)
	backend, err := signing.NewFile(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	clock := func() time.Time { return now }
	first, err := function.OpenGrants(ctx, w.db, backend, clock)
	if err != nil {
		t.Fatal(err)
	}
	second, err := function.OpenGrants(ctx, w.db, backend, clock)
	if err != nil {
		t.Fatal(err)
	}
	claims := function.GrantClaims{OrganizationID: w.org, AppID: w.app, EnvironmentID: w.envs["development"], Function: fnName, Version: 1}
	g, _, err := first.Issue(claims, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := second.Verify(g); err != nil || got.Function != fnName {
		t.Errorf("another replica refused the grant: %+v %v", got, err)
	}
	stranger, err := function.NewGrants(bytes.Repeat([]byte{7}, 32), clock, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stranger.Verify(g); code(err) != plxerr.AuthenticationRequired {
		t.Errorf("a grant signed with another secret: %v", err)
	}
}

// Verifies: FN-032, SEC-092.
func TestRecordingFailuresAreLoggedWithoutFunctionOutput_FN_032(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t, nil, nil)
	sink := &failureLog{}
	svc := w.service(t, func(o *function.Options) { o.Log = slog.New(sink) })

	meta := w.meta(1)
	meta.AppID = "not-a-uuid"
	svc.Record(ctx, meta, []function.LogLine{{Level: 1, Message: "secret-looking output", Time: time.Now()}})
	svc.Observe(ctx, function.Observation{OrganizationID: w.org, AppID: w.app, EnvironmentID: "nope", Function: "bank.fee", Version: 1, Result: "ok"})
	svc.Observe(ctx, function.Observation{OrganizationID: "nope", AppID: w.app, EnvironmentID: w.envs["development"], Function: "bank.fee", Version: 1, Result: "ok"})

	logged := sink.all()
	if len(logged) != 3 {
		t.Fatalf("logged %d failures, want 3: %q", len(logged), logged)
	}
	for _, line := range logged {
		if strings.Contains(line, "secret-looking output") {
			t.Errorf("a log line carries function output: %q", line)
		}
	}
	if !strings.Contains(logged[0], "storing function logs failed") || !strings.Contains(logged[0], "invocation app") {
		t.Errorf("first failure = %q", logged[0])
	}
	if !strings.Contains(logged[1], "storing a function invocation failed") || !strings.Contains(logged[1], "invocation environment") {
		t.Errorf("second failure = %q", logged[1])
	}

	lines, _, err := w.svc.ListLogs(ctx, w.viewer, w.query(0), storage.Cursor{}, 10)
	if err != nil || len(lines) != 0 {
		t.Errorf("a failed recording stored %d lines (%v)", len(lines), err)
	}
	// Without a logger a failure is dropped, and recording nothing is a no-op.
	w.svc.Record(ctx, meta, []function.LogLine{{Message: "x", Time: time.Now()}})
	w.svc.Record(ctx, w.meta(1), nil)
}

// Verifies: FN-032.
func TestStoredLogLinesAreBoundedAndClean_FN_032(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t, nil, nil)
	meta := w.meta(1)
	meta.Traceparent = "00-ZZZZ651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
	long := strings.Repeat("a", 8*1024-1) + "é" + "tail"
	now := time.Now()
	w.svc.Record(ctx, meta, []function.LogLine{
		{Level: 0, Message: "debug line", Time: now.Add(-4 * time.Second)},
		{Level: 3, Message: "nul\x00byte", Time: now.Add(-3 * time.Second)},
		{Level: 99, Message: "bad \xff utf8", Time: now.Add(-2 * time.Second)},
		{Level: 2, Message: long, Time: now.Add(-time.Second)},
	})

	lines, _, err := w.svc.ListLogs(ctx, w.viewer, w.query(1), storage.Cursor{}, 10)
	if err != nil || len(lines) != 4 {
		t.Fatalf("ListLogs: %d lines %v", len(lines), err)
	}
	// Newest first.
	clipped, bad, nul, debug := lines[0], lines[1], lines[2], lines[3]
	if clipped.Level != "warn" || !strings.HasSuffix(clipped.Message, "…") || strings.Contains(clipped.Message, "tail") || len(clipped.Message) > 8*1024+len("…") || strings.Contains(clipped.Message, "�") {
		t.Errorf("a long line was not cut on a character boundary: level %q, %d bytes", clipped.Level, len(clipped.Message))
	}
	if bad.Level != "error" || bad.Message != "bad � utf8" {
		t.Errorf("an unknown level and invalid UTF-8: %q %q", bad.Level, bad.Message)
	}
	if nul.Level != "error" || nul.Message != "nulbyte" {
		t.Errorf("a NUL byte reached the store: %q %q", nul.Level, nul.Message)
	}
	if debug.Level != "debug" {
		t.Errorf("level 0 = %q", debug.Level)
	}
	for _, l := range lines {
		if l.TraceID != "" {
			t.Errorf("a traceparent that is not W3C produced the trace id %q", l.TraceID)
		}
	}
}

// Verifies: FN-006, SRV-022.
func TestReadingFunctionsNeedsThePermissionAndTheAppsEnvironment_FN_006(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t, nil, nil)
	env := w.envs["development"]
	w.deployed(t, "development", fnName, declaredKV)
	stranger := auth.Principal{OrganizationID: w.org}

	if _, err := w.svc.GetFunction(ctx, stranger, w.app, env, fnName); err == nil {
		t.Error("a principal without function.read got a function")
	}
	if _, err := w.svc.ListFunctions(ctx, stranger, w.app, env, "", 10); err == nil {
		t.Error("a principal without function.read listed functions")
	}
	if _, err := w.svc.ListVersions(ctx, stranger, w.app, env, fnName, 0, 10); err == nil {
		t.Error("a principal without function.read listed versions")
	}

	if _, err := w.svc.GetFunction(ctx, w.viewer, w.app, env, "payments.missing"); code(err) != plxerr.FunctionNotFound {
		t.Errorf("a function that is not deployed: %v", err)
	}
	foreign := w.otherEnvs["development"]
	if _, err := w.svc.GetFunction(ctx, w.viewer, w.app, foreign, fnName); code(err) != plxerr.ResourceNotFound {
		t.Errorf("GetFunction in an environment of another app: %v", err)
	}
	if _, err := w.svc.ListFunctions(ctx, w.viewer, w.app, foreign, "", 10); code(err) != plxerr.ResourceNotFound {
		t.Errorf("ListFunctions in an environment of another app: %v", err)
	}
	if _, err := w.svc.ListVersions(ctx, w.viewer, w.app, foreign, fnName, 0, 10); code(err) != plxerr.ResourceNotFound {
		t.Errorf("ListVersions in an environment of another app: %v", err)
	}

	got, err := w.svc.GetFunction(ctx, w.viewer, w.app, env, fnName)
	if err != nil || got.Latest.Name != fnName {
		t.Errorf("a viewer reads a function: %+v %v", got, err)
	}
	if vs, err := w.svc.ListVersions(ctx, w.viewer, w.app, env, fnName, 1, 10); err != nil || len(vs) != 0 {
		t.Errorf("versions below 1: %d %v", len(vs), err)
	}
	if fs, err := w.svc.ListFunctions(ctx, w.viewer, w.app, env, fnName, 10); err != nil || len(fs) != 0 {
		t.Errorf("functions after %q: %d %v", fnName, len(fs), err)
	}
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/bundle"
	"github.com/nightCode42/plux3/backend/internal/bundle/fbs"
	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/release"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/security/settings"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// linkerFake links by joining the names of its sources, and keeps what it
// was asked.
type linkerFake struct {
	mu    sync.Mutex
	calls [][]string
	diags plxerr.Diagnostics
}

func (l *linkerFake) Link(_ context.Context, in function.LinkInput) (function.LinkOutput, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.diags.HasErrors() {
		return function.LinkOutput{Diagnostics: l.diags}, nil
	}
	module, index, names := []byte("\x00asm linked:"), map[string]uint32{}, []string{}
	for i, s := range in.Sources {
		module = append(module, s.Name+";"...)
		index[s.Name] = uint32(i) //nolint:gosec // G115: a handful of sources.
		names = append(names, s.Name)
	}
	l.calls = append(l.calls, names)
	return function.LinkOutput{Module: module, ABIVersion: 1, Index: index}, nil
}

func (l *linkerFake) linked() [][]string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([][]string(nil), l.calls...)
}

// withFunctions gives the release service the function service as the
// server wires it: the catalogue reads the effective security setting
// deviceFunctions unless the test overrides it.
func withFunctions(t *testing.T, linker function.Linker, deviceFunctions func(context.Context, string, string) (bool, error)) func(*release.Options) {
	t.Helper()
	return func(o *release.Options) {
		if deviceFunctions == nil {
			deviceFunctions = func(ctx context.Context, appID, environmentID string) (bool, error) {
				values, _, err := o.SecurityConfig.Effective(ctx, appID, environmentID)
				return values.Get(settings.DeviceFunctions).Bool(), err
			}
		}
		svc, err := function.NewService(function.Options{
			DB: o.DB, Audit: o.Audit, Objects: o.Objects, IDs: o.IDs, Linker: linker, DeviceFunctions: deviceFunctions,
			Limits: func(ctx context.Context, tx pgx.Tx, organizationID, appID string) (limits.Set, error) {
				return o.Tenancy.Effective(ctx, tx, organizationID, appID, "")
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		o.Functions = svc
	}
}

// declareDeployed stores the declaration of every deployed version, as the
// worker does when a build succeeds.
func (f *fixture) declareDeployed(t *testing.T) {
	t.Helper()
	err := f.db.InTx(context.Background(), storage.Tenant{OrganizationID: f.org}, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO function_declarations (build_id, organization_id, declaration)
		  SELECT build_id, organization_id, jsonb_build_object('name', name, 'placement', placement)
		    FROM function_versions ON CONFLICT DO NOTHING`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// deployDeclared deploys versions with their declarations.
func (f *fixture) deployDeclared(t *testing.T, versions ...deployed) {
	t.Helper()
	f.deploy(t, versions...)
	f.declareDeployed(t)
}

// bundleOf reads the stored bundle of a plugin version.
func (f *fixture) bundleOf(t *testing.T, plugin string, version int64) *bundle.Bundle {
	t.Helper()
	ctx := context.Background()
	v, err := f.rel.GetVersion(ctx, f.viewer, plugin, version)
	if err != nil {
		t.Fatal(err)
	}
	data, _, err := f.store.Get(ctx, "bundles/"+v.BundleSHA256[:2]+"/"+v.BundleSHA256)
	if err != nil {
		t.Fatal(err)
	}
	b, err := bundle.Read(data, bundle.ReadOptions{Limits: limits.Defaults(), Supports: func(string) bool { return true }})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// wasmOf returns the wasm section of a bundle, or nil.
func wasmOf(b *bundle.Bundle) *fbs.WasmModule {
	for _, s := range b.Sections {
		if s.Kind == bundle.SectionWasm {
			return fbs.GetRootAsWasmModule(s.Data, 0)
		}
	}
	return nil
}

// grantVersions returns the version each function grant of a plugin bundle
// carries.
func grantVersions(b *bundle.Bundle) map[string]uint32 {
	caps := b.Meta.Capabilities(nil)
	out := map[string]uint32{}
	var g fbs.FunctionGrant
	for i := range caps.FunctionsLength() {
		caps.Functions(&g, i)
		out[string(g.Function())] = g.Version()
	}
	return out
}

// Verifies: FN-051, FN-006, FN-055, ACT-001.
// A publish resolves the functions a plugin grants against the environment:
// the bundle's grants carry the resolved versions, its wasm section holds
// the one module linking the plugin's device functions, a release created
// afterwards recompiles to the same bundle and pins the same versions, and
// the same set is never linked twice.
func TestPublishPacksTheDeviceModuleAndPinsTheVersions_FN_051(t *testing.T) {
	t.Parallel()
	linker := &linkerFake{}
	f := newFixtureWith(t, "loan-calculator", limits.Set{}, withFunctions(t, linker, nil))
	ctx := context.Background()
	f.grantFunctions(t, [][2]string{{"loans.quote", ""}, {"loans.rate", "stable"}, {"loans.score", ""}, {"loans.risk", ""}})
	f.deployDeclared(t,
		deployed{"loans.quote", 1, "server", true, ""}, deployed{"loans.quote", 2, "server", true, ""},
		deployed{"loans.rate", 1, "server", true, "stable"}, deployed{"loans.rate", 2, "server", true, ""},
		deployed{"loans.score", 1, "device", true, ""}, deployed{"loans.risk", 1, "device", true, ""}, deployed{"loans.risk", 2, "device", true, ""})
	f.publishAll(t)

	b := f.bundleOf(t, f.loans, 1)
	if got := grantVersions(b); got["loans.quote"] != 2 || got["loans.rate"] != 1 || got["loans.score"] != 1 || got["loans.risk"] != 2 || len(got) != 4 {
		t.Errorf("grant versions = %v; the newest version, or the alias's", got)
	}
	w := wasmOf(b)
	if w == nil || w.FunctionsLength() != 2 || !strings.HasPrefix(string(w.ModuleBytes()), "\x00asm linked:loans.risk;loans.score;") {
		t.Fatalf("the wasm section = %v; want one module linking the two device functions, by name", w)
	}
	var d fbs.DeviceFunction
	for i, want := range []struct {
		name    string
		version uint32
	}{{"loans.risk", 2}, {"loans.score", 1}} {
		w.Functions(&d, i)
		if string(d.Name()) != want.name || d.Version() != want.version || d.Index() != uint32(i) { //nolint:gosec // G115: two functions.
			t.Errorf("device function %d = %s v%d at %d, want %+v", i, d.Name(), d.Version(), d.Index(), want)
		}
	}
	if got := linker.linked(); len(got) != 1 || strings.Join(got[0], ",") != "loans.risk,loans.score" {
		t.Errorf("links = %v; one link of the two sources, by name", got)
	}

	// The release recompiles the versions' sources; the module comes from
	// the cache, so the bundles are the ones that were published.
	rel, diags, err := f.rel.CreateRelease(ctx, f.owner, f.app, f.envs["development"].ID, nil, "First")
	if err != nil || diags.HasErrors() {
		t.Fatalf("CreateRelease: %v %v", diags, err)
	}
	wantPinned(t, rel.Functions,
		deployed{"loans.quote", 2, "server", true, ""}, deployed{"loans.rate", 1, "server", true, ""},
		deployed{"loans.risk", 2, "device", true, ""}, deployed{"loans.score", 1, "device", true, ""})
	if len(linker.linked()) != 1 {
		t.Errorf("creating the release linked again: %v", linker.linked())
	}

	// Publishing the same plugin again links nothing.
	f.publish(t, f.loans, false)
	if len(linker.linked()) != 1 {
		t.Errorf("a second publish linked again: %v", linker.linked())
	}
	if again := wasmOf(f.bundleOf(t, f.loans, 2)); again == nil || string(again.ModuleBytes()) != string(w.ModuleBytes()) {
		t.Error("the second publish packed another module")
	}
}

// Verifies: FN-051.
// A plugin whose functions are all server functions has no wasm section and
// is never linked.
func TestPublishLinksNothingForServerFunctions_FN_051(t *testing.T) {
	t.Parallel()
	linker := &linkerFake{}
	f := newFixtureWith(t, "loan-calculator", limits.Set{}, withFunctions(t, linker, nil))
	f.grantFunctions(t, [][2]string{{"loans.quote", ""}})
	f.deployDeclared(t, deployed{"loans.quote", 1, "server", true, ""})
	f.publishAll(t)
	b := f.bundleOf(t, f.loans, 1)
	if wasmOf(b) != nil || len(linker.linked()) != 0 || grantVersions(b)["loans.quote"] != 1 {
		t.Errorf("wasm %v, links %v, grants %v", wasmOf(b), linker.linked(), grantVersions(b))
	}
}

// Verifies: FN-051.
// A set of device functions that does not link fails the publish with the
// builder's diagnostics, and nothing is recorded.
func TestPublishFailsWithTheBuildersDiagnosticsWhenTheSetDoesNotLink_FN_051(t *testing.T) {
	t.Parallel()
	linker := &linkerFake{diags: plxerr.Diagnostics{plxerr.NewDiagnostic(plxerr.FunctionBuildFailed, plxerr.Location{}, "error[E0425]: cannot find value")}}
	f := newFixtureWith(t, "loan-calculator", limits.Set{}, withFunctions(t, linker, nil))
	f.grantFunctions(t, [][2]string{{"loans.score", ""}})
	f.deployDeclared(t, deployed{"loans.score", 1, "device", true, ""})
	f.publish(t, "", false)
	job := f.publish(t, f.loans, false)
	if job.State != release.StateFailed || len(job.Diagnostics) == 0 || job.Diagnostics[0].Code != plxerr.FunctionModuleInvalid ||
		!strings.Contains(job.Diagnostics[0].Message, "error[E0425]") {
		t.Fatalf("job = %+v", job)
	}
	if _, err := f.rel.GetVersion(context.Background(), f.viewer, f.loans, 1); code(err) != plxerr.ResourceNotFound {
		t.Errorf("a failed publish left a version: %v", err)
	}
}

// Verifies: FN-055, FN-008, ACT-001.
// The compiler checks the grants against the catalogue at publish: a device
// function needs the security setting deviceFunctions, and an unknown
// function or alias is refused.
func TestPublishChecksGrantsAgainstTheCatalogue_FN_055(t *testing.T) {
	t.Parallel()
	off := func(context.Context, string, string) (bool, error) { return false, nil }
	cases := []struct {
		name    string
		grants  [][2]string
		deploy  []deployed
		devices func(context.Context, string, string) (bool, error)
		want    plxerr.Code
	}{
		{"device functions off", [][2]string{{"loans.score", ""}}, []deployed{{"loans.score", 1, "device", true, ""}}, off, plxerr.DeviceFunctionsDisabled},
		{"not deployed", [][2]string{{"loans.quote", ""}}, nil, nil, plxerr.FunctionReferenceUnresolved},
		{"no such alias", [][2]string{{"loans.quote", "stable"}}, []deployed{{"loans.quote", 1, "server", true, ""}}, nil, plxerr.FunctionReferenceUnresolved},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			f := newFixtureWith(t, "loan-calculator", limits.Set{}, withFunctions(t, &linkerFake{}, c.devices))
			f.grantFunctions(t, c.grants)
			f.deployDeclared(t, c.deploy...)
			f.publish(t, "", false)
			job := f.publish(t, f.loans, false)
			if job.State != release.StateFailed || len(job.Diagnostics) == 0 || job.Diagnostics[0].Code != c.want {
				t.Errorf("job = %+v; want a failure with %s", job, c.want)
			}
		})
	}
}

// Verifies: FN-006, REL-003.
// A bundle carries the function versions it was compiled against, so a
// version deployed after the publish makes the release refuse the stale
// bundle until the plugin is published again.
func TestReleaseRefusesBundlesCompiledAgainstOlderFunctionVersions_FN_006(t *testing.T) {
	t.Parallel()
	f := newFixtureWith(t, "loan-calculator", limits.Set{}, withFunctions(t, &linkerFake{}, nil))
	ctx := context.Background()
	dev := f.envs["development"].ID
	f.grantFunctions(t, [][2]string{{"loans.quote", ""}})
	f.deployDeclared(t, deployed{"loans.quote", 1, "server", true, ""})
	f.publishAll(t)
	f.deployDeclared(t, deployed{"loans.quote", 2, "server", true, ""})

	_, diags, err := f.rel.CreateRelease(ctx, f.owner, f.app, dev, nil, "Stale")
	if err != nil || !diags.HasErrors() || diags[0].Code != plxerr.ReleaseInconsistent {
		t.Fatalf("CreateRelease = %v, %v; want %s", diags, err, plxerr.ReleaseInconsistent)
	}
	f.publish(t, f.loans, false)
	rel, diags, err := f.rel.CreateRelease(ctx, f.owner, f.app, dev, nil, "Fresh")
	if err != nil || diags.HasErrors() {
		t.Fatalf("CreateRelease after republishing: %v %v", diags, err)
	}
	wantPinned(t, rel.Functions, deployed{"loans.quote", 2, "server", true, ""})
}

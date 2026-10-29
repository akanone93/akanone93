// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

const grantedDeclaration = `{"name":"payments.charge","placement":"server","assurance":"hardware",` +
	`"capabilities":{"http":["api.example.com"],"kv":true,"secrets":["key"],"functions":["payments.refund"],"claims":true}}`

// wantGranted is the capabilities grantedDeclaration declares.
var wantGranted = function.Capabilities{
	HTTP: []string{"api.example.com"}, KV: true, Secrets: []string{"key"}, Functions: []string{"payments.refund"}, Claims: true,
}

// checkFields fails the test unless v reports the source id, the assurance
// and the capabilities its build declared.
func checkFields(t *testing.T, where string, v function.Version, source string) {
	t.Helper()
	if v.SourceID != source {
		t.Errorf("%s: source = %q, want %q", where, v.SourceID, source)
	}
	if v.Declared == nil {
		t.Errorf("%s: no declaration", where)
		return
	}
	if v.Declared.Assurance != "hardware" || !reflect.DeepEqual(v.Declared.Capabilities, wantGranted) {
		t.Errorf("%s: declared = %+v, want assurance hardware and %+v", where, v.Declared, wantGranted)
	}
}

// Verifies: FN-006, FN-012.
// Every read of a version reports the source it was built from and what its
// build declared: after a deploy, in the function and version lists, and
// after a promotion, which reports the build the copy was made from. The
// values come from the build's records, so nothing is stored on the version.
func TestVersionReportsCapabilitiesAssuranceAndSource_FN_012(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t, nil, nil)
	w.builder.set(goodOutput(fnName), nil)
	b := w.built(t, "development", fnName)
	w.declare(t, b.ID, grantedDeclaration)
	v, err := w.svc.Deploy(ctx, w.developer, b.ID, "stable")
	if err != nil {
		t.Fatal(err)
	}
	checkFields(t, "Deploy", v, b.SourceID)

	dev := w.envs["development"]
	f, err := w.svc.GetFunction(ctx, w.viewer, w.app, dev, fnName)
	if err != nil {
		t.Fatal(err)
	}
	checkFields(t, "GetFunction", f.Latest, b.SourceID)
	fns, err := w.svc.ListFunctions(ctx, w.viewer, w.app, dev, "", 10)
	if err != nil || len(fns) != 1 {
		t.Fatalf("ListFunctions: %+v %v", fns, err)
	}
	checkFields(t, "ListFunctions", fns[0], b.SourceID)
	vs, err := w.svc.ListVersions(ctx, w.viewer, w.app, dev, fnName, 0, 10)
	if err != nil || len(vs) != 1 {
		t.Fatalf("ListVersions: %+v %v", vs, err)
	}
	checkFields(t, "ListVersions", vs[0], b.SourceID)

	prod := w.envs["production"]
	promoted, err := w.svc.Promote(ctx, w.owner, w.app, dev, prod, fnName, v.Version, "")
	if err != nil {
		t.Fatal(err)
	}
	checkFields(t, "Promote", promoted, b.SourceID)
	again, err := w.svc.Promote(ctx, w.owner, w.app, dev, prod, fnName, v.Version, "")
	if err != nil {
		t.Fatal(err)
	}
	checkFields(t, "Promote of a held version", again, b.SourceID)
	inProd, err := w.svc.ListVersions(ctx, w.viewer, w.app, prod, fnName, 0, 10)
	if err != nil || len(inProd) != 1 {
		t.Fatalf("ListVersions in production: %+v %v", inProd, err)
	}
	checkFields(t, "ListVersions in production", inProd[0], b.SourceID)
}

// Verifies: FN-012.
// A version whose build recorded no declaration, as every version deployed
// before declarations were kept, is granted nothing: it reports no
// declaration, its source id is still known, and the page of versions is
// still served beside versions that have one.
func TestVersionWithoutDeclarationIsGrantedNothing_FN_012(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t, nil, nil)
	w.builder.set(goodOutput(fnName), nil)
	bare := w.built(t, "development", fnName)
	v1, err := w.svc.Deploy(ctx, w.developer, bare.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if v1.Declared != nil || v1.SourceID != bare.SourceID || v1.SourceID == "" {
		t.Errorf("a version without a declaration = source %q, declared %+v", v1.SourceID, v1.Declared)
	}
	declared := w.deployed(t, "development", fnName, grantedDeclaration)
	checkFields(t, "Deploy", declared, declared.SourceID)

	vs, err := w.svc.ListVersions(ctx, w.viewer, w.app, w.envs["development"], fnName, 0, 10)
	if err != nil || len(vs) != 2 {
		t.Fatalf("ListVersions: %+v %v", vs, err)
	}
	if vs[0].Version != 2 || vs[0].Declared == nil || vs[1].Version != 1 || vs[1].Declared != nil || vs[1].SourceID != bare.SourceID {
		t.Errorf("versions = %+v", vs)
	}
	// The invocation path agrees: a version without a declaration is not run.
	if _, err := w.svc.LoadInvocable(ctx, w.org, w.app, w.envs["development"], fnName, 1); err == nil {
		t.Error("a version without a declaration was loaded")
	}
}

// Verifies: FN-006, FN-012.
// The capabilities a version reports are exactly the ones its build
// declared: a function that declares none is granted none.
func TestVersionCapabilitiesAreOnlyThoseDeclared_FN_012(t *testing.T) {
	t.Parallel()
	w := newWorld(t, nil, nil)
	v := w.deployed(t, "development", fnName, `{"name":"payments.charge","placement":"server"}`)
	if v.Declared == nil || !reflect.DeepEqual(v.Declared.Capabilities, function.Capabilities{}) || v.Declared.Assurance != "" {
		t.Errorf("declared = %+v, want nothing granted and no assurance", v.Declared)
	}
}

// Verifies: FN-018, SRV-022.
// A version's source id is an identifier, not a grant: another organisation
// cannot use it to read or build the source, a member without function.deploy
// on the app cannot use it either.
func TestSourceIDOfAVersionOpensNothingElsewhere_FN_018(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t, nil, nil)
	v := w.deployed(t, "development", fnName, grantedDeclaration)
	if v.SourceID == "" {
		t.Fatal("the version reports no source")
	}

	o2, err := w.tenancy.CreateOrganization(ctx, w.adminID, "beta", "Beta")
	if err != nil {
		t.Fatal(err)
	}
	stranger, err := w.auth.Resolve(ctx, w.adminID, o2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.svc.RequestBuild(ctx, stranger, v.SourceID, ""); code(err) != plxerr.ResourceNotFound {
		t.Errorf("another organisation builds the source: %v", err)
	}
	if _, err := w.svc.RunEditorTests(ctx, stranger, v.SourceID); code(err) != plxerr.ResourceNotFound {
		t.Errorf("another organisation tests the source: %v", err)
	}
	if _, err := w.svc.GetFunction(ctx, stranger, w.app, w.envs["development"], fnName); err == nil {
		t.Error("another organisation reads the function")
	}

	// A viewer reads the version, and so the id, but holds no permission to
	// use the source.
	if _, err := w.svc.RequestBuild(ctx, w.viewer, v.SourceID, ""); code(err) != plxerr.PermissionDenied {
		t.Errorf("a viewer builds the source: %v", err)
	}
}

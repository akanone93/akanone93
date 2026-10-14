// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Verifies: FN-006.
// A version number is counted per function per app: a build deployed in
// production after one in development takes the next number, and a version
// promoted to production keeps its number, module, signature and build.
func TestVersionNumbersAreCountedPerApp_FN_006(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t, nil, nil)
	dev, prod := w.envs["development"], w.envs["production"]

	w.builder.set(goodOutput(fnName), nil)
	v1, err := w.svc.Deploy(ctx, w.developer, w.built(t, "development", fnName).ID, "")
	if err != nil || v1.Version != 1 {
		t.Fatalf("development deploy: %v %+v", err, v1)
	}
	other := goodOutput(fnName)
	other.Module = append(other.Module, "-production"...)
	w.builder.set(other, nil)
	v2, err := w.svc.Deploy(ctx, w.owner, w.built(t, "production", fnName).ID, "")
	if err != nil || v2.Version != 2 {
		t.Fatalf("production deploy after a development deploy: %v %+v", err, v2)
	}

	promoted, err := w.svc.Promote(ctx, w.owner, w.app, dev, prod, fnName, 1, "stable")
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if promoted.Version != 1 || promoted.EnvironmentID != prod || promoted.BuildID != v1.BuildID ||
		!bytes.Equal(promoted.ModuleSHA256, v1.ModuleSHA256) || !bytes.Equal(promoted.Signature, v1.Signature) ||
		!bytes.Equal(promoted.Statement, v1.Statement) || promoted.KeyID != v1.KeyID || promoted.ModuleKey != v1.ModuleKey ||
		promoted.InputSchema != v1.InputSchema || promoted.TestsPassed != v1.TestsPassed {
		t.Errorf("promoted = %+v, source = %+v", promoted, v1)
	}
	if bytes.Equal(v2.ModuleSHA256, v1.ModuleSHA256) {
		t.Fatal("the test needs two different modules")
	}

	list, err := w.svc.ListVersions(ctx, w.owner, w.app, prod, fnName, 0, 10)
	if err != nil || len(list) != 2 || list[0].Version != 2 || list[1].Version != 1 {
		t.Fatalf("production versions = %+v, %v", list, err)
	}
	res, err := w.svc.ResolveVersion(ctx, w.org, w.app, prod, fnName, "stable")
	if err != nil || res.Version != 1 || !bytes.Equal(res.ModuleSHA256, v1.ModuleSHA256) {
		t.Errorf("the alias in production = %+v, %v", res, err)
	}
	if _, err := w.svc.ResolveVersion(ctx, w.org, w.app, dev, fnName, "stable"); code(err) != plxerr.FunctionAliasNotFound {
		t.Errorf("an alias is per environment: %v", err)
	}

	// The next development deploy continues the app's numbering.
	w.builder.set(goodOutput(fnName), nil)
	v3, err := w.svc.Deploy(ctx, w.developer, w.built(t, "development", fnName).ID, "")
	if err != nil || v3.Version != 3 {
		t.Errorf("development deploy after production: %v %+v", err, v3)
	}
	if n := w.auditCount(t, audit.FunctionPromoted); n != 1 {
		t.Errorf("function.promoted entries = %d, want 1", n)
	}
}

// Verifies: FN-006.
// Promoting a version the target already holds changes nothing but the
// alias; a refused promotion stores nothing.
func TestPromoteIsIdempotent_FN_006(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t, nil, nil)
	dev, prod := w.envs["development"], w.envs["production"]
	w.builder.set(goodOutput(fnName), nil)
	if _, err := w.svc.Deploy(ctx, w.developer, w.built(t, "development", fnName).ID, ""); err != nil {
		t.Fatal(err)
	}

	first, err := w.svc.Promote(ctx, w.owner, w.app, dev, prod, fnName, 1, "")
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	again, err := w.svc.Promote(ctx, w.owner, w.app, dev, prod, fnName, 1, "")
	if err != nil || again.Version != 1 || !again.CreatedAt.Equal(first.CreatedAt) || again.BuildID != first.BuildID {
		t.Errorf("a second Promote = %+v, %v; first = %+v", again, err, first)
	}
	if n := w.auditCount(t, audit.FunctionPromoted); n != 1 {
		t.Errorf("function.promoted entries = %d, want 1", n)
	}
	if _, err := w.svc.Promote(ctx, w.owner, w.app, dev, prod, fnName, 1, "prod"); err != nil {
		t.Fatalf("a repeated Promote with an alias: %v", err)
	}
	got, err := w.svc.GetFunction(ctx, w.owner, w.app, prod, fnName)
	if err != nil || len(got.Aliases) != 1 || got.Aliases[0] != (function.Alias{Name: "prod", Version: 1}) {
		t.Errorf("aliases in production = %+v, %v", got.Aliases, err)
	}
	if n := w.auditCount(t, audit.FunctionAliasSet); n != 1 {
		t.Errorf("function.alias_set entries = %d, want 1", n)
	}

	for name, call := range map[string]func() (function.Version, error){
		"a version the source does not hold": func() (function.Version, error) {
			return w.svc.Promote(ctx, w.owner, w.app, dev, prod, fnName, 7, "")
		},
		"a function the source does not hold": func() (function.Version, error) {
			return w.svc.Promote(ctx, w.owner, w.app, dev, prod, "payments.refund", 1, "")
		},
		"the source as the target": func() (function.Version, error) {
			return w.svc.Promote(ctx, w.owner, w.app, dev, dev, fnName, 1, "")
		},
		"an invalid alias": func() (function.Version, error) {
			return w.svc.Promote(ctx, w.owner, w.app, dev, prod, fnName, 1, "Not An Alias")
		},
		"version zero": func() (function.Version, error) {
			return w.svc.Promote(ctx, w.owner, w.app, dev, prod, fnName, 0, "")
		},
		"an environment of another app": func() (function.Version, error) {
			return w.svc.Promote(ctx, w.owner, w.app, dev, w.otherEnvs["production"], fnName, 1, "")
		},
	} {
		if _, err := call(); err == nil {
			t.Errorf("%s was promoted", name)
		}
	}
	if _, err := w.svc.Promote(ctx, w.owner, w.app, dev, prod, fnName, 7, ""); code(err) != plxerr.FunctionNotFound {
		t.Errorf("a missing version: %v", err)
	}
	if _, err := w.svc.Promote(ctx, w.owner, w.app, dev, dev, fnName, 1, ""); code(err) != plxerr.InvalidFormat {
		t.Errorf("the same environment: %v", err)
	}
}

// Verifies: FN-031.
// A version whose tests failed is promoted outside production, marked as
// failing, and refused for a production environment.
func TestPromoteRefusesFailingTestsInProduction_FN_031(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t, nil, nil)
	dev, prod := w.envs["development"], w.envs["production"]
	failing := goodOutput(fnName)
	failing.TestResults = append(failing.TestResults, function.TestResult{Name: "subtracts", Passed: false, Message: "got 3"})
	w.builder.set(failing, nil)
	v, err := w.svc.Deploy(ctx, w.developer, w.built(t, "development", fnName).ID, "")
	if err != nil || v.TestsPassed {
		t.Fatalf("Deploy: %v %+v", err, v)
	}
	if _, err := w.svc.Promote(ctx, w.owner, w.app, dev, prod, fnName, 1, ""); code(err) != plxerr.FunctionTestsFailed {
		t.Errorf("promoting a version with failing tests to production: %v", err)
	}
	if _, err := w.svc.GetFunction(ctx, w.owner, w.app, prod, fnName); code(err) != plxerr.FunctionNotFound {
		t.Errorf("a refused promotion left a version behind: %v", err)
	}
	if n := w.auditCount(t, audit.FunctionPromoted); n != 0 {
		t.Errorf("function.promoted entries = %d, want 0", n)
	}
	// Production's own deploy refuses the same build for the same reason, so
	// the two paths agree (FN-031).
	w.builder.set(failing, nil)
	if _, err := w.svc.Deploy(ctx, w.owner, w.built(t, "production", fnName).ID, ""); code(err) != plxerr.FunctionTestsFailed {
		t.Errorf("deploying the same build to production: %v", err)
	}
}

// Verifies: FN-007.
// A promotion needs the permission a deploy needs in the target: a
// developer promotes to development alone, production needs
// function.deploy.production and a second factor, and a viewer cannot
// promote.
func TestPromotePermissions_FN_007(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t, nil, nil)
	dev, prod := w.envs["development"], w.envs["production"]
	w.builder.set(goodOutput(fnName), nil)
	if _, err := w.svc.Deploy(ctx, w.owner, w.built(t, "production", fnName).ID, ""); err != nil {
		t.Fatal(err)
	}

	if _, err := w.svc.Promote(ctx, w.developer, w.app, prod, dev, fnName, 1, ""); err != nil {
		t.Errorf("a developer promoting to development: %v", err)
	}
	if _, err := w.svc.Promote(ctx, w.viewer, w.app, prod, dev, fnName, 1, "x"); code(err) != plxerr.PermissionDenied {
		t.Errorf("a viewer promoting: %v", err)
	}

	w.builder.set(goodOutput("payments.refund"), nil)
	if _, err := w.svc.Deploy(ctx, w.developer, w.built(t, "development", "payments.refund").ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := w.svc.Promote(ctx, w.developer, w.app, dev, prod, "payments.refund", 1, ""); code(err) != plxerr.PermissionDenied {
		t.Errorf("a developer promoting to production: %v", err)
	}
	owner := w.owner
	owner.SecondFactor = false
	if _, err := w.svc.Promote(ctx, owner, w.app, dev, prod, "payments.refund", 1, ""); code(err) != plxerr.MultiFactorRequired {
		t.Errorf("promoting to production without a second factor: %v", err)
	}
	if _, err := w.svc.Promote(ctx, w.owner, w.app, dev, prod, "payments.refund", 1, ""); err != nil {
		t.Errorf("the owner promoting to production: %v", err)
	}
}

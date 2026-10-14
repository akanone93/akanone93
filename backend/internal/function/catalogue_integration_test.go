// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function_test

import (
	"context"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
)

// deployWith builds, declares and deploys a version of a function with the
// placement and schemas given, and points the alias at it when it is set.
func (w *world) deployWith(t *testing.T, env, name string, placement function.Placement, declaration, alias string) function.Version {
	t.Helper()
	out := goodOutput(name)
	out.Placement = placement
	out.Module = []byte("\x00asm module of " + name + string(placement) + declaration + alias)
	w.builder.set(out, nil)
	b := w.built(t, env, name)
	w.declare(t, b.ID, declaration)
	v, err := w.svc.Deploy(context.Background(), w.developer, b.ID, alias)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// Verifies: FN-004, FN-006, FN-008, ACT-001.
// The catalogue resolves a name to its newest version, or an alias to the
// version it points at, with the placement, the declared capabilities and
// limits and the schemas of that version.
func TestCatalogueResolvesVersionsAndAliases_FN_006(t *testing.T) {
	t.Parallel()
	w := newWorld(t, nil, nil)
	w.deployWith(t, "development", fnName, function.PlacementServer,
		`{"name":"payments.charge","placement":"server","capabilities":{"http":["api.example.com"],"kv":true},"limits":{"timeoutMs":900}}`, "stable")
	v2 := w.deployWith(t, "development", fnName, function.PlacementServer,
		`{"name":"payments.charge","placement":"server","capabilities":{"secrets":["key"]}}`, "")
	w.deployWith(t, "staging", "payments.refund", function.PlacementServer, `{"name":"payments.refund","placement":"server"}`, "")

	c, err := w.svc.Catalogue(context.Background(), w.org, w.app, w.envs["development"])
	if err != nil {
		t.Fatal(err)
	}
	newest, ok := c.Function(fnName, "")
	if !ok || newest.Version != 2 || newest.Placement != schema.PlacementServer || string(newest.InputSchema) != `{"type":"object"}` ||
		string(newest.OutputSchema) != `{"type":"string"}` || newest.Capabilities == nil || len(newest.Capabilities.Secrets) != 1 || newest.Limits != nil {
		t.Errorf("newest = %+v, %v (version %d was deployed last)", newest, ok, v2.Version)
	}
	stable, ok := c.Function(fnName, "stable")
	if !ok || stable.Version != 1 || stable.Capabilities == nil || stable.Capabilities.HTTP[0] != "api.example.com" ||
		stable.Limits == nil || *stable.Limits.TimeoutMs != 900 {
		t.Errorf("stable = %+v, %v", stable, ok)
	}
	for _, unknown := range [][2]string{{fnName, "nope"}, {"payments.refund", ""}, {"payments.other", ""}} {
		if _, ok := c.Function(unknown[0], unknown[1]); ok {
			t.Errorf("Function(%q, %q) is known; it is not deployed in the environment", unknown[0], unknown[1])
		}
	}

	// A caller that changes what it was given does not change the snapshot.
	stable.Capabilities.HTTP[0] = "evil.example.com"
	if again, _ := c.Function(fnName, "stable"); again.Capabilities.HTTP[0] != "api.example.com" {
		t.Error("the catalogue is not an immutable snapshot")
	}
	// A version deployed later is not in a snapshot taken before.
	w.deployWith(t, "development", fnName, function.PlacementServer, `{"name":"payments.charge","placement":"server"}`, "")
	if later, _ := c.Function(fnName, ""); later.Version != 2 {
		t.Errorf("the snapshot moved to version %d", later.Version)
	}
}

// Verifies: FN-055.
// The catalogue reports the effective security setting deviceFunctions, and
// reads it as off where the service has no way to read it.
func TestCatalogueReportsTheDeviceFunctionsSetting_FN_055(t *testing.T) {
	t.Parallel()
	w := newWorld(t, nil, nil)
	for _, on := range []bool{false, true} {
		w.deviceFunctions.Store(on)
		c, err := w.svc.Catalogue(context.Background(), w.org, w.app, w.envs["development"])
		if err != nil || c.DeviceFunctionsEnabled() != on {
			t.Errorf("deviceFunctions = %v, %v; want %v", c != nil && c.DeviceFunctionsEnabled(), err, on)
		}
	}
	if _, err := w.svc.Catalogue(context.Background(), w.org, w.app, "00000000-0000-7000-8000-000000000000"); code(err) != plxerr.ResourceNotFound {
		t.Errorf("an unknown environment: %v", err)
	}
}

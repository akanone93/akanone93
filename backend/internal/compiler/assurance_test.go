// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// requireAssurance makes the plugin's first source ask for a level and,
// when route is not empty, reach its origin that way.
func requireAssurance(level, route string) func(*testing.T, fstest.MapFS) {
	return onSource("0", func(_ *testing.T, src, _ map[string]any) {
		src["requiresAssurance"] = level
		if route != "" {
			src["route"] = route
		}
	})
}

// Verifies: SEC-007.
// A source that asks for an assurance level carries it in its
// configuration, for the device to enforce; AL0 asks for nothing.
func TestDataSourceAssuranceIsEncoded(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ level, want string }{{"AL2", "AL2"}, {"AL0", ""}, {"", ""}} {
		m := project(t, dataDir)
		if c.level != "" {
			requireAssurance(c.level, "")(t, m)
		}
		res := compileFS(m)
		onlyRaised(t, res)
		cfg, strs := sourceConfig(t, readAll(t, res)[1], "tasks")
		got := ""
		if v := valueEntry(cfg, strs, "requiresAssurance"); v != nil {
			got = strs(v.S())
		}
		if got != c.want {
			t.Errorf("requiresAssurance %q: the bundle carries %q, want %q", c.level, got, c.want)
		}
	}
}

// Verifies: SEC-007, SEC-030.
// A direct source that asks for an assurance level is warned about
// (PLX-1504), because only the gateway can enforce the level on the
// server; a gateway source, and a direct one that asks for none, are not.
func TestDirectSourceAssuranceIsWarnedAbout(t *testing.T) {
	t.Parallel()
	const ptr = "/dataSources/0/requiresAssurance"
	m := project(t, dataDir)
	requireAssurance("AL1", "direct")(t, m)
	wantDiag(t, compileFS(m), plxerr.DirectSourceAssuranceUnenforceable, shopFile, ptr)

	for name, edit := range map[string]func(*testing.T, fstest.MapFS){
		"through the gateway": requireAssurance("AL1", "plux"),
		"by default":          requireAssurance("AL1", ""),
		"direct, AL0":         requireAssurance("AL0", "direct"),
	} {
		m := project(t, dataDir)
		edit(t, m)
		if hasDiag(compileFS(m).Diagnostics, plxerr.DirectSourceAssuranceUnenforceable, shopFile, ptr) {
			t.Errorf("%s: PLX-1504 reported", name)
		}
	}
}

// betaGate is the routing project's guard flow.
const betaGate = "plugins/nav/actions/beta-gate.graph.json"

// guardReading makes the routing project's guard decide on an expression.
func guardReading(t *testing.T, when string) *Result {
	t.Helper()
	m := project(t, routingDir)
	edit(t, m, betaGate, func(doc map[string]any) {
		at(t, doc, "steps/0/input")["when"] = map[string]any{"$expr": when}
	})
	return compileFS(m)
}

// Verifies: NAV-009.
// A guard reads the app's state like a page expression does, and a path
// that names no state is reported where it is written (B19 (a)).
func TestGuardsReadAppState(t *testing.T) {
	t.Parallel()
	base := compileFS(project(t, routingDir)).Diagnostics
	if got := guardReading(t, "app.counter > 0").Diagnostics; len(got) != len(base) {
		t.Errorf("a guard reading app.counter:\n%s", list(got))
	}
	for _, when := range []string{"app.nothing > 0", "plugin.nothing > 0"} {
		res := guardReading(t, when)
		found := false
		for _, d := range res.Diagnostics {
			found = found || d.File == betaGate && strings.HasPrefix(d.Path, "/steps/0/input/when")
		}
		if !found {
			t.Errorf("%q: no diagnostic at the guard's condition:\n%s", when, list(res.Diagnostics))
		}
	}
}

// transactionSource makes the plugin's first source a transaction that
// reaches its origin by route (empty: the default).
func transactionSource(route string) func(*testing.T, fstest.MapFS) {
	return onSource("0", func(_ *testing.T, src, _ map[string]any) {
		src["transaction"] = map[string]any{"amount": "/amount", "currency": "/currency", "payee": "/to/iban", "summary": "/memo"}
		if route != "" {
			src["route"] = route
		}
	})
}

// Verifies: SEC-030, SEC-028.
// A source's route and the mapping of its transaction travel in its
// configuration, for the runtime to build the confirmed summary from.
func TestRouteAndTransactionAreEncoded(t *testing.T) {
	t.Parallel()
	m := project(t, dataDir)
	transactionSource("plux")(t, m)
	res := compileFS(m)
	onlyRaised(t, res)
	cfg, strs := sourceConfig(t, readAll(t, res)[1], "tasks")
	route := valueEntry(cfg, strs, "route")
	if route == nil || strs(route.S()) != "plux" {
		t.Fatalf("route = %v", route)
	}
	tx := valueEntry(cfg, strs, "transaction")
	if tx == nil {
		t.Fatal("the bundle carries no transaction mapping")
	}
	for key, want := range map[string]string{"amount": "/amount", "currency": "/currency", "payee": "/to/iban", "summary": "/memo"} {
		if v := valueEntry(tx, strs, key); v == nil || strs(v.S()) != want {
			t.Errorf("transaction.%s = %v, want %s", key, v, want)
		}
	}

	plain := compileFS(project(t, dataDir))
	cfg, strs = sourceConfig(t, readAll(t, plain)[1], "tasks")
	if valueEntry(cfg, strs, "route") != nil || valueEntry(cfg, strs, "transaction") != nil {
		t.Error("a source with no route and no transaction carries them")
	}
}

// Verifies: SEC-030, SEC-032.
// A transaction cannot be direct: only the gateway verifies its signature.
func TestDirectTransactionIsAnError(t *testing.T) {
	t.Parallel()
	m := project(t, dataDir)
	transactionSource("direct")(t, m)
	wantDiag(t, compileFS(m), plxerr.TransactionRequiresGateway, shopFile, "/dataSources/0/route")
	for _, route := range []string{"", "plux"} {
		m := project(t, dataDir)
		transactionSource(route)(t, m)
		if hasDiag(compileFS(m).Diagnostics, plxerr.TransactionRequiresGateway, shopFile, "/dataSources/0/route") {
			t.Errorf("route %q: PLX-6073 reported", route)
		}
	}
}

// Verifies: SEC-030.
// A direct source is refused where the app does not allow direct calls:
// under the maximum profile, or when its settings turn them off.
func TestDirectSourceRefusedWhereNotAllowed(t *testing.T) {
	t.Parallel()
	direct := onSource("0", func(_ *testing.T, src, _ map[string]any) { src["route"] = "direct" })
	for _, app := range map[string]func(map[string]any){
		"the maximum profile": func(doc map[string]any) { doc["securityProfile"] = "maximum" },
		"settings that forbid it": func(doc map[string]any) {
			doc["security"] = map[string]any{"settings": map[string]any{"allowDirectDataSources": false}}
		},
	} {
		m := project(t, dataDir)
		direct(t, m)
		edit(t, m, "app.json", app)
		wantDiag(t, compileFS(m), plxerr.DirectDataSourceRefused, shopFile, "/dataSources/0/route")
	}
	for name, app := range map[string]func(map[string]any){
		"the strict profile": func(doc map[string]any) { doc["securityProfile"] = "strict" },
		"settings that allow it": func(doc map[string]any) {
			doc["security"] = map[string]any{"settings": map[string]any{"allowDirectDataSources": true}}
		},
	} {
		m := project(t, dataDir)
		direct(t, m)
		edit(t, m, "app.json", app)
		if hasDiag(compileFS(m).Diagnostics, plxerr.DirectDataSourceRefused, shopFile, "/dataSources/0/route") {
			t.Errorf("%s: PLX-6063 reported", name)
		}
	}
	m := project(t, dataDir)
	onSource("0", func(_ *testing.T, src, _ map[string]any) { src["route"] = "plux" })(t, m)
	edit(t, m, "app.json", func(doc map[string]any) { doc["securityProfile"] = "maximum" })
	if hasDiag(compileFS(m).Diagnostics, plxerr.DirectDataSourceRefused, shopFile, "/dataSources/0/route") {
		t.Error("a gateway source under maximum was refused")
	}
}

// Verifies: SEC-030, SEC-031.
// The compilation lists the data sources that go through the gateway with
// the assurance they need and their transaction mapping, for the server to
// check against its registered upstreams; a direct source is not listed.
func TestGatewaySourcesAreListed(t *testing.T) {
	t.Parallel()
	m := project(t, dataDir)
	transactionSource("plux")(t, m)
	requireAssurance("AL2", "")(t, m)
	res := compileFS(m)
	onlyRaised(t, res)
	var found *GatewaySource
	for i := range res.GatewaySources {
		if res.GatewaySources[i].Name == "tasks" {
			found = &res.GatewaySources[i]
		}
	}
	if found == nil {
		t.Fatalf("sources = %+v", res.GatewaySources)
	}
	if found.Assurance != "AL2" || found.Transaction == nil || found.Transaction.Payee != "/to/iban" || found.Transaction.Summary != "/memo" {
		t.Errorf("source = %+v", found)
	}

	m = project(t, dataDir)
	requireAssurance("", "direct")(t, m)
	for _, g := range compileFS(m).GatewaySources {
		if g.Name == "tasks" {
			t.Errorf("a direct source is listed: %+v", g)
		}
	}
}

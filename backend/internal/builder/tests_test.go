// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/fnhost"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// testEngine returns a function host engine for the tests.
func testEngine(t *testing.T) *fnhost.Engine {
	t.Helper()
	e, err := fnhost.NewEngine(t.Context(), fnhost.Config{Clock: time.Now, Random: rand.Reader, MaxMemoryBytes: 256 << 20})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close(context.WithoutCancel(t.Context())) })
	return e
}

// probeModule is the conformance probe; its entry 0 echoes the input.
func probeModule(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "schema", "testdata", "functions", "probe.wasm"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// declarationWith returns a declaration whose test cases are cases.
func declarationWith(cases ...schema.FunctionTestCase) *Declaration {
	return &Declaration{Function: schema.FunctionDocument{Name: "probe.echo", Placement: schema.PlacementServer, Tests: cases}}
}

// raw is a JSON literal as a test case value.
func raw(s string) json.RawMessage { return json.RawMessage(s) }

// Verifies: FN-031.
func TestRunTestsComparesOutputsOnTheFunctionHost_FN_031(t *testing.T) {
	decl := declarationWith(
		schema.FunctionTestCase{Name: "echo", Input: raw(`{"b":2,"a":1}`), Output: raw(`{"a":1,"b":2}`)},
		schema.FunctionTestCase{Name: "wrong output", Input: raw(`{"a":1}`), Output: raw(`{"a":2}`)},
		schema.FunctionTestCase{Name: "expects an error", Input: raw(`1`), Error: &schema.FunctionTestError{Code: "x.denied"}},
		schema.FunctionTestCase{Name: "with claims", Input: raw(`"in"`), Claims: raw(`{"tier":"gold"}`), Output: raw(`"in"`)},
	)
	got, err := RunTests(t.Context(), testEngine(t), probeModule(t), decl, limits.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	passed := []bool{true, false, false, true}
	if len(got) != len(passed) {
		t.Fatalf("results = %+v", got)
	}
	for i, want := range passed {
		if got[i].Passed != want || got[i].Name != decl.Function.Tests[i].Name || (want && got[i].Message != "") || (!want && got[i].Message == "") {
			t.Errorf("result %d = %+v; passed should be %v", i, got[i], want)
		}
	}
}

// Verifies: FN-031.
func TestRunTestsWithoutCasesReturnsAnEmptyList_FN_031(t *testing.T) {
	got, err := RunTests(t.Context(), testEngine(t), probeModule(t), declarationWith(), limits.Defaults())
	if err != nil || got == nil || len(got) != 0 {
		t.Errorf("results = %#v, %v", got, err)
	}
}

// Verifies: FN-031, LIM-001.
func TestRunTestsRefusesMoreCasesThanTheRegistryAllows_FN_031(t *testing.T) {
	set, err := limits.Defaults().Tighten(limits.FunctionTestCases, limits.ScopeApp, 1)
	if err != nil {
		t.Fatal(err)
	}
	decl := declarationWith(
		schema.FunctionTestCase{Name: "a", Input: raw(`1`), Output: raw(`1`)},
		schema.FunctionTestCase{Name: "b", Input: raw(`1`), Output: raw(`1`)},
	)
	_, err = RunTests(t.Context(), testEngine(t), probeModule(t), decl, set)
	if code, _ := plxerr.CodeOf(err); code != plxerr.FunctionBuildLimitExceeded {
		t.Errorf("error = %v", err)
	}
}

// Verifies: FN-031, FN-011.
func TestRunTestsAppliesTheDocumentsOwnLimits_FN_031(t *testing.T) {
	decl := declarationWith(schema.FunctionTestCase{Name: "big", Input: raw(`"abcdefghij"`), Output: raw(`"abcdefghij"`)})
	small := int64(4)
	decl.Function.Limits = &schema.FunctionLimits{OutputBytes: &small}
	got, err := RunTests(t.Context(), testEngine(t), probeModule(t), decl, limits.Defaults())
	if err != nil || len(got) != 1 || got[0].Passed {
		t.Fatalf("results = %+v, %v; the output limit of the document should fail the case", got, err)
	}
	lim := testLimits(limits.Defaults(), decl)
	if lim.OutputBytes != 4 || lim.Timeout != 2*time.Second {
		t.Errorf("limits = %+v", lim)
	}
}

// Verifies: FN-031, FN-012.
func TestRunTestsGrantsOnlyTheDeclaredCapabilities_FN_031(t *testing.T) {
	yes := true
	decl := declarationWith()
	if g := testGrants(decl); g.KV || g.Claims || len(g.HTTPDomains) != 0 {
		t.Errorf("a document with no capabilities grants %+v", g)
	}
	decl.Function.Capabilities = &schema.FunctionCapabilities{KV: &yes, HTTP: []string{"api.example.com"}}
	if g := testGrants(decl); !g.KV || g.Claims || len(g.HTTPDomains) != 1 {
		t.Errorf("grants = %+v", g)
	}
}

// Verifies: FN-031, FN-022.
func TestJudgeCaseMatchesBusinessErrors_FN_031(t *testing.T) {
	want := schema.FunctionTestCase{Name: "denied", Error: &schema.FunctionTestError{Code: "x.denied"}}
	if r := judgeCase(want, fnhost.Outcome{Business: &fnhost.BusinessError{Code: "x.denied", MessageKey: "k"}}, nil); !r.Passed {
		t.Errorf("a matching business error failed: %+v", r)
	}
	if r := judgeCase(want, fnhost.Outcome{Business: &fnhost.BusinessError{Code: "x.other", MessageKey: "k"}}, nil); r.Passed {
		t.Error("a different business error passed")
	}
	out := schema.FunctionTestCase{Name: "out", Output: raw(`1`)}
	if r := judgeCase(out, fnhost.Outcome{Business: &fnhost.BusinessError{Code: "x.denied", MessageKey: "k"}}, nil); r.Passed {
		t.Error("a business error passed a case that expects an output")
	}
	if r := judgeCase(out, fnhost.Outcome{}, plxerr.New(plxerr.FunctionTimeout, "too slow")); r.Passed || r.Message == "" {
		t.Errorf("a system error must fail the case with a message: %+v", r)
	}
}

// Verifies: FN-004.
func TestWasmSchemaReaderRefusesAModuleWithoutSchemas_FN_004(t *testing.T) {
	r := NewWasmSchemaReader(testEngine(t), fnhost.Limits{Timeout: time.Second, MemoryBytes: 16 << 20, OutputBytes: 1 << 16})
	_, err := r.Schemas(t.Context(), probeModule(t), 1)
	if code, _ := plxerr.CodeOf(err); code != plxerr.FunctionSchemaDerivationFailed {
		t.Errorf("error = %v; want PLX-7204", err)
	}
}

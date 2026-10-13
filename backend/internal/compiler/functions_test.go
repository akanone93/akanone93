// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/nightCode42/plux3/backend/internal/bundle"
	"github.com/nightCode42/plux3/backend/internal/bundle/fbs"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// fakeCatalogue is a FunctionCatalogue over a fixed table.
type fakeCatalogue struct {
	functions map[string]FunctionInfo
	device    bool
}

func (c fakeCatalogue) Function(name, _ string) (FunctionInfo, bool) {
	info, ok := c.functions[name]
	return info, ok
}

func (c fakeCatalogue) DeviceFunctionsEnabled() bool { return c.device }

// fakeModules is a DeviceModules that returns one module and records what
// it was asked for.
type fakeModules struct {
	module DeviceModule
	err    error
	asked  *[]DeviceFunctionRef
}

func (m fakeModules) Module(_ string, fns []DeviceFunctionRef) (DeviceModule, error) {
	if m.asked != nil {
		*m.asked = fns
	}
	return m.module, m.err
}

const (
	quoteInput  = `{"type":"object","required":["amount"],"properties":{"amount":{"type":"number"},"months":{"type":"integer"},"kind":{"enum":["flat","reducing"]},"note":{"type":["string","null"]}},"additionalProperties":false}`
	quoteOutput = `{"$ref":"#/$defs/Quote","$defs":{"Quote":{"type":"object","required":["total"],"properties":{"total":{"type":"number"},"label":{"anyOf":[{"type":"string"},{"type":"null"}]}}}}}`
)

// serverQuote is a server-placed function with schemas.
func serverQuote() FunctionInfo {
	return FunctionInfo{Placement: schema.PlacementServer, InputSchema: []byte(quoteInput), OutputSchema: []byte(quoteOutput), Version: 3}
}

// withFunctions grants the functions to the loans plugin and returns the
// project.
func withFunctions(t *testing.T, grants ...string) fstest.MapFS {
	t.Helper()
	m := fixture(t)
	edit(t, m, pluginFile, func(doc map[string]any) {
		var fns []any
		for i, g := range grants {
			fns = append(fns, map[string]any{"id": "01c0c450-6c00-7000-8000-0000000fff0" + string(rune('1'+i)), "function": g})
		}
		doc["capabilities"].(map[string]any)["functions"] = fns
		doc["state"] = append(doc["state"].([]any), map[string]any{
			"id": "01c0c450-6c00-7000-8000-0000000fffa1", "name": "label", "persistence": "session", "type": "string?", "default": nil,
		})
	})
	return m
}

// invoke inserts an invokeFunction step with id "charge" at index 1 of the
// calculate graph, followed by an optional setState of plugin.label.
func invoke(t *testing.T, m fstest.MapFS, input map[string]any, label string) {
	t.Helper()
	edit(t, m, calculateGraph, func(doc map[string]any) {
		steps := doc["steps"].([]any)
		steps[0].(map[string]any)["next"] = "charge"
		step := map[string]any{"action": "invokeFunction", "id": "charge", "input": input, "next": "track"}
		rest := append([]any{step}, steps[1:]...)
		if label != "" {
			step["next"] = "relabel"
			relabel := map[string]any{
				"action": "setState", "id": "relabel", "next": "track",
				"input": map[string]any{"path": "plugin.label", "value": map[string]any{"$expr": label}},
			}
			rest = append([]any{step, relabel}, steps[1:]...)
		}
		doc["steps"] = append([]any{steps[0]}, rest...)
	})
}

func quoteInputValue(t *testing.T) map[string]any {
	t.Helper()
	return map[string]any{"function": "loans.quote", "input": map[string]any{
		"amount": map[string]any{"$expr": "page.months"}, "months": map[string]any{"$expr": "page.months"}, "kind": "flat",
	}}
}

func catalogueOf(fns map[string]FunctionInfo, device bool) Options {
	opts := DefaultOptions()
	opts.Functions = fakeCatalogue{functions: fns, device: device}
	return opts
}

// errorsOf lists the error diagnostics.
func errorsOf(res *Result) []plxerr.Diagnostic {
	var out []plxerr.Diagnostic
	for _, d := range res.Diagnostics {
		if d.Severity == plxerr.SeverityError {
			out = append(out, d)
		}
	}
	return out
}

// TestInvokeFunctionChecksTheCatalogue_ACT_001 checks a valid
// invokeFunction step and each way its function reference can fail.
// Verifies: ACT-001, FN-004, FN-006.
func TestInvokeFunctionChecksTheCatalogue_ACT_001(t *testing.T) {
	t.Parallel()
	opts := catalogueOf(map[string]FunctionInfo{"loans.quote": serverQuote()}, true)

	t.Run("valid", func(t *testing.T) {
		t.Parallel()
		m := withFunctions(t, "loans.quote")
		invoke(t, m, quoteInputValue(t), "")
		res := Compile(m, opts)
		if len(errorsOf(res)) != 0 || res.Plugins == nil {
			t.Fatalf("diagnostics:\n%s", list(res.Diagnostics))
		}
		if slices.Contains(res.Plugins[0].Features, bundle.FeatureWasm) {
			t.Error("a server function raised the wasm feature")
		}
	})
	t.Run("not granted", func(t *testing.T) {
		t.Parallel()
		m := withFunctions(t, "loans.quote")
		in := quoteInputValue(t)
		in["function"] = "loans.other"
		invoke(t, m, in, "")
		wantDiag(t, Compile(m, opts), plxerr.FunctionReferenceUnresolved, calculateGraph, "/steps/1/input/function")
	})
	t.Run("unknown to the catalogue", func(t *testing.T) {
		t.Parallel()
		m := withFunctions(t, "loans.quote")
		invoke(t, m, quoteInputValue(t), "")
		res := Compile(m, catalogueOf(nil, true))
		wantDiag(t, res, plxerr.FunctionReferenceUnresolved, calculateGraph, "/steps/1/input/function")
		wantDiag(t, res, plxerr.FunctionReferenceUnresolved, pluginFile, "/capabilities/functions/0/function")
	})
	t.Run("no catalogue still needs a grant", func(t *testing.T) {
		t.Parallel()
		m := withFunctions(t)
		edit(t, m, pluginFile, func(doc map[string]any) { delete(doc["capabilities"].(map[string]any), "functions") })
		invoke(t, m, quoteInputValue(t), "")
		wantDiag(t, compileFS(m), plxerr.FunctionReferenceUnresolved, calculateGraph, "/steps/1/input/function")
	})
}

// TestInvokeFunctionInputIsTypeChecked_ACT_001 checks the step's input
// against the function's input schema.
// Verifies: ACT-001, FN-004.
func TestInvokeFunctionInputIsTypeChecked_ACT_001(t *testing.T) {
	t.Parallel()
	opts := catalogueOf(map[string]FunctionInfo{"loans.quote": serverQuote()}, true)
	cases := []struct {
		name   string
		mutate func(in map[string]any)
		ptr    string
	}{
		{"a literal of the wrong type", func(in map[string]any) { in["input"].(map[string]any)["amount"] = "ten" }, "/steps/1/input/input/amount"},
		{"a non-integral integer", func(in map[string]any) { in["input"].(map[string]any)["months"] = 2.5 }, "/steps/1/input/input/months"},
		{"a missing required property", func(in map[string]any) { delete(in["input"].(map[string]any), "amount") }, "/steps/1/input/input"},
		{"an unknown member of a closed object", func(in map[string]any) { in["input"].(map[string]any)["extra"] = 1 }, "/steps/1/input/input/extra"},
		{"a value outside the enum", func(in map[string]any) { in["input"].(map[string]any)["kind"] = "spiral" }, "/steps/1/input/input/kind"},
		{"null for a non-nullable", func(in map[string]any) { in["input"].(map[string]any)["months"] = nil }, "/steps/1/input/input/months"},
		{"an expression of the wrong type", func(in map[string]any) {
			in["input"].(map[string]any)["amount"] = map[string]any{"$expr": "page.months > 3"}
		}, "/steps/1/input/input/amount"},
		{"no object at all", func(in map[string]any) { in["input"] = "ten" }, "/steps/1/input"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			m := withFunctions(t, "loans.quote")
			in := quoteInputValue(t)
			c.mutate(in)
			invoke(t, m, in, "")
			wantDiag(t, Compile(m, opts), plxerr.FunctionTypeMismatch, calculateGraph, c.ptr)
		})
	}

	t.Run("nullable and unmapped schemas accept", func(t *testing.T) {
		t.Parallel()
		m := withFunctions(t, "loans.quote", "loans.free")
		in := quoteInputValue(t)
		in["input"].(map[string]any)["note"] = nil
		invoke(t, m, in, "")
		fns := map[string]FunctionInfo{
			"loans.quote": serverQuote(),
			"loans.free":  {Placement: schema.PlacementServer, InputSchema: []byte(`{"oneOf":[{"type":"string"},{"type":"integer"}]}`)},
		}
		if res := Compile(m, catalogueOf(fns, true)); len(errorsOf(res)) != 0 {
			t.Fatalf("diagnostics:\n%s", list(res.Diagnostics))
		}
	})
}

// TestInvokeFunctionOutputIsTyped_ACT_001 checks that a step's output
// has the type the function's output schema gives, and that using it as
// another type is PLX-7032.
// Verifies: ACT-001, FN-004.
func TestInvokeFunctionOutputIsTyped_ACT_001(t *testing.T) {
	t.Parallel()
	opts := catalogueOf(map[string]FunctionInfo{"loans.quote": serverQuote()}, true)

	m := withFunctions(t, "loans.quote")
	invoke(t, m, quoteInputValue(t), "steps.charge.output?.label")
	if res := Compile(m, opts); len(errorsOf(res)) != 0 {
		t.Fatalf("a string? output into a string? state: %s", list(res.Diagnostics))
	}

	m = withFunctions(t, "loans.quote")
	invoke(t, m, quoteInputValue(t), "steps.charge.output?.total")
	wantDiag(t, Compile(m, opts), plxerr.FunctionTypeMismatch, calculateGraph, "/steps/2/input/value/$expr")
}

// TestDeviceFunctionsRefuseServerCapabilities_FN_008 checks that a
// device-placed function that asks for http, secrets, claims or other
// functions is refused, in its document and in the catalogue.
// Verifies: FN-008, FN-052.
func TestDeviceFunctionsRefuseServerCapabilities_FN_008(t *testing.T) {
	t.Parallel()
	yes := true
	asks := map[string]schema.FunctionCapabilities{
		"http":      {HTTP: []string{"api.example.com"}},
		"secrets":   {Secrets: []string{"API_KEY"}},
		"claims":    {Claims: &yes},
		"functions": {Functions: []string{"loans.quote"}},
	}
	for name, caps := range asks {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := withFunctions(t, "loans.calc")
			opts := catalogueOf(map[string]FunctionInfo{"loans.calc": {Placement: schema.PlacementDevice, Capabilities: &caps, Version: 1}}, true)
			opts.DeviceModules = fakeModules{module: DeviceModule{Wasm: []byte("\x00asm"), ABIVersion: 1, Index: map[string]uint32{"loans.calc": 0}}}
			wantDiag(t, Compile(m, opts), plxerr.DeviceFunctionServerCapability, pluginFile, "/capabilities/functions/0/function")
		})
	}
	t.Run("kv is allowed", func(t *testing.T) {
		t.Parallel()
		m := withFunctions(t, "loans.calc")
		opts := catalogueOf(map[string]FunctionInfo{"loans.calc": {Placement: schema.PlacementDevice, Capabilities: &schema.FunctionCapabilities{KV: &yes}, Version: 1}}, true)
		opts.DeviceModules = fakeModules{module: DeviceModule{Wasm: []byte("\x00asm"), ABIVersion: 1, Index: map[string]uint32{"loans.calc": 0}}}
		if res := Compile(m, opts); len(errorsOf(res)) != 0 {
			t.Fatalf("diagnostics:\n%s", list(res.Diagnostics))
		}
	})
	t.Run("in the function document", func(t *testing.T) {
		t.Parallel()
		m := fixture(t)
		m["functions/loans.calc/function.json"] = &fstest.MapFile{Data: []byte(`{"schemaVersion":"1.0.0","kind":"function","id":"01c0c450-6c00-7000-8000-0000000fff51","name":"loans.calc","placement":"device","capabilities":{"claims":true}}`)}
		wantDiag(t, compileFS(m), plxerr.DeviceFunctionServerCapability, "functions/loans.calc/function.json", "/capabilities/claims")
		m["functions/loans.calc/function.json"] = &fstest.MapFile{Data: []byte(`{"schemaVersion":"1.0.0","kind":"function","id":"01c0c450-6c00-7000-8000-0000000fff51","name":"loans.calc","placement":"server","capabilities":{"claims":true}}`)}
		if res := compileFS(m); len(errorsOf(res)) != 0 {
			t.Fatalf("a server function may ask for claims:\n%s", list(res.Diagnostics))
		}
	})
}

// TestDeviceFunctionsFollowTheSetting_FN_055 checks that device placement
// is refused where the security setting deviceFunctions is off.
// Verifies: FN-055.
func TestDeviceFunctionsFollowTheSetting_FN_055(t *testing.T) {
	t.Parallel()
	fns := map[string]FunctionInfo{"loans.calc": {Placement: schema.PlacementDevice, Version: 1}, "loans.quote": serverQuote()}
	modules := fakeModules{module: DeviceModule{Wasm: []byte("\x00asm"), ABIVersion: 1, Index: map[string]uint32{"loans.calc": 0}}}

	opts := catalogueOf(fns, false)
	opts.DeviceModules = modules
	wantDiag(t, Compile(withFunctions(t, "loans.calc"), opts), plxerr.DeviceFunctionsDisabled, pluginFile, "/capabilities/functions/0/function")

	if res := Compile(withFunctions(t, "loans.quote"), opts); len(errorsOf(res)) != 0 {
		t.Errorf("a server function is not affected:\n%s", list(res.Diagnostics))
	}
	opts = catalogueOf(fns, true)
	opts.DeviceModules = modules
	if res := Compile(withFunctions(t, "loans.calc"), opts); len(errorsOf(res)) != 0 {
		t.Errorf("enabled:\n%s", list(res.Diagnostics))
	}
}

// deviceOptions grants two device functions, deliberately not in name
// order, and a server function.
func deviceOptions(asked *[]DeviceFunctionRef) Options {
	limitsOf := &schema.FunctionLimits{}
	timeout, memory := int64(250), int64(8<<20)
	limitsOf.TimeoutMs, limitsOf.MemoryBytes = &timeout, &memory
	opts := catalogueOf(map[string]FunctionInfo{
		"loans.zeta":  {Placement: schema.PlacementDevice, Version: 7, Limits: limitsOf},
		"loans.alpha": {Placement: schema.PlacementDevice, Version: 2},
		"loans.quote": serverQuote(),
	}, true)
	opts.DeviceModules = fakeModules{asked: asked, module: DeviceModule{
		Wasm: []byte("\x00asm\x01\x00\x00\x00device"), ABIVersion: 1, Index: map[string]uint32{"loans.alpha": 0, "loans.zeta": 1},
	}}
	return opts
}

// wasmOf decodes the wasm section of the loans bundle.
func wasmOf(t *testing.T, res *Result) (*fbs.WasmModule, *Bundle) {
	t.Helper()
	if res.Plugins == nil {
		t.Fatalf("no bundle:\n%s", list(res.Diagnostics))
	}
	b, err := bundle.Read(res.Plugins[0].Data, bundle.ReadOptions{Limits: limits.Defaults(), Supports: func(string) bool { return true }})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range b.Sections {
		if s.Kind == bundle.SectionWasm {
			return fbs.GetRootAsWasmModule(s.Data, 0), res.Plugins[0]
		}
	}
	return nil, res.Plugins[0]
}

// TestDeviceModuleIsPackedIntoThePluginBundle_FN_051 checks the wasm
// section: the one module of the plugin, its hash and ABI version, and the
// functions sorted by name with their versions and limits; and that the
// bundle requires the feature.
// Verifies: FN-051, FN-006, FN-053.
func TestDeviceModuleIsPackedIntoThePluginBundle_FN_051(t *testing.T) {
	t.Parallel()
	var asked []DeviceFunctionRef
	res := Compile(withFunctions(t, "loans.zeta", "loans.quote", "loans.alpha"), deviceOptions(&asked))
	if len(errorsOf(res)) != 0 {
		t.Fatalf("diagnostics:\n%s", list(res.Diagnostics))
	}
	if want := []DeviceFunctionRef{{"loans.alpha", 2}, {"loans.zeta", 7}}; !slices.Equal(asked, want) {
		t.Errorf("the module source was asked for %v, want %v", asked, want)
	}
	w, pl := wasmOf(t, res)
	if w == nil {
		t.Fatal("no wasm section")
	}
	module := []byte("\x00asm\x01\x00\x00\x00device")
	sum := sha256.Sum256(module)
	if !bytes.Equal(w.ModuleBytes(), module) || !bytes.Equal(w.HashBytes(), sum[:]) || w.AbiVersion() != 1 {
		t.Errorf("module, hash or ABI version wrong")
	}
	if w.FunctionsLength() != 2 {
		t.Fatalf("%d functions", w.FunctionsLength())
	}
	var f fbs.DeviceFunction
	want := []struct {
		name                     string
		index, version, tms, mem uint32
	}{{"loans.alpha", 0, 2, 0, 0}, {"loans.zeta", 1, 7, 250, 8 << 20}}
	for i, e := range want {
		w.Functions(&f, i)
		if string(f.Name()) != e.name || f.Index() != e.index || f.Version() != e.version || f.TimeoutMs() != e.tms || f.MemoryBytes() != e.mem {
			t.Errorf("function %d = %s/%d/%d/%d/%d, want %+v", i, f.Name(), f.Index(), f.Version(), f.TimeoutMs(), f.MemoryBytes(), e)
		}
	}
	if !slices.Contains(pl.Features, bundle.FeatureWasm) {
		t.Errorf("features %v lack %s", pl.Features, bundle.FeatureWasm)
	}
}

// TestDeviceModuleSectionIsDeterministic_FN_051 checks that the same
// project gives byte-identical bundles, whatever order the grants are in.
// Verifies: FN-051, CMP-002.
func TestDeviceModuleSectionIsDeterministic_FN_051(t *testing.T) {
	t.Parallel()
	a := Compile(withFunctions(t, "loans.zeta", "loans.alpha"), deviceOptions(nil))
	b := Compile(withFunctions(t, "loans.zeta", "loans.alpha"), deviceOptions(nil))
	c := Compile(withFunctions(t, "loans.alpha", "loans.zeta"), deviceOptions(nil))
	for _, r := range []*Result{a, b, c} {
		if r.Plugins == nil {
			t.Fatalf("diagnostics:\n%s", list(r.Diagnostics))
		}
	}
	if !bytes.Equal(a.Plugins[0].Data, b.Plugins[0].Data) || a.Plugins[0].Hash != b.Plugins[0].Hash {
		t.Error("two compilations differ")
	}
	wa, _ := wasmOf(t, a)
	wc, _ := wasmOf(t, c)
	if !bytes.Equal(wa.Table().Bytes, wc.Table().Bytes) {
		t.Error("the wasm section depends on the order of the grants")
	}
}

// TestNoDeviceFunctionsNoWasmSection_FN_051 checks that a plugin without
// device-placed functions gets neither a section nor the feature.
// Verifies: FN-051, CMP-002.
func TestNoDeviceFunctionsNoWasmSection_FN_051(t *testing.T) {
	t.Parallel()
	plain := compileFS(fixture(t))
	granted := Compile(withFunctions(t, "loans.quote"), deviceOptions(nil))
	for _, res := range []*Result{plain, granted} {
		if w, pl := wasmOf(t, res); w != nil || slices.Contains(pl.Features, bundle.FeatureWasm) {
			t.Error("a wasm section or feature without device functions")
		}
	}
}

// TestDeviceModuleFailures_FN_051 checks the diagnostics for a module that
// is missing, does not export a function, or is too large.
// Verifies: FN-051.
func TestDeviceModuleFailures_FN_051(t *testing.T) {
	t.Parallel()
	m := func() fstest.MapFS { return withFunctions(t, "loans.alpha") }

	opts := deviceOptions(nil)
	opts.DeviceModules = nil
	wantDiag(t, Compile(m(), opts), plxerr.FunctionModuleInvalid, pluginFile, "/capabilities/functions")

	opts = deviceOptions(nil)
	opts.DeviceModules = fakeModules{err: errors.New("link failed")}
	wantDiag(t, Compile(m(), opts), plxerr.FunctionModuleInvalid, pluginFile, "/capabilities/functions")

	opts = deviceOptions(nil)
	opts.DeviceModules = fakeModules{module: DeviceModule{Wasm: []byte("\x00asm"), ABIVersion: 1, Index: map[string]uint32{}}}
	wantDiag(t, Compile(m(), opts), plxerr.FunctionModuleInvalid, pluginFile, "/capabilities/functions")

	opts = deviceOptions(nil)
	tight, err := limits.Defaults().Tighten(limits.BundleDeviceFunctionModuleSize, limits.ScopeApp, 4)
	if err != nil {
		t.Fatal(err)
	}
	opts.Limits = tight
	wantDiag(t, Compile(m(), opts), plxerr.LimitExceeded, pluginFile, "")
}

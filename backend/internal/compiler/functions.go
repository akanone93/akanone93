// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"crypto/sha256"
	"encoding/json"
	"math"
	"regexp"
	"slices"
	"strconv"

	flatbuffers "github.com/google/flatbuffers/go"

	"github.com/nightCode42/plux3/backend/internal/bundle"
	"github.com/nightCode42/plux3/backend/internal/bundle/fbs"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/pxl"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/schema/registry"
)

// FunctionInfo is what the compiler knows about a function of the app and
// environment being compiled (FN-004, FN-006, FN-008).
type FunctionInfo struct {
	// Placement is where the function runs.
	Placement schema.Placement
	// Capabilities are the host capabilities the function's declaration
	// asks for; nil asks for none.
	Capabilities *schema.FunctionCapabilities
	// Limits are the function's own limits; nil leaves the registry values.
	Limits *schema.FunctionLimits
	// InputSchema and OutputSchema are the JSON Schemas derived from the
	// function's Rust types (FN-004). Empty means unknown, which the
	// compiler treats as any value.
	InputSchema, OutputSchema json.RawMessage
	// Version is the version the name and alias resolve to (FN-006).
	Version uint32
}

// FunctionCatalogue answers what the compiler needs to know about
// functions. It is injected, so the compiler stays a pure library (L-5);
// implementations must be safe for concurrent use and deterministic for
// one app, environment and moment: the same question gives the same answer
// throughout one compilation (CMP-002).
type FunctionCatalogue interface {
	// Function describes the function with a name for the app and
	// environment being compiled, at the version the alias selects (the
	// newest when alias is empty). ok is false for an unknown function or
	// alias.
	Function(name, alias string) (info FunctionInfo, ok bool)
	// DeviceFunctionsEnabled reports the effective security setting
	// deviceFunctions (FN-055).
	DeviceFunctionsEnabled() bool
}

// DeviceFunctionRef names a device-placed function a module must export.
type DeviceFunctionRef struct {
	// Name is the dotted function name.
	Name string
	// Version is the pinned version.
	Version uint32
}

// DeviceModule is the one module linking a plugin's device-placed
// functions (FN-051).
type DeviceModule struct {
	// Wasm is the module.
	Wasm []byte
	// ABIVersion is the host ABI version the module targets (FN-015).
	ABIVersion uint32
	// Index maps each function name to the index its dispatch export
	// takes.
	Index map[string]uint32
}

// DeviceModules supplies the linked device module of a plugin. It is
// injected: linking runs in the builder, not in the compiler. Safe for
// concurrent use; deterministic for the same functions and versions.
type DeviceModules interface {
	// Module returns the module linking the functions, which are sorted by
	// name, for the plugin with the key.
	Module(pluginKey string, functions []DeviceFunctionRef) (DeviceModule, error)
}

// serverOnlyCapabilities lists the capabilities a device-placed function
// asks for, which only a server function may have (FN-008, FN-052).
func serverOnlyCapabilities(c *schema.FunctionCapabilities) []string {
	if c == nil {
		return nil
	}
	var out []string
	if len(c.HTTP) > 0 {
		out = append(out, "http")
	}
	if len(c.Secrets) > 0 {
		out = append(out, "secrets")
	}
	if c.Claims != nil && *c.Claims {
		out = append(out, "claims")
	}
	if len(c.Functions) > 0 {
		out = append(out, "functions")
	}
	return out
}

// grantOf finds the grant a name refers to in a plugin: the granted
// function, or the alias of a grant.
func grantOf(pl *plugin, name string) (int, *schema.FunctionGrant) {
	if pl == nil || pl.doc.Capabilities == nil {
		return -1, nil
	}
	for i := range pl.doc.Capabilities.Functions {
		g := &pl.doc.Capabilities.Functions[i]
		if g.Function == name || g.Alias == name {
			return i, g
		}
	}
	return -1, nil
}

// functionInfo asks the catalogue about a grant; nil without a catalogue
// or for an unknown function.
func (u *unit) functionInfo(g *schema.FunctionGrant) *FunctionInfo {
	if u.opts.Functions == nil {
		return nil
	}
	info, ok := u.opts.Functions.Function(g.Function, g.Alias)
	if !ok {
		return nil
	}
	return &info
}

// stepFunction is the function an invokeFunction step calls, or nil when
// the reference is not resolved (which checkFunctionRef reports).
func (u *unit) stepFunction(g *graph, st schema.Step) *FunctionInfo {
	_, grant := grantOf(g.plugin, literalString(st.Input["function"]))
	if grant == nil {
		return nil
	}
	return u.functionInfo(grant)
}

// checkFunctionRef resolves the function of an invokeFunction step: it
// must be granted by the plugin's capabilities and known to the catalogue
// (PLX-7031).
func (u *unit) checkFunctionRef(g *graph, in registry.Input, raw json.RawMessage, c vctx) *value {
	name := literalString(raw)
	if name == "" {
		u.report(c.code, c.file, c.ptr, "input %q names a %s and is written as a literal string", in.Name, in.Ref)
		return nil
	}
	_, grant := grantOf(g.plugin, name)
	if grant == nil {
		u.report(plxerr.FunctionReferenceUnresolved, c.file, c.ptr, "function %q is not granted by the plugin's capabilities.functions", name)
		return nil
	}
	if u.opts.Functions != nil && u.functionInfo(grant) == nil {
		u.report(plxerr.FunctionReferenceUnresolved, c.file, c.ptr, "function %q is unknown to the app's function catalogue", grant.Function)
		return nil
	}
	u.graph.add(Edge{From: c.from, Kind: EdgeUsesFunction, To: name, File: c.file, Path: c.ptr})
	return &value{kind: fbs.ValueKindString, s: name}
}

// functionInput checks the input of an invokeFunction step against the
// function's input schema (ACT-001, FN-004) and returns its value as the
// step carries it.
func (u *unit) functionInput(g *graph, raw json.RawMessage, c vctx) *value {
	st := g.steps[stepIndex(c.ptr)]
	if info := u.stepFunction(g, st); info != nil && len(info.InputSchema) > 0 {
		if v, ok := decodeJSON(raw); ok {
			c.code = plxerr.FunctionTypeMismatch
			u.matchFunctionValue(c, v, parseFnSchema(info.InputSchema))
		}
	}
	return u.inferred(c, raw)
}

// matchFunctionValue checks a decoded input value against a function type
// and reports each mismatch at the value's pointer.
func (u *unit) matchFunctionValue(c vctx, v any, t *fnType) {
	if t.kind == fnAny {
		return
	}
	if obj, ok := v.(map[string]any); ok && isBinding(obj) {
		u.matchFunctionBinding(c, obj, t)
		return
	}
	if v == nil {
		if !t.nullable {
			u.report(c.code, c.file, c.ptr, "null is not a %s", t)
		}
		return
	}
	switch t.kind {
	case fnBool, fnInt, fnDouble, fnString, fnEnum:
		u.matchFunctionScalar(c, v, t)
	case fnList:
		items, ok := v.([]any)
		if !ok {
			u.report(c.code, c.file, c.ptr, "expected %s", t)
			return
		}
		for i, it := range items {
			u.matchFunctionValue(c.at(strconv.Itoa(i)), it, t.elem)
		}
	case fnMap:
		obj, ok := v.(map[string]any)
		if !ok {
			u.report(c.code, c.file, c.ptr, "expected %s", t)
			return
		}
		for _, k := range sortedKeys(obj) {
			u.matchFunctionValue(c.at(k), obj[k], t.elem)
		}
	case fnObject:
		u.matchFunctionObject(c, v, t)
	}
}

// matchFunctionBinding checks a binding: an expression by its inferred
// type, a translation as a string; any other binding is not judged.
func (u *unit) matchFunctionBinding(c vctx, obj map[string]any, t *fnType) {
	switch {
	case obj["$expr"] != nil:
		if e, ok := u.exprs[c.file+"#"+c.ptr]; ok && e.typ != nil && !t.accepts(e.typ) {
			u.report(c.code, c.file, c.ptr+"/$expr", "expected %s, the expression is %s", t, e.typ)
		}
	case obj["$t"] != nil:
		if t.kind != fnString {
			u.report(c.code, c.file, c.ptr, "a translation is a string, not a %s", t)
		}
	}
}

// matchFunctionScalar checks a literal against a scalar type.
func (u *unit) matchFunctionScalar(c vctx, v any, t *fnType) {
	ok := false
	switch x := v.(type) {
	case bool:
		ok = t.kind == fnBool
	case string:
		ok = t.kind == fnString || t.kind == fnEnum && slices.Contains(t.members, x)
	case json.Number:
		if i, err := x.Float64(); err == nil {
			ok = t.kind == fnDouble || t.kind == fnInt && i == math.Trunc(i)
		}
	}
	if !ok {
		u.report(c.code, c.file, c.ptr, "expected %s", t)
	}
}

// matchFunctionObject checks an object literal: required properties
// present, each property of its type, and no others when the schema is
// closed.
func (u *unit) matchFunctionObject(c vctx, v any, t *fnType) {
	obj, ok := v.(map[string]any)
	if !ok {
		u.report(c.code, c.file, c.ptr, "expected an object")
		return
	}
	for _, name := range sortedKeys(t.required) {
		if _, set := obj[name]; !set {
			u.report(c.code, c.file, c.ptr, "the function input needs the property %q", name)
		}
	}
	for _, k := range sortedKeys(obj) {
		ft, known := t.fields[k]
		switch {
		case known:
			u.matchFunctionValue(c.at(k), obj[k], ft)
		case t.closed:
			u.report(c.code, c.file, c.ptr+plxerr.Pointer(k), "the function input has no property %q", k)
		}
	}
}

// functionOutput declares the output type of an invokeFunction step in
// synth and returns it as a type expression, or "" when the function or
// its schema is unknown. The step may not have run, so the type is
// nullable.
func (t *typer) functionOutput(g *graph, st schema.Step, synth map[string]pxl.TypeSpec) string {
	info := t.u.stepFunction(g, st)
	if info == nil || len(info.OutputSchema) == 0 {
		return ""
	}
	out := parseFnSchema(info.OutputSchema)
	out.nullable = true
	expr := out.typeExpr("PluxFn"+upperFirst(st.ID)+"Out", synth)
	if expr != "" {
		t.u.noteFunctionStep(g.file, st.ID)
	}
	return expr
}

// noteFunctionStep records a step whose output the function schema types,
// so that a mismatch on its use is reported as PLX-7032.
func (u *unit) noteFunctionStep(file, id string) {
	if u.fnSteps == nil {
		u.fnSteps = map[string]map[string]bool{}
	}
	if u.fnSteps[file] == nil {
		u.fnSteps[file] = map[string]bool{}
	}
	u.fnSteps[file][id] = true
}

// functionOutputUse matches a read of a step's output.
var functionOutputUse = regexp.MustCompile(`\bsteps\.([A-Za-z][A-Za-z0-9]*)\.output\b`)

// mismatchCode is the code of a type mismatch of an expression: PLX-7032
// when the expression reads the output of an invokeFunction step, whose
// type the function's schema gives (ACT-001).
func (u *unit) mismatchCode(c vctx, e *expr) plxerr.Code {
	for _, m := range functionOutputUse.FindAllStringSubmatch(e.src, -1) {
		if u.fnSteps[c.file][m[1]] {
			return plxerr.FunctionTypeMismatch
		}
	}
	return c.code
}

// functions is the checking stage for function grants (FN-008, FN-055):
// what each plugin's grants ask of the catalogue and of the security
// settings, and what each function document of the project declares.
func functions(u *unit) {
	for _, fn := range u.project.Functions {
		if fn.Doc != nil {
			u.checkFunctionDocument(fn)
		}
	}
	if u.opts.Functions == nil {
		return
	}
	for _, pl := range u.plugins {
		if pl.doc.Capabilities == nil {
			continue
		}
		for i := range pl.doc.Capabilities.Functions {
			u.checkGrant(pl, i)
		}
	}
}

// checkFunctionDocument rejects a device-placed function document that
// declares server-only capabilities (FN-008).
func (u *unit) checkFunctionDocument(fn schema.Loaded[schema.FunctionDocument]) {
	if fn.Doc.Placement != schema.PlacementDevice {
		return
	}
	for _, c := range serverOnlyCapabilities(fn.Doc.Capabilities) {
		u.report(plxerr.DeviceFunctionServerCapability, fn.Source.File, plxerr.Pointer("capabilities", c),
			"function %q is placed on the device and so cannot declare %s, a server-only capability", fn.Doc.Name, c)
	}
}

// checkGrant checks one grant of a plugin against the catalogue: it must
// exist, a device-placed function asks for no server capability (PLX-7030)
// and the security settings must allow device placement (PLX-7014).
func (u *unit) checkGrant(pl *plugin, i int) {
	grant := &pl.doc.Capabilities.Functions[i]
	ptr := plxerr.Pointer("capabilities", "functions", strconv.Itoa(i))
	info := u.functionInfo(grant)
	if info == nil {
		u.report(plxerr.FunctionReferenceUnresolved, pl.file, ptr+"/function", "function %q is unknown to the app's function catalogue", grant.Function)
		return
	}
	if info.Placement != schema.PlacementDevice {
		return
	}
	for _, c := range serverOnlyCapabilities(info.Capabilities) {
		u.report(plxerr.DeviceFunctionServerCapability, pl.file, ptr+"/function",
			"function %q is placed on the device and so cannot declare %s, a server-only capability", grant.Function, c)
	}
	if !u.opts.Functions.DeviceFunctionsEnabled() {
		u.report(plxerr.DeviceFunctionsDisabled, pl.file, ptr+"/function",
			"function %q is placed on the device, which the security setting deviceFunctions does not allow", grant.Function)
	}
}

// grantVersions lists, for each function grant of a plugin in document
// order, the version the catalogue resolves it to (FN-006), so that the
// device sends the version the release pins. It is nil without a
// catalogue, and 0 for a grant the catalogue does not know.
func (u *unit) grantVersions(pl *plugin) []uint32 {
	if u.opts.Functions == nil || pl.doc.Capabilities == nil {
		return nil
	}
	out := make([]uint32, len(pl.doc.Capabilities.Functions))
	for i := range pl.doc.Capabilities.Functions {
		if info := u.functionInfo(&pl.doc.Capabilities.Functions[i]); info != nil {
			out[i] = info.Version
		}
	}
	return out
}

// deviceFunctionsOf lists the device-placed functions a plugin grants,
// sorted by name, with their descriptions. Grants of one function under
// several aliases count once.
func (u *unit) deviceFunctionsOf(pl *plugin) ([]DeviceFunctionRef, map[string]*FunctionInfo) {
	infos := map[string]*FunctionInfo{}
	if u.opts.Functions == nil || pl.doc.Capabilities == nil {
		return nil, infos
	}
	var refs []DeviceFunctionRef
	for i := range pl.doc.Capabilities.Functions {
		grant := &pl.doc.Capabilities.Functions[i]
		info := u.functionInfo(grant)
		if info == nil || info.Placement != schema.PlacementDevice || infos[grant.Function] != nil {
			continue
		}
		infos[grant.Function] = info
		refs = append(refs, DeviceFunctionRef{Name: grant.Function, Version: info.Version})
	}
	slices.SortFunc(refs, func(a, b DeviceFunctionRef) int { return compareStrings(a.Name, b.Name) })
	return refs, infos
}

// compareStrings orders strings bytewise.
func compareStrings(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// wasmSection adds the plugin's device module to its bundle (FN-051): one
// module linking all its device-placed functions, with the functions it
// exports sorted by name. A plugin without device functions has none.
func (u *unit) wasmSection(o *out, pl *plugin) {
	refs, infos := u.deviceFunctionsOf(pl)
	if len(refs) == 0 {
		return
	}
	if u.opts.DeviceModules == nil {
		u.report(plxerr.FunctionModuleInvalid, pl.file, "/capabilities/functions", "plugin %q grants device-placed functions, but no device module source is available", pl.key)
		return
	}
	mod, err := u.opts.DeviceModules.Module(pl.key, refs)
	if err != nil {
		u.report(plxerr.FunctionModuleInvalid, pl.file, "/capabilities/functions", "the device module of plugin %q: %v", pl.key, err)
		return
	}
	for _, r := range refs {
		if _, ok := mod.Index[r.Name]; !ok {
			u.report(plxerr.FunctionModuleInvalid, pl.file, "/capabilities/functions", "the device module of plugin %q does not export %q", pl.key, r.Name)
			return
		}
	}
	if max := u.opts.Limits.Get(limits.BundleDeviceFunctionModuleSize); int64(len(mod.Wasm)) > max {
		u.report(plxerr.LimitExceeded, pl.file, "", "the device module has %d bytes, above bundle.deviceFunctionModuleSize = %d", len(mod.Wasm), max)
		return
	}
	o.features[bundle.FeatureWasm] = true
	o.add(bundle.SectionWasm, o.id, encodeWasmModule(o.id, mod, refs, infos))
}

// encodeWasmModule writes the wasm section: the module, its SHA-256, its
// ABI version and the exported functions sorted by name.
func encodeWasmModule(id [16]byte, mod DeviceModule, refs []DeviceFunctionRef, infos map[string]*FunctionInfo) []byte {
	b := flatbuffers.NewBuilder(len(mod.Wasm) + 512)
	sum := sha256.Sum256(mod.Wasm)
	offs := make([]flatbuffers.UOffsetT, len(refs))
	for i, r := range refs {
		name := b.CreateString(r.Name)
		var timeout, memory uint32
		if l := infos[r.Name].Limits; l != nil {
			timeout, memory = optionalU32(l.TimeoutMs), optionalU32(l.MemoryBytes)
		}
		fbs.DeviceFunctionStart(b)
		fbs.DeviceFunctionAddName(b, name)
		fbs.DeviceFunctionAddIndex(b, mod.Index[r.Name])
		fbs.DeviceFunctionAddVersion(b, r.Version)
		fbs.DeviceFunctionAddTimeoutMs(b, timeout)
		fbs.DeviceFunctionAddMemoryBytes(b, memory)
		offs[i] = fbs.DeviceFunctionEnd(b)
	}
	fns := offsetVector(b, offs)
	hash := b.CreateByteVector(sum[:])
	code := b.CreateByteVector(mod.Wasm)
	fbs.WasmModuleStart(b)
	fbs.WasmModuleAddId(b, createUUIDBytes(b, id))
	fbs.WasmModuleAddHash(b, hash)
	fbs.WasmModuleAddModule(b, code)
	fbs.WasmModuleAddAbiVersion(b, mod.ABIVersion)
	fbs.WasmModuleAddFunctions(b, fns)
	return finish(b, fbs.WasmModuleEnd(b), bundle.SectionWasm)
}

// optionalU32 converts an optional limit to the uint32 the table holds; an
// absent limit is 0, the registry default (FN-053).
func optionalU32(v *int64) uint32 {
	if v == nil || *v <= 0 {
		return 0
	}
	return uint32(min(*v, math.MaxUint32)) //nolint:gosec // G115: clamped.
}

// createUUIDBytes writes a UUID given as bytes as a bundle Uuid struct.
func createUUIDBytes(b *flatbuffers.Builder, id [16]byte) flatbuffers.UOffsetT {
	hi, lo := uuidHalves(id)
	return fbs.CreateUuid(b, hi, lo)
}

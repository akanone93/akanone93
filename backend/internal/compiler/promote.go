// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"strconv"
	"strings"
	"testing/fstest"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/pxl"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// promotedKinds are the document kinds whose trees can hold a Variant node
// or a PXL binding of an experiment.
var promotedKinds = map[schema.DocumentKind]bool{
	schema.KindApp: true, schema.KindPlugin: true, schema.KindPage: true, schema.KindComponent: true,
	schema.KindTemplate: true, schema.KindActionGraph: true,
}

// PromoteWinner returns the project of fsys with the experiment resolved to
// its winning variant (ABT-005): every Variant node of the experiment is
// replaced by the winner's subtree, the control's where the winner has
// none, every `experiment.<key>` binding becomes the winner's name and
// every `experiment.<key>.<prop>` binding the winner's value of the prop,
// and the experiment's document is dropped. Compiling the result gives the
// bundles of a project written with the winner directly, byte for byte
// (CMP-002). The pass reads fsys alone and is deterministic.
//
// It reports PLX-2300 for an experiment the project does not define and
// PLX-2301 when the winner is not one of its variants; a problem of the
// project itself is reported as the loader does, and nothing is promoted.
func PromoteWinner(fsys fs.FS, experimentKey, winner string) (fs.FS, plxerr.Diagnostics) {
	v, err := sharedValidator()
	if err != nil {
		return nil, plxerr.Diagnostics{plxerr.NewDiagnostic(plxerr.InternalCompilerError, plxerr.Location{}, "the embedded JSON Schemas do not compile: %v", err)}
	}
	project, diags := schema.NewLoader(v, schema.DefaultMigrator(), limits.Defaults()).Load(fsys)
	if diags.HasErrors() {
		return nil, diags
	}
	p, drop, diag := newPromoter(project, experimentKey, winner)
	if diag != nil {
		return nil, plxerr.Diagnostics{*diag}
	}
	out := fstest.MapFS{}
	err = fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || name == drop {
			return err
		}
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		if place, ok := schema.PlaceOf(name); ok && promotedKinds[place.Kind] {
			data = p.document(data)
		}
		out[name] = &fstest.MapFile{Data: data}
		return nil
	})
	if err != nil {
		return nil, plxerr.Diagnostics{plxerr.NewDiagnostic(plxerr.FileSystemError, plxerr.Location{}, "read the project: %v", err)}
	}
	p.diags.Sort()
	if p.diags.HasErrors() {
		return nil, p.diags
	}
	return out, nil
}

// promoter rewrites documents for one experiment and winner.
type promoter struct {
	key, winner string
	// props are the winner's value of each declared prop, as PXL source.
	props map[string]string
	// maxDepth bounds the syntax tree of an expression it rewrites.
	maxDepth int
	diags    plxerr.Diagnostics
}

// newPromoter finds the experiment and its winner and returns the file of
// the experiment's document.
func newPromoter(project *schema.Project, key, winner string) (*promoter, string, *plxerr.Diagnostic) {
	var exp *schema.Loaded[schema.ExperimentDocument]
	for i := range project.Experiments {
		if d := project.Experiments[i].Doc; d != nil && d.Key == key {
			exp = &project.Experiments[i]
		}
	}
	if exp == nil {
		d := plxerr.NewDiagnostic(plxerr.ExperimentKeyUnknown, plxerr.Location{}, "the project defines no experiment %q", key)
		return nil, "", &d
	}
	var won *schema.ExperimentVariant
	for i := range exp.Doc.Variants {
		if exp.Doc.Variants[i].Name == winner {
			won = &exp.Doc.Variants[i]
		}
	}
	if won == nil {
		d := plxerr.NewDiagnostic(plxerr.WinnerNotAVariant, plxerr.Location{File: exp.Source.File, Path: "/variants"}, "%q is not a variant of experiment %q", winner, key)
		return nil, "", &d
	}
	p := &promoter{key: key, winner: winner, props: map[string]string{}, maxDepth: int(limits.Defaults().Get(limits.PXLNestingDepth))}
	for i, prop := range exp.Doc.Props {
		lit, ok := pxlLiteral(prop.Type, won.Props[prop.Name])
		if !ok {
			p.diags = append(p.diags, plxerr.NewDiagnostic(plxerr.ValueTypeMismatch, plxerr.Location{File: exp.Source.File, Path: plxerr.Pointer("props", strconv.Itoa(i))},
				"the value of prop %q (%s) cannot be written into a binding", prop.Name, prop.Type))
			continue
		}
		p.props[prop.Name] = lit
	}
	return p, exp.Source.File, nil
}

// document rewrites one document, and returns it unchanged when it holds
// nothing of the experiment or cannot be decoded (the loader has accepted
// it, so the latter does not happen).
func (p *promoter) document(data []byte) []byte {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var doc any
	if dec.Decode(&doc) != nil {
		return data
	}
	out := p.walk(doc)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if enc.Encode(out) != nil {
		return data
	}
	return buf.Bytes()
}

// walk rewrites a JSON value: a Variant node of the experiment becomes the
// winner's arm and an expression has its bindings inlined.
func (p *promoter) walk(v any) any {
	switch x := v.(type) {
	case []any:
		for i := range x {
			x[i] = p.walk(x[i])
		}
	case map[string]any:
		if arm, ok := p.arm(x); ok {
			return p.walk(arm)
		}
		for k, e := range x {
			if s, isExpr := e.(string); isExpr && k == "$expr" {
				x[k] = p.inline(s)
				continue
			}
			x[k] = p.walk(e)
		}
	}
	return v
}

// arm returns the subtree a Variant node of the experiment is replaced by.
func (p *promoter) arm(m map[string]any) (any, bool) {
	if m["type"] != variantType || m["experiment"] != p.key {
		return nil, false
	}
	if variants, ok := m["variants"].(map[string]any); ok {
		if won, ok := variants[p.winner]; ok {
			return won, true
		}
	}
	if control, ok := m["control"]; ok {
		return control, true
	}
	return nil, false
}

// inline replaces the bindings of the experiment in a PXL expression by
// constants: `experiment.<key>.<prop>` by the winner's declared prop value
// and `experiment.<key>` by the winner's name. The expression is parsed, so
// string literals and other paths are left alone; one the parser refuses is
// returned unchanged (the loader has accepted it, so that does not happen).
func (p *promoter) inline(src string) string {
	out, err := pxl.ReplaceMembers(src, "experiment", p.maxDepth, func(path []string) (string, bool) {
		if path[0] != p.key {
			return "", false
		}
		switch len(path) {
		case 1:
			return strconv.Quote(p.winner), true
		case 2:
			lit, declared := p.props[path[1]]
			return lit, declared
		default:
			return "", false
		}
	})
	if err != nil {
		return src
	}
	return out
}

// pxlLiteral writes a prop value as a PXL constant. It handles the types
// whose constants PXL has a literal for: bool, int, double and string,
// each nullable.
func pxlLiteral(typ string, raw json.RawMessage) (string, bool) {
	if string(raw) == "null" && strings.HasSuffix(typ, "?") {
		return "null", true
	}
	switch strings.TrimSuffix(typ, "?") {
	case "bool":
		return string(raw), string(raw) == "true" || string(raw) == "false"
	case "int":
		n, err := strconv.ParseInt(string(raw), 10, 64)
		return signed(strconv.FormatInt(n, 10), n < 0), err == nil
	case "double":
		f, err := strconv.ParseFloat(string(raw), 64)
		s := strconv.FormatFloat(f, 'f', -1, 64)
		if !strings.ContainsAny(s, ".") {
			s += ".0"
		}
		return signed(s, f < 0), err == nil
	case "string":
		var s string
		if len(raw) == 0 || raw[0] != '"' || json.Unmarshal(raw, &s) != nil || strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 && r != '\n' && r != '\t' && r != '\r' }) {
			return "", false
		}
		r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\t", `\t`, "\r", `\r`)
		return `"` + r.Replace(s) + `"`, true
	}
	return "", false
}

// signed parenthesises a negative literal, so it binds as a constant
// wherever the binding stood.
func signed(lit string, negative bool) string {
	if negative {
		return "(" + lit + ")"
	}
	return lit
}

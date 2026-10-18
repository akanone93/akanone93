// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Package catalogue compares the widget and action catalogues of the
// specification (Appendix C and D) with the descriptor registries and the
// runtime's builders and handlers, so that a catalogue entry cannot pass its
// phase without being built.
package catalogue

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Entry is one catalogue row item: a widget type or an action name.
type Entry struct {
	Name  string
	Phase int
}

var (
	phaseCell  = regexp.MustCompile(`^P(\d+)$`)
	widgetName = regexp.MustCompile("`([A-Z][A-Za-z0-9]*)`")
	actionName = regexp.MustCompile("`([a-z][A-Za-z0-9]*)`")
	parens     = regexp.MustCompile(`\([^)]*\)`)
)

func cells(line string) []string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "|") {
		return nil
	}
	parts := strings.Split(strings.Trim(line, "|"), "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func phaseOf(cell string) (int, bool) {
	m := phaseCell.FindStringSubmatch(cell)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	return n, err == nil
}

// ParseWidgetRow extracts the widgets of one Appendix C table row. A name
// cell is the first cell or the cell right after a phase cell; it is paired
// with the next phase cell. Text in parentheses is ignored; every backticked
// identifier counts, "/" alternatives included, except dotted ones such as
// `Text.rich`.
func ParseWidgetRow(line string) []Entry {
	var out []Entry
	var pending []string
	expectName := true
	for _, c := range cells(line) {
		if p, ok := phaseOf(c); ok {
			for _, n := range pending {
				out = append(out, Entry{Name: n, Phase: p})
			}
			pending = nil
			expectName = true
			continue
		}
		if !expectName {
			continue
		}
		expectName = false
		c = parens.ReplaceAllString(c, "")
		for _, m := range widgetName.FindAllStringSubmatch(c, -1) {
			pending = append(pending, m[1])
		}
	}
	return out
}

// ParseActionRow extracts the actions of one Appendix D table row: the
// lowercase-initial identifiers of the first cell, with the last phase cell.
func ParseActionRow(line string) []Entry {
	cs := cells(line)
	if len(cs) < 2 {
		return nil
	}
	phase, found := 0, false
	for _, c := range cs[1:] {
		if p, ok := phaseOf(c); ok {
			phase, found = p, true
		}
	}
	if !found {
		return nil
	}
	var out []Entry
	for _, m := range actionName.FindAllStringSubmatch(cs[0], -1) {
		out = append(out, Entry{Name: m[1], Phase: phase})
	}
	return out
}

// section returns the lines between the heading starting with from and the
// heading starting with to.
func section(doc, from, to string) []string {
	var out []string
	in := false
	for _, l := range strings.Split(doc, "\n") {
		switch {
		case strings.HasPrefix(l, from):
			in = true
		case in && strings.HasPrefix(l, to):
			return out
		case in:
			out = append(out, l)
		}
	}
	return out
}

// ParseAppendices reads Appendix C and D from the specification text.
func ParseAppendices(doc string) (widgets, actions map[string]int) {
	widgets, actions = map[string]int{}, map[string]int{}
	for _, l := range section(doc, "## Appendix C — Widget Catalogue", "## Appendix D") {
		for _, e := range ParseWidgetRow(l) {
			if _, dup := widgets[e.Name]; !dup {
				widgets[e.Name] = e.Phase
			}
		}
	}
	for _, l := range section(doc, "## Appendix D — Built-in Action Catalogue", "## Appendix E") {
		for _, e := range ParseActionRow(l) {
			if _, dup := actions[e.Name]; !dup {
				actions[e.Name] = e.Phase
			}
		}
	}
	return widgets, actions
}

// Descriptor is the part of a widget or action descriptor the check needs.
type Descriptor struct {
	ID    int
	Phase int
}

type descriptorJSON struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	ID    int    `json:"id"`
	Phase string `json:"phase"`
}

func loadDescriptors(dir string, key func(descriptorJSON) string, out map[string]Descriptor) error {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return fmt.Errorf("list %s: %w", dir, err)
	}
	sort.Strings(files)
	for _, f := range files {
		data, err := os.ReadFile(f) //nolint:gosec // G304: paths come from a glob under the repository root.
		if err != nil {
			return fmt.Errorf("read %s: %w", f, err)
		}
		var d descriptorJSON
		if err := json.Unmarshal(data, &d); err != nil {
			return fmt.Errorf("parse %s: %w", f, err)
		}
		p, _ := phaseOf(d.Phase)
		if k := key(d); k != "" {
			out[k] = Descriptor{ID: d.ID, Phase: p}
		}
	}
	return nil
}

var (
	generatedLine = regexp.MustCompile(`^\s*(\d+):\s*[A-Za-z_]\w*,`)
	widgetIDsRef  = regexp.MustCompile(`WidgetIds\.([a-z]\w*)`)
	widgetIDConst = regexp.MustCompile(`\b([a-z]\w*)WidgetId\b`)
	stringLiteral = regexp.MustCompile(`'([A-Z][A-Za-z0-9]*)'`)
	actionKey     = regexp.MustCompile(`'([a-z][A-Za-z0-9]*)'\s*:`)
	enumBody      = regexp.MustCompile(`(?s)\benum\s+StructuralAction\s*\{(.*?)(?:;|\})`)
	lineComment   = regexp.MustCompile(`//[^\n]*`)
	identifier    = regexp.MustCompile(`[A-Za-z_]\w*`)
	classDecl     = regexp.MustCompile(`\bclass\s+(\w+)`)
	intConstDecl  = regexp.MustCompile(`static const int (\w+)\s*=\s*(\d+);`)
	buildersHead  = regexp.MustCompile(`get builders\s*(?:=>|\{)`)
	builderKey    = regexp.MustCompile(`(?:^|[{,])\s*(?:([A-Z]\w*)\.(\w+)|(\d+))\s*:`)
)

var dartReserved = map[string]bool{
	"assert": true, "break": true, "case": true, "catch": true, "class": true, "const": true,
	"continue": true, "default": true, "do": true, "else": true, "enum": true, "extends": true,
	"false": true, "final": true, "finally": true, "for": true, "if": true, "in": true, "is": true,
	"new": true, "null": true, "rethrow": true, "return": true, "super": true, "switch": true,
	"this": true, "throw": true, "true": true, "try": true, "var": true, "void": true,
	"while": true, "with": true,
}

// ConstName returns the WidgetIds constant name of a widget type: the type in
// lowerCamel, with a trailing underscore for Dart reserved words.
func ConstName(typeName string) string {
	if typeName == "" {
		return ""
	}
	n := strings.ToLower(typeName[:1]) + typeName[1:]
	if dartReserved[n] {
		n += "_"
	}
	return n
}

// ScanStructuralActions returns the members of `enum StructuralAction`, whose
// actions the interpreter dispatches by enum value instead of by name key.
func ScanStructuralActions(src string) []string {
	m := enumBody.FindStringSubmatch(lineComment.ReplaceAllString(src, ""))
	if m == nil {
		return nil
	}
	return identifier.FindAllString(m[1], -1)
}

// Builders is the set of widget IDs and type names known to the renderer.
type Builders struct {
	IDs   map[int]bool
	Names map[string]bool // type names found as string literals
	refs  map[string]bool // WidgetIds constant names

	keyIDs  map[int]bool    // integer keys of a `get builders` map literal
	consts  map[string]int  // "Class.name" of static const int declarations
	keyRefs map[string]bool // "Class.name" keys of a `get builders` map literal
}

// NewBuilders returns an empty Builders holding the given generated IDs.
func NewBuilders(ids map[int]bool) *Builders {
	return &Builders{
		IDs: ids, Names: map[string]bool{}, refs: map[string]bool{},
		keyIDs: map[int]bool{}, consts: map[string]int{}, keyRefs: map[string]bool{},
	}
}

// ScanGeneratedBuilders collects the keys of the generatedBuilders map.
func ScanGeneratedBuilders(src string) map[int]bool {
	ids := map[int]bool{}
	in := false
	for _, l := range strings.Split(src, "\n") {
		if strings.Contains(l, "generatedBuilders") && strings.Contains(l, "{") {
			in = true
			continue
		}
		if !in {
			continue
		}
		if strings.HasPrefix(l, "}") {
			break
		}
		if m := generatedLine.FindStringSubmatch(l); m != nil {
			n, _ := strconv.Atoi(m[1])
			ids[n] = true
		}
	}
	return ids
}

// ScanHandWritten records literals and WidgetIds references of a hand-written
// Dart source.
func (b *Builders) ScanHandWritten(src string) {
	for _, m := range stringLiteral.FindAllStringSubmatch(src, -1) {
		b.Names[m[1]] = true
	}
	for _, m := range widgetIDsRef.FindAllStringSubmatch(src, -1) {
		b.refs[m[1]] = true
	}
	for _, m := range widgetIDConst.FindAllStringSubmatch(src, -1) {
		b.refs[m[1]] = true
	}
	cls := ""
	for _, l := range strings.Split(src, "\n") {
		if m := classDecl.FindStringSubmatch(l); m != nil {
			cls = m[1]
		}
		if m := intConstDecl.FindStringSubmatch(l); m != nil && cls != "" {
			n, _ := strconv.Atoi(m[2])
			b.consts[cls+"."+m[1]] = n
		}
	}
	for _, loc := range buildersHead.FindAllStringIndex(src, -1) {
		open := strings.IndexByte(src[loc[0]:], '{')
		if open < 0 {
			continue
		}
		for _, m := range builderKey.FindAllStringSubmatch(mapBody(src[loc[0]+open:]), -1) {
			if m[3] != "" {
				n, _ := strconv.Atoi(m[3])
				b.keyIDs[n] = true
			} else {
				b.keyRefs[m[1]+"."+m[2]] = true
			}
		}
	}
}

// mapBody returns the text of the brace-delimited block that starts at s[0].
func mapBody(s string) string {
	depth := 0
	for i, c := range s {
		switch c {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[:i]
			}
		}
	}
	return s
}

// Has reports whether a widget is built. Builders registered by optional
// packages count when their ID is a `get builders` key, either as an integer
// literal or as a `Class.constant` reference to a static const int.
func (b *Builders) Has(typeName string, id int) bool {
	if b.IDs[id] || b.keyIDs[id] || b.Names[typeName] || b.refs[ConstName(typeName)] {
		return true
	}
	for ref := range b.keyRefs {
		if v, ok := b.consts[ref]; ok && v == id {
			return true
		}
	}
	return false
}

func handWrittenDart(root string, visit func(src string)) error {
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		n := d.Name()
		if d.IsDir() || !strings.HasSuffix(n, ".dart") || strings.HasSuffix(n, ".g.dart") || strings.HasSuffix(n, "_generated.dart") {
			return nil
		}
		data, err := os.ReadFile(p) //nolint:gosec // G304: walked under the repository root.
		if err != nil {
			return fmt.Errorf("read %s: %w", p, err)
		}
		visit(string(data))
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk %s: %w", root, err)
	}
	return nil
}

// Check runs the comparison for every appendix entry up to phase and returns
// the sorted findings.
func Check(root string, phase int) ([]string, error) {
	spec, err := os.ReadFile(filepath.Join(root, "docs", "requirements.md")) //nolint:gosec // G304: fixed path under root.
	if err != nil {
		return nil, fmt.Errorf("read the specification: %w", err)
	}
	widgets, actions := ParseAppendices(string(spec))

	wd, ad := map[string]Descriptor{}, map[string]Descriptor{}
	for _, layer := range []string{"layer1", "layer2"} {
		if err := loadDescriptors(filepath.Join(root, "schema", "widgets", layer), func(d descriptorJSON) string { return d.Type }, wd); err != nil {
			return nil, err
		}
	}
	if err := loadDescriptors(filepath.Join(root, "schema", "actions"), func(d descriptorJSON) string { return d.Name }, ad); err != nil {
		return nil, err
	}

	b, err := loadBuilders(root)
	if err != nil {
		return nil, err
	}
	pkgs, err := filepath.Glob(filepath.Join(root, "packages", "*", "lib"))
	if err != nil {
		return nil, fmt.Errorf("list the packages: %w", err)
	}
	for _, p := range pkgs {
		if err := handWrittenDart(p, b.ScanHandWritten); err != nil {
			return nil, err
		}
	}
	handlers, err := handlerNames(pkgs)
	if err != nil {
		return nil, err
	}

	out := widgetFindings(widgets, wd, b, phase)
	out = append(out, actionFindings(actions, ad, handlers, phase)...)
	sort.Strings(out)
	return out, nil
}

// loadBuilders returns the builders the generated render code declares, or
// none when the generated file does not exist yet.
func loadBuilders(root string) (*Builders, error) {
	b := NewBuilders(map[int]bool{})
	gen, err := os.ReadFile(filepath.Join(root, "packages", "plux_flutter", "lib", "src", "render", "generated", "render.g.dart")) //nolint:gosec // G304: fixed path under root.
	if err == nil {
		b.IDs = ScanGeneratedBuilders(string(gen))
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read the generated render code: %w", err)
	}
	return b, nil
}

// handlerNames returns the action names the hand-written Dart of pkgs handles,
// by their key or by a structural action.
func handlerNames(pkgs []string) (map[string]bool, error) {
	handlers := map[string]bool{}
	for _, p := range pkgs {
		if err := handWrittenDart(p, func(src string) {
			for _, m := range actionKey.FindAllStringSubmatch(src, -1) {
				handlers[m[1]] = true
			}
			for _, n := range ScanStructuralActions(src) {
				handlers[n] = true
			}
		}); err != nil {
			return nil, err
		}
	}
	return handlers, nil
}

// widgetFindings compares the widgets of the appendix up to phase with their
// descriptors and builders, and returns the findings in no particular order.
func widgetFindings(widgets map[string]int, wd map[string]Descriptor, b *Builders, phase int) []string {
	var out []string
	for name, ph := range widgets {
		if ph > phase {
			continue
		}
		d, ok := wd[name]
		if !ok {
			out = append(out, "WIDGET "+name+": missing descriptor")
			continue
		}
		if d.Phase != ph {
			out = append(out, fmt.Sprintf("WIDGET %s: phase mismatch (appendix P%d, descriptor P%d)", name, ph, d.Phase))
		}
		if !b.Has(name, d.ID) {
			out = append(out, "WIDGET "+name+": no builder")
		}
	}
	for name := range wd {
		if _, ok := widgets[name]; !ok {
			out = append(out, "WIDGET "+name+": not in appendix")
		}
	}
	return out
}

// actionFindings compares the actions of the appendix up to phase with their
// descriptors and handlers, and returns the findings in no particular order.
func actionFindings(actions map[string]int, ad map[string]Descriptor, handlers map[string]bool, phase int) []string {
	var out []string
	for name, ph := range actions {
		if ph > phase {
			continue
		}
		d, ok := ad[name]
		if !ok {
			out = append(out, "ACTION "+name+": missing descriptor")
			continue
		}
		if d.Phase != ph {
			out = append(out, fmt.Sprintf("ACTION %s: phase mismatch (appendix P%d, descriptor P%d)", name, ph, d.Phase))
		}
		if !handlers[name] {
			out = append(out, "ACTION "+name+": no handler")
		}
	}
	for name := range ad {
		if _, ok := actions[name]; !ok {
			out = append(out, "ACTION "+name+": not in appendix")
		}
	}
	return out
}

// ParsePhase converts "P8" to 8.
func ParsePhase(s string) (int, error) {
	n, ok := phaseOf(s)
	if !ok {
		return 0, fmt.Errorf("invalid phase %q (want e.g. P8)", s)
	}
	return n, nil
}

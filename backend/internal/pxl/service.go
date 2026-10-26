// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package pxl

import (
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// This file is the PXL language service of Studio (PXL-007, ADR-0099): the
// editor features — diagnostics, completion, hover types and signature help
// — built on the parser and the type checker of the compiler, so Studio and
// the server never disagree. Every offset and range is in code points, the
// unit of CMP-004; the editor converts to its own unit.

// Service answers editor questions about expressions of one use site.
type Service struct {
	env  *Env
	opts Options
}

// NewService returns the language service for expressions checked against
// env with opts (limits and narrowing paths of the use site).
func NewService(env *Env, opts Options) *Service { return &Service{env: env, opts: opts} }

// Diagnostic is a compile-time finding with its code-point range.
type Diagnostic struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Start    int    `json:"start"`
	End      int    `json:"end"`
}

// Analysis is the outcome of checking an expression: its result type when
// it compiles, and the diagnostics.
type Analysis struct {
	// Type is the result type; empty when the expression has errors.
	Type        string       `json:"type"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// Check compiles src and reports its result type and diagnostics. It is the
// call the conformance vectors run against (PXL-007).
func (s *Service) Check(src string) Analysis {
	out := Analysis{Diagnostics: []Diagnostic{}}
	prog, typ, diags := Compile(src, s.env, s.opts, plxerr.Location{})
	for _, d := range diags {
		x := Diagnostic{Code: d.Code.String(), Severity: d.Severity.String(), Message: d.Message}
		if d.Range != nil {
			x.Start, x.End = d.Range.Start, d.Range.End
		}
		out.Diagnostics = append(out.Diagnostics, x)
	}
	if prog != nil && typ != nil {
		out.Type = typ.String()
	}
	return out
}

// trace records, while the checker runs, the type of every syntax node and
// the variables visible at every identifier.
type trace struct {
	types map[*node]*Type
	vars  map[*node]map[string]*Type
}

func newTrace() *trace {
	return &trace{types: map[*node]*Type{}, vars: map[*node]map[string]*Type{}}
}

// note records the outcome of checking n.
func (t *trace) note(c *checker, n *node, x *ir) {
	if x != nil {
		t.types[n] = x.typ
	}
	if n.kind != nIdent {
		return
	}
	vars := map[string]*Type{}
	for _, scope := range c.scopes {
		for name, v := range scope {
			vars[name] = v.typ
		}
	}
	t.vars[n] = vars
}

// examine parses and checks src, keeping the types of its nodes, also of
// an expression that has errors.
func (s *Service) examine(src string) (*node, *trace, bool) {
	if int64(len(src)) > s.opts.MaxLength*utf8.UTFMax || !utf8.ValidString(src) {
		return nil, nil, false
	}
	tree, serr := parse(src, int(s.opts.MaxDepth))
	if serr != nil {
		return nil, nil, false
	}
	tr := newTrace()
	c := &checker{env: s.env, limits: s.opts.Limits, features: map[string]bool{}, nonNull: map[string]bool{}, trace: tr}
	c.assume(s.opts.NonNull)
	c.check(tree)
	return tree, tr, true
}

// byteOffset converts a code-point offset of src to a byte offset.
func byteOffset(src string, cp int) (int, bool) {
	if cp < 0 {
		return 0, false
	}
	b := 0
	for range cp {
		if b >= len(src) {
			return 0, false
		}
		_, size := utf8.DecodeRuneInString(src[b:])
		b += size
	}
	return b, true
}

// codePoints converts a byte offset of src to a code-point offset.
func codePoints(src string, b int) int { return utf8.RuneCountInString(src[:min(b, len(src))]) }

// children lists the syntax children of n.
func children(n *node) []*node {
	var out []*node
	for _, c := range []*node{n.a, n.b, n.c} {
		if c != nil {
			out = append(out, c)
		}
	}
	out = append(out, n.args...)
	return append(out, n.keys...)
}

// Hover is the answer to a hover request.
type Hover struct {
	// Start and End delimit the hovered expression.
	Start int `json:"start"`
	End   int `json:"end"`
	// Type is the type of the expression; for a function name, its result.
	Type string `json:"type"`
	// Signatures lists the overloads when the name of a function is hovered.
	Signatures []string `json:"signatures,omitempty"`
}

// Hover returns the type of the innermost expression at offset, or the
// overloads of the function whose name is there.
func (s *Service) Hover(src string, offset int) (Hover, bool) {
	off, ok := byteOffset(src, offset)
	if !ok {
		return Hover{}, false
	}
	tree, tr, ok := s.examine(src)
	if !ok {
		return Hover{}, false
	}
	var found *node
	var walk func(n *node)
	walk = func(n *node) {
		if off < n.span.Start || off >= n.span.End {
			return
		}
		if _, typed := tr.types[n]; typed {
			found = n
		}
		for _, c := range children(n) {
			walk(c)
		}
	}
	walk(tree)
	if found == nil {
		return Hover{}, false
	}
	h := Hover{Start: codePoints(src, found.span.Start), End: codePoints(src, found.span.End), Type: tr.types[found].String()}
	if found.kind == nCall && off < found.span.Start+len(found.op) {
		h.End = h.Start + utf8.RuneCountInString(found.op)
		for _, sig := range signatures()[found.op] {
			h.Signatures = append(h.Signatures, sig.label())
		}
	}
	return h, true
}

// label renders an overload as `name(p type, …) result`.
func (s signature) label() string {
	parts := s.parameters()
	return s.def.name + "(" + strings.Join(parts, ", ") + ") " + s.def.result
}

// parameters renders the parameters of an overload as `name type`; the last
// of a variadic overload ends in "…".
func (s signature) parameters() []string {
	out := make([]string, len(s.def.params))
	for i, p := range s.def.params {
		out[i] = p.name + " " + p.typ
	}
	if s.def.variadic && len(out) > 0 {
		out[len(out)-1] += "…"
	}
	return out
}

// Completion is one proposal.
type Completion struct {
	Label string `json:"label"`
	// Kind is variable, root, field, function, method or keyword.
	Kind   string `json:"kind"`
	Detail string `json:"detail,omitempty"`
	// Insert replaces the typed word.
	Insert string `json:"insert"`
}

// CompletionList is the proposals for the word ending at the offset; From
// and To delimit the word they replace.
type CompletionList struct {
	From  int          `json:"from"`
	To    int          `json:"to"`
	Items []Completion `json:"items"`
}

// placeholder stands in for the word being typed so the incomplete
// expression parses.
const placeholder = "_pxl_"

var macroSignatures = map[string]string{
	"map":    "list<T>.map(x, expr) list<U>",
	"filter": "list<T>.filter(x, predicate) list<T>",
	"any":    "list<T>.any(x, predicate) bool",
	"all":    "list<T>.all(x, predicate) bool",
	"sortBy": "list<T>.sortBy(x, key) list<T>",
}

// isWordByte reports whether c can be part of an identifier.
func isWordByte(c byte) bool { return isLetter(c) || isDigit(c) }

// inString reports whether the end of prefix is inside a string literal.
func inString(prefix string) bool {
	var quote byte
	for i := 0; i < len(prefix); i++ {
		c := prefix[i]
		switch {
		case quote == 0 && isQuote(c):
			quote = c
		case quote != 0 && c == '\\':
			i++
		case quote != 0 && c == quote:
			quote = 0
		}
	}
	return quote != 0
}

// closers returns the brackets that close the ones src leaves open, so an
// expression typed up to the cursor still parses.
func closers(src string) string {
	var open []byte
	var quote byte
	for i := 0; i < len(src); i++ {
		c := src[i]
		if quote != 0 {
			switch c {
			case '\\':
				i++
			case quote:
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			quote = c
		case '(', '[', '{':
			open = append(open, c)
		case ')', ']', '}':
			if len(open) > 0 {
				open = open[:len(open)-1]
			}
		}
	}
	var b strings.Builder
	for i := len(open) - 1; i >= 0; i-- {
		b.WriteByte(map[byte]byte{'(': ')', '[': ']', '{': '}'}[open[i]])
	}
	return b.String()
}

// Complete proposes what can be typed at offset: members after a dot,
// otherwise roots, macro variables, functions and keywords.
func (s *Service) Complete(src string, offset int) CompletionList {
	list := CompletionList{Items: []Completion{}}
	off, ok := byteOffset(src, offset)
	if !ok || inString(src[:off]) {
		return list
	}
	ws, we := off, off
	for ws > 0 && isWordByte(src[ws-1]) {
		ws--
	}
	for we < len(src) && isWordByte(src[we]) {
		we++
	}
	if ws < off && isDigit(src[ws]) {
		return list
	}
	list.From, list.To = codePoints(src, ws), offset
	word := strings.ToLower(src[ws:off])
	member := strings.HasSuffix(strings.TrimRight(src[:ws], " \t\r\n"), ".")
	var tree *node
	var tr *trace
	head, tail := src[:ws]+placeholder, src[we:]
	for _, candidate := range []string{head + tail, head + tail + closers(head+tail), head + closers(src[:ws])} {
		if t, x, ok := s.examine(candidate); ok {
			tree, tr = t, x
			break
		}
	}
	if tree == nil {
		return list
	}
	var items []Completion
	if member {
		items = members(tree, tr)
	} else {
		items = s.names(tree, tr)
	}
	for _, it := range items {
		if strings.HasPrefix(strings.ToLower(it.Label), word) {
			list.Items = append(list.Items, it)
		}
	}
	return list
}

// find returns the first node of the tree for which match holds.
func find(n *node, match func(*node) bool) *node {
	if match(n) {
		return n
	}
	for _, c := range children(n) {
		if x := find(c, match); x != nil {
			return x
		}
	}
	return nil
}

// members lists what can follow the dot before the placeholder.
func members(tree *node, tr *trace) []Completion {
	n := find(tree, func(n *node) bool { return (n.kind == nField || n.kind == nSafe) && n.op == placeholder })
	if n == nil {
		return nil
	}
	if n.a.kind == nIdent && namespaces[n.a.op] {
		var out []Completion
		prefix := n.a.op + "."
		for _, name := range functionNames() {
			if short, ok := strings.CutPrefix(name, prefix); ok {
				out = append(out, Completion{Label: short, Kind: "function", Detail: signatures()[name][0].label(), Insert: short + "("})
			}
		}
		return out
	}
	t := tr.types[n.a]
	if t == nil {
		return nil
	}
	base := t.nonNull()
	var out []Completion
	switch base.kind {
	case KindObject:
		for _, f := range base.named.FieldOrder {
			out = append(out, Completion{Label: f, Kind: "field", Detail: base.named.Fields[f].String(), Insert: f})
		}
	case KindList:
		for _, m := range stdMacros {
			out = append(out, Completion{Label: m, Kind: "method", Detail: macroSignatures[m], Insert: m + "("})
		}
	}
	return out
}

// functionNames lists the standard-library function names in order.
func functionNames() []string {
	names := make([]string, 0, len(signatures()))
	for name := range signatures() {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// names lists what can start an expression at the placeholder.
func (s *Service) names(tree *node, tr *trace) []Completion {
	var out []Completion
	if n := find(tree, func(n *node) bool { return n.kind == nIdent && n.op == placeholder }); n != nil {
		vars := tr.vars[n]
		names := make([]string, 0, len(vars))
		for name := range vars {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			out = append(out, Completion{Label: name, Kind: "variable", Detail: vars[name].String(), Insert: name})
		}
	}
	for _, name := range s.env.Roots() {
		t, _ := s.env.Root(name)
		out = append(out, Completion{Label: name, Kind: "root", Detail: t.String(), Insert: name})
	}
	for _, name := range functionNames() {
		out = append(out, Completion{Label: name, Kind: "function", Detail: signatures()[name][0].label(), Insert: name + "("})
	}
	for _, kw := range []string{"true", "false", "null"} {
		out = append(out, Completion{Label: kw, Kind: "keyword", Insert: kw})
	}
	return out
}

// Signature is one overload shown by signature help.
type Signature struct {
	Label      string   `json:"label"`
	Parameters []string `json:"parameters"`
}

// SignatureHelp describes the call the cursor is inside.
type SignatureHelp struct {
	Signatures      []Signature `json:"signatures"`
	ActiveSignature int         `json:"activeSignature"`
	ActiveParameter int         `json:"activeParameter"`
}

// frame is an open bracket of the expression before the cursor.
type frame struct {
	call   string // function name; ".name" for a macro; empty for other brackets
	commas int
}

// callContext finds the innermost call the end of prefix is inside, and the
// index of the argument being typed.
func callContext(prefix string) (name string, active int, ok bool) {
	var stack []frame
	var quote byte
	for i := 0; i < len(prefix); i++ {
		c := prefix[i]
		if quote != 0 {
			switch c {
			case '\\':
				i++
			case quote:
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			quote = c
		case '(':
			stack = append(stack, frame{call: callName(prefix[:i])})
		case '[', '{':
			stack = append(stack, frame{})
		case ')', ']', '}':
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case ',':
			if len(stack) > 0 {
				stack[len(stack)-1].commas++
			}
		}
	}
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i].call != "" {
			return stack[i].call, stack[i].commas, true
		}
	}
	return "", 0, false
}

// callName returns the function a "(" that follows before names, or "".
func callName(before string) string {
	end := len(before)
	start := end
	for start > 0 && isWordByte(before[start-1]) {
		start--
	}
	name := before[start:end]
	if name == "" || !isLetter(name[0]) || isKeyword(name) {
		return ""
	}
	if start == 0 || before[start-1] != '.' {
		return name
	}
	rs := start - 1
	re := rs
	for rs > 0 && isWordByte(before[rs-1]) {
		rs--
	}
	if namespaces[before[rs:re]] {
		return before[rs:re] + "." + name
	}
	return "." + name
}

// SignatureHelp returns the overloads of the function being called at
// offset and the argument being typed.
func (*Service) SignatureHelp(src string, offset int) (SignatureHelp, bool) {
	off, ok := byteOffset(src, offset)
	if !ok {
		return SignatureHelp{}, false
	}
	name, active, ok := callContext(src[:off])
	if !ok {
		return SignatureHelp{}, false
	}
	if macro, isMacro := strings.CutPrefix(name, "."); isMacro {
		label, known := macroSignatures[macro]
		if !known {
			return SignatureHelp{}, false
		}
		return SignatureHelp{Signatures: []Signature{{Label: label, Parameters: []string{"x", "expr"}}}, ActiveParameter: min(active, 1)}, true
	}
	sigs, known := signatures()[name]
	if !known {
		return SignatureHelp{}, false
	}
	help := SignatureHelp{ActiveParameter: active}
	pick := -1
	for i, sig := range sigs {
		help.Signatures = append(help.Signatures, Signature{Label: sig.label(), Parameters: sig.parameters()})
		if pick < 0 && (active < len(sig.params) || sig.def.variadic) {
			pick = i
		}
	}
	help.ActiveSignature = max(pick, 0)
	chosen := sigs[help.ActiveSignature]
	if last := len(chosen.params) - 1; last >= 0 && active > last && chosen.def.variadic {
		help.ActiveParameter = last
	}
	return help, true
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"maps"

	"github.com/nightCode42/plux3/backend/internal/l10n"
	"github.com/nightCode42/plux3/backend/internal/l10n/mf"
	"github.com/nightCode42/plux3/backend/internal/pxl"
	"github.com/nightCode42/plux3/backend/internal/schema"
)

// designPageCacheSize bounds the pages a DesignEvaluator keeps compiled.
const designPageCacheSize = 64

// DesignEvaluator evaluates the expressions of pages with design-time values
// for Studio's canvas preview (STU-005, ADR-0112): a widget bound to
// `40.0 * device.textScale` or `format.number(item)` is drawn with its value,
// as the device draws it. Each expression runs as the page's check compiled
// it, in its own scope, within the PXL limits. It keeps each page's programs
// until the page or the project changes, the locales it loaded and the last
// messages it was given. It is not safe for concurrent use; Studio's
// WebAssembly module is single threaded.
type DesignEvaluator struct {
	pages    map[string]*designPage
	locales  map[string]*l10n.Locale
	messages map[string]string
	parsed   map[string]*mf.Message
}

// designPage is a page's expressions by source, as checked with a project.
type designPage struct {
	validator *Validator
	data      []byte
	exprs     map[string]designExpr
	limits    pxl.Limits
}

// designExpr is an expression compiled in its scope's environment.
type designExpr struct {
	program *pxl.Program
	typ     *pxl.Type
	env     *pxl.Env
}

// DesignRequest is a batch of design-time evaluations.
type DesignRequest struct {
	// Locale is the tag the format and l10n groups use.
	Locale string `json:"locale"`
	// Messages are the locale's messages by key, which t() formats; absent,
	// the last ones given stay.
	Messages map[string]string `json:"messages,omitempty"`
	// Now is the frozen current time, in milliseconds since the epoch.
	Now   int64              `json:"now"`
	Pages []DesignPageSource `json:"pages"`
	// Values are sets of design-time root values; a set may extend an
	// earlier one (Base), as a list's item extends its page.
	Values []DesignValues `json:"values"`
	Items  []DesignItem   `json:"items"`
}

// DesignPageSource is a page as the canvas draws it, unsaved edits included,
// or a component, whose own expressions (reading `props` and `component`)
// are evaluated as the loaded project holds it.
type DesignPageSource struct {
	File string          `json:"file"`
	Data json.RawMessage `json:"data"`
}

// DesignValues are root values by root name; a root that is absent, or a
// field that does not fit its type, reads as its type's empty value
// (pxl.DesignFromJSON). With Base, the roots of that earlier set are kept
// where Roots does not name them.
type DesignValues struct {
	Base  *int           `json:"base,omitempty"`
	Roots map[string]any `json:"roots"`
}

// DesignItem is one expression of a page with the index of its values.
type DesignItem struct {
	Page   int    `json:"page"`
	Values int    `json:"values"`
	Src    string `json:"src"`
}

// NewDesignEvaluator returns an evaluator with nothing cached.
func NewDesignEvaluator() *DesignEvaluator {
	return &DesignEvaluator{
		pages:   map[string]*designPage{},
		locales: map[string]*l10n.Locale{},
		parsed:  map[string]*mf.Message{},
	}
}

// Evaluate answers a request, given as JSON, with the project of v:
// {"results": [{"value": …} | {"error": "…"}, …]}, one per item, in order.
// An expression the page does not hold, or one that fails at run time,
// answers its error; a malformed request answers {"error": "…"}. It never
// panics.
func (d *DesignEvaluator) Evaluate(v *Validator, request []byte) (out []byte) {
	defer func() {
		if r := recover(); r != nil {
			out = designError(fmt.Sprintf("the evaluation failed unexpectedly: %v", r))
		}
	}()
	var req DesignRequest
	dec := json.NewDecoder(bytes.NewReader(request))
	dec.UseNumber()
	if err := dec.Decode(&req); err != nil {
		return designError("request: " + err.Error())
	}
	if req.Messages != nil {
		d.messages = req.Messages
		d.parsed = map[string]*mf.Message{}
	}
	loc := d.locale(req.Locale)
	b := &designBatch{
		req:    &req,
		ctx:    &pxl.LocaleContext{Locale: loc, Translate: d.translator(loc)},
		pages:  make([]*designPage, len(req.Pages)),
		inputs: map[designInputKey]map[string]pxl.Value{},
	}
	for i, p := range req.Pages {
		b.pages[i] = d.page(v, p)
	}
	results := make([]map[string]any, len(req.Items))
	for i, it := range req.Items {
		results[i] = b.eval(it)
	}
	data, err := json.Marshal(map[string]any{"results": results})
	if err != nil {
		return designError(err.Error())
	}
	return data
}

// designBatch is one request being answered: its pages and the inputs it
// converted, by environment and values set.
type designBatch struct {
	req    *DesignRequest
	ctx    *pxl.LocaleContext
	pages  []*designPage
	inputs map[designInputKey]map[string]pxl.Value
}

type designInputKey struct {
	env    *pxl.Env
	values int
}

// eval evaluates one item.
func (b *designBatch) eval(it DesignItem) map[string]any {
	if it.Page < 0 || it.Page >= len(b.pages) || it.Values < 0 || it.Values >= len(b.req.Values) {
		return map[string]any{"error": "the item names no page or values"}
	}
	page := b.pages[it.Page]
	if page == nil {
		return map[string]any{"error": "the page does not check"}
	}
	e, ok := page.exprs[it.Src]
	if !ok {
		return map[string]any{"error": "the page holds no such expression that checks"}
	}
	v, err := e.program.EvalIn(b.inputsOf(e.env, it.Values, 0), page.limits, b.ctx)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	return map[string]any{"value": pxl.ToJSON(e.typ, v)}
}

// inputsOf converts a values set for an environment once per batch: the
// roots of its base, then its own, then the frozen time.
func (b *designBatch) inputsOf(env *pxl.Env, at, depth int) map[string]pxl.Value {
	key := designInputKey{env: env, values: at}
	if in, ok := b.inputs[key]; ok {
		return in
	}
	set := b.req.Values[at]
	var in map[string]pxl.Value
	// A base comes earlier in the list, so a chain of bases ends.
	if base := set.Base; base != nil && *base >= 0 && *base < at && depth < len(b.req.Values) {
		in = maps.Clone(b.inputsOf(env, *base, depth+1))
		for name, raw := range set.Roots {
			if t, ok := env.Root(name); ok && name != pxl.NowRoot {
				in[name] = pxl.DesignFromJSON(t, raw)
			}
		}
	} else {
		in = map[string]pxl.Value{}
		for _, name := range env.Roots() {
			if t, ok := env.Root(name); ok {
				in[name] = pxl.DesignFromJSON(t, set.Roots[name])
			}
		}
	}
	in[pxl.NowRoot] = pxl.DateTime{Millis: b.req.Now}
	b.inputs[key] = in
	return in
}

// page returns a page's expressions by source, checked again only when the
// page or the project changed; nil when the page does not check.
func (d *DesignEvaluator) page(v *Validator, src DesignPageSource) *designPage {
	if p, ok := d.pages[src.File]; ok && p.validator == v && bytes.Equal(p.data, src.Data) {
		return p
	}
	var u *unit
	var ok bool
	if place, known := schema.PlaceOf(src.File); known && place.Kind == schema.KindComponent {
		u, ok = v.checkComponent(src.File)
	} else {
		u, _, ok = v.checkPage(src.File, src.Data)
	}
	if !ok {
		return nil
	}
	exprs := map[string]designExpr{}
	for _, key := range sortedKeys(u.exprs) {
		e := u.exprs[key]
		if e.file != src.File || e.prog == nil || e.scope == nil {
			continue
		}
		if _, seen := exprs[e.src]; seen {
			continue
		}
		if env, err := u.env(e.scope); err == nil {
			exprs[e.src] = designExpr{program: e.prog, typ: e.typ, env: env}
		}
	}
	if len(d.pages) >= designPageCacheSize {
		d.pages = map[string]*designPage{}
	}
	p := &designPage{validator: v, data: bytes.Clone(src.Data), exprs: exprs, limits: pxl.LimitsFrom(v.opts.Limits)}
	d.pages[src.File] = p
	return p
}

// locale returns the embedded locale data for tag, loaded once; nil when no
// locale on its chain has data, so the format and l10n groups fail.
func (d *DesignEvaluator) locale(tag string) *l10n.Locale {
	if loc, ok := d.locales[tag]; ok {
		return loc
	}
	loc, err := l10n.Load(cmp.Or(tag, "en"))
	if err != nil {
		loc = nil
	}
	d.locales[tag] = loc
	return loc
}

// translator formats the last messages given, each parsed once.
func (d *DesignEvaluator) translator(loc *l10n.Locale) func(key string, args map[string]string) (string, bool) {
	return func(key string, args map[string]string) (string, bool) {
		text, ok := d.messages[key]
		if !ok || loc == nil {
			return "", false
		}
		m, ok := d.parsed[key]
		if !ok {
			parsed, err := mf.Parse(text)
			if err != nil {
				return "", false
			}
			m = parsed
			d.parsed[key] = m
		}
		values := make(map[string]any, len(args))
		for k, a := range args {
			values[k] = a
		}
		s, err := m.Format(loc, values)
		return s, err == nil
	}
}

func designError(msg string) []byte {
	data, _ := json.Marshal(map[string]string{"error": msg})
	return data
}

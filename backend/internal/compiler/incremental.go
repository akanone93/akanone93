// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"bytes"
	"crypto/sha256"
	"io/fs"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/nightCode42/plux3/backend/internal/icons"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/pxl"
	"github.com/nightCode42/plux3/backend/internal/schema"
)

// Incremental compilation (CMP-051). Compile builds every bundle from the
// whole project; Recompile builds the same bundles after the pages named
// as dirty changed, and re-checks and re-encodes only those pages. What it
// does not redo it takes from a Memo, which CompileMemo or an earlier
// Recompile returned. The result is byte-identical to Compile's (CMP-002),
// or Recompile declines and the caller runs a full compilation.
//
// A Memo is only as good as the rule that decides when it may be used, so
// Recompile declines whenever it cannot show that the unchanged pages'
// products are still what Compile would make:
//
//   - a document other than the dirty pages and the action graphs bound to
//     them differs from the Memo's project, or the project's files differ;
//   - the checking stages report anything on the dirty pages (the previous
//     compilation had no diagnostics, so any diagnostic is new, and a full
//     compilation reports it with its cross-document context);
//   - the dirty pages no longer contribute something the project-wide
//     aggregates held (a feature, an icon, a calendar, a currency, a
//     translation key): another page may or may not still contribute it, and
//     only a full compilation knows.
//
// The aggregates that reach a bundle from many pages are the unit fields
// the facts type lists; TestUnitFieldsAreClassified keeps that list honest.

// Memo is what Recompile reuses from an earlier compilation of the same
// project with the same Options: the encoded section of every page and
// component with the programs, styles and assets it registered, the lowered
// action graphs, the project-wide aggregates and the reference graph. It is
// owned by the caller, is not safe for concurrent use, and is replaced by
// the Memo each Recompile returns.
type Memo struct {
	project *schema.Project
	// sources are the hashes of the project's documents, by file.
	sources map[string][sha256.Size]byte
	// pages and comps are by document file.
	pages map[string]*pageRecord
	comps map[string]*sectionRecord
	// lowered are the graphs of each plugin bundle, by plugin key.
	lowered   map[string][]*graph
	appGraphs []*graph
	facts     facts
	graph     *Graph
	// money remembers which documents declare a type that mentions money,
	// by file (declaresMoney).
	money map[string]moneyEntry
}

// sectionRecord is a page or component section and what encoding it
// registered in its bundle.
type sectionRecord struct {
	data    []byte
	contrib *contribution
}

// pageRecord is a page's section and its timelines, which the bundle's
// timelines section is written from.
type pageRecord struct {
	sectionRecord
	timelines []*timeline
}

// contribution is what encoding one section registered in its bundle: the
// programs and styles it refers to by content ID, the assets it uses, the
// features its programs need and the locations of its nodes.
type contribution struct {
	programs map[uint64]*pxl.Program
	styles   map[uint64]*style
	// encoded and canonical are the encodings of the programs and styles,
	// which merge compares when two sections register one content ID.
	encoded   map[uint64][]byte
	canonical map[uint64][]byte
	assets    map[string]bool
	features  map[string]bool
	nodes     []location
}

func newMemo() *Memo {
	return &Memo{pages: map[string]*pageRecord{}, comps: map[string]*sectionRecord{}, lowered: map[string][]*graph{}, money: map[string]moneyEntry{}}
}

// Project returns the project the Memo describes.
func (m *Memo) Project() *schema.Project { return m.project }

// Graph returns the reference graph of the project the Memo describes; the
// caller must not modify it.
func (m *Memo) Graph() *Graph { return m.graph }

// CompileMemo is Compile that also returns a Memo for Recompile; the Memo
// is nil when the compilation has diagnostics.
func CompileMemo(fsys fs.FS, opts Options) (*Result, *Memo) {
	return compileWith(fsys, opts, true)
}

// finishMemo completes the Memo a capturing compilation filled while it
// encoded.
func (u *unit) finishMemo() *Memo {
	m := u.next
	m.project = u.project
	m.sources = sourcesOf(u.project)
	m.facts = u.factsOf(false)
	m.graph = &Graph{Edges: slices.Clone(u.graph.Edges)}
	return m
}

// Recompile compiles the project p, which differs from the one m describes
// only in the pages named by dirty (their document files) and the action
// graphs bound to them, with the Options m was made with. It checks and
// encodes the dirty pages, takes everything else from m, and returns the
// result that Compile would return for p, except that the reports of the
// whole project (Accessibility, Localisation, StoredState, DeviceUses,
// Collections, GatewaySources) are absent: a release is made by Compile.
// It also returns the Memo of p. ok is false when the reuse rules of this
// file's comment do not hold, or the compilation fails; the caller then
// runs CompileMemo. It never panics.
func Recompile(m *Memo, p *schema.Project, opts Options, dirty []string) (res *Result, next *Memo, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			res, next, ok = nil, nil, false
		}
	}()
	if m == nil || p == nil || p.App.Doc == nil {
		return nil, nil, false
	}
	focus, ok := m.focusOf(p, dirty)
	if !ok || !m.sameOutside(p, focus) {
		return nil, nil, false
	}
	u := newUnit(opts)
	u.project, u.focus = p, focus
	run(u, stagesWhere(true))
	if len(u.diags) > 0 {
		return nil, nil, false
	}
	before, ok := m.focusedFacts(opts, focus)
	fresh := u.factsOf(true)
	if !ok || !fresh.covers(before) {
		return nil, nil, false
	}
	total := m.facts.union(fresh)
	u.adopt(total)
	u.memo, u.next = m, m.successor()
	for _, s := range stagesWhere(false) {
		s.run(u)
	}
	if len(u.diags) > 0 || u.app == nil {
		return nil, nil, false
	}
	next = u.next
	next.project, next.sources, next.facts = p, sourcesOf(p), total
	next.graph = m.mergeGraph(u.graph, focus)
	res = &Result{App: u.app, Plugins: u.outputs, Graph: next.graph, Files: u.files, Project: p}
	if p.NativeCatalogue != nil {
		res.Natives = p.NativeCatalogue.Doc
	}
	return res, next, true
}

// stagesWhere returns the checking stages, or the producing ones.
func stagesWhere(checking bool) []stage {
	return slices.DeleteFunc(slices.Clone(pipeline), func(s stage) bool { return s.checking != checking })
}

// successor returns a Memo that starts as m and takes the records of the
// compilation in progress.
func (m *Memo) successor() *Memo {
	n := newMemo()
	maps.Copy(n.pages, m.pages)
	maps.Copy(n.comps, m.comps)
	maps.Copy(n.lowered, m.lowered)
	maps.Copy(n.money, m.money)
	n.appGraphs = m.appGraphs
	return n
}

// focusOf returns the files Recompile checks: the dirty pages and the
// action graphs bound to them, in the project it was given and in the
// Memo's, since an edit may bind a graph to another page.
func (m *Memo) focusOf(p *schema.Project, dirty []string) (focus map[string]bool, ok bool) {
	if len(dirty) == 0 {
		return nil, false
	}
	focus = map[string]bool{}
	ids := map[string]bool{}
	for _, file := range dirty {
		now, found := pageIDOf(p, file)
		was, existed := pageIDOf(m.project, file)
		if !found || !existed || now != was {
			return nil, false
		}
		focus[file] = true
		ids[now] = true
	}
	for _, proj := range []*schema.Project{p, m.project} {
		for i := range proj.Plugins {
			for _, g := range proj.Plugins[i].Graphs {
				if g.Doc != nil && g.Source != nil && ids[g.Doc.Page] {
					focus[g.Source.File] = true
				}
			}
		}
	}
	return focus, true
}

// pageIDOf returns the ID of the page held by a file of the project.
func pageIDOf(p *schema.Project, file string) (string, bool) {
	for i := range p.Plugins {
		for _, pg := range p.Plugins[i].Pages {
			if pg.Doc != nil && pg.Source != nil && pg.Source.File == file {
				return pg.Doc.ID, true
			}
		}
	}
	return "", false
}

// sameOutside reports whether p has the documents and asset files of the
// Memo's project, except that the focus files may differ.
func (m *Memo) sameOutside(p *schema.Project, focus map[string]bool) bool {
	now := sourcesOf(p)
	if len(now) != len(m.sources) {
		return false
	}
	for file, h := range now {
		was, ok := m.sources[file]
		if !ok || (!focus[file] && was != h) {
			return false
		}
	}
	if len(p.AssetFiles) != len(m.project.AssetFiles) {
		return false
	}
	for name, data := range p.AssetFiles {
		if was, ok := m.project.AssetFiles[name]; !ok || !bytes.Equal(was, data) {
			return false
		}
	}
	return true
}

// loadedType reports whether t is a type of the project model that holds
// document sources; sourcesOf walks only those, so a document's content is
// not searched.
func loadedType(t reflect.Type) bool {
	name := t.Name()
	return t.PkgPath() == reflect.TypeFor[schema.Project]().PkgPath() && (name == "Project" || name == "Plugin" || strings.HasPrefix(name, "Loaded["))
}

// sourcesOf returns the hash of every document of the project by file. It
// finds the documents by reflection, so a document kind added to the model
// is covered without a change here.
func sourcesOf(p *schema.Project) map[string][sha256.Size]byte {
	out := map[string][sha256.Size]byte{}
	collectSources(reflect.ValueOf(p), out)
	return out
}

func collectSources(v reflect.Value, out map[string][sha256.Size]byte) {
	switch v.Kind() {
	case reflect.Pointer:
		collectPointer(v, out)
	case reflect.Struct:
		collectStruct(v, out)
	case reflect.Slice:
		if loadedType(v.Type().Elem()) {
			for i := range v.Len() {
				collectSources(v.Index(i), out)
			}
		}
	default:
	}
}

// collectPointer records the source a pointer points at, or walks the model
// type it points at.
func collectPointer(v reflect.Value, out map[string][sha256.Size]byte) {
	if v.IsNil() {
		return
	}
	if s, ok := v.Interface().(*schema.Source); ok {
		out[s.File] = s.Hash
		return
	}
	if loadedType(v.Type().Elem()) {
		collectSources(v.Elem(), out)
	}
}

// collectStruct walks the fields of a model type.
func collectStruct(v reflect.Value, out map[string][sha256.Size]byte) {
	if !loadedType(v.Type()) {
		return
	}
	for i := range v.NumField() {
		if v.Type().Field(i).IsExported() {
			collectSources(v.Field(i), out)
		}
	}
}

// mergeGraph returns the reference graph of the new project: the Memo's
// edges located outside the focus, and those the focused checking made
// inside it.
func (m *Memo) mergeGraph(fresh *Graph, focus map[string]bool) *Graph {
	var added []Edge
	for _, e := range fresh.Edges {
		if focus[e.File] {
			added = append(added, e)
		}
	}
	slices.SortFunc(added, compareEdges)
	added = slices.Compact(added)
	// The Memo's edges are sorted, so the kept ones merge with the new ones
	// in one pass.
	edges := make([]Edge, 0, len(m.graph.Edges)+len(added))
	for _, e := range m.graph.Edges {
		if focus[e.File] {
			continue
		}
		for len(added) > 0 && compareEdges(added[0], e) < 0 {
			edges, added = append(edges, added[0]), added[1:]
		}
		edges = append(edges, e)
	}
	return &Graph{Edges: append(edges, added...)}
}

// pageBytes returns the section of a page: the Memo's for a page that is
// not in focus, otherwise encoded. A compilation that captures a Memo
// records what encoding registered in the bundle.
func (u *unit) pageBytes(o *out, pg *page) []byte {
	switch {
	case u.memo != nil && !u.inFocus(pg.file):
		rec, ok := u.memo.pages[pg.file]
		if !ok {
			u.internalError("no record of the page %s", pg.file)
			return nil
		}
		u.merge(o, rec.contrib)
		pg.timelines = rec.timelines
		u.next.pages[pg.file] = rec
		return rec.data
	case u.next != nil:
		scratch := o.scratch()
		data := u.pageSection(scratch, pg)
		c := scratch.contribution()
		u.merge(o, c)
		u.next.pages[pg.file] = &pageRecord{sectionRecord{data, c}, pg.timelines}
		return data
	default:
		return u.pageSection(o, pg)
	}
}

// componentBytes is pageBytes for a component.
func (u *unit) componentBytes(o *out, c *component) []byte {
	switch {
	case u.memo != nil && !u.inFocus(c.file):
		rec, ok := u.memo.comps[c.file]
		if !ok {
			u.internalError("no record of the component %s", c.file)
			return nil
		}
		u.merge(o, rec.contrib)
		u.next.comps[c.file] = rec
		return rec.data
	case u.next != nil:
		scratch := o.scratch()
		data := u.componentSection(scratch, c)
		rec := &sectionRecord{data, scratch.contribution()}
		u.merge(o, rec.contrib)
		u.next.comps[c.file] = rec
		return data
	default:
		return u.componentSection(o, c)
	}
}

// scratch returns an empty bundle under construction like o, to encode one
// section into and learn what it registers.
func (o *out) scratch() *out { return newOut(o.kind, o.pl, o.id, o.key) }

// contribution returns what the sections encoded into a scratch bundle
// registered.
func (o *out) contribution() *contribution {
	c := &contribution{
		programs: o.programs, styles: o.styles, assets: o.assets, features: o.features, nodes: o.srcmap.nodes,
		encoded: make(map[uint64][]byte, len(o.programs)), canonical: make(map[uint64][]byte, len(o.styles)),
	}
	for id, p := range o.programs {
		c.encoded[id] = p.Encode()
	}
	for id, st := range o.styles {
		c.canonical[id] = canonical(st.value)
	}
	return c
}

// merge registers a contribution in a bundle as encoding the section there
// would have, reporting colliding content IDs (PLX-2202).
func (u *unit) merge(o *out, c *contribution) {
	if o.merged == nil {
		o.merged = &contribution{encoded: map[uint64][]byte{}, canonical: map[uint64][]byte{}}
	}
	for _, id := range slices.Sorted(maps.Keys(c.programs)) {
		if prev, ok := o.merged.encoded[id]; ok && !bytes.Equal(prev, c.encoded[id]) {
			u.report(plxerr.ContentIDCollision, "", "", "programs %x collide", id)
		}
		o.programs[id] = c.programs[id]
		o.merged.encoded[id] = c.encoded[id]
	}
	for _, id := range slices.Sorted(maps.Keys(c.styles)) {
		if prev, ok := o.merged.canonical[id]; ok && !bytes.Equal(prev, c.canonical[id]) {
			u.report(plxerr.ContentIDCollision, "", "", "styles %x collide", id)
		}
		o.styles[id] = c.styles[id]
		o.merged.canonical[id] = c.canonical[id]
	}
	maps.Copy(o.assets, c.assets)
	maps.Copy(o.features, c.features)
	o.srcmap.nodes = append(o.srcmap.nodes, c.nodes...)
}

// noteLowered records the graphs each bundle carries, for the Memo.
func (u *unit) noteLowered() {
	if u.next == nil {
		return
	}
	for _, pl := range u.plugins {
		u.next.lowered[pl.key] = pl.lowered
	}
	u.next.appGraphs = u.appGraphs
}

// lowerReusing is lower for Recompile: a bundle carries the graphs the
// Memo recorded, except those of the pages in focus, whose graphs were
// checked and lowered again.
func (u *unit) lowerReusing(byID func(a, b *graph) int) {
	u.appGraphs = u.memo.appGraphs
	for _, pl := range u.plugins {
		var all []*graph
		for _, g := range u.memo.lowered[pl.key] {
			if g.page == nil || !u.focus[g.page.file] {
				all = append(all, g)
			}
		}
		for _, g := range slices.Concat(pl.graphs, pl.inline) {
			if g.page != nil && u.focus[g.page.file] {
				all = append(all, g)
			}
		}
		slices.SortFunc(all, byID)
		pl.lowered = all
	}
	u.noteLowered()
}

// factFields are the fields of the unit that facts carries.
var factFields = []string{"calendars", "use", "features", "icons"}

// facts are the aggregates of the whole project that reach a bundle from
// many documents (see TestUnitFieldsAreClassified), each a string such as
// "feature\x00plugin\x00name", and the translation keys the documents in
// focus reference, which decide the unused-key warning.
type facts struct {
	set  map[string]struct{}
	keys map[string]struct{}
}

func newFacts() facts { return facts{set: map[string]struct{}{}, keys: map[string]struct{}{}} }

// factSep separates the parts of a fact.
const factSep = "\x00"

func (f facts) add(parts ...string) { f.set[strings.Join(parts, factSep)] = struct{}{} }

// covers reports whether f holds every fact of g.
func (f facts) covers(g facts) bool {
	for k := range g.set {
		if _, ok := f.set[k]; !ok {
			return false
		}
	}
	for k := range g.keys {
		if _, ok := f.keys[k]; !ok {
			return false
		}
	}
	return true
}

// union returns the aggregates of f and g, without the key references,
// which describe documents in focus only.
func (f facts) union(g facts) facts {
	u := newFacts()
	maps.Copy(u.set, f.set)
	maps.Copy(u.set, g.set)
	return u
}

// factsOf returns the unit's aggregates; with withKeys also the translation
// keys the documents in focus reference.
func (u *unit) factsOf(withKeys bool) facts {
	f := newFacts()
	for pl, set := range u.features {
		for name := range set {
			f.add("feature", pluginKeyOf(pl), name)
		}
	}
	for pl, bySet := range u.icons {
		for set, names := range bySet {
			for name := range names {
				f.add("icon", pluginKeyOf(pl), string(set), name)
			}
		}
	}
	for c := range u.calendars {
		f.add("calendar", c)
	}
	for c := range u.use.currencies {
		f.add("currency", c)
	}
	if u.use.dynamicCurrency {
		f.add("dynamicCurrency")
	}
	if u.use.territories {
		f.add("territories")
	}
	if tk := u.project.TranslationKeys; withKeys && tk != nil && tk.Doc != nil {
		u.eachKeyUse(tk, func(id, file string) {
			if u.inFocus(file) {
				f.keys[id] = struct{}{}
			}
		})
	}
	return f
}

// pluginKeyOf returns a plugin's key, or "" for the app.
func pluginKeyOf(pl *plugin) string {
	if pl == nil {
		return ""
	}
	return pl.key
}

// adopt adds the aggregates in f to the unit's.
func (u *unit) adopt(f facts) {
	byKey := map[string]*plugin{"": nil}
	for _, pl := range u.plugins {
		byKey[pl.key] = pl
	}
	for fact := range f.set {
		parts := strings.Split(fact, factSep)
		switch parts[0] {
		case "feature":
			u.adoptFeature(byKey, parts[1], parts[2])
		case "icon":
			u.adoptIcon(byKey, parts[1], icons.Set(parts[2]), parts[3])
		case "calendar":
			if u.calendars == nil {
				u.calendars = map[string]bool{}
			}
			u.calendars[parts[1]] = true
		case "currency":
			u.use.addCurrency(parts[1])
		case "dynamicCurrency":
			u.use.dynamicCurrency = true
		case "territories":
			u.use.territories = true
		}
	}
}

// adoptFeature records a feature a bundle requires.
func (u *unit) adoptFeature(byKey map[string]*plugin, plugin, feature string) {
	pl, ok := byKey[plugin]
	if !ok {
		return
	}
	if u.features[pl] == nil {
		u.features[pl] = map[string]bool{}
	}
	u.features[pl][feature] = true
}

// adoptIcon records an icon a bundle uses.
func (u *unit) adoptIcon(byKey map[string]*plugin, plugin string, set icons.Set, name string) {
	pl, ok := byKey[plugin]
	if !ok {
		return
	}
	if u.icons[pl] == nil {
		u.icons[pl] = map[icons.Set]map[string]bool{}
	}
	if u.icons[pl][set] == nil {
		u.icons[pl][set] = map[string]bool{}
	}
	u.icons[pl][set][name] = true
}

// focusedFacts returns the aggregates of the focused documents as the
// Memo's project had them: what Recompile compares the edit against.
func (m *Memo) focusedFacts(opts Options, focus map[string]bool) (facts, bool) {
	u := newUnit(opts)
	u.project, u.focus = m.project, focus
	run(u, stagesWhere(true))
	if len(u.diags) > 0 {
		return facts{}, false
	}
	return u.factsOf(true), true
}

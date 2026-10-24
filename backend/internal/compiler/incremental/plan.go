// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package incremental

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// groups indexes the pages of a project and the action graphs bound to
// them: the unit the compiler recompiles.
type groups struct {
	// pageFile maps a page ID to its document file.
	pageFile map[string]string
	// owner maps the document file of a page, or of an action graph bound
	// to a page, to the page's file.
	owner map[string]string
	// pages and graphs are the loaded documents, by file.
	pages  map[string]*schema.Loaded[schema.PageDocument]
	graphs map[string]*schema.Loaded[schema.ActionGraphDocument]
}

// groupsOf indexes a project.
func groupsOf(p *schema.Project) *groups {
	g := &groups{
		pageFile: map[string]string{}, owner: map[string]string{},
		pages: map[string]*schema.Loaded[schema.PageDocument]{}, graphs: map[string]*schema.Loaded[schema.ActionGraphDocument]{},
	}
	for i := range p.Plugins {
		pl := &p.Plugins[i]
		for j := range pl.Pages {
			if pg := &pl.Pages[j]; pg.Doc != nil && pg.Source != nil {
				g.pageFile[pg.Doc.ID] = pg.Source.File
				g.owner[pg.Source.File] = pg.Source.File
				g.pages[pg.Source.File] = pg
			}
		}
	}
	for i := range p.Plugins {
		pl := &p.Plugins[i]
		for j := range pl.Graphs {
			gr := &pl.Graphs[j]
			if gr.Doc == nil || gr.Source == nil {
				continue
			}
			g.graphs[gr.Source.File] = gr
			if page, ok := g.pageFile[gr.Doc.Page]; ok && gr.Doc.Page != "" {
				g.owner[gr.Source.File] = page
			}
		}
	}
	return g
}

// files returns the document files of the pages' groups, sorted.
func (g *groups) files(pages []string) []string {
	set := map[string]bool{}
	for _, page := range pages {
		set[page] = true
	}
	for file, page := range g.owner {
		if slices.Contains(pages, page) {
			set[file] = true
		}
	}
	out := make([]string, 0, len(set))
	for file := range set {
		out = append(out, file)
	}
	slices.Sort(out)
	return out
}

// updates are the changed documents of the pages and their graphs, parsed.
type updates struct {
	pages  map[string]schema.Loaded[schema.PageDocument]
	graphs map[string]schema.Loaded[schema.ActionGraphDocument]
	// edited are the pages whose own documents, or whose graphs, changed
	// in content (not only in layout).
	edited map[string]bool
}

// parseUpdates parses the changed files, which must all be pages or action
// graphs bound to pages. The reason is non-empty when they are not, or when
// a document has a problem the full compiler is to report.
func (c *Cache) parseUpdates(snap *snapshot, files []string, old *groups) (up *updates, reason string) {
	loader, err := c.loaderFor()
	if err != nil {
		return nil, "the loader is unavailable"
	}
	up = &updates{pages: map[string]schema.Loaded[schema.PageDocument]{}, graphs: map[string]schema.Loaded[schema.ActionGraphDocument]{}, edited: map[string]bool{}}
	max := c.opts.Limits.Get(limits.DocumentFileSize)
	for _, file := range files {
		if _, ok := old.owner[file]; !ok {
			return nil, fmt.Sprintf("%s is not a page or an action graph of a page", file)
		}
		data := snap.data[file]
		if int64(len(data)) > max {
			return nil, fmt.Sprintf("%s is above document.fileSize", file)
		}
		kind := schema.KindActionGraph
		if _, isPage := old.pages[file]; isPage {
			kind = schema.KindPage
		}
		src, diags := loader.ParseDocument(file, data, kind)
		if src == nil || diags.HasErrors() {
			return nil, fmt.Sprintf("%s has problems", file)
		}
		if reason := up.add(old, src); reason != "" {
			return nil, reason
		}
	}
	return up, ""
}

// add decodes a parsed document, checks that it keeps the identity the
// project's layout was checked against, and records the pages it edits.
func (up *updates) add(old *groups, src *schema.Source) string {
	if pg, ok := old.pages[src.File]; ok {
		var doc schema.PageDocument
		if err := json.Unmarshal(src.Canonical, &doc); err != nil {
			return fmt.Sprintf("%s does not decode", src.File)
		}
		if doc.ID != pg.Doc.ID || doc.Key != pg.Doc.Key {
			return fmt.Sprintf("%s changed its id or key", src.File)
		}
		if src.Hash != pg.Source.Hash {
			up.pages[src.File] = schema.Loaded[schema.PageDocument]{Doc: &doc, Source: src}
			up.edited[src.File] = true
		}
		return ""
	}
	gr := old.graphs[src.File]
	var doc schema.ActionGraphDocument
	if err := json.Unmarshal(src.Canonical, &doc); err != nil {
		return fmt.Sprintf("%s does not decode", src.File)
	}
	if doc.ID != gr.Doc.ID || doc.Key != gr.Doc.Key {
		return fmt.Sprintf("%s changed its id or key", src.File)
	}
	if src.Hash == gr.Source.Hash {
		return ""
	}
	page, ok := old.pageFile[doc.Page]
	if !ok || doc.Page == "" {
		return fmt.Sprintf("%s is no longer bound to a page of the project", src.File)
	}
	up.graphs[src.File] = schema.Loaded[schema.ActionGraphDocument]{Doc: &doc, Source: src}
	up.edited[old.owner[src.File]] = true
	up.edited[page] = true
	return ""
}

// patch returns a copy of the project in which the updated documents
// replace the old ones; the project itself is not modified.
func (up *updates) patch(p *schema.Project) *schema.Project {
	n := *p
	n.Plugins = slices.Clone(p.Plugins)
	for i := range n.Plugins {
		pl := &n.Plugins[i]
		pl.Pages = replaced(pl.Pages, up.pages)
		pl.Graphs = replaced(pl.Graphs, up.graphs)
	}
	return &n
}

// replaced returns list with the documents of by in place of the ones in the
// same file; list itself when none is there.
func replaced[T any](list []schema.Loaded[T], by map[string]schema.Loaded[T]) []schema.Loaded[T] {
	var out []schema.Loaded[T]
	for i, l := range list {
		if l.Source == nil {
			continue
		}
		if r, ok := by[l.Source.File]; ok {
			if out == nil {
				out = slices.Clone(list)
			}
			out[i] = r
		}
	}
	if out == nil {
		return list
	}
	return out
}

// refs maps the identifier a reference points at to the files that hold
// references to it, from the reference graph.
func refs(g *compiler.Graph) map[string][]string {
	out := map[string][]string{}
	for _, e := range g.Edges {
		if !slices.Contains(out[e.To], e.File) {
			out[e.To] = append(out[e.To], e.File)
		}
	}
	return out
}

// targetsOf lists the identifiers other documents can use to refer to a
// page or to something in it: its ID, key and route, and the IDs of
// everything it declares, and of its bound graphs.
func targetsOf(old, now *groups, page string) map[string]bool {
	set := map[string]bool{}
	for _, g := range []*groups{old, now} {
		pg, ok := g.pages[page]
		if !ok {
			continue
		}
		set[pg.Doc.ID], set[pg.Doc.Key], set[pg.Doc.Route] = true, true, true
		collectIDs(pg.Source.Tree, set)
		for file, owner := range g.owner {
			if gr, ok := g.graphs[file]; ok && owner == page {
				collectIDs(gr.Source.Tree, set)
			}
		}
	}
	delete(set, "")
	return set
}

// collectIDs adds the value of every "id" key of a document tree.
func collectIDs(v any, set map[string]bool) {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			if s, ok := e.(string); ok && k == "id" {
				set[s] = true
			}
			collectIDs(e, set)
		}
	case []any:
		for _, e := range x {
			collectIDs(e, set)
		}
	}
}

// dependants returns the edited pages and every page that refers, directly
// or through other pages, to something in them (CMP-051). The reason is
// non-empty when a document that is not a page refers to one: it would have
// to be recompiled too.
func dependants(edited map[string]bool, old, now *groups, byTarget map[string][]string) (pages []string, reason string) {
	dirty := map[string]bool{}
	var queue []string
	for page := range edited {
		dirty[page] = true
		queue = append(queue, page)
	}
	for len(queue) > 0 {
		page := queue[0]
		queue = queue[1:]
		for target := range targetsOf(old, now, page) {
			for _, file := range byTarget[target] {
				owner, ok := old.owner[file]
				if !ok {
					return nil, fmt.Sprintf("%s refers to the page %s and is not a page", file, page)
				}
				if !dirty[owner] {
					dirty[owner] = true
					queue = append(queue, owner)
				}
			}
		}
	}
	for page := range dirty {
		pages = append(pages, page)
	}
	slices.Sort(pages)
	return pages, ""
}

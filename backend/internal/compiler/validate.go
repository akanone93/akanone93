// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"encoding/json"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/pxl"
	"github.com/nightCode42/plux3/backend/internal/schema"
)

// Validator validates edited pages of one project as the user types
// (SCH-042, NFR-033): the project is loaded once, and each edit re-parses
// only the edited page and runs the checking stages.
type Validator struct {
	loader  *schema.Loader
	project *schema.Project
	opts    Options
}

// NewValidator loads the project in fsys and returns a validator with the
// diagnostics of the loaded project. It never panics (CMP-052).
func NewValidator(fsys fs.FS, opts Options) (v *Validator, diags plxerr.Diagnostics) {
	defer func() {
		if r := recover(); r != nil {
			v = nil
			diags = plxerr.Diagnostics{plxerr.NewDiagnostic(plxerr.InternalCompilerError, plxerr.Location{}, "the validator failed unexpectedly: %v", r)}
		}
	}()
	sv, err := sharedValidator()
	if err != nil {
		return nil, plxerr.Diagnostics{plxerr.NewDiagnostic(plxerr.InternalCompilerError, plxerr.Location{}, "the embedded JSON Schemas do not compile: %v", err)}
	}
	loader := schema.NewLoader(sv, schema.DefaultMigrator(), opts.Limits)
	project, diags := loader.Load(fsys)
	return &Validator{loader: loader, project: project, opts: opts}, diags
}

// ValidatePage validates the content of a page file — edited, or new in
// an existing plugin directory — against the rest of the project and
// returns the diagnostics located in that file and in the action graphs
// the page owns, which an edit of its state or parameters can break. It
// checks only that page and its graphs; redirect loops are followed
// through every page.
func (v *Validator) ValidatePage(file string, data []byte) (diags plxerr.Diagnostics) {
	defer func() {
		if r := recover(); r != nil {
			diags = append(diags, plxerr.NewDiagnostic(plxerr.InternalCompilerError, plxerr.Location{File: file}, "the validator failed unexpectedly: %v", r))
		}
	}()
	u, diags, ok := v.checkPage(file, data)
	if !ok {
		return diags
	}
	files := map[string]bool{file: true}
	for _, pl := range u.plugins {
		for _, g := range pl.graphs {
			if g.page != nil && g.page.file == file {
				files[g.file] = true
			}
		}
	}
	for _, d := range u.diags {
		if files[d.File] {
			diags = append(diags, d)
		}
	}
	diags.Sort()
	return diags
}

// ValidateDocument validates the content of any project file as the user types, by the kind its path holds (SCH-006,
// STU-064): a page against the rest of the project, as ValidatePage does; any other document (the app document, a
// plugin's manifest or canvas, the theme, translations, an action graph, a component) against its own schema and the
// rule that a keyed file is named after its key. Checking every file as a page told a valid plugin.json or canvas.json
// that it was in the wrong place (PLX-1020). A path the layout does not know is reported as such. It never panics
// (CMP-052).
func (v *Validator) ValidateDocument(file string, data []byte) (diags plxerr.Diagnostics) {
	defer func() {
		if r := recover(); r != nil {
			diags = append(diags, plxerr.NewDiagnostic(plxerr.InternalCompilerError, plxerr.Location{File: file}, "the validator failed unexpectedly: %v", r))
		}
	}()
	place, ok := schema.PlaceOf(file)
	if !ok {
		return plxerr.Diagnostics{plxerr.NewDiagnostic(plxerr.InvalidProjectLayout, plxerr.Location{File: file}, "%s is not a document path of the project layout", file)}
	}
	if place.Kind == schema.KindPage {
		return v.ValidatePage(file, data)
	}
	src, diags := v.loader.ParseDocument(file, data, place.Kind)
	if src == nil || diags.HasErrors() {
		diags.Sort()
		return diags
	}
	if place.Key != "" {
		name, _ := src.Tree["key"].(string)
		switch place.Kind {
		case schema.KindTranslations:
			name, _ = src.Tree["locale"].(string)
		case schema.KindFunction:
			name, _ = src.Tree["name"].(string)
		}
		if name != place.Key {
			diags = append(diags, plxerr.NewDiagnostic(plxerr.InvalidProjectLayout, plxerr.Location{File: file, Path: "/key"},
				"%s must be named after its key %q", file, name))
		}
	}
	if place.Kind == schema.KindPlugin && place.Plugin != "" {
		if key, _ := src.Tree["key"].(string); key != place.Plugin {
			diags = append(diags, plxerr.NewDiagnostic(plxerr.InvalidProjectLayout, plxerr.Location{File: file, Path: "/key"},
				"plugin.json has key %q, but its directory is plugins/%s", key, place.Plugin))
		}
	}
	diags.Sort()
	return diags
}

// ValidateProject checks the whole loaded project as Compile does and returns
// the diagnostics of the files that are not pages: the app's own documents
// (app.json, the theme, the translations and their keys), the plugins'
// manifests, components and the rest, whose findings no page validation
// reports, such as a translation missing in a locale (PLX-1606) or a key
// nothing uses (PLX-1608). Studio shows them beside the pages' own on opening
// a project (PXL-007's validator, STU-062). It never panics (CMP-052).
func (v *Validator) ValidateProject() (diags plxerr.Diagnostics) {
	defer func() {
		if r := recover(); r != nil {
			diags = plxerr.Diagnostics{plxerr.NewDiagnostic(plxerr.InternalCompilerError, plxerr.Location{}, "the validator failed unexpectedly: %v", r)}
		}
	}()
	if v.project == nil {
		return nil
	}
	pages := map[string]bool{}
	for _, pl := range v.project.Plugins {
		for _, pg := range pl.Pages {
			pages[pg.Source.File] = true
		}
	}
	// The whole pipeline, as Compile runs it: some findings, such as a
	// locale's missing message, are reported while the bundles are built. The
	// bundles stay in memory and are dropped.
	u := newUnit(v.opts)
	u.project = v.project
	run(u, pipeline)
	for _, d := range u.diags {
		if !pages[d.File] {
			diags = append(diags, d)
		}
	}
	diags.Sort()
	return diags
}

// checkPage parses the page and runs the checking stages on it. It returns
// the unit when the page parsed and belongs to a plugin directory; otherwise
// the diagnostics say why not.
func (v *Validator) checkPage(file string, data []byte) (*unit, plxerr.Diagnostics, bool) {
	src, diags := v.loader.ParseDocument(file, data, schema.KindPage)
	if src == nil || diags.HasErrors() {
		return nil, diags, false
	}
	var doc schema.PageDocument
	if err := json.Unmarshal(src.Canonical, &doc); err != nil {
		return nil, append(diags, plxerr.NewDiagnostic(plxerr.InternalCompilerError, plxerr.Location{File: file}, "decode validated page: %v", err)), false
	}
	project, ok := v.withPage(file, schema.Loaded[schema.PageDocument]{Doc: &doc, Source: src})
	if !ok {
		return nil, append(diags, plxerr.NewDiagnostic(plxerr.InvalidProjectLayout, plxerr.Location{File: file}, "no plugin directory holds %s", file)), false
	}
	u := newUnit(v.opts)
	u.project = project
	u.focus = map[string]bool{file: true}
	u.validating = true
	checks := slices.DeleteFunc(slices.Clone(pipeline), func(s stage) bool { return !s.checking })
	run(u, checks)
	return u, diags, true
}

// PXLSite is where a PXL expression of a page can be written: the binding
// object's JSON Pointer and what an expression there can read (PXL-007).
type PXLSite struct {
	Pointer string   `json:"pointer"`
	Env     int      `json:"env"`
	NonNull []string `json:"nonNull,omitempty"`
}

// PXLSites checks the page like ValidatePage and returns the expression
// sites it found, by pointer, with their distinct environments, so the
// language service completes and types expressions against the real scope.
func (v *Validator) PXLSites(file string, data []byte) (sites []PXLSite, envs []pxl.EnvSpec) {
	defer func() {
		if recover() != nil {
			sites, envs = nil, nil
		}
	}()
	u, _, ok := v.checkPage(file, data)
	if !ok {
		return nil, nil
	}
	index := map[string]int{}
	for _, key := range sortedKeys(u.exprs) {
		e := u.exprs[key]
		if e.file != file || e.scope == nil {
			continue
		}
		k := e.scope.key()
		i, seen := index[k]
		if !seen {
			i = len(envs)
			index[k] = i
			envs = append(envs, u.envSpec(e.scope))
		}
		sites = append(sites, PXLSite{Pointer: e.ptr, Env: i, NonNull: e.scope.nonNull})
	}
	return sites, envs
}

// withPage returns a copy of the project in which file holds page; the
// loaded project itself is never modified.
func (v *Validator) withPage(file string, page schema.Loaded[schema.PageDocument]) (*schema.Project, bool) {
	p := *v.project
	p.Plugins = slices.Clone(v.project.Plugins)
	for i := range p.Plugins {
		pl := &p.Plugins[i]
		if path.Dir(path.Dir(file)) != pl.Dir || !strings.HasSuffix(path.Dir(file), "/pages") {
			continue
		}
		pl.Pages = slices.Clone(pl.Pages)
		for j := range pl.Pages {
			if pl.Pages[j].Source != nil && pl.Pages[j].Source.File == file {
				pl.Pages[j] = page
				return &p, true
			}
		}
		pl.Pages = append(pl.Pages, page)
		return &p, true
	}
	return nil, false
}

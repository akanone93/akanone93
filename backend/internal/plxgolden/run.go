// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package plxgolden

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/plxtest"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/signing/runkey"
)

// Options are what a run is prepared from.
type Options struct {
	// Result is the compiled project; it must have compiled without errors.
	Result *compiler.Result
	// Plugin narrows the run to one plugin key; empty runs every plugin.
	Plugin string
	// Frames is the frame table; DefaultFrames when it has no frames.
	Frames FrameSet
	// Fonts are the vendored font files (LoadFonts).
	Fonts map[string][]byte
	// Entropy makes the key of the run.
	Entropy io.Reader
	// Runtime says where the generated project gets plux_flutter from.
	Runtime plxtest.Dependency
	// Limits are the limits cases and images are held to.
	Limits limits.Set
}

// Plan is a prepared run: the Flutter project and the cases it renders.
type Plan struct {
	Files map[string][]byte
	Cases []Case
	// Out is the directory the images are written to, below the work
	// directory the plan is written to.
	Out string
}

// Rendered is a case with the image the runtime drew.
type Rendered struct {
	Case
	PNG    []byte
	SHA256 []byte
}

// Prepare derives the cases and generates the Flutter project. It starts
// nothing and writes nothing. out is the absolute directory the images go
// to.
func Prepare(o Options, out string) (*Plan, error) {
	res := o.Result
	if res == nil || res.Project == nil || res.App == nil || res.Diagnostics.HasErrors() {
		return nil, fmt.Errorf("plxgolden: the project did not compile")
	}
	if o.Limits.IsZero() {
		o.Limits = limits.Defaults()
	}
	if len(o.Frames.Frames) == 0 {
		fs, err := DefaultFrames()
		if err != nil {
			return nil, err
		}
		o.Frames = fs
	}
	app := res.Project.App.Doc
	if app == nil {
		return nil, fmt.Errorf("plxgolden: the project has no app document")
	}
	matrix, err := BuildMatrix(app, o.Frames)
	if err != nil {
		return nil, err
	}
	var pages []Page
	dataSources := map[string]bool{}
	for _, d := range app.DataSources {
		dataSources[d.Name] = true
	}
	for _, pl := range res.Project.Plugins {
		if pl.Doc == nil || (o.Plugin != "" && pl.Doc.Key != o.Plugin) {
			continue
		}
		for _, d := range pl.Doc.DataSources {
			dataSources[d.Name] = true
		}
		for _, pg := range pl.Pages {
			if pg.Doc == nil || pg.Doc.Route == "" {
				continue
			}
			pages = append(pages, Page{Plugin: pl.Doc.Key, Route: pg.Doc.Route, Params: pageParams(pg.Doc.Params)})
			for _, d := range pg.Doc.DataSources {
				dataSources[d.Name] = true
			}
		}
	}
	if len(pages) == 0 {
		return nil, plxerr.New(plxerr.GoldenHarnessFailed, "the project has no page with a route to render")
	}
	cases := Cases(matrix, pages)
	perPlugin := map[string]int{}
	for _, c := range cases {
		perPlugin[c.Plugin]++
	}
	max := o.Limits.Get(limits.GoldenCasesPerRun)
	for _, key := range slices.Sorted(mapKeys(perPlugin)) {
		if int64(perPlugin[key]) > max {
			return nil, plxerr.New(plxerr.LimitExceeded, "plugin %q has %d golden cases; golden.casesPerRun allows %d; narrow goldens.matrix", key, perPlugin[key], max)
		}
	}
	key, err := runkey.New(o.Entropy)
	if err != nil {
		return nil, fmt.Errorf("plxgolden: %w", err)
	}
	rel, err := plxtest.BuildRelease(res, key)
	if err != nil {
		return nil, fmt.Errorf("plxgolden: %w", err)
	}
	in := plxtest.GoldenInput{Release: rel, Natives: res.Natives, Runtime: o.Runtime, DataSources: slices.Sorted(mapKeys(dataSources)), Out: out, Fonts: o.Fonts}
	for _, c := range cases {
		in.Cases = append(in.Cases, plxtest.GoldenCase{
			ID: c.ID, Route: c.Route, Width: c.Frame.Width, Height: c.Frame.Height, Ratio: c.Frame.PixelRatio,
			TextScale: c.Scale, Dark: c.Theme == ThemeDark, Locale: c.Locale, Params: c.Params,
		})
	}
	return &Plan{Files: plxtest.GenerateGolden(in), Cases: cases, Out: out}, nil
}

// pageParams are the inputs a page is shown with: each parameter's mock,
// else its default, so a page that needs an input still renders.
func pageParams(params []schema.Param) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	for _, p := range params {
		switch {
		case len(p.Mock) > 0:
			out[p.Name] = p.Mock
		case len(p.Default) > 0:
			out[p.Name] = p.Default
		}
	}
	return out
}

func mapKeys[K comparable, V any](m map[K]V) func(func(K) bool) {
	return func(yield func(K) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}

// Execute writes the plan below work, runs Flutter and collects the images.
// A case that failed to render is reported with the harness's message.
func Execute(ctx context.Context, p *Plan, work, flutter string, log io.Writer, lim limits.Set) ([]Rendered, error) {
	if lim.IsZero() {
		lim = limits.Defaults()
	}
	pl := &plxtest.Plan{Files: p.Files}
	if err := pl.Write(work); err != nil {
		return nil, fmt.Errorf("plxgolden: %w", err)
	}
	if err := os.MkdirAll(p.Out, 0o750); err != nil {
		return nil, fmt.Errorf("plxgolden: %w", err)
	}
	report, err := plxtest.Flutter(ctx, flutter, work, log)
	if err != nil {
		return nil, plxerr.New(plxerr.GoldenHarnessFailed, "%v", err)
	}
	named := make([]plxtest.Case, len(p.Cases))
	for i, c := range p.Cases {
		named[i] = plxtest.Case{File: plxtest.GoldenGroup, Name: c.ID}
	}
	results, err := plxtest.ParseReport(bytes.NewReader(report), named)
	if err != nil {
		return nil, plxerr.New(plxerr.GoldenHarnessFailed, "%v", err)
	}
	var failed []string
	for i, r := range results {
		if r.Status != plxtest.Passed {
			failed = append(failed, fmt.Sprintf("%s: %s", p.Cases[i].ID, r.Message))
		}
	}
	if len(failed) > 0 {
		shown := failed[:min(len(failed), 5)]
		return nil, plxerr.New(plxerr.GoldenHarnessFailed, "%d of %d cases did not render: %s", len(failed), len(p.Cases), strings.Join(shown, "; "))
	}
	maxImage := lim.Get(limits.GoldenImageBytes)
	out := make([]Rendered, len(p.Cases))
	for i, c := range p.Cases {
		data, err := os.ReadFile(filepath.Join(p.Out, fmt.Sprintf("%04d.png", i)))
		if err != nil {
			return nil, plxerr.New(plxerr.GoldenHarnessFailed, "case %s wrote no image: %v", c.ID, err)
		}
		if int64(len(data)) > maxImage {
			return nil, plxerr.New(plxerr.LimitExceeded, "the image of case %s has %d bytes; golden.imageBytes allows %d", c.ID, len(data), maxImage)
		}
		sum := sha256.Sum256(data)
		out[i] = Rendered{Case: c, PNG: data, SHA256: sum[:]}
	}
	return out, nil
}

// Status is how a rendered case compares with its baseline.
type Status string

// The verdicts.
const (
	Match   Status = "match"
	New     Status = "new"
	Changed Status = "changed"
)

// Verdict is the comparison of one case.
type Verdict struct {
	Rendered
	Status Status
}

// Compare compares the rendered cases with the approved image hashes by
// case ID. Images are rendered deterministically on the canonical renderer,
// so they are compared byte for byte (ADR-0070).
func Compare(rendered []Rendered, baselines map[string][]byte) []Verdict {
	out := make([]Verdict, len(rendered))
	for i, r := range rendered {
		v := Verdict{Rendered: r, Status: Match}
		if b, ok := baselines[r.ID]; !ok {
			v.Status = New
		} else if !bytes.Equal(b, r.SHA256) {
			v.Status = Changed
		}
		out[i] = v
	}
	return out
}

// ForPlugin returns the verdicts of one plugin.
func ForPlugin(vs []Verdict, plugin string) []Verdict {
	var out []Verdict
	for _, v := range vs {
		if v.Plugin == plugin {
			out = append(out, v)
		}
	}
	return out
}

// Plugins lists the plugin keys of the verdicts, sorted.
func Plugins(vs []Verdict) []string {
	seen := map[string]bool{}
	for _, v := range vs {
		seen[v.Plugin] = true
	}
	return slices.Sorted(mapKeys(seen))
}

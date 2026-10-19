// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Package plxgolden is the client side of golden (visual regression) tests
// (TST-003, ADR-0070): the matrix of cases, the generated Flutter test
// project that renders them with the real runtime, the vendored fonts and
// the comparison with the server's baselines.
package plxgolden

import (
	_ "embed" // the frame table is a file beside this package
	"encoding/json"
	"fmt"
	"slices"
	"strconv"

	"github.com/nightCode42/plux3/backend/internal/l10n/pseudo"
	"github.com/nightCode42/plux3/backend/internal/schema"
)

// framesJSON is a copy of schema/goldens/frames.json, the table of ADR-0070;
// a test keeps the two equal.
//
//go:embed frames.json
var framesJSON []byte

// Frame is a device frame: a logical size and a pixel ratio.
type Frame struct {
	ID         string  `json:"id"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	PixelRatio float64 `json:"pixelRatio"`
}

// FrameSet is the table of frames and the default text scales.
type FrameSet struct {
	Frames     []Frame   `json:"frames"`
	TextScales []float64 `json:"textScales"`
}

// DefaultFrames returns the frames of ADR-0070.
func DefaultFrames() (FrameSet, error) {
	var fs FrameSet
	if err := json.Unmarshal(framesJSON, &fs); err != nil {
		return FrameSet{}, fmt.Errorf("plxgolden: frames.json: %w", err)
	}
	return fs, nil
}

// The themes a run renders.
const (
	ThemeLight = "light"
	ThemeDark  = "dark"
)

// Matrix is what a run renders: the cross product of its lists for every
// page.
type Matrix struct {
	Frames     []Frame
	Themes     []string
	TextScales []float64
	Locales    []string
}

// Page is a page that can be shown: it has a route.
type Page struct {
	Plugin string
	Route  string
	// Params are the values the page is shown with: each parameter's mock,
	// else its default.
	Params map[string]json.RawMessage
}

// Case is one screen of a run.
type Case struct {
	// ID is stable across runs: route, frame, locale, theme and text scale.
	ID     string
	Plugin string
	Route  string
	Frame  Frame
	Locale string
	Theme  string
	Scale  float64
	Params map[string]json.RawMessage
}

// BuildMatrix derives the matrix from the app: every frame, both themes,
// the text scales 1.0 and 2.0, the app's locales and, when it turns them
// on, the pseudo-locales; goldens.matrix narrows each list (ADR-0070).
func BuildMatrix(app *schema.AppDocument, set FrameSet) (Matrix, error) {
	m := Matrix{Frames: set.Frames, Themes: []string{ThemeLight, ThemeDark}, TextScales: set.TextScales}
	m.Locales = slices.Clone(app.SupportedLocales)
	if len(m.Locales) == 0 && app.DefaultLocale != "" {
		m.Locales = []string{app.DefaultLocale}
	}
	if l := app.Localisation; l != nil && l.PseudoLocales != nil && *l.PseudoLocales {
		m.Locales = append(m.Locales, pseudo.Tags()...)
	}
	g := app.Goldens
	if g == nil || g.Matrix == nil {
		return m, nil
	}
	n := g.Matrix
	if len(n.Frames) > 0 {
		m.Frames = nil
		for _, id := range n.Frames {
			i := slices.IndexFunc(set.Frames, func(f Frame) bool { return f.ID == id })
			if i < 0 {
				return Matrix{}, fmt.Errorf("goldens.matrix.frames: unknown frame %q", id)
			}
			m.Frames = append(m.Frames, set.Frames[i])
		}
	}
	if len(n.Themes) > 0 {
		m.Themes = nil
		for _, t := range n.Themes {
			m.Themes = append(m.Themes, string(t))
		}
	}
	if len(n.TextScales) > 0 {
		m.TextScales = slices.Clone(n.TextScales)
	}
	if len(n.Locales) > 0 {
		m.Locales = slices.Clone(n.Locales)
	}
	return m, nil
}

// Cases lists the cases of the pages in a fixed order: by page, frame,
// locale, theme and text scale.
func Cases(m Matrix, pages []Page) []Case {
	var out []Case
	for _, p := range pages {
		for _, f := range m.Frames {
			for _, l := range m.Locales {
				for _, t := range m.Themes {
					for _, s := range m.TextScales {
						out = append(out, Case{
							ID:     p.Route + "/" + f.ID + "/" + l + "/" + t + "/x" + strconv.FormatFloat(s, 'f', -1, 64),
							Plugin: p.Plugin, Route: p.Route, Params: p.Params, Frame: f, Locale: l, Theme: t, Scale: s,
						})
					}
				}
			}
		}
	}
	return out
}

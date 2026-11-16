// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"testing"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// An alias that names no token, or loops, is reported where it is written;
// one that resolves, inside a composite too, is not [THM-001].
func TestTokenAliasesResolveOrAreReported(t *testing.T) {
	m := fixture(t)
	edit(t, m, themeFile, func(doc map[string]any) {
		tokens := at(t, doc, "tokens")
		tokens["alias"] = map[string]any{
			"$type": "color",
			"ok":    map[string]any{"$value": "{alias.base}"},
			"base":  map[string]any{"$value": "#4f46e5"},
			"gone":  map[string]any{"$value": "{color.nothing}", "$extensions": map[string]any{"dev.plux.dark": "{alias.base}"}},
			"loopA": map[string]any{"$value": "{alias.loopB}"},
			"loopB": map[string]any{"$value": "{alias.loopA}"},
			"dark":  map[string]any{"$value": "#000000", "$extensions": map[string]any{"dev.plux.dark": "{alias.none}"}},
		}
		tokens["text"] = map[string]any{"$type": "typography", "body": map[string]any{
			"$value": map[string]any{"fontFamily": "{font.none}", "fontSize": 16},
		}}
	})
	res := compileFS(m)
	wantDiag(t, res, plxerr.UnresolvedReference, themeFile, "/tokens/alias/gone/$value")
	wantDiag(t, res, plxerr.UnresolvedReference, themeFile, "/tokens/alias/loopA/$value")
	wantDiag(t, res, plxerr.UnresolvedReference, themeFile, "/tokens/alias/dark/$extensions/dev.plux.dark")
	wantDiag(t, res, plxerr.UnresolvedReference, themeFile, "/tokens/text/body/$value/fontFamily")
	for _, d := range res.Diagnostics {
		if d.File == themeFile && (d.Path == "/tokens/alias/ok/$value" || d.Path == "/tokens/alias/gone/$extensions/dev.plux.dark") {
			t.Errorf("a resolving alias is reported: %s", d)
		}
	}
}

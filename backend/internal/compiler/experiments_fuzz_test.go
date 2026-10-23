// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"testing"
	"testing/fstest"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// experimentFiles are the documents FuzzCompileExperiments replaces.
var experimentFiles = []string{expPage, expCTA, expHero, expSegment, expFlag, expPolicy}

// FuzzCompileExperiments replaces one document of the experiments project
// with arbitrary bytes: the compiler reports, never panics, and produces no
// bundle from an invalid project; a promotion does the same.
// Verifies: ABT-001, ABT-005, CMP-052.
func FuzzCompileExperiments(f *testing.F) {
	base := experimentsProject(f)
	for i, file := range experimentFiles {
		f.Add(uint8(i), base[file].Data)
	}
	f.Add(uint8(0), []byte(`{"id":"01f0c450-6c00-7000-8000-000000000710","key":"home","kind":"page","pageKind":"screen","schemaVersion":"1.0.0","root":{"id":"01f0c450-6c00-7000-8000-000000000711","type":"Variant","experiment":"heroLayout","variants":{"wide":{"id":"01f0c450-6c00-7000-8000-000000000712","type":"Variant"}}}}`))
	f.Add(uint8(3), []byte(`{"kind":"segment","rule":{"all":[{"all":[{"any":[]}]}]}}`))
	f.Fuzz(func(t *testing.T, which uint8, data []byte) {
		m := fstest.MapFS{}
		for k, v := range base {
			m[k] = v
		}
		m[experimentFiles[int(which)%len(experimentFiles)]] = &fstest.MapFile{Data: data}
		res := compileFS(m)
		for _, d := range res.Diagnostics {
			if d.Code == plxerr.InternalCompilerError {
				t.Fatalf("internal error: %s", d)
			}
		}
		if res.Diagnostics.HasErrors() && res.App != nil {
			t.Fatal("bundles despite errors")
		}
		if out, diags := PromoteWinner(m, "heroLayout", "wide"); out == nil && !diags.HasErrors() {
			t.Fatal("a promotion gave neither a project nor an error")
		}
	})
}

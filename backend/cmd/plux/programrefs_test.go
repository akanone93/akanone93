// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/bundle"
	"github.com/nightCode42/plux3/backend/internal/bundle/fbs"
)

// valueExprs adds the program IDs of every expression in v to ids.
func valueExprs(v *fbs.Value, ids map[uint64]bool) {
	if v.Kind() == fbs.ValueKindExpr {
		ids[uint64(v.I())] = true //nolint:gosec // G115: the ID's bits are stored as they are.
	}
	for j := 0; j < v.ItemsLength(); j++ {
		var it fbs.Value
		if v.Items(&it, j) {
			valueExprs(&it, ids)
		}
	}
	for j := 0; j < v.EntriesLength(); j++ {
		var e fbs.Entry
		var ev fbs.Value
		if v.Entries(&e, j) && e.Value(&ev) != nil {
			valueExprs(&ev, ids)
		}
	}
}

// Every program a bundle's styles refer to — the expressions inside value
// objects, such as the steps of a status timeline — is in the bundle's
// programs section: the runtime finds a program by its content ID and
// refuses the bundle otherwise (PLX-3040). The reference apps are the
// projects that use such objects.
//
// Verifies: CMP-003, BND-004.
func TestStyleProgramsAreInTheBundle(t *testing.T) {
	t.Parallel()
	for _, fixture := range []string{"plux_express", "plux_bank"} {
		t.Run(fixture, func(t *testing.T) {
			t.Parallel()
			out := t.TempDir()
			if code, _, stderr := execute("build", "-o", out, filepath.Join(documents, fixture)); code != exitOK {
				t.Fatalf("build = %d, stderr:\n%s", code, stderr)
			}
			files, err := filepath.Glob(filepath.Join(out, "*", "*.pxb"))
			if err != nil {
				t.Fatal(err)
			}
			top, err := filepath.Glob(filepath.Join(out, "*.pxb"))
			if err != nil {
				t.Fatal(err)
			}
			files = append(files, top...)
			if len(files) == 0 {
				t.Fatal("the build made no bundles")
			}
			checked := 0
			for _, f := range files {
				data, err := os.ReadFile(f)
				if err != nil {
					t.Fatal(err)
				}
				c, err := bundle.ReadStructure(data)
				if err != nil {
					t.Fatal(err)
				}
				have, want := map[uint64]bool{}, map[uint64]bool{}
				for _, s := range c.Sections {
					switch s.Kind {
					case bundle.SectionPXL:
						ps := fbs.GetRootAsPrograms(s.Data, 0)
						for j := 0; j < ps.ProgramsLength(); j++ {
							var p fbs.Program
							if ps.Programs(&p, j) {
								have[p.Id()] = true
							}
						}
					case bundle.SectionStyles:
						ss := fbs.GetRootAsStyles(s.Data, 0)
						for j := 0; j < ss.StylesLength(); j++ {
							var st fbs.Style
							var v fbs.Value
							if ss.Styles(&st, j) && st.Value(&v) != nil {
								valueExprs(&v, want)
							}
						}
					}
				}
				for id := range want {
					checked++
					if !have[id] {
						t.Errorf("%s: a style refers to program %d, which the bundle lacks", filepath.Base(f), id)
					}
				}
			}
			if checked == 0 && fixture == "plux_express" {
				t.Error("no style holds an expression: the test checks nothing")
			}
		})
	}
}

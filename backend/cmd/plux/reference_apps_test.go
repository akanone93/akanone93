// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// referenceGoldens is where the reference apps' bundles are pinned; the
// runtime's tests of their P9 scenarios read them.
var referenceGoldens = filepath.Join("..", "..", "..", "schema", "testdata", "bundles")

// buildReference builds a reference fixture offline and returns the report
// and the directory the bundles are in.
func buildReference(t *testing.T, fixture string) (report, string) {
	t.Helper()
	out := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := run([]string{"build", "--json", "-o", out, filepath.Join(documents, fixture)}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("build = %d, stdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	var rep report
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	return rep, out
}

// The reference apps' P9 scenarios compile and their bundles are pinned
// byte for byte: Plux Express carries a Variant node on the catalogue
// banner and a route guard on a flag; Plux Bank carries the secure
// transfer page, its fallback page and the release policy that asks two
// approvers from distinct teams. A second build is identical.
//
// Verifies: DX-004, ABT-001, NAV-009, GOV-020, CMP-002, QA-003.
func TestReferenceAppsCompileTheirP9Scenarios_DX_004(t *testing.T) {
	t.Parallel()
	for _, fixture := range []string{"plux_express", "plux_bank"} {
		t.Run(fixture, func(t *testing.T) {
			t.Parallel()
			rep, out := buildReference(t, fixture)
			if !rep.OK || rep.Errors != 0 {
				t.Fatalf("report: %+v", rep)
			}
			_, again := buildReference(t, fixture)
			for _, b := range rep.Bundles {
				rel, err := filepath.Rel(out, b.File)
				if err != nil {
					t.Fatal(err)
				}
				got := readFile(t, filepath.Join(out, rel))
				if !bytes.Equal(got, readFile(t, filepath.Join(again, rel))) {
					t.Errorf("%s: two builds of the same project differ", rel)
				}
				golden := filepath.Join(referenceGoldens, fixture, b.Key+".pxb")
				if *updateGolden {
					if err := os.MkdirAll(filepath.Dir(golden), 0o750); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(golden, got, 0o600); err != nil {
						t.Fatal(err)
					}
				}
				if !bytes.Equal(got, readFile(t, golden)) {
					t.Errorf("%s differs from the golden %s; run the test with -update and review", rel, golden)
				}
			}
			if fixture == "plux_express" && !hasFeature(rep, "shop", "experiments.variant.v1") {
				t.Errorf("the shop bundle lacks the variant feature: %+v", rep.Bundles)
			}
		})
	}
}

// readFile reads a file of a test.
func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // G304: a path under the repository or a test directory.
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// hasFeature reports whether the bundle of a key requires a feature.
func hasFeature(rep report, key, feature string) bool {
	for _, b := range rep.Bundles {
		if b.Key == key && slices.Contains(b.Features, feature) {
			return true
		}
	}
	return false
}

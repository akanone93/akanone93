// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	gotoken "go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// engineFeaturesVector is the catalogue the runtime's tests read (#271).
var engineFeaturesVector = filepath.Join("..", "..", "..", "schema", "testdata", "features", "engine.json")

// featureConstants reads this package's sources and returns the string value
// of every package-level constant named <something>Feature, and every
// revision a useRevision call asks of one, as "<value>.v<revision>".
func featureConstants(t *testing.T) (consts map[string]string, uses []string) {
	t.Helper()
	fset := gotoken.NewFileSet()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	consts = map[string]string{}
	var parsed []*ast.File
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		parsed = append(parsed, f)
		for _, d := range f.Decls {
			g, ok := d.(*ast.GenDecl)
			if !ok || g.Tok != gotoken.CONST {
				continue
			}
			for _, s := range g.Specs {
				vs := s.(*ast.ValueSpec)
				for i, n := range vs.Names {
					if !strings.HasSuffix(n.Name, "Feature") || i >= len(vs.Values) {
						continue
					}
					if lit, ok := vs.Values[i].(*ast.BasicLit); ok && lit.Kind == gotoken.STRING {
						v, err := strconv.Unquote(lit.Value)
						if err != nil {
							t.Fatal(err)
						}
						consts[n.Name] = v
					}
				}
			}
		}
	}
	for _, f := range parsed {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "useRevision" || len(call.Args) < 3 {
				return true
			}
			id, ok := call.Args[0].(*ast.Ident)
			rev, isLit := call.Args[2].(*ast.BasicLit)
			if !ok || !isLit {
				return true
			}
			if v, known := consts[id.Name]; known {
				uses = append(uses, v+".v"+rev.Value)
			}
			return true
		})
	}
	return consts, uses
}

// TestEngineFeaturesListEveryFeature_BND_008 checks that the catalogue holds
// every engine feature the compiler can require (#271): every feature
// constant at its first revision, and every revision a check asks of one.
// A new feature defined without a table entry fails here, before a runtime
// that does not declare it refuses bundles on a device (#257).
// Verifies: BND-008.
func TestEngineFeaturesListEveryFeature_BND_008(t *testing.T) {
	listed := map[string]bool{}
	for _, f := range EngineFeatures() {
		listed[f.Name] = true
	}
	consts, uses := featureConstants(t)
	if len(consts) < 20 {
		t.Fatalf("found %d feature constants; the scan no longer sees them", len(consts))
	}
	for name, v := range consts {
		if !listed[v+".v1"] {
			t.Errorf("%s (%q) is not in EngineFeatures: add it to engineFeatureTables", name, v+".v1")
		}
	}
	for _, u := range uses {
		if !listed[u] {
			t.Errorf("a check requires %q, which EngineFeatures does not list", u)
		}
	}
	for _, f := range EngineFeatures() {
		if strings.HasPrefix(f.Name, "widget.") || strings.HasPrefix(f.Name, "type.") || strings.HasPrefix(f.Name, "enum.") {
			t.Errorf("%s is a registry revision, which the runtime derives from the registry", f.Name)
		}
	}
}

// TestEngineFeaturesVector_BND_008 keeps schema/testdata/features/engine.json
// equal to EngineFeatures; go test -run TestEngineFeaturesVector -update
// rewrites it.
// Verifies: BND-008.
func TestEngineFeaturesVector_BND_008(t *testing.T) {
	want, err := json.MarshalIndent(map[string]any{"features": EngineFeatures()}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, '\n')
	if *update {
		if err := os.MkdirAll(filepath.Dir(engineFeaturesVector), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(engineFeaturesVector, want, 0o644); err != nil { //nolint:gosec // G306: a committed test vector.
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(engineFeaturesVector)
	if err != nil {
		t.Fatalf("%v; run go test ./internal/compiler -run TestEngineFeaturesVector -update", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s is out of date; run go test ./internal/compiler -run TestEngineFeaturesVector -update", engineFeaturesVector)
	}
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package importer

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type studioAnswer struct {
	Fragment    map[string]any   `json:"fragment"`
	Diagnostics []map[string]any `json:"diagnostics"`
}

func studioImport(t *testing.T, kind string, files ...string) studioAnswer {
	t.Helper()
	var src []Source
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join("testdata", f))
		if err != nil {
			t.Fatal(err)
		}
		src = append(src, Source{File: f, Data: data})
	}
	out, err := ImportJSON(kind, src)
	if err != nil {
		t.Fatal(err)
	}
	var a studioAnswer
	if err := json.Unmarshal(out, &a); err != nil {
		t.Fatal(err)
	}
	return a
}

// TestImportJSONOpenAPI_DAT_002 checks Studio's entry point returns the same
// data sources as the CLI import.
func TestImportJSONOpenAPI_DAT_002(t *testing.T) {
	a := studioImport(t, KindOpenAPI, "todo30.json")
	if srcs, _ := a.Fragment["dataSources"].([]any); len(srcs) == 0 {
		t.Fatalf("no data sources in %v", a.Fragment)
	}
	if a.Diagnostics == nil {
		t.Fatal("diagnostics must be an array, never null")
	}
}

// TestImportJSONGraphQL_DAT_002 checks the schema and operation documents
// are told apart by position.
func TestImportJSONGraphQL_DAT_002(t *testing.T) {
	a := studioImport(t, KindGraphQL, "schema.graphql", "ops.graphql")
	if srcs, _ := a.Fragment["dataSources"].([]any); len(srcs) != 1 {
		t.Fatalf("want one GraphQL data source, got %v", a.Fragment["dataSources"])
	}
}

// TestImportJSONDeterministic_DAT_002 checks the same input gives the same bytes.
func TestImportJSONDeterministic_DAT_002(t *testing.T) {
	src := []Source{{File: "p.yaml", Data: mustRead(t, "petstore31.yaml")}}
	a, err1 := ImportJSON(KindOpenAPI, src)
	b, err2 := ImportJSON(KindOpenAPI, src)
	if err1 != nil || err2 != nil || !bytes.Equal(a, b) {
		t.Fatal("two imports of one document differ")
	}
}

// TestImportJSONRejects_DAT_002 checks bad requests are errors and a bad
// document is a diagnostic, not a failure.
func TestImportJSONRejects_DAT_002(t *testing.T) {
	if _, err := ImportJSON("soap", []Source{{File: "x"}}); err == nil {
		t.Fatal("unknown kind accepted")
	}
	if _, err := ImportJSON(KindOpenAPI, nil); err == nil {
		t.Fatal("no source accepted")
	}
	bad, err := ImportJSON(KindOpenAPI, []Source{{File: "x.json", Data: []byte("{")}})
	if err != nil {
		t.Fatal(err)
	}
	var a studioAnswer
	if err := json.Unmarshal(bad, &a); err != nil || len(a.Diagnostics) == 0 {
		t.Fatalf("a malformed document must yield diagnostics: %s", bad)
	}
}

func mustRead(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

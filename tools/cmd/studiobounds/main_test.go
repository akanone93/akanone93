// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"slices"
	"testing"
	"testing/fstest"
)

func run(t *testing.T, files map[string]string) []string {
	t.Helper()
	m := fstest.MapFS{}
	for k, v := range files {
		m[k] = &fstest.MapFile{Data: []byte(v)}
	}
	vs, err := scan(m)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, v := range vs {
		got = append(got, v.String())
	}
	return got
}

func TestRules(t *testing.T) {
	tests := []struct {
		name string
		file string
		src  string
		want string
	}{
		{"package imports web", "packages/ui/src/a.ts", "import x from \"@plux/web\";", "packages/ui/src/a.ts:1: packages-no-apps: @plux/web"},
		{"package imports bff", "packages/ui/src/a.ts", "export * from '@plux/bff/x';", "packages/ui/src/a.ts:1: packages-no-apps: @plux/bff/x"},
		{"package relative into apps", "packages/ui/src/a/b.ts", "import x from \"../../../../apps/web/src/y\";", "packages/ui/src/a/b.ts:1: packages-no-apps: ../../../../apps/web/src/y"},
		{"canvas imports react", "packages/canvas/src/a.tsx", "import { useState } from \"react\";", "packages/canvas/src/a.tsx:1: pure-no-ui: react"},
		{"layout imports ui", "packages/layout/src/a.ts", "const m = await import(\"@plux/ui\");", "packages/layout/src/a.ts:1: pure-no-ui: @plux/ui"},
		{"layout imports base-ui", "packages/layout/src/a.ts", "import \"@base-ui-components/react\";", "packages/layout/src/a.ts:1: pure-no-ui: @base-ui-components/react"},
		{"bff imports react", "apps/bff/src/a.ts", "import r from \"react\";", "apps/bff/src/a.ts:1: bff-no-ui: react"},
		{"bff imports canvas", "apps/bff/src/a.ts", "import c from \"@plux/canvas\";", "apps/bff/src/a.ts:1: bff-no-ui: @plux/canvas"},
		{"web imports pg", "apps/web/src/a.ts", "import pg from \"pg\";", "apps/web/src/a.ts:1: no-direct-storage: pg"},
		{"bff imports aws", "apps/bff/src/a.ts", "import s from \"@aws-sdk/client-s3\";", "apps/bff/src/a.ts:1: no-direct-storage: @aws-sdk/client-s3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, map[string]string{tt.file: tt.src})
			if !slices.Equal(got, []string{tt.want}) {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCleanTree(t *testing.T) {
	got := run(t, map[string]string{
		"apps/web/src/a.tsx":                  "import r from \"react\";\nimport { c } from \"@plux/canvas\";\nimport p from \"./pg\";",
		"apps/bff/src/a.ts":                   "import { x } from \"@plux/schema\";",
		"packages/canvas/src/a.ts":            "import { y } from \"./b\";\nimport { z } from \"../c\";",
		"packages/ui/src/a.tsx":               "import r from \"react\";\nimport { l } from \"@plux/layout\";",
		"packages/ui/node_modules/q/src/a.ts": "import x from \"pg\";",
		"packages/ui/src/node_modules/m/a.ts": "import x from \"pg\";",
		"packages/ui/src/readme.md":           "import x from \"pg\";",
	})
	if len(got) != 0 {
		t.Fatalf("unexpected violations: %q", got)
	}
}

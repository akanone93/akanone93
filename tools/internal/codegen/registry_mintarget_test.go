// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package codegen

import (
	"slices"
	"sort"
	"strings"
	"testing"
)

// TestRegistryDartMinTargetTypes checks that registry.g.dart lists exactly
// the widgets whose descriptor sets minTarget, sorted by name.
//
// Verifies: A11Y-004.
func TestRegistryDartMinTargetTypes(t *testing.T) {
	t.Parallel()
	r := loadRegistry(t)
	var want []string
	for _, w := range r.Widgets {
		if w.Accessibility.MinTarget {
			want = append(want, w.Type)
		}
	}
	if len(want) == 0 {
		t.Fatal("no widget sets minTarget")
	}
	sort.Strings(want)
	files, err := RegistryFiles(r)
	if err != nil {
		t.Fatal(err)
	}
	const open = "const Set<String> minTargetTypes = {\n"
	var got []string
	for _, f := range files {
		if f.Path != registryDartPath {
			continue
		}
		text := string(f.Content)
		start := strings.Index(text, open)
		if start < 0 {
			t.Fatal("registry.g.dart lacks minTargetTypes")
		}
		body := text[start+len(open):]
		body = body[:strings.Index(body, "};")]
		for _, l := range strings.Split(strings.TrimSpace(body), "\n") {
			got = append(got, strings.Trim(strings.TrimSpace(l), ",'"))
		}
	}
	if !slices.Equal(got, want) {
		t.Errorf("minTargetTypes = %v, want %v", got, want)
	}
}

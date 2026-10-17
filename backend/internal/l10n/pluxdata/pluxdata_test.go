// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package pluxdata

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"
)

// Verifies: I18N-002, ACT-020.
func TestStrings(t *testing.T) {
	var enKeys []string
	for _, tag := range []string{"en", "de", "ar", "am"} {
		b, err := Strings(tag)
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]string
		if err := json.Unmarshal(b, &doc); err != nil {
			t.Fatal(err)
		}
		keys := make([]string, 0, len(doc))
		for k, v := range doc {
			if v == "" {
				t.Errorf("%s: %s is empty", tag, k)
			}
			keys = append(keys, k)
		}
		slices.Sort(keys)
		if tag == "en" {
			enKeys = keys
		} else if !slices.Equal(keys, enKeys) {
			t.Errorf("%s: keys differ from en", tag)
		}
	}
	if _, err := Strings("xx"); !errors.Is(err, ErrNoStrings) {
		t.Errorf("xx: %v", err)
	}
	if _, err := Strings("../x"); !errors.Is(err, ErrNoStrings) {
		t.Errorf("path: %v", err)
	}
	if got := Locales(); len(got) != 4 {
		t.Errorf("locales: %v", got)
	}
}

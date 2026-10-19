// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package flutterdata

import (
	"encoding/json"
	"errors"
	"testing"
)

// Verifies: I18N-002.
func TestStrings(t *testing.T) {
	b, err := Strings("de")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]map[string]string
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["material"]["cutButtonLabel"] == "" || doc["widgets"]["copyButtonLabel"] == "" {
		t.Errorf("de lacks the Material or widgets strings")
	}
	if _, err := Strings("xx"); !errors.Is(err, ErrNoStrings) {
		t.Errorf("xx: %v", err)
	}
	if _, err := Strings("../x"); !errors.Is(err, ErrNoStrings) {
		t.Errorf("path: %v", err)
	}
	if got := Locales(); len(got) < 50 {
		t.Errorf("%d locales", len(got))
	}
}

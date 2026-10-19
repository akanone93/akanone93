// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package phone

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/pxl/regex"
)

var formatsPath = filepath.Join("..", "..", "..", "..", "schema", "testdata", "phone", "formats.json")

// formatGolden is the shared golden of formatted example numbers.
type formatGolden struct {
	Description string `json:"description"`
	Results     string `json:"results"`
}

// digitsOf keeps the ASCII digits of s.
func digitsOf(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
}

// TestFormatExamples formats every example number of the metadata in its
// own region and from an unrelated one, checks that the international form loses
// and adds no digit (a national form may add digits, as Argentina's 15), and compares all results with the golden the Dart runtime checks
// too.
//
// Verifies: PXL-006.
func TestFormatExamples(t *testing.T) {
	m := loadMetadata(t)
	var results strings.Builder
	for _, x := range m.Territories {
		if x.ID == "001" {
			continue
		}
		for _, ty := range x.Types {
			if ty.Example == "" {
				continue
			}
			cc := strconv.Itoa(x.CountryCode)
			national, intl := Format(ty.Example, x.ID), Format("+"+cc+ty.Example, "ZZ")
			if got, want := digitsOf(intl), cc+ty.Example; got != want {
				t.Errorf("%s %s: international %q has digits %s, want %s", x.ID, ty.Type, intl, got, want)
			}
			results.WriteString(national + "|" + intl + "\n")
		}
	}
	got := formatGolden{
		Description: "format.phone results over the example numbers of schema/pxl/phone.json, shared by Go and Dart: for each territory in order (not 001), each type with an example, the national form in its region and the international form, separated by |, one per line. Written by the Go test with -update.",
		Results:     results.String(),
	}
	if *update {
		out, _ := json.MarshalIndent(got, "", "  ")
		if err := os.WriteFile(formatsPath, append(out, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	data, err := os.ReadFile(formatsPath)
	if err != nil {
		t.Fatal(err)
	}
	var want formatGolden
	if err := json.Unmarshal(data, &want); err != nil {
		t.Fatal(err)
	}
	if want.Results != got.Results {
		t.Error("results differ from schema/testdata/phone/formats.json; run with -update and review")
	}
}

// TestFormatTablePatternsCompile checks that every pattern of the format
// table is in the pxl.regex.v1 subset.
//
// Verifies: PXL-006.
func TestFormatTablePatternsCompile(t *testing.T) {
	for line := range strings.SplitSeq(strings.TrimSpace(formatTable), "\n") {
		f := strings.Split(line, " ")
		for i := 1; i+1 < len(f); i += 4 {
			for _, s := range []string{f[i], f[i+1]} {
				if s == "~" {
					continue
				}
				if _, err := regex.Compile(s, regex.Trusted); err != nil {
					t.Errorf("%s %q: %v", f[0], s, err)
				}
			}
		}
	}
}

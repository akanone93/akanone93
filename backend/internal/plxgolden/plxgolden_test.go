// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package plxgolden_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/plxgolden"
	"github.com/nightCode42/plux3/backend/internal/schema"
)

const fontsDir = "../../../schema/goldens/fonts"

func frames(t *testing.T) plxgolden.FrameSet {
	t.Helper()
	fs, err := plxgolden.DefaultFrames()
	if err != nil {
		t.Fatal(err)
	}
	return fs
}

// Verifies: TST-003.
// The embedded frame table is the file ADR-0070 names, and holds its four
// frames and the two text scales.
func TestFramesMatchTheSchemaFile(t *testing.T) {
	t.Parallel()
	want, err := os.ReadFile("../../../schema/goldens/frames.json")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("frames.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Error("internal/plxgolden/frames.json differs from schema/goldens/frames.json; copy the schema file")
	}
	fs := frames(t)
	var ids []string
	for _, f := range fs.Frames {
		ids = append(ids, f.ID)
	}
	if strings.Join(ids, " ") != "small-360x640 pixel-8 iphone-15 tablet-820x1180" {
		t.Errorf("frames = %v", ids)
	}
	if fs.Frames[1].Width != 412 || fs.Frames[1].Height != 915 || fs.Frames[1].PixelRatio != 2.625 {
		t.Errorf("Pixel 8 = %+v", fs.Frames[1])
	}
	if !slices.Equal(fs.TextScales, []float64{1, 2}) {
		t.Errorf("text scales = %v", fs.TextScales)
	}
}

// Verifies: TST-003, A11Y-005, I18N-010.
// The matrix is every frame, both themes, the text scales 1 and 2 and the
// app's locales, with the pseudo-locales when the app turns them on; the
// cases are in a fixed order with stable IDs.
func TestMatrixDefaultsAndCases(t *testing.T) {
	t.Parallel()
	yes := true
	app := &schema.AppDocument{SupportedLocales: []string{"en", "ar"}, Localisation: &schema.LocalisationSettings{PseudoLocales: &yes}}
	m, err := plxgolden.BuildMatrix(app, frames(t))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(m.Locales, []string{"en", "ar", "en-XA", "ar-XB"}) || !slices.Equal(m.Themes, []string{"light", "dark"}) || len(m.Frames) != 4 {
		t.Fatalf("matrix = %+v", m)
	}
	cases := plxgolden.Cases(m, []plxgolden.Page{{Plugin: "shop", Route: "cart"}})
	if len(cases) != 4*4*2*2 {
		t.Fatalf("cases = %d, want %d", len(cases), 4*4*2*2)
	}
	if cases[0].ID != "cart/small-360x640/en/light/x1" || cases[len(cases)-1].ID != "cart/tablet-820x1180/ar-XB/dark/x2" {
		t.Errorf("first and last = %q, %q", cases[0].ID, cases[len(cases)-1].ID)
	}
	seen := map[string]bool{}
	for _, c := range cases {
		if seen[c.ID] {
			t.Errorf("duplicate case ID %q", c.ID)
		}
		seen[c.ID] = true
	}
}

// Verifies: TST-003.
// goldens.matrix narrows each list, and names only known frames.
func TestMatrixNarrowing(t *testing.T) {
	t.Parallel()
	app := &schema.AppDocument{
		SupportedLocales: []string{"en", "de"},
		Goldens: &schema.GoldenSettings{Matrix: &schema.GoldenMatrix{
			Frames: []string{"iphone-15"}, Themes: []schema.GoldenTheme{schema.GoldenThemeDark}, TextScales: []float64{2}, Locales: []string{"de"},
		}},
	}
	m, err := plxgolden.BuildMatrix(app, frames(t))
	if err != nil {
		t.Fatal(err)
	}
	cases := plxgolden.Cases(m, []plxgolden.Page{{Route: "home"}})
	if len(cases) != 1 || cases[0].ID != "home/iphone-15/de/dark/x2" {
		t.Errorf("cases = %+v", cases)
	}
	app.Goldens.Matrix.Frames = []string{"nokia-3310"}
	if _, err := plxgolden.BuildMatrix(app, frames(t)); err == nil || !strings.Contains(err.Error(), "nokia-3310") {
		t.Errorf("unknown frame: %v", err)
	}
}

// Verifies: TST-003.
// The vendored fonts match the SHA-256 pinned in VERSION, which lists the
// Regular and Bold files of the three families and the pinned npm tarballs;
// a changed font is refused.
func TestVendoredFontsArePinned(t *testing.T) {
	t.Parallel()
	fonts, err := plxgolden.LoadFonts(fontsDir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for n := range fonts {
		names = append(names, n)
	}
	slices.Sort(names)
	want := []string{
		"NotoNaskhArabic_400Regular.ttf", "NotoNaskhArabic_700Bold.ttf", "NotoSansEthiopic_400Regular.ttf",
		"NotoSansEthiopic_700Bold.ttf", "NotoSans_400Regular.ttf", "NotoSans_700Bold.ttf",
	}
	if !slices.Equal(names, want) {
		t.Errorf("fonts = %v", names)
	}
	version, err := os.ReadFile(filepath.Join(fontsDir, "VERSION"))
	if err != nil {
		t.Fatal(err)
	}
	for _, pin := range []string{
		"@expo-google-fonts/noto-sans 0.4.2 98cf14a19b8e67a0e76bfd4d5df98937b9734e6dde570bd750f2259042042548",
		"@expo-google-fonts/noto-naskh-arabic 0.4.5 2f93452f15b3bdd86b922be17f247986d4d0e2ae7a1ca51f5d80326b2680628e",
		"@expo-google-fonts/noto-sans-ethiopic 0.4.2 3030fe790f54df5297904e82da1d5216df61064af4e2ca2972ad166a6ad81f49",
	} {
		if !strings.Contains(string(version), "package "+pin) {
			t.Errorf("VERSION lacks the pin %q", pin)
		}
	}
	for name, data := range fonts {
		sum := sha256.Sum256(data)
		if !strings.Contains(string(version), "file "+name+" "+hex.EncodeToString(sum[:])) {
			t.Errorf("%s is not pinned", name)
		}
	}

	copyDir := t.TempDir()
	for _, n := range append(names, "VERSION") {
		data, err := os.ReadFile(filepath.Join(fontsDir, n))
		if err != nil {
			t.Fatal(err)
		}
		if n == "NotoSans_700Bold.ttf" {
			data = append(data, 1)
		}
		if err := os.WriteFile(filepath.Join(copyDir, n), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := plxgolden.LoadFonts(copyDir); err == nil || !strings.Contains(err.Error(), "NotoSans_700Bold.ttf") {
		t.Errorf("a changed font was accepted: %v", err)
	}
}

// Verifies: TST-003.
// A font directory is found in a parent of the project.
func TestFindFonts(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, filepath.FromSlash(plxgolden.FontsPath))
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "VERSION"), []byte("file x.ttf 00\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(root, "apps", "bank")
	if err := os.MkdirAll(project, 0o750); err != nil {
		t.Fatal(err)
	}
	if got, ok := plxgolden.FindFonts(project); !ok || got != dir {
		t.Errorf("FindFonts = %q, %v; want %q", got, ok, dir)
	}
	if _, ok := plxgolden.FindFonts(t.TempDir()); ok {
		t.Error("found fonts in an empty directory")
	}
}

// Verifies: TST-003.
// A rendered case is a match when its hash is the baseline's, changed when
// it is another and new when the case has no baseline.
func TestCompare(t *testing.T) {
	t.Parallel()
	h := func(s string) []byte { sum := sha256.Sum256([]byte(s)); return sum[:] }
	rendered := []plxgolden.Rendered{
		{Case: plxgolden.Case{ID: "a"}, SHA256: h("a")},
		{Case: plxgolden.Case{ID: "b"}, SHA256: h("b2")},
		{Case: plxgolden.Case{ID: "c"}, SHA256: h("c")},
	}
	got := plxgolden.Compare(rendered, map[string][]byte{"a": h("a"), "b": h("b")})
	want := []plxgolden.Status{plxgolden.Match, plxgolden.Changed, plxgolden.New}
	for i, v := range got {
		if v.Status != want[i] {
			t.Errorf("case %s: %s, want %s", v.ID, v.Status, want[i])
		}
	}
}

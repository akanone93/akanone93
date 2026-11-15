// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nightCode42/plux3/backend/internal/bundle"
	"github.com/nightCode42/plux3/backend/internal/bundle/fbs"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// Verifies: CMP-030.
// The asset index lists each image's transcoded variants, sorted, from
// what the asset pipeline supplies; the bundle reads back.
func TestAssetVariantsAreIndexed(t *testing.T) {
	t.Parallel()
	opts := DefaultOptions()
	asked := 0
	opts.AssetVariants = func(sum [sha256.Size]byte) []AssetVariant {
		asked++
		return []AssetVariant{
			{MediaType: "image/webp", Density: 3, Width: 90, Height: 60, Hash: sha256.Sum256([]byte("w3")), Size: 30},
			{MediaType: "image/avif", Density: 1, Width: 30, Height: 20, Hash: sha256.Sum256([]byte("a1")), Size: 10},
			{MediaType: "image/webp", Density: 1, Width: 30, Height: 20, Hash: sha256.Sum256([]byte("w1")), Size: 11},
		}
	}
	res := Compile(fixture(t), opts)
	if res.Diagnostics.HasErrors() {
		t.Fatal(list(res.Diagnostics))
	}
	if asked == 0 {
		t.Fatal("the variants were never asked for")
	}
	read := readAll(t, res)
	var found bool
	for _, b := range read {
		for _, s := range b.Sections {
			if s.Kind != bundle.SectionAssetsIndex {
				continue
			}
			idx := fbs.GetRootAsAssetIndex(s.Data, 0)
			var a fbs.Asset
			if !idx.Assets(&a, 0) {
				t.Fatal("an empty asset index")
			}
			if a.VariantsLength() != 3 {
				t.Fatalf("%d variants", a.VariantsLength())
			}
			var v fbs.AssetVariant
			want := []struct {
				media   string
				density uint32
			}{{"image/avif", 1}, {"image/webp", 1}, {"image/webp", 3}}
			for i, w := range want {
				a.Variants(&v, i)
				if string(v.MediaType()) != w.media || v.Density() != w.density {
					t.Errorf("variant %d = %s %d×", i, v.MediaType(), v.Density())
				}
			}
			found = true
		}
	}
	if !found {
		t.Fatal("no asset index")
	}
}

// Verifies: AST-003.
// An asset file above asset.fileSize, and a plugin whose assets exceed
// plugin.assetBytes, fail the compilation with a clear diagnostic.
func TestAssetSizeLimits(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		key  limits.Key
		file string
	}{
		{limits.AssetFileSize, "assets/index.json"},
		{limits.PluginAssetBytes, "plugins/loans/plugin.json"},
	} {
		opts := DefaultOptions()
		set, err := opts.Limits.Tighten(c.key, limits.ScopeInstallation, 10)
		if err != nil {
			t.Fatal(err)
		}
		opts.Limits = set
		res := Compile(fixture(t), opts)
		found := false
		for _, d := range res.Diagnostics {
			if d.Code == plxerr.LimitExceeded && d.File == c.file && d.Severity == plxerr.SeverityError {
				found = true
			}
		}
		if !found || res.App != nil {
			t.Errorf("%s: %s", c.key, list(res.Diagnostics))
		}
	}
}

// Verifies: LIM-003.
// Passing 80% of a size limit is a warning; the compilation still
// succeeds.
func TestApproachingLimitsWarn(t *testing.T) {
	t.Parallel()
	res := Compile(fixture(t), DefaultOptions())
	if res.Diagnostics.HasErrors() {
		t.Fatal(list(res.Diagnostics))
	}
	var size int64
	for _, b := range res.Plugins {
		size = max(size, int64(len(b.Data)))
	}
	opts := DefaultOptions()
	set, err := opts.Limits.Tighten(limits.BundlePluginSize, limits.ScopeInstallation, size*100/85)
	if err != nil {
		t.Fatal(err)
	}
	opts.Limits = set
	res = Compile(fixture(t), opts)
	found := false
	for _, d := range res.Diagnostics {
		if d.Code == plxerr.LimitApproaching && d.Severity == plxerr.SeverityWarning {
			found = true
		}
	}
	if !found || res.App == nil {
		t.Errorf("no warning at 85%% of bundle.pluginSize: %s", list(res.Diagnostics))
	}
}

// Verifies: STU-052, STU-134, AST-001.
// The screenshots Studio keeps for itself as app assets (screen thumbnails, slot previews, translation context) are
// in the draft but in no bundle: the app bundle carries the app's own assets only.
func TestBundlesLeaveOutStudioScreenshots(t *testing.T) {
	t.Parallel()
	m := fixture(t)
	var idx map[string]any
	if err := json.Unmarshal(m["assets/index.json"].Data, &idx); err != nil {
		t.Fatal(err)
	}
	png := m["assets/images/logo.png"].Data
	shots := map[string]string{
		"thumbnails/screen-home.png": "01a0c450-6c00-700d-8000-0000000192a1",
		"previews/slot-card.png":     "01a0c450-6c00-700d-8000-0000000192a2",
		"l10n-context/screen-x.png":  "01a0c450-6c00-700d-8000-0000000192a3",
	}
	entries := idx["assets"].([]any)
	i := 0
	for _, file := range slices.Sorted(maps.Keys(shots)) {
		i++
		entries = append(entries, map[string]any{"id": shots[file], "key": fmt.Sprintf("shot-%d", i), "file": file, "mediaType": "image/png"})
		m["assets/"+file] = &fstest.MapFile{Data: png}
	}
	idx["assets"] = entries
	data, err := json.Marshal(idx)
	if err != nil {
		t.Fatal(err)
	}
	m["assets/index.json"] = &fstest.MapFile{Data: data}
	res := Compile(m, DefaultOptions())
	if res.Diagnostics.HasErrors() {
		t.Fatal(list(res.Diagnostics))
	}
	for _, b := range readAll(t, res) {
		for _, s := range b.Sections {
			if s.Kind != bundle.SectionAssetsIndex {
				continue
			}
			x := fbs.GetRootAsAssetIndex(s.Data, 0)
			var a fbs.Asset
			for j := range x.AssetsLength() {
				x.Assets(&a, j)
				if strings.HasPrefix(string(a.Key()), "shot-") {
					t.Errorf("a bundle carries Studio's screenshot %s", a.Key())
				}
			}
		}
	}
}

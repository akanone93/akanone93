// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package baseline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	flatbuffers "github.com/google/flatbuffers/go"

	"github.com/nightCode42/plux3/backend/internal/bundle"
	"github.com/nightCode42/plux3/backend/internal/bundle/fbs"
	"github.com/nightCode42/plux3/backend/internal/compiler/media"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// digest is the hex SHA-256 of a name, which the fake store holds as the
// file of that hash.
func digest(name string) string {
	sum := sha256.Sum256([]byte(name))
	return hex.EncodeToString(sum[:])
}

// appBundle is an app bundle whose assets-index holds one PNG with two
// WebP variants and one font.
func appBundle(t *testing.T) []byte {
	t.Helper()
	b := flatbuffers.NewBuilder(0)
	hash := func(name string) flatbuffers.UOffsetT {
		sum := sha256.Sum256([]byte(name))
		return b.CreateByteVector(sum[:])
	}
	asset := func(mediaType, file string, variants ...string) flatbuffers.UOffsetT {
		vs := make([]flatbuffers.UOffsetT, len(variants))
		for i, v := range variants {
			mt, h := b.CreateString(media.WebP), hash(v)
			fbs.AssetVariantStart(b)
			fbs.AssetVariantAddMediaType(b, mt)
			fbs.AssetVariantAddHash(b, h)
			vs[i] = fbs.AssetVariantEnd(b)
		}
		fbs.AssetStartVariantsVector(b, len(vs))
		for i := len(vs) - 1; i >= 0; i-- {
			b.PrependUOffsetT(vs[i])
		}
		list := b.EndVector(len(vs))
		mt, h := b.CreateString(mediaType), hash(file)
		fbs.AssetStart(b)
		fbs.AssetAddMediaType(b, mt)
		fbs.AssetAddHash(b, h)
		fbs.AssetAddVariants(b, list)
		return fbs.AssetEnd(b)
	}
	offs := []flatbuffers.UOffsetT{asset(media.PNG, "logo.png", "logo@1.webp", "logo@2.webp"), asset(media.TTF, "font.ttf")}
	fbs.AssetIndexStartAssetsVector(b, len(offs))
	for i := len(offs) - 1; i >= 0; i-- {
		b.PrependUOffsetT(offs[i])
	}
	list := b.EndVector(len(offs))
	fbs.AssetIndexStart(b)
	fbs.AssetIndexAddAssets(b, list)
	bundle.Finish(b, fbs.AssetIndexEnd(b), bundle.SectionAssetsIndex)
	data, err := bundle.Encode(bundle.KindApp, []bundle.Section{{Kind: bundle.SectionAssetsIndex, Data: b.FinishedBytes()}})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// store is a fake object store: bundles by hash, asset files by the name
// whose hash they are; tamper returns other bytes for every read.
type store struct {
	bundles map[string][]byte
	names   map[string]string
	tamper  bool
}

func (s store) Bundle(_ context.Context, sha string) ([]byte, error) {
	data, ok := s.bundles[sha]
	if !ok {
		return nil, errors.New("no such bundle")
	}
	if s.tamper {
		data = append(slices.Clone(data), 0)
	}
	return data, nil
}

func (s store) Asset(_ context.Context, sha string) ([]byte, error) {
	name, ok := s.names[sha]
	if !ok {
		return nil, errors.New("no such asset")
	}
	if s.tamper {
		name += "!"
	}
	return []byte(name), nil
}

func fixture(t *testing.T) (Release, store) {
	t.Helper()
	data := appBundle(t)
	b, err := bundle.ReadStructure(data)
	if err != nil {
		t.Fatal(err)
	}
	sha := hex.EncodeToString(b.Hash[:])
	names := map[string]string{}
	for _, n := range []string{"logo@1.webp", "logo@2.webp", "font.ttf", "logo.png"} {
		names[digest(n)] = n
	}
	rel := Release{
		App: "app-1", Environment: "production", Channel: "production", Sequence: 7,
		Versions: []Version{{Plugin: "", Version: 3, SHA256: sha, KeyID: "k1", Algorithm: "ed25519", Signature: []byte{1, 2, 3}}},
		Keys:     []Key{{KeyID: "k1", Algorithm: "ed25519", Role: "targets", PublicKey: "0a0b"}},
		Root:     []byte(`{"signed":{}}`),
	}
	return rel, store{bundles: map[string][]byte{sha: data}, names: names}
}

// Verifies: GEN-001, SYN-007, AST-001.
// A release's baseline holds its keys, root, bundles, the asset files
// they index (a raster image's WebP variants, not the image) and
// baseline.json naming them with their signatures, as plux pull writes
// them (build log row 294).
func TestFilesOfARelease(t *testing.T) {
	t.Parallel()
	rel, s := fixture(t)
	files, err := Files(context.Background(), rel, s, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"assets/" + digest("font.ttf"), "assets/" + digest("logo@1.webp"), "assets/" + digest("logo@2.webp"),
		"baseline.json", "bundles/_app.pxb", "keys.json", "root.json",
	}
	var got []string
	for p := range files {
		got = append(got, p)
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("files %v, want %v", got, want)
	}
	var b Baseline
	if err := json.Unmarshal(files["baseline.json"], &b); err != nil {
		t.Fatal(err)
	}
	if b.Sequence != 7 || len(b.Bundles) != 1 || b.Bundles[0].File != "bundles/_app.pxb" || b.Bundles[0].Signature != "AQID" ||
		len(b.Assets) != 3 || len(b.Keys) != 1 || b.App != "app-1" {
		t.Errorf("baseline.json: %+v", b)
	}
}

// Verifies: SEC-053, SYN-007.
// No plaintext of a confidential release enters a baseline, nor of a
// release before the channel's first: the keys and root alone.
func TestFilesWithoutBundles(t *testing.T) {
	t.Parallel()
	for name, change := range map[string]func(*Release){
		"confidential":         func(r *Release) { r.Confidential = true },
		"a sealed version":     func(r *Release) { r.Versions[0].RequiredFeatures = []string{bundle.FeatureConfidential} },
		"before first release": func(r *Release) { r.Sequence, r.Versions = 0, nil },
	} {
		rel, s := fixture(t)
		change(&rel)
		files, err := Files(context.Background(), rel, s, 1<<20)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(files) != 2 || files["keys.json"] == nil || files["root.json"] == nil {
			t.Errorf("%s: %d files", name, len(files))
		}
	}
}

// Verifies: SEC-052, LIM-001.
// A bundle or asset file that does not match its hash, a release larger
// than the limit, and an environment without keys are refused.
func TestFilesRefusals(t *testing.T) {
	t.Parallel()
	rel, s := fixture(t)
	s.tamper = true
	if _, err := Files(context.Background(), rel, s, 1<<20); err == nil {
		t.Error("a tampered bundle was embedded")
	}
	rel, s = fixture(t)
	if code, _ := plxerr.CodeOf(func() error { _, err := Files(context.Background(), rel, s, 64); return err }()); code != plxerr.LimitExceeded {
		t.Errorf("over the limit: %v", code)
	}
	rel.Keys = nil
	if code, _ := plxerr.CodeOf(func() error { _, err := Files(context.Background(), rel, s, 1<<20); return err }()); code != plxerr.PreconditionFailed {
		t.Errorf("no keys: %v", code)
	}
	// An asset file that does not match its hash.
	rel, s = fixture(t)
	for h := range s.names {
		s.names[h] += "?"
	}
	if _, err := Files(context.Background(), rel, s, 1<<20); err == nil {
		t.Error("a tampered asset file was embedded")
	}
}

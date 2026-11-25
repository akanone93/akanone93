// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Package baseline is the release a host app embeds and starts from
// (SYN-007): its format, written by `plux pull` and by the server's project
// download (GEN-001), and the assembly of its files from a release.
package baseline

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/nightCode42/plux3/backend/internal/bundle"
	"github.com/nightCode42/plux3/backend/internal/bundle/fbs"
	"github.com/nightCode42/plux3/backend/internal/compiler/media"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Baseline is baseline.json: the release a host app embeds.
type Baseline struct {
	App         string  `json:"app"`
	Environment string  `json:"environment"`
	Channel     string  `json:"channel"`
	Sequence    int64   `json:"releaseSequence"`
	Bundles     []Entry `json:"bundles"`
	Assets      []Asset `json:"assets"`
	Keys        []Key   `json:"keys"`
}

// Entry is one bundle of a baseline with the signature publish made over
// its bundle hash, so the runtime verifies it under the embedded keys
// before loading it (SEC-052, ADR-0029).
type Entry struct {
	Plugin    string `json:"plugin"`
	Version   int64  `json:"version"`
	SHA256    string `json:"sha256"`
	File      string `json:"file"`
	KeyID     string `json:"keyId"`
	Algorithm string `json:"algorithm"`
	Signature string `json:"signature"`
}

// Asset is one asset file of a baseline, named by its SHA-256.
type Asset struct {
	SHA256 string `json:"sha256"`
	File   string `json:"file"`
}

// Key is a root public key of keys.json, the public key hex-encoded.
type Key struct {
	KeyID     string `json:"keyId"`
	Algorithm string `json:"algorithm"`
	Role      string `json:"role"`
	PublicKey string `json:"publicKey"`
}

// BundleFile is the file of a plugin's bundle in a baseline; the app
// bundle is bundles/_app.pxb, since no plugin key starts with an
// underscore.
func BundleFile(plugin string) string {
	if plugin == "" {
		plugin = "_app"
	}
	return "bundles/" + plugin + ".pxb"
}

// AssetFiles lists, sorted and once each, the asset files a baseline
// embeds for the assets its bundles index (AST-001): the WebP variants of
// a raster image at every density, so that the device picks its density
// at run time and every platform decodes it; the vector_graphics variant
// of an SVG, never the SVG itself (CMP-031); the file itself for anything
// else.
func AssetFiles(bundles [][]byte) ([]string, error) {
	seen := map[string]bool{}
	for _, data := range bundles {
		b, err := bundle.ReadStructure(data)
		if err != nil {
			return nil, fmt.Errorf("read a bundle: %w", err)
		}
		for _, s := range b.Sections {
			if s.Kind == bundle.SectionAssetsIndex {
				indexFiles(fbs.GetRootAsAssetIndex(s.Data, 0), seen)
			}
		}
	}
	out := make([]string, 0, len(seen))
	for h := range seen {
		out = append(out, h)
	}
	slices.Sort(out)
	return out, nil
}

// indexFiles adds the files of every asset of an index to seen.
func indexFiles(idx *fbs.AssetIndex, seen map[string]bool) {
	var a fbs.Asset
	for i := range idx.AssetsLength() {
		if idx.Assets(&a, i) {
			for _, h := range assetFiles(&a) {
				seen[h] = true
			}
		}
	}
}

// assetFiles names the files of one asset a baseline embeds.
func assetFiles(a *fbs.Asset) []string {
	var webp, vec []string
	var v fbs.AssetVariant
	for j := range a.VariantsLength() {
		if !a.Variants(&v, j) {
			continue
		}
		switch string(v.MediaType()) {
		case media.WebP:
			webp = append(webp, hex.EncodeToString(v.HashBytes()))
		case media.VectorGraphics:
			vec = append(vec, hex.EncodeToString(v.HashBytes()))
		}
	}
	switch {
	case string(a.MediaType()) == media.SVG:
		return vec
	case len(webp) > 0:
		return webp
	}
	return []string{hex.EncodeToString(a.HashBytes())}
}

// Version is one bundle of the release a baseline embeds.
type Version struct {
	Plugin           string
	Version          int64
	SHA256, KeyID    string
	Algorithm        string
	Signature        []byte
	RequiredFeatures []string
}

// Release is what a baseline is assembled from: the channel's release,
// its environment's keys and root.
type Release struct {
	App, Environment, Channel string
	// Sequence is the channel's release, 0 before its first: a baseline
	// is then the keys alone, and the app syncs its first release.
	Sequence int64
	// Confidential is true for a release delivered sealed (SEC-053): its
	// bundles are never embedded, so that a host app's binary carries no
	// plaintext of them, and the app syncs them sealed instead.
	Confidential bool
	Versions     []Version
	Keys         []Key
	// Root is the environment's newest root document, nil for none.
	Root []byte
}

// Fetcher reads the objects of a release by their SHA-256.
type Fetcher interface {
	Bundle(ctx context.Context, sha string) ([]byte, error)
	Asset(ctx context.Context, sha string) ([]byte, error)
}

// Files assembles a baseline's files by their path under the host app's
// assets/plux directory: keys.json, root.json when the environment has a
// root, and, for a release that is neither absent nor confidential, its
// bundles, the asset files they index and baseline.json. Every bundle is
// checked against its hash (BND-005) and every asset file against its
// SHA-256; more than maxBytes in all is refused.
func Files(ctx context.Context, r Release, fetch Fetcher, maxBytes int64) (map[string][]byte, error) {
	if len(r.Keys) == 0 {
		return nil, plxerr.New(plxerr.PreconditionFailed, "the environment has no signing key yet")
	}
	a := &assembly{out: map[string][]byte{}, max: maxBytes}
	if err := a.add("keys.json", encode(r.Keys)); err != nil {
		return nil, err
	}
	if r.Root != nil {
		if err := a.add("root.json", r.Root); err != nil {
			return nil, err
		}
	}
	if r.Sequence == 0 || r.Confidential || slices.ContainsFunc(r.Versions, confidentialVersion) {
		return a.out, nil
	}
	b := Baseline{App: r.App, Environment: r.Environment, Channel: r.Channel, Sequence: r.Sequence, Bundles: []Entry{}, Assets: []Asset{}, Keys: r.Keys}
	bundles, err := a.bundles(ctx, r.Versions, fetch, &b)
	if err != nil {
		return nil, err
	}
	if err := a.assets(ctx, bundles, fetch, &b); err != nil {
		return nil, err
	}
	return a.out, a.add("baseline.json", encode(b))
}

// assembly collects a baseline's files within a size.
type assembly struct {
	out   map[string][]byte
	total int64
	max   int64
}

// add adds a file, refusing one past the size.
func (a *assembly) add(path string, data []byte) error {
	if a.total += int64(len(data)); a.total > a.max {
		return plxerr.New(plxerr.LimitExceeded, "the embedded release is larger than %d bytes", a.max)
	}
	a.out[path] = data
	return nil
}

// bundles adds the release's bundles, each checked against its hash, and
// names them in b.
func (a *assembly) bundles(ctx context.Context, versions []Version, fetch Fetcher, b *Baseline) ([][]byte, error) {
	var all [][]byte
	for _, v := range versions {
		data, err := fetch.Bundle(ctx, v.SHA256)
		if err != nil {
			return nil, fmt.Errorf("baseline: bundle %s: %w", v.SHA256, err)
		}
		if s, err := bundle.ReadStructure(data); err != nil || hex.EncodeToString(s.Hash[:]) != v.SHA256 {
			return nil, plxerr.New(plxerr.BundleMalformed, "bundle %s does not match its hash", v.SHA256)
		}
		file := BundleFile(v.Plugin)
		if err := a.add(file, data); err != nil {
			return nil, err
		}
		all = append(all, data)
		b.Bundles = append(b.Bundles, Entry{
			Plugin: v.Plugin, Version: v.Version, SHA256: v.SHA256, File: file,
			KeyID: v.KeyID, Algorithm: v.Algorithm, Signature: base64.StdEncoding.EncodeToString(v.Signature),
		})
	}
	return all, nil
}

// assets adds the asset files the bundles index, each checked against its
// SHA-256, and names them in b.
func (a *assembly) assets(ctx context.Context, bundles [][]byte, fetch Fetcher, b *Baseline) error {
	files, err := AssetFiles(bundles)
	if err != nil {
		return err
	}
	for _, h := range files {
		data, err := fetch.Asset(ctx, h)
		if err != nil {
			return fmt.Errorf("baseline: asset %s: %w", h, err)
		}
		if sum := sha256.Sum256(data); hex.EncodeToString(sum[:]) != h {
			return plxerr.New(plxerr.BundleMalformed, "asset file %s does not match its hash", h)
		}
		if err := a.add("assets/"+h, data); err != nil {
			return err
		}
		b.Assets = append(b.Assets, Asset{SHA256: h, File: "assets/" + h})
	}
	return nil
}

// confidentialVersion reports whether a bundle is delivered sealed.
func confidentialVersion(v Version) bool {
	return slices.Contains(v.RequiredFeatures, bundle.FeatureConfidential)
}

// encode writes a document as `plux pull` does: indented, with a newline.
func encode(v any) []byte {
	data, _ := json.MarshalIndent(v, "", "  ") //nolint:errcheck,errchkjson // plain structs
	return append(data, '\n')
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package plxtest

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/nightCode42/plux3/backend/internal/bundle"
	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/schema/uuid7"
	"github.com/nightCode42/plux3/backend/internal/signing/runkey"
)

// BaselineDir is where the test project keeps the release, as the host
// apps keep what `plux pull` writes (SYN-007).
const BaselineDir = "assets/plux"

// Release is a compiled project as a device finds it embedded: every
// bundle signed with the key of the run, listed in baseline.json with that
// key in keys.json. The runtime verifies it as it verifies any release
// (SEC-052); the harness trusts the public key and nothing else.
type Release struct {
	// AppID is the ID of the app document, which the runtime is started
	// with.
	AppID string
	// KeyID and PublicKey identify the key of the run.
	KeyID     string
	PublicKey []byte
	// Files are the baseline's files by path relative to BaselineDir.
	Files map[string][]byte
	// Kinds are the page kinds of the routes that are not screens: the
	// harness opens those pages as the runtime presents them, as a dialog
	// or a sheet (SCH-022).
	Kinds map[string]schema.PageKind
	// DeviceFunctions says whether a bundle carries device functions
	// (section.wasm.v1): the harness then registers the device function
	// engine, as a host that runs them does (FN-050).
	DeviceFunctions bool
}

// The baseline's JSON, as `plux pull` writes it and the runtime reads it.
type (
	baseline struct {
		App         string  `json:"app"`
		Environment string  `json:"environment"`
		Channel     string  `json:"channel"`
		Sequence    int64   `json:"releaseSequence"`
		Bundles     []entry `json:"bundles"`
		Assets      []asset `json:"assets"`
		Keys        []key   `json:"keys"`
	}
	entry struct {
		Plugin    string `json:"plugin"`
		Version   int64  `json:"version"`
		SHA256    string `json:"sha256"`
		File      string `json:"file"`
		KeyID     string `json:"keyId"`
		Algorithm string `json:"algorithm"`
		Signature string `json:"signature"`
	}
	asset struct {
		SHA256 string `json:"sha256"`
		File   string `json:"file"`
	}
	key struct {
		KeyID     string `json:"keyId"`
		Algorithm string `json:"algorithm"`
		Role      string `json:"role"`
		PublicKey string `json:"publicKey"`
	}
)

// BuildRelease signs the bundles of a compilation with k. The signature
// covers the bundle's hash, as publishing signs it (ADR-0004).
func BuildRelease(res *compiler.Result, k *runkey.Key) (*Release, error) {
	if res.App == nil {
		return nil, errors.New("plxtest: the compilation made no app bundle")
	}
	appID := uuid7.UUID(res.App.ID).String()
	rel := &Release{AppID: appID, KeyID: k.ID(), PublicKey: k.Public(), Files: map[string][]byte{}, Kinds: pageKinds(res.Project)}
	b := baseline{
		App: appID, Environment: "production", Channel: "production", Sequence: 1,
		Bundles: []entry{}, Assets: []asset{},
		Keys: []key{{KeyID: k.ID(), Algorithm: runkey.Algorithm, Role: "targets", PublicKey: hex.EncodeToString(k.Public())}},
	}
	for _, c := range append([]*compiler.Bundle{res.App}, res.Plugins...) {
		plugin, file := c.Key, "bundles/"+c.Key+".pxb"
		if c == res.App {
			plugin, file = "", "bundles/_app.pxb"
		}
		rel.Files[file] = c.Data
		rel.DeviceFunctions = rel.DeviceFunctions || slices.Contains(c.Features, bundle.FeatureWasm)
		b.Bundles = append(b.Bundles, entry{
			Plugin: plugin, Version: 1, SHA256: hex.EncodeToString(c.Hash[:]), File: file,
			KeyID: k.ID(), Algorithm: runkey.Algorithm, Signature: base64.StdEncoding.EncodeToString(k.Sign(c.Hash[:])),
		})
	}
	for _, sum := range slices.SortedFunc(maps.Keys(res.Files), func(a, b [sha256.Size]byte) int { return bytes.Compare(a[:], b[:]) }) {
		name := "assets/" + hex.EncodeToString(sum[:])
		rel.Files[name] = res.Files[sum]
		b.Assets = append(b.Assets, asset{SHA256: hex.EncodeToString(sum[:]), File: name})
	}
	for name, v := range map[string]any{"baseline.json": b, "keys.json": b.Keys} {
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("plxtest: encode %s: %w", name, err)
		}
		rel.Files[name] = append(data, '\n')
	}
	return rel, nil
}

// pageKinds are the kinds of the project's routed pages that are not
// screens, by route.
func pageKinds(project *schema.Project) map[string]schema.PageKind {
	kinds := map[string]schema.PageKind{}
	if project == nil {
		return kinds
	}
	for _, pl := range project.Plugins {
		for _, pg := range pl.Pages {
			if pg.Doc != nil && pg.Doc.Route != "" && pg.Doc.PageKind != "" && pg.Doc.PageKind != schema.PageKindScreen {
				kinds[pg.Doc.Route] = pg.Doc.PageKind
			}
		}
	}
	return kinds
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"

	embedded "github.com/nightCode42/plux3/backend/internal/baseline"
)

// baselineAsset is one asset file of a baseline, named by its SHA-256.
type baselineAsset = embedded.Asset

// maxAssetFile bounds one downloaded asset file: the hard maximum of
// asset.fileSize.
const maxAssetFile = 100 << 20

// assetFileName matches the files pull writes under assets/.
var assetFileName = regexp.MustCompile(`^[0-9a-f]{64}$`)

// baselineAssetFiles lists the asset files a baseline embeds for the
// assets its bundles index (embedded.AssetFiles).
func baselineAssetFiles(bundles [][]byte) ([]string, error) {
	return embedded.AssetFiles(bundles) //nolint:wrapcheck // the package's own message
}

// writeAssets downloads the asset files into dir/assets, checking each
// against its SHA-256, and removes the files of earlier pulls the
// release no longer uses.
func (cl *clients) writeAssets(ctx context.Context, dir string, files []string) ([]baselineAsset, error) {
	assets := filepath.Join(dir, "assets")
	if err := os.MkdirAll(assets, 0o755); err != nil { //nolint:gosec // G301: project files.
		return nil, fmt.Errorf("create %s: %w", assets, err)
	}
	out := make([]baselineAsset, 0, len(files))
	for _, h := range files {
		path := filepath.Join(assets, h)
		if !fileHasHash(path, h) {
			data, err := cl.downloadAsset(ctx, h)
			if err != nil {
				return nil, err
			}
			if err := writeFile(path, data); err != nil {
				return nil, err
			}
		}
		out = append(out, baselineAsset{SHA256: h, File: "assets/" + h})
	}
	entries, err := os.ReadDir(assets)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", assets, err)
	}
	for _, e := range entries {
		if assetFileName.MatchString(e.Name()) && !slices.Contains(files, e.Name()) {
			if err := os.Remove(filepath.Join(assets, e.Name())); err != nil {
				return nil, fmt.Errorf("remove a stale asset: %w", err)
			}
		}
	}
	return out, nil
}

// fileHasHash reports whether path holds bytes with SHA-256 h.
func fileHasHash(path, h string) bool {
	data, err := os.ReadFile(path) //nolint:gosec // G304: a file pull itself writes.
	if err != nil {
		return false
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]) == h
}

// downloadAsset fetches an asset file by its SHA-256 and checks it.
func (cl *clients) downloadAsset(ctx context.Context, h string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cl.server+"/v1/objects/assets/"+h[:2]+"/"+h, nil)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	res, err := cl.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download asset %s: %w", h, err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download asset %s: status %d", h, res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, maxAssetFile+1))
	if err != nil {
		return nil, fmt.Errorf("download asset %s: %w", h, err)
	}
	sum := sha256.Sum256(data)
	if len(data) > maxAssetFile || hex.EncodeToString(sum[:]) != h {
		return nil, fmt.Errorf("asset %s does not match its hash", h)
	}
	return data, nil
}

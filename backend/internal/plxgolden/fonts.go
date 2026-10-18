// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package plxgolden

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FontsPath is where the vendored fonts live in a Plux checkout.
const FontsPath = "schema/goldens/fonts"

// FindFonts looks for the vendored fonts directory in dir and its parents:
// a Plux checkout, or a project inside one.
func FindFonts(dir string) (string, bool) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	for {
		candidate := filepath.Join(abs, filepath.FromSlash(FontsPath))
		if _, err := os.Stat(filepath.Join(candidate, "VERSION")); err == nil {
			return candidate, true
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", false
		}
		abs = parent
	}
}

// ParseVersion reads the font files and their SHA-256 values out of the
// VERSION file of the fonts directory.
func ParseVersion(data []byte) (map[string]string, error) {
	out := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 3 && f[0] == "file" {
			out[f[1]] = f[2]
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("plxgolden: VERSION: %w", err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("plxgolden: VERSION lists no font files")
	}
	return out, nil
}

// LoadFonts reads the vendored fonts from dir and checks each against the
// SHA-256 pinned in its VERSION file: a run rendered with other fonts would
// compare against baselines it cannot match (ADR-0070).
func LoadFonts(dir string) (map[string][]byte, error) {
	version, err := os.ReadFile(filepath.Join(dir, "VERSION"))
	if err != nil {
		return nil, fmt.Errorf("plxgolden: %w", err)
	}
	pinned, err := ParseVersion(version)
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	for name, want := range pinned {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("plxgolden: %w", err)
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != want {
			return nil, fmt.Errorf("plxgolden: font %s does not match the SHA-256 pinned in %s/VERSION", name, dir)
		}
		out[name] = data
	}
	return out, nil
}

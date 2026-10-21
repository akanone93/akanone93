// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package reconstruct

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strings"
)

// openURI is what opens a reconstruction release in the viewer host:
// plux-viewer://reconstruction/<id>?server=<server URL>.
func openURI(serverURL, id string) string {
	return "plux-viewer://reconstruction/" + id + "?server=" + url.QueryEscape(strings.TrimSuffix(serverURL, "/"))
}

// parseHash reads a "sha256:<hex>" bundle hash.
func parseHash(s string) ([]byte, bool) {
	h, ok := strings.CutPrefix(s, "sha256:")
	if !ok {
		return nil, false
	}
	b, err := hex.DecodeString(h)
	return b, err == nil && len(b) == sha256.Size
}

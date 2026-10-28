// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package document

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// collectIDs maps every identifier found under an "id" member, at any depth
// of any document, to a new identifier derived from the new plugin key and
// the old identifier, so that a duplicate is reproducible (STU-034).
func collectIDs(docs []Document, key string) map[string]string {
	out := map[string]string{}
	for _, d := range docs {
		v, err := decodeJSON(d.Content)
		if err != nil {
			continue
		}
		gatherIDs(v, key, out)
	}
	return out
}

func gatherIDs(v any, key string, out map[string]string) {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if id, ok := child.(string); ok && k == "id" && id != "" {
				if _, seen := out[id]; !seen {
					out[id] = deriveID(key, id)
				}
				continue
			}
			gatherIDs(child, key, out)
		}
	case []any:
		for _, child := range t {
			gatherIDs(child, key, out)
		}
	}
}

// deriveID returns an identifier of the same shape as old: a UUID stays a
// UUID, anything else keeps its length.
func deriveID(key, old string) string {
	sum := sha256.Sum256([]byte(key + "\x00" + old))
	h := hex.EncodeToString(sum[:])
	if len(old) == 36 && old[8] == '-' && old[13] == '-' && old[18] == '-' && old[23] == '-' {
		return h[0:8] + "-" + h[8:12] + "-7" + h[13:16] + "-8" + h[17:20] + "-" + h[20:32]
	}
	for len(h) < len(old) {
		h += h
	}
	return h[:len(old)]
}

func decodeJSON(b []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return v, nil
}

// rewriteIDs replaces every identifier of renamed in a document: object
// keys and string values, whole or as a delimited token inside a longer
// string such as a pointer or a template.
func rewriteIDs(content []byte, renamed map[string]string) ([]byte, error) {
	v, err := decodeJSON(content)
	if err != nil {
		return content, nil //nolint:nilerr // not JSON: copied as it is
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(swapIDs(v, renamed)); err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func swapIDs(v any, renamed map[string]string) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, child := range t {
			out[swapToken(k, renamed)] = swapIDs(child, renamed)
		}
		return out
	case []any:
		for i, child := range t {
			t[i] = swapIDs(child, renamed)
		}
		return t
	case string:
		return swapToken(t, renamed)
	}
	return v
}

func isIDRune(r rune) bool {
	return r == '-' || r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
}

// swapToken replaces each maximal run of identifier characters that is a
// known identifier.
func swapToken(s string, renamed map[string]string) string {
	if fresh, ok := renamed[s]; ok {
		return fresh
	}
	var b strings.Builder
	start := -1
	flush := func(end int) {
		tok := s[start:end]
		if fresh, ok := renamed[tok]; ok {
			tok = fresh
		}
		b.WriteString(tok)
		start = -1
	}
	for i, r := range s {
		switch {
		case isIDRune(r) && start < 0:
			start = i
		case !isIDRune(r) && start >= 0:
			flush(i)
			b.WriteRune(r)
		case !isIDRune(r):
			b.WriteRune(r)
		}
	}
	if start >= 0 {
		flush(len(s))
	}
	return b.String()
}

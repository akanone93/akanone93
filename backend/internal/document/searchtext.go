// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package document

import (
	"strings"

	"github.com/nightCode42/plux3/backend/internal/schema"
)

// searchText returns what a document adds to the text search beyond its
// path and key (SRV-061): the name, title and description of a page,
// component, template or function, and every key of a translation key
// catalogue with its description. Migration 0026 explains where it is
// indexed.
func searchText(kind schema.DocumentKind, tree map[string]any) string {
	var words []string
	for _, field := range []string{"name", "title", "description"} {
		if s, ok := tree[field].(string); ok && s != "" {
			words = append(words, s)
		}
	}
	if kind == schema.KindTranslationKeys {
		keys, _ := tree["keys"].([]any)
		for _, k := range keys {
			entry, _ := k.(map[string]any)
			for _, field := range []string{"key", "description"} {
				if s, ok := entry[field].(string); ok && s != "" {
					words = append(words, s)
				}
			}
		}
	}
	return strings.Join(words, "\n")
}

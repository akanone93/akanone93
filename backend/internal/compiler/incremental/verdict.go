// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package incremental

import (
	"slices"

	"github.com/nightCode42/plux3/backend/internal/schema"
)

// PageVerdict says whether a page keeps its state when its new version
// replaces the one running on a device (DEV-010). The state of a page is
// its entries (STA-001, scope `page`), each a scope of its own named
// `page.<name>`: the runtime keeps the entries whose scope is compatible and
// resets the others, saying so in the stream.
type PageVerdict struct {
	// ID, Key and File identify the page.
	ID, Key, File string
	// Compatible is true when no scope is incompatible: the page's state
	// schema is unchanged, or only grew.
	Compatible bool
	// Incompatible lists the scopes the runtime resets, sorted: entries
	// that were removed, and entries whose name, type, persistence,
	// sensitivity or computed-ness changed, because a stored value of the
	// old declaration does not mean the same under the new one (STA-040).
	Incompatible []string
}

// verdictOf compares a page with its previous version; a page without one
// has no state to keep.
func verdictOf(old, now *schema.PageDocument, file string) PageVerdict {
	v := PageVerdict{ID: now.ID, Key: now.Key, File: file, Compatible: true}
	if old == nil {
		return v
	}
	was := make(map[string]schema.StateEntry, len(old.State))
	for _, e := range old.State {
		was[e.ID] = e
	}
	kept := map[string]bool{}
	for _, e := range now.State {
		kept[e.ID] = true
		if prev, ok := was[e.ID]; ok && !sameShape(prev, e) {
			v.Incompatible = append(v.Incompatible, scope(e.Name))
		}
	}
	for _, e := range old.State {
		if !kept[e.ID] {
			v.Incompatible = append(v.Incompatible, scope(e.Name))
		}
	}
	slices.Sort(v.Incompatible)
	v.Incompatible = slices.Compact(v.Incompatible)
	v.Compatible = len(v.Incompatible) == 0
	return v
}

// scope names the state scope of a page entry.
func scope(name string) string { return "page." + name }

// sameShape reports whether two declarations of one state entry store a
// value the same way. Defaults, descriptions, exposure and migrations are
// not part of the shape: the stored value outlives them.
func sameShape(a, b schema.StateEntry) bool {
	return a.Name == b.Name && a.Type == b.Type && a.Persistence == b.Persistence &&
		(a.Computed == nil) == (b.Computed == nil) && isTrue(a.Sensitive) == isTrue(b.Sensitive)
}

func isTrue(b *bool) bool { return b != nil && *b }

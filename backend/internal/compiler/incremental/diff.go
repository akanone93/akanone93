// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package incremental

import (
	"bytes"
	"cmp"
	"fmt"
	"slices"

	"github.com/nightCode42/plux3/backend/internal/bundle"
	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/schema/uuid7"
)

// Section is a section of a bundle that is new or whose bytes changed, with
// its bytes; Data aliases the bundle and must not be modified.
type Section struct {
	// Bundle is the key of the bundle: the app's or a plugin's.
	Bundle string
	Kind   bundle.SectionKind
	ID     bundle.ID
	Data   []byte
}

// SectionRef identifies a section that the new bundles no longer hold.
type SectionRef struct {
	Bundle string
	Kind   bundle.SectionKind
	ID     bundle.ID
}

// sectionDiff lists the sections of next that are not in prev or differ from
// theirs, and the ones of prev that next lacks, both sorted by bundle, kind
// and ID.
func sectionDiff(prev, next *compiler.Result) (changed []Section, removed []SectionRef, err error) {
	was := bundlesOf(prev)
	now := bundlesOf(next)
	for key, b := range now {
		old := was[key]
		if old != nil && bytes.Equal(old.Data, b.Data) {
			continue
		}
		c, r, err := bundleDiff(key, old, b)
		if err != nil {
			return nil, nil, err
		}
		changed, removed = append(changed, c...), append(removed, r...)
	}
	for key, b := range was {
		if now[key] == nil {
			_, r, err := bundleDiff(key, b, nil)
			if err != nil {
				return nil, nil, err
			}
			removed = append(removed, r...)
		}
	}
	slices.SortFunc(changed, func(a, b Section) int { return compareRefs(a.Bundle, a.Kind, a.ID, b.Bundle, b.Kind, b.ID) })
	slices.SortFunc(removed, func(a, b SectionRef) int { return compareRefs(a.Bundle, a.Kind, a.ID, b.Bundle, b.Kind, b.ID) })
	return changed, removed, nil
}

func compareRefs(ab string, ak bundle.SectionKind, ai bundle.ID, bb string, bk bundle.SectionKind, bi bundle.ID) int {
	return cmp.Or(cmp.Compare(ab, bb), cmp.Compare(ak, bk), bytes.Compare(ai[:], bi[:]))
}

// bundlesOf lists the bundles of a result by key.
func bundlesOf(r *compiler.Result) map[string]*compiler.Bundle {
	out := map[string]*compiler.Bundle{}
	if r == nil || r.App == nil {
		return out
	}
	out[r.App.Key] = r.App
	for _, b := range r.Plugins {
		out[b.Key] = b
	}
	return out
}

// bundleDiff compares the sections of two versions of a bundle; either may
// be nil.
func bundleDiff(key string, old, now *compiler.Bundle) (changed []Section, removed []SectionRef, err error) {
	read := func(b *compiler.Bundle) (map[sectionKey]bundle.Section, error) {
		out := map[sectionKey]bundle.Section{}
		if b == nil {
			return out, nil
		}
		parsed, err := bundle.ReadStructure(b.Data)
		if err != nil {
			return nil, fmt.Errorf("incremental.Compile: read bundle %s: %w", key, err)
		}
		for _, s := range parsed.Sections {
			out[sectionKey{s.Kind, s.ID}] = s
		}
		return out, nil
	}
	was, err := read(old)
	if err != nil {
		return nil, nil, err
	}
	is, err := read(now)
	if err != nil {
		return nil, nil, err
	}
	for k, s := range is {
		if prev, ok := was[k]; !ok || prev.Hash != s.Hash {
			changed = append(changed, Section{Bundle: key, Kind: k.kind, ID: k.id, Data: s.Data})
		}
	}
	for k := range was {
		if _, ok := is[k]; !ok {
			removed = append(removed, SectionRef{Bundle: key, Kind: k.kind, ID: k.id})
		}
	}
	return changed, removed, nil
}

// sectionKey identifies a section within a bundle.
type sectionKey struct {
	kind bundle.SectionKind
	id   bundle.ID
}

// verdicts returns the verdict of every page whose section changed, sorted by
// page file.
func verdicts(changed []Section, prev, next *schema.Project) []PageVerdict {
	if next == nil {
		return nil
	}
	pages := pagesByID(next)
	var old map[string]*schema.Loaded[schema.PageDocument]
	if prev != nil {
		old = pagesByID(prev)
	}
	var out []PageVerdict
	for _, s := range changed {
		if s.Kind != bundle.SectionPage {
			continue
		}
		id := uuid7.UUID(s.ID).String()
		now, ok := pages[id]
		if !ok {
			continue
		}
		var was *schema.PageDocument
		if p := old[id]; p != nil {
			was = p.Doc
		}
		out = append(out, verdictOf(was, now.Doc, now.Source.File))
	}
	slices.SortFunc(out, func(a, b PageVerdict) int { return cmp.Compare(a.File, b.File) })
	return out
}

// pagesByID indexes the pages of a project by their ID.
func pagesByID(p *schema.Project) map[string]*schema.Loaded[schema.PageDocument] {
	out := map[string]*schema.Loaded[schema.PageDocument]{}
	for i := range p.Plugins {
		for j := range p.Plugins[i].Pages {
			if pg := &p.Plugins[i].Pages[j]; pg.Doc != nil && pg.Source != nil {
				out[pg.Doc.ID] = pg
			}
		}
	}
	return out
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package symbolicate

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/nightCode42/plux3/backend/internal/bundle"
	"github.com/nightCode42/plux3/backend/internal/bundle/fbs"
)

// ErrNotFound says the path names no node of the bundle and source map.
var ErrNotFound = errors.New("symbolicate: the node is not in the source map")

// Location is where a node is written.
type Location struct {
	// File is the document file, relative to the project.
	File string
	// Pointer is the JSON pointer of the node inside the file.
	Pointer string
}

// Path is a parsed runtime node path.
type Path struct {
	Plugin, Page string
	// Index is the node's index in the page's node array.
	Index uint32
}

// ParsePath reads "plugin/pageKey/index[/…]".
func ParsePath(s string) (Path, error) {
	parts := strings.Split(s, "/")
	if len(parts) < 3 || parts[0] == "" || parts[1] == "" {
		return Path{}, fmt.Errorf("symbolicate: %q is not a node path of the form plugin/page/index", s)
	}
	n, err := strconv.ParseUint(parts[2], 10, 32)
	if err != nil {
		return Path{}, fmt.Errorf("symbolicate: %q has no node index: %w", s, err)
	}
	return Path{Plugin: parts[0], Page: parts[1], Index: uint32(n)}, nil
}

// Resolve finds the source location of the node at path, given the plugin's
// bundle and its source map. It returns ErrNotFound when the bundle does not
// belong to the path's plugin, has no such page, or the map has no such node.
func Resolve(bundleData, sourceMap []byte, path string) (Location, error) {
	p, err := ParsePath(path)
	if err != nil {
		return Location{}, err
	}
	hi, lo, err := pageSection(bundleData, p)
	if err != nil {
		return Location{}, err
	}
	return lookup(sourceMap, hi, lo, p.Index)
}

// pageSection returns the section identifier of the path's page.
func pageSection(bundleData []byte, p Path) (hi, lo uint64, err error) {
	b, err := bundle.ReadStructure(bundleData)
	if err != nil {
		return 0, 0, fmt.Errorf("symbolicate: read the bundle: %w", err)
	}
	var meta *fbs.Meta
	for _, s := range b.Sections {
		if s.Kind == bundle.SectionMeta {
			meta = fbs.GetRootAsMeta(s.Data, 0)
			break
		}
	}
	if meta == nil || string(meta.Key()) != p.Plugin {
		return 0, 0, ErrNotFound
	}
	var page fbs.PageEntry
	var id fbs.Uuid
	for i := range meta.PagesLength() {
		if meta.Pages(&page, i) && string(page.Key()) == p.Page && page.Id(&id) != nil {
			return id.Hi(), id.Lo(), nil
		}
	}
	return 0, 0, ErrNotFound
}

// lookup finds a node of a section in the source map. The compiler writes
// the nodes sorted by section and index, but the search does not rely on it.
func lookup(sourceMap []byte, hi, lo uint64, index uint32) (Location, error) {
	sm := fbs.GetRootAsSourceMap(sourceMap, 0)
	var loc fbs.Location
	var section fbs.Uuid
	for i := range sm.NodesLength() {
		if !sm.Nodes(&loc, i) || loc.Index() != index || loc.Section(&section) == nil {
			continue
		}
		if section.Hi() != hi || section.Lo() != lo {
			continue
		}
		if int(loc.File()) >= sm.FilesLength() {
			return Location{}, fmt.Errorf("symbolicate: node %d names file %d of %d", index, loc.File(), sm.FilesLength())
		}
		return Location{File: string(sm.Files(int(loc.File()))), Pointer: string(loc.Pointer())}, nil
	}
	return Location{}, ErrNotFound
}

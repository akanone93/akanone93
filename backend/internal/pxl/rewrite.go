// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package pxl

import (
	"fmt"
	"sort"
	"strings"
)

// ReplaceMembers rewrites the member paths of an expression that start at
// the identifier root: for each path, outermost first, replace receives the
// names after root (`experiment.banner.title` gives ["banner", "title"]) and
// returns the source text that takes its place, or false to leave it and try
// the shorter path inside it. String literals and member names of other
// paths are never touched, because the expression is parsed rather than
// scanned. maxDepth bounds the syntax tree as the compiler does.
func ReplaceMembers(src, root string, maxDepth int, replace func(path []string) (string, bool)) (string, error) {
	tree, err := parse(src, maxDepth)
	if err != nil {
		return "", fmt.Errorf("pxl.ReplaceMembers: %w", err)
	}
	var edits []edit
	collectMembers(tree, root, replace, &edits)
	if len(edits) == 0 {
		return src, nil
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].span.Start < edits[j].span.Start })
	var b strings.Builder
	at := 0
	for _, e := range edits {
		b.WriteString(src[at:e.span.Start])
		b.WriteString(e.text)
		at = e.span.End
	}
	b.WriteString(src[at:])
	return b.String(), nil
}

// edit is one replacement of a source span.
type edit struct {
	span Span
	text string
}

// collectMembers walks the tree and records a replacement for each member
// path of root that replace accepts; it does not descend into a replaced
// path, so replacements never overlap.
func collectMembers(n *node, root string, replace func([]string) (string, bool), edits *[]edit) {
	if n == nil {
		return
	}
	if path, ok := memberPath(n, root); ok && len(path) > 0 {
		if text, ok := replace(path); ok {
			*edits = append(*edits, edit{span: n.span, text: text})
			return
		}
	}
	for _, c := range []*node{n.a, n.b, n.c} {
		collectMembers(c, root, replace, edits)
	}
	for _, c := range n.args {
		collectMembers(c, root, replace, edits)
	}
	for _, c := range n.keys {
		collectMembers(c, root, replace, edits)
	}
}

// memberPath returns the names after root when n is a plain chain of
// fields (`root.a.b`) on the identifier root.
func memberPath(n *node, root string) ([]string, bool) {
	switch n.kind {
	case nIdent:
		return nil, n.op == root
	case nField:
		path, ok := memberPath(n.a, root)
		if !ok {
			return nil, false
		}
		return append(path, n.op), true
	default:
		return nil, false
	}
}

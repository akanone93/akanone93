// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
)

const searchUsage = "Usage: plux search [--kind kinds] [--json] <query>\n\n" +
	"Searches the apps, plugins, pages, components, templates, translation keys and\n" +
	"functions the caller can read. Kinds, comma-separated: app, plugin, page,\n" +
	"component, template, translation_key, function. Empty means all."

// searchPageSize is the number of hits one search prints.
const searchPageSize = 50

// hitView is the JSON form of one search hit.
func hitView(h *pluxv1.SearchHit) map[string]any {
	return map[string]any{
		"kind": h.GetKind(), "id": h.GetId(), "app_id": h.GetAppId(), "name": h.GetName(),
		"path": h.GetPath(), "snippet": h.GetSnippet(), "rank": h.GetRank(),
	}
}

// hitLine is the text form of one search hit.
func hitLine(h *pluxv1.SearchHit) string {
	return fmt.Sprintf("%s\t%s\t%s\t%s\n", h.GetKind(), h.GetName(), h.GetPath(), h.GetSnippet())
}

// search runs a full-text search over the caller's readable content
// (SRV-061) and prints its first page of hits.
func (e env) search(args []string) int {
	var kinds string
	var c common
	set := flag.NewFlagSet("search", flag.ContinueOnError)
	c.register(set, false)
	set.StringVar(&kinds, "kind", "", "restrict the hits to these `kinds`, comma-separated")
	rest, code, ok := parseInterleaved(set, args, e.stderr, searchUsage)
	if !ok {
		return code
	}
	if len(rest) != 1 {
		set.Usage()
		return exitUsage
	}
	if err := c.resolve(); err != nil {
		return e.fail("search", err)
	}
	cl, err := e.connect(c, true)
	if err != nil {
		return e.fail("search", err)
	}
	res, err := cl.search.Search(context.Background(), connect.NewRequest(&pluxv1.SearchRequest{
		Query: rest[0], Kinds: splitList(kinds), Page: &pluxv1.Page{PageSize: searchPageSize},
	}))
	if err != nil {
		return e.fail("search", err)
	}
	return e.printHits(c.json, res.Msg.GetHits())
}

// printHits prints search hits as JSON or as text lines.
func (e env) printHits(jsonOut bool, hits []*pluxv1.SearchHit) int {
	views := make([]map[string]any, 0, len(hits))
	var text strings.Builder
	for _, h := range hits {
		views = append(views, hitView(h))
		text.WriteString(hitLine(h))
	}
	if len(hits) == 0 {
		text.WriteString("No matches.\n")
	}
	return e.emit(jsonOut, map[string]any{"hits": views}, text.String())
}

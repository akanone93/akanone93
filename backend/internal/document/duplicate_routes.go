// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package document

import (
	"context"
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// routeOpeners are the actions whose input names a route (schema/actions): their targets follow a renamed route.
var routeOpeners = map[string]bool{"navigate": true, "openDialog": true, "openBottomSheet": true}

// maxRouteLength is the longest route name the schema accepts (common.schema.json routeName).
const maxRouteLength = 100

// appRoutes is every route name the app's draft uses: each page's route, or its key when it names none, which is
// the route the compiler gives it (SCH-025).
func (s *Service) appRoutes(ctx context.Context, p auth.Principal, appID string) (map[string]bool, error) {
	out := map[string]bool{}
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		d, err := s.resolve(ctx, q, p, appID, "")
		if err != nil {
			return err
		}
		fsys, err := s.project(ctx, q, d)
		if err != nil {
			return err
		}
		for name, f := range fsys {
			if !isPagePath(name) {
				continue
			}
			if r := pageRoute(f.Data); r != "" {
				out[r] = true
			}
		}
		return nil
	})
	return out, err
}

// isPagePath reports whether a project path holds a page document.
func isPagePath(p string) bool {
	return strings.HasPrefix(p, "plugins/") && strings.Contains(p, "/pages/") && strings.HasSuffix(p, ".page.json")
}

// pageRoute is a page document's route: its route member, else its key.
func pageRoute(content []byte) string {
	v, err := decodeJSON(content)
	if err != nil {
		return ""
	}
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	if r, ok := m["route"].(string); ok && r != "" {
		return r
	}
	k, _ := m["key"].(string)
	return k
}

// copyRoutes gives every page a duplicate copies a route of its own: route names are unique in the app (SCH-025),
// so a copy that kept them would clash with its source at once (PLX-1021). Each becomes `<route>-copy`, then
// `<route>-copy-2` and so on, the first name nobody uses; the map says which route became which.
func copyRoutes(docs []Document, from string, taken map[string]bool) map[string]string {
	out := map[string]string{}
	for _, d := range docs {
		if !strings.HasPrefix(d.Path, from) || !isPagePath(d.Path) {
			continue
		}
		r := pageRoute(d.Content)
		if r == "" {
			continue
		}
		if _, done := out[r]; done {
			continue
		}
		next := freeRoute(r, taken)
		taken[next] = true
		out[r] = next
	}
	return out
}

// freeRoute is the first of `<route>-copy`, `<route>-copy-2`, … that is not taken, shortened to the schema's length.
func freeRoute(route string, taken map[string]bool) string {
	for n := 1; ; n++ {
		suffix := "-copy"
		if n > 1 {
			suffix += "-" + strconv.Itoa(n)
		}
		base := route
		if len(base)+len(suffix) > maxRouteLength {
			base = strings.TrimRight(base[:maxRouteLength-len(suffix)], "-")
		}
		if candidate := base + suffix; !taken[candidate] {
			return candidate
		}
	}
}

// retargetRoutes sets a copied page's own route and points every action of a copied document that opens one of the
// source's routes at the copy's: the copy's links stay inside the copy, as they were inside the source. Routes the
// source did not own (other plugins, native routes) are left as they are.
func retargetRoutes(content []byte, docPath string, routes map[string]string) ([]byte, error) {
	if len(routes) == 0 {
		return content, nil
	}
	v, err := decodeJSON(content)
	if err != nil {
		return content, nil //nolint:nilerr // not JSON: copied as it is
	}
	if m, ok := v.(map[string]any); ok && isPagePath(docPath) {
		own := pageRoute(content)
		if next, ok := routes[own]; ok {
			m["route"] = next
		}
	}
	walkRoutes(v, routes)
	out, err := encodeJSON(v)
	if err != nil {
		return nil, fmt.Errorf("document: rewrite routes of %s: %w", path.Base(docPath), err)
	}
	return out, nil
}

func walkRoutes(v any, routes map[string]string) {
	switch t := v.(type) {
	case map[string]any:
		retargetAction(t, routes)
		for _, child := range t {
			walkRoutes(child, routes)
		}
	case []any:
		for _, child := range t {
			walkRoutes(child, routes)
		}
	}
}

// retargetAction points one action that opens a route at the route it became.
func retargetAction(m map[string]any, routes map[string]string) {
	if action, _ := m["action"].(string); !routeOpeners[action] {
		return
	}
	input, ok := m["input"].(map[string]any)
	if !ok {
		return
	}
	if r, ok := input["route"].(string); ok {
		if next, ok := routes[r]; ok {
			input["route"] = next
		}
	}
}

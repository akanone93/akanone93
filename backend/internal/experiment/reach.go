// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package experiment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/document"
	"github.com/nightCode42/plux3/backend/internal/schema"
)

// variantType is the node type that shows one subtree per variant.
const variantType = "Variant"

// reachedKinds are the document kinds whose trees can hold a Variant node or
// a PXL binding of an experiment.
var reachedKinds = map[schema.DocumentKind]bool{
	schema.KindApp: true, schema.KindPlugin: true, schema.KindPage: true, schema.KindComponent: true,
	schema.KindTemplate: true, schema.KindActionGraph: true,
}

// launchRisk computes the risk attributes of starting an experiment from the
// app's draft (ABT-007). A nil documents service, which only a service built
// without promotion has, reports none.
func (s *Service) launchRisk(ctx context.Context, p auth.Principal, e Experiment) ([]schema.PolicyRisk, error) {
	if s.o.Documents == nil {
		return nil, nil
	}
	files, err := s.o.Documents.Export(ctx, p, e.AppID, "")
	if err != nil {
		return nil, fmt.Errorf("experiment: read the draft to rate the launch: %w", err)
	}
	return reachRisk(files, e.Key), nil
}

// scope is what the documents of one plugin, or of the app, contribute to the
// reach of an experiment.
type scope struct {
	// uses is whether a document of the scope shows the experiment other than
	// on a page.
	uses bool
	// securePages is whether a page of the scope is marked secure, and
	// moneyPages whether a page declares a data source with a transaction
	// mapping.
	securePages, moneyPages bool
	// money is whether the plugin, or the app, declares such a data source.
	money bool
	// pages lists the pages that show the experiment, as secure and money
	// flags.
	pages []pageReach
}

// pageReach is a page that shows an experiment.
type pageReach struct{ secure, money bool }

// reachRisk decides which risk attributes an experiment carries: a page
// marked secure shows it (touchesSecurePage), or the plugin or app a page,
// component, template or action graph that shows it belongs to declares a data
// source with a transaction mapping (touchesMoneyFlow). Where the experiment
// appears in a component, template or action graph, which pages show it
// cannot be told, so every page of the plugin (every page of the app for an
// app-level document) is taken to. The result is sorted.
func reachRisk(files []document.File, key string) []schema.PolicyRisk {
	scopes := map[string]*scope{}
	at := func(plugin string) *scope {
		if scopes[plugin] == nil {
			scopes[plugin] = &scope{}
		}
		return scopes[plugin]
	}
	for _, f := range files {
		place, ok := schema.PlaceOf(f.Path)
		if !ok || !reachedKinds[place.Kind] {
			continue
		}
		sc := at(place.Plugin)
		switch place.Kind {
		case schema.KindApp:
			sc.money = sc.money || appMoney(f.Content)
			sc.uses = sc.uses || mentions(f.Content, key)
		case schema.KindPlugin:
			sc.money = sc.money || pluginMoney(f.Content)
			sc.uses = sc.uses || mentions(f.Content, key)
		case schema.KindPage:
			page := readPage(f.Content)
			sc.securePages = sc.securePages || page.secure
			sc.moneyPages = sc.moneyPages || page.money
			if mentions(f.Content, key) {
				sc.pages = append(sc.pages, page)
			}
		default:
			sc.uses = sc.uses || mentions(f.Content, key)
		}
	}
	return risksOf(scopes)
}

// risksOf combines the scopes: the app scope ("") reaches every plugin.
func risksOf(scopes map[string]*scope) []schema.PolicyRisk {
	appMoneyFlow := scopes[""] != nil && scopes[""].money
	var secure, money bool
	for plugin, sc := range scopes {
		for _, pg := range sc.pages {
			secure = secure || pg.secure
			money = money || pg.money || sc.money || appMoneyFlow
		}
		if !sc.uses {
			continue
		}
		for other, reached := range scopes {
			if plugin != "" && other != plugin {
				continue
			}
			secure = secure || reached.securePages
			money = money || reached.money || reached.moneyPages || appMoneyFlow
		}
	}
	var out []schema.PolicyRisk
	if secure {
		out = append(out, schema.PolicyRiskTouchesSecurePage)
	}
	if money {
		out = append(out, schema.PolicyRiskTouchesMoneyFlow)
	}
	slices.Sort(out)
	return out
}

// readPage decodes the facts of a page that matter to risk; a page that does
// not decode is neither secure nor money (the loader refuses it when the
// draft is compiled).
func readPage(data []byte) pageReach {
	var doc schema.PageDocument
	if json.Unmarshal(data, &doc) != nil {
		return pageReach{}
	}
	secure := doc.Security != nil && doc.Security.Secure != nil && *doc.Security.Secure
	return pageReach{secure: secure, money: hasTransaction(doc.DataSources)}
}

// pluginMoney reports whether a plugin document declares a data source with
// a transaction mapping.
func pluginMoney(data []byte) bool {
	var doc schema.PluginDocument
	return json.Unmarshal(data, &doc) == nil && hasTransaction(doc.DataSources)
}

// appMoney reports whether the app document declares a data source with a
// transaction mapping.
func appMoney(data []byte) bool {
	var doc schema.AppDocument
	return json.Unmarshal(data, &doc) == nil && hasTransaction(doc.DataSources)
}

// hasTransaction reports whether any data source maps a transaction, which is
// how a flow that involves money is marked (SEC-028).
func hasTransaction(sources []schema.DataSource) bool {
	for _, d := range sources {
		if d.Transaction != nil {
			return true
		}
	}
	return false
}

// mentions reports whether a document shows the experiment: a Variant node
// naming it, or a PXL binding `experiment.<key>`.
func mentions(data []byte, key string) bool {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var doc any
	return dec.Decode(&doc) == nil && shows(doc, key)
}

// shows walks a decoded document for a use of the experiment.
func shows(v any, key string) bool {
	switch x := v.(type) {
	case []any:
		for _, e := range x {
			if shows(e, key) {
				return true
			}
		}
	case map[string]any:
		if x["type"] == variantType && x["experiment"] == key {
			return true
		}
		for k, e := range x {
			if src, isString := e.(string); isString && k == "$expr" && binds(src, key) {
				return true
			}
			if shows(e, key) {
				return true
			}
		}
	}
	return false
}

// binds reports whether a PXL expression reads `experiment.<key>`. It matches
// on the text, so an expression that only quotes the binding in a string is
// counted too, which errs towards asking for an approval.
func binds(src, key string) bool {
	needle := "experiment." + key
	for rest, offset := src, 0; ; {
		i := strings.Index(rest, needle)
		if i < 0 {
			return false
		}
		start, end := offset+i, offset+i+len(needle)
		if !identChar(src, start-1) && !identChar(src, end) && !(start > 0 && src[start-1] == '.') {
			return true
		}
		rest, offset = rest[i+1:], offset+i+1
	}
}

// identChar reports whether the byte at i of s continues an identifier.
func identChar(s string, i int) bool {
	if i < 0 || i >= len(s) {
		return false
	}
	c := s[i]
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

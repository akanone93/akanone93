// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package walkthrough

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// gatewayKinds are the data-source kinds that go through the gateway unless
// they are declared direct.
var gatewayKinds = []string{"rest", "graphql", "websocket", "sse"}

// sourceDocument is what a project document says about data sources and
// environment values.
type sourceDocument struct {
	DataSources []struct {
		Name, Kind, Route string
		Transaction       map[string]string
		Config            struct {
			BaseURL    string
			Operations map[string]struct{ Method, Path string }
		}
	}
	Environments []struct {
		Key    string
		Values map[string]any
	}
}

// gatewaySource is what an upstream is registered from: the variable of the
// source's base URL and the operations devices may call.
type gatewaySource struct {
	baseVariable string
	operations   []map[string]any
}

// projectSources reads the project's gateway data sources and its
// environment values.
func projectSources(project string) (map[string]gatewaySource, map[string]map[string]string, error) {
	sources, values := map[string]gatewaySource{}, map[string]map[string]string{}
	err := filepath.WalkDir(project, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".json" {
			return err
		}
		data, err := os.ReadFile(path) //nolint:gosec // inside the project the run copied
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		var doc sourceDocument
		if json.Unmarshal(data, &doc) == nil {
			doc.addTo(sources, values)
		}
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("read the project's data sources: %w", err)
	}
	return sources, values, nil
}

// addTo adds the document's gateway sources and string values.
// Each operation is registered with its method and the literal start of its
// path; a write of a source that declares a transaction is registered as a
// transaction with the same fields, which the release check asks for
// (PLX-6072).
func (doc sourceDocument) addTo(sources map[string]gatewaySource, values map[string]map[string]string) {
	for _, s := range doc.DataSources {
		if !slices.Contains(gatewayKinds, s.Kind) || s.Route == "direct" {
			continue
		}
		src := gatewaySource{baseVariable: s.Config.BaseURL}
		for _, name := range slices.Sorted(maps.Keys(s.Config.Operations)) {
			op := s.Config.Operations[name]
			method := cmp.Or(strings.ToUpper(op.Method), "POST")
			prefix, _, _ := strings.Cut(op.Path, "{")
			registered := map[string]any{"name": name, "methods": []string{method}, "pathPrefix": strings.TrimSuffix(prefix, "/")}
			if t := s.Transaction; t != nil && method != "GET" {
				registered["transaction"] = map[string]any{"amount": t["amount"], "currency": t["currency"], "payee": t["payee"], "summary": t["summary"]}
			}
			src.operations = append(src.operations, registered)
		}
		sources[s.Name] = src
	}
	for _, e := range doc.Environments {
		for k, v := range e.Values {
			if text, ok := v.(string); ok {
				if values[e.Key] == nil {
					values[e.Key] = map[string]string{}
				}
				values[e.Key][k] = text
			}
		}
	}
}

// environmentIDs maps the app's environment keys to their IDs.
func (e *Env) environmentIDs(ctx context.Context, w *walk) (map[string]string, error) {
	listed, err := w.rpc(ctx, e.Admin, appService+"ListEnvironments", map[string]any{"appId": e.App})
	if err != nil {
		return nil, err
	}
	ids := map[string]string{}
	list, _ := listed["environments"].([]any)
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			key, _ := m["key"].(string)
			id, _ := m["id"].(string)
			ids[key] = id
		}
	}
	return ids, nil
}

// registerUpstreams registers, as the administrator, an upstream for each of
// the project's gateway data sources in every environment a release of step
// 2 reaches, as an operator does before the app ships (SEC-031): a release
// whose sources have no upstream is refused (PLX-6060). The synthetic fleet
// never calls them, so they need no credential; an environment without its
// own values takes production's.
func (e *Env) registerUpstreams(ctx context.Context, w *walk) error {
	sources, values, err := projectSources(e.Project)
	if err != nil || len(sources) == 0 {
		return err
	}
	ids, err := e.environmentIDs(ctx, w)
	if err != nil {
		return err
	}
	for _, env := range []string{publishEnv, e.FleetEnv, e.ProdEnv} {
		id := ids[env]
		if id == "" {
			return fmt.Errorf("the app has no environment %q", env)
		}
		vals := values[env]
		if vals == nil {
			vals = values[e.ProdEnv]
		}
		for _, name := range slices.Sorted(maps.Keys(sources)) {
			base := vals[sources[name].baseVariable]
			if base == "" {
				return errors.New("the project has no base URL for the data source " + name)
			}
			if _, err := w.rpc(ctx, e.Admin, securityAdminService+"CreateUpstream", map[string]any{"upstream": map[string]any{
				"appId": e.App, "environmentId": id, "name": name, "baseUrl": base,
				"allowedMethods": []string{"GET", "POST", "PUT", "PATCH", "DELETE"},
				"operations":     sources[name].operations,
			}}); err != nil {
				return err
			}
		}
	}
	return nil
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package walkthrough

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
)

// gatewayKinds are the data-source kinds that go through the gateway unless
// they are declared direct.
var gatewayKinds = []string{"rest", "graphql", "websocket", "sse"}

// sourceDocument is what a project document says about data sources and
// environment values.
type sourceDocument struct {
	DataSources []struct {
		Name, Kind, Route string
		Config            struct{ BaseURL string }
	}
	Environments []struct {
		Key    string
		Values map[string]any
	}
}

// projectSources reads the project's gateway data sources (name to the
// variable of its base URL) and its environment values.
func projectSources(project string) (map[string]string, map[string]map[string]string, error) {
	sources, values := map[string]string{}, map[string]map[string]string{}
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
func (doc sourceDocument) addTo(sources map[string]string, values map[string]map[string]string) {
	for _, s := range doc.DataSources {
		if slices.Contains(gatewayKinds, s.Kind) && s.Route != "direct" {
			sources[s.Name] = s.Config.BaseURL
		}
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
			base := vals[sources[name]]
			if base == "" {
				return errors.New("the project has no base URL for the data source " + name)
			}
			if _, err := w.rpc(ctx, e.Admin, securityAdminService+"CreateUpstream", map[string]any{"upstream": map[string]any{
				"appId": e.App, "environmentId": id, "name": name, "baseUrl": base,
				"allowedMethods": []string{"GET", "POST", "PUT", "PATCH", "DELETE"},
			}}); err != nil {
				return err
			}
		}
	}
	return nil
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package document

import (
	"context"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/library"
	"github.com/nightCode42/plux3/backend/internal/schema/jcs"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

var _ library.Instances = (*Service)(nil)

// ComponentInstances finds every node of the app's drafts that instantiates
// the component with the document identifier componentID, with the names of
// the props it overrides and the slots it fills, in file and path order
// (SCH-030, STU-084). Documents that are not JSON are skipped: the compiler
// reports them.
func (s *Service) ComponentInstances(ctx context.Context, p auth.Principal, appID, componentID string) ([]library.Instance, error) {
	var out []library.Instance
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		d, err := s.resolve(ctx, q, p, appID, "")
		if err != nil {
			return err
		}
		if err := authorize(p, auth.PluginRead, appID); err != nil {
			return err
		}
		lim, err := s.limitsFor(ctx, tx, d)
		if err != nil {
			return err
		}
		fsys, err := s.project(ctx, q, d)
		if err != nil {
			return err
		}
		return fs.WalkDir(fsys, ".", func(name string, e fs.DirEntry, err error) error {
			if err != nil || e.IsDir() || !strings.HasSuffix(name, ".json") {
				return err
			}
			data, err := fs.ReadFile(fsys, name)
			if err != nil {
				return fmt.Errorf("document: read %s: %w", name, err)
			}
			tree, err := jcs.Parse(data, int(lim.Get(limits.DocumentJSONDepth)))
			if err != nil {
				return nil //nolint:nilerr // the compiler reports a document that is not JSON
			}
			findInstances(tree, "", name, componentID, &out)
			return nil
		})
	})
	slices.SortFunc(out, func(a, b library.Instance) int {
		return strings.Compare(a.File+"\x00"+a.Path, b.File+"\x00"+b.Path)
	})
	return out, err
}

func findInstances(v any, pointer, file, componentID string, out *[]library.Instance) {
	switch t := v.(type) {
	case map[string]any:
		if ref, ok := t["component"].(map[string]any); ok && ref["id"] == componentID {
			*out = append(*out, library.Instance{File: file, Path: pointer, Props: sortedKeys(t["props"]), Slots: sortedKeys(t["slots"])})
		}
		for k, child := range t {
			findInstances(child, pointer+"/"+strings.NewReplacer("~", "~0", "/", "~1").Replace(k), file, componentID, out)
		}
	case []any:
		for i, child := range t {
			findInstances(child, fmt.Sprintf("%s/%d", pointer, i), file, componentID, out)
		}
	}
}

// sortedKeys returns the keys of an object, or none for any other value.
func sortedKeys(v any) []string {
	m, _ := v.(map[string]any)
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

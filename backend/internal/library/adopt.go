// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package library

import (
	"cmp"
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/jcs"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// Adopt records the version of a component an app uses (STU-085). It is
// explicit and per app, and an item of another organisation can be adopted
// only when it is public. The instances whose overrides the version no
// longer fits are returned, never silently dropped: adopting does not change
// any document.
func (s *Service) Adopt(ctx context.Context, p auth.Principal, appID, id string, version int32) (Adoption, []Conflict, error) {
	if err := p.AuthorizeApp(auth.PluginEdit, appID); err != nil {
		return Adoption{}, nil, err //nolint:wrapcheck // a domain error
	}
	app, err := parseID(appID, "app")
	if err != nil {
		return Adoption{}, nil, err
	}
	var out Adoption
	var props, slots []string
	var documentID string
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		// Row-level security shows only the organisation's own apps.
		if _, err := q.GetApp(ctx, app); err != nil {
			return failure(err, "app")
		}
		it, err := load(ctx, q, p, KindComponent, id)
		if err != nil {
			return err
		}
		if it.OrganizationID != p.OrganizationID && it.Visibility != Public {
			return plxerr.New(plxerr.ResourceNotFound, "no such component")
		}
		v, err := q.GetLibraryVersion(ctx, dbgen.GetLibraryVersionParams{ItemID: storage.MustUUID(it.ID), Version: version})
		if err != nil {
			return failure(err, "component version")
		}
		lim, err := s.limitsFor(ctx, tx, p)
		if err != nil {
			return err
		}
		tree, err := jcs.Parse(v.Content, int(lim.Get(limits.DocumentJSONDepth)))
		if err != nil {
			return plxerr.Wrap(plxerr.InvalidFormat, err, "the stored version is not JSON")
		}
		props, slots = namesOf(tree, "props"), namesOf(tree, "slots")
		documentID = it.DocumentID
		row, err := q.UpsertLibraryAdoption(ctx, dbgen.UpsertLibraryAdoptionParams{
			AppID: app, ItemID: storage.MustUUID(it.ID), OrganizationID: storage.MustUUID(p.OrganizationID), Version: version, AdoptedBy: p.UserID,
		})
		if err != nil {
			return failure(err, "adoption")
		}
		out = adoptionOf(row)
		return s.record(ctx, tx, p, audit.LibraryVersionAdopted, it, nil, v.Content, "app "+appID+" adopted version "+itoa(version))
	})
	if err != nil {
		return Adoption{}, nil, err
	}
	if s.o.Instances == nil {
		return out, nil, nil
	}
	instances, err := s.o.Instances.ComponentInstances(ctx, p, appID, documentID)
	if err != nil {
		return Adoption{}, nil, err //nolint:wrapcheck // a domain error
	}
	return out, ConflictsOf(instances, props, slots), nil
}

// ConflictsOf returns the instances that override a prop or fill a slot the
// version no longer has, in file and path order.
func ConflictsOf(instances []Instance, props, slots []string) []Conflict {
	var out []Conflict
	for _, in := range instances {
		c := Conflict{File: in.File, Path: in.Path, RemovedProps: missing(in.Props, props), RemovedSlots: missing(in.Slots, slots)}
		if len(c.RemovedProps) > 0 || len(c.RemovedSlots) > 0 {
			out = append(out, c)
		}
	}
	slices.SortFunc(out, func(a, b Conflict) int {
		return cmp.Or(strings.Compare(a.File, b.File), strings.Compare(a.Path, b.Path))
	})
	return out
}

// missing returns, sorted, the names that are not in have.
func missing(names, have []string) []string {
	var out []string
	for _, n := range names {
		if !slices.Contains(have, n) {
			out = append(out, n)
		}
	}
	slices.Sort(out)
	return out
}

// namesOf returns the names of the entries of a component's props or slots.
func namesOf(tree any, field string) []string {
	obj, _ := tree.(map[string]any)
	list, _ := obj[field].([]any)
	var out []string
	for _, e := range list {
		if m, ok := e.(map[string]any); ok {
			if n, ok := m["name"].(string); ok {
				out = append(out, n)
			}
		}
	}
	return out
}

// Adoptions lists the versions an app adopted.
func (s *Service) Adoptions(ctx context.Context, p auth.Principal, appID string) ([]Adoption, error) {
	if err := p.AuthorizeApp(auth.PluginRead, appID); err != nil {
		return nil, err //nolint:wrapcheck // a domain error
	}
	app, err := parseID(appID, "app")
	if err != nil {
		return nil, err
	}
	var out []Adoption
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListLibraryAdoptions(ctx, app)
		if err != nil {
			return failure(err, "adoption")
		}
		for _, r := range rows {
			out = append(out, adoptionOf(r))
		}
		return nil
	})
	return out, err
}

func adoptionOf(r dbgen.LibraryAdoption) Adoption {
	return Adoption{AppID: storage.ID(r.AppID), ItemID: storage.ID(r.ItemID), Version: r.Version, AdoptedBy: r.AdoptedBy, AdoptedAt: r.AdoptedAt.Time}
}

func itoa(n int32) string { return strconv.Itoa(int(n)) }

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release

import (
	"bytes"
	"cmp"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// ReleaseFunction is one function version a release pins (FN-006): the
// version an alias, or the newest version, resolved to when the release was
// created.
type ReleaseFunction struct {
	// Name is the dotted function name.
	Name string
	// Version is the immutable function version.
	Version int32
	// ModuleSHA256 is the hex SHA-256 of the version's module.
	ModuleSHA256 string
	// Placement is "server" or "device".
	Placement string
}

// functionGrant is a function a plugin of a release grants, with the alias
// that selects its version; an empty alias selects the newest.
type functionGrant struct {
	name, alias string
	// compiled is the version the compilation resolved the grant to, which
	// the bundle carries; 0 when it compiled without a catalogue.
	compiled uint32
}

// grantsOf lists the functions the plugins of a compilation grant, once
// each and sorted, so that a release resolves them in a fixed order
// (CMP-002).
func grantsOf(res *compiler.Result, cat compiler.FunctionCatalogue) []functionGrant {
	if res == nil || res.Project == nil {
		return nil
	}
	var out []functionGrant
	for _, pl := range res.Project.Plugins {
		if pl.Doc == nil || pl.Doc.Capabilities == nil {
			continue
		}
		for _, g := range pl.Doc.Capabilities.Functions {
			grant := functionGrant{name: g.Function, alias: g.Alias}
			if cat != nil {
				if info, ok := cat.Function(g.Function, g.Alias); ok {
					grant.compiled = info.Version
				}
			}
			out = append(out, grant)
		}
	}
	slices.SortFunc(out, func(a, b functionGrant) int {
		return cmp.Or(cmp.Compare(a.name, b.name), cmp.Compare(a.alias, b.alias))
	})
	return slices.Compact(out)
}

// pinFunctions resolves every granted function in the transaction that
// records the release and stores what it resolved (FN-006). A function
// that is not deployed in the environment (PLX-7012), an alias that does
// not exist (PLX-7013) or, in a production environment, a version whose
// tests failed (PLX-7020, FN-031) refuses the release. Two plugins that reach one function through aliases that
// stand for different versions leave the release without one behaviour, so
// that is refused as well.
func (s *Service) pinFunctions(ctx context.Context, tx pgx.Tx, env dbgen.Environment, releaseID pgtype.UUID, grants []functionGrant) ([]ReleaseFunction, error) {
	q := dbgen.New(tx)
	var pinned []ReleaseFunction
	for _, g := range grants {
		r, err := function.ResolveVersionTx(ctx, tx, storage.ID(env.AppID), storage.ID(env.ID), g.name, g.alias)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		if g.compiled != 0 && uint32(max(r.Version, 0)) != g.compiled { //nolint:gosec // G115: clamped to non-negative.
			return nil, plxerr.New(plxerr.ReleaseInconsistent, "function %s was deployed as version %d while the release was created, after its bundles were compiled against version %d; create the release again", g.name, r.Version, g.compiled)
		}
		if !r.TestsPassed && env.Production {
			return nil, plxerr.New(plxerr.FunctionTestsFailed, "version %d of function %s failed its tests and cannot be released", r.Version, g.name)
		}
		if i := slices.IndexFunc(pinned, func(p ReleaseFunction) bool { return p.Name == g.name }); i >= 0 {
			if pinned[i].Version != r.Version {
				return nil, plxerr.New(plxerr.ReleaseInconsistent, "plugins reach function %s as versions %d and %d; a release pins one", g.name, pinned[i].Version, r.Version)
			}
			continue
		}
		err = q.InsertReleaseFunction(ctx, dbgen.InsertReleaseFunctionParams{
			ReleaseID: releaseID, OrganizationID: env.OrganizationID, AppID: env.AppID, EnvironmentID: env.ID,
			Name: g.name, Version: r.Version, ModuleSha256: r.ModuleSHA256, Placement: string(r.Placement),
		})
		if err != nil {
			return nil, failure(err, "release")
		}
		pinned = append(pinned, ReleaseFunction{Name: g.name, Version: r.Version, ModuleSHA256: hex.EncodeToString(r.ModuleSHA256), Placement: string(r.Placement)})
	}
	return pinned, nil
}

// requirePromoted refuses to point a channel of env at a release unless
// every function version the release pins is held by env: the same name,
// the same version and the same module (FN-006). A version reaches another
// environment only by plux fn promote, which copies it without a rebuild,
// so a release never runs a build its environment was not given. In a
// production environment a version whose tests failed is refused as well
// (FN-031).
func requirePromoted(ctx context.Context, q *dbgen.Queries, env dbgen.Environment, releaseID pgtype.UUID) error {
	pins, err := q.ListReleaseFunctions(ctx, []pgtype.UUID{releaseID})
	if err != nil {
		return fmt.Errorf("release: list pinned functions: %w", err)
	}
	for _, pin := range pins {
		held, err := q.GetFunctionVersion(ctx, dbgen.GetFunctionVersionParams{EnvironmentID: env.ID, Name: pin.Name, Version: pin.Version})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("release: read a function version: %w", err)
		}
		if err != nil || !bytes.Equal(held.ModuleSha256, pin.ModuleSha256) {
			return plxerr.New(plxerr.FunctionNotFound, "the release pins version %d of function %s, which environment %s does not hold; run plux fn promote %s@%d --from <environment> --to %s first", pin.Version, pin.Name, env.Key, pin.Name, pin.Version, env.Key)
		}
		if env.Production && !held.TestsPassed {
			return plxerr.New(plxerr.FunctionTestsFailed, "version %d of function %s failed its tests and cannot be released to the production environment %s", pin.Version, pin.Name, env.Key)
		}
	}
	return nil
}

// copyFunctions gives a release the function versions of an earlier one: a
// rollback reproduces the release, so it pins what the release pinned, not
// what the aliases stand for now (REL-006, FN-006).
func copyFunctions(ctx context.Context, q *dbgen.Queries, to, from pgtype.UUID) error {
	if err := q.CopyReleaseFunctions(ctx, dbgen.CopyReleaseFunctionsParams{ReleaseID: to, FromReleaseID: from}); err != nil {
		return failure(err, "release")
	}
	return nil
}

// functionsOf reads the function versions the releases pin, by release ID.
func functionsOf(ctx context.Context, q *dbgen.Queries, ids ...pgtype.UUID) (map[string][]ReleaseFunction, error) {
	rows, err := q.ListReleaseFunctions(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("release: list pinned functions: %w", err)
	}
	out := map[string][]ReleaseFunction{}
	for _, r := range rows {
		id := storage.ID(r.ReleaseID)
		out[id] = append(out[id], ReleaseFunction{Name: r.Name, Version: r.Version, ModuleSHA256: hex.EncodeToString(r.ModuleSha256), Placement: r.Placement})
	}
	return out, nil
}

// withFunctions fills the pinned functions of a release.
func withFunctions(ctx context.Context, q *dbgen.Queries, rel *Release, id pgtype.UUID) error {
	byRelease, err := functionsOf(ctx, q, id)
	if err != nil {
		return err
	}
	rel.Functions = byRelease[rel.ID]
	return nil
}

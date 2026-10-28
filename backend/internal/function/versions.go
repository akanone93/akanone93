// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// Deploy turns a build into the next version of its function and, when
// alias is set, points the alias at it (FN-006). Only a succeeded build
// with the builder's signature and passing tests is deployable (FN-009,
// FN-031). A production environment needs function.deploy.production,
// which requires a second factor; any other needs function.deploy (FN-007).
func (s *Service) Deploy(ctx context.Context, p auth.Principal, buildID, alias string) (Version, error) {
	if alias != "" {
		if err := ValidateAlias(alias); err != nil {
			return Version{}, err
		}
	}
	id, err := parseID(buildID, "build")
	if err != nil {
		return Version{}, err
	}
	var out Version
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, err := q.GetFunctionBuild(ctx, id)
		if err != nil {
			return failure(err, "function build")
		}
		env, err := environmentOf(ctx, q, storage.ID(row.AppID), storage.ID(row.EnvironmentID))
		if err != nil {
			return err
		}
		if err := authorize(p, deployPermission(env.Production), storage.ID(row.AppID)); err != nil {
			return err
		}
		build, err := buildOf(row)
		if err != nil {
			return err
		}
		if err := checkDeployable(build, env.Production); err != nil {
			return err
		}
		if err := s.requireDeployment(ctx, tx, p, deployment{
			app: storage.ID(row.AppID), env: env, name: row.Name, subject: "function:" + row.Name + "@build:" + storage.ID(row.ID),
			content: deployContent(row.Name, 0, row.ModuleSha256, row.Placement, alias),
			summary: "function " + row.Name + " is deployed to " + env.Key,
		}); err != nil {
			return err
		}
		out, err = s.insertVersion(ctx, tx, p, row, alias, testsPassed(build))
		return err
	})
	if err != nil {
		return out, err
	}
	s.prepare(ctx, p.OrganizationID, out)
	return out, nil
}

// insertVersion assigns the next number of the function in its app under
// the function's lock, stores the version and the alias, and audits the
// deploy. The number is counted across the app's environments, so one
// number names one signed build everywhere (FN-006).
func (s *Service) insertVersion(ctx context.Context, tx pgx.Tx, p auth.Principal, row dbgen.FunctionBuild, alias string, passed bool) (Version, error) {
	q := dbgen.New(tx)
	info, err := manifestEntry(row)
	if err != nil {
		return Version{}, err
	}
	if err := q.LockFunction(ctx, lockScope("deploy", row.AppID, row.Name)); err != nil {
		return Version{}, fmt.Errorf("function: lock a function: %w", err)
	}
	next, err := q.NextFunctionVersion(ctx, dbgen.NextFunctionVersionParams{AppID: row.AppID, Name: row.Name})
	if err != nil {
		return Version{}, fmt.Errorf("function: next version: %w", err)
	}
	v, err := q.InsertFunctionVersion(ctx, dbgen.InsertFunctionVersionParams{
		OrganizationID: row.OrganizationID, AppID: row.AppID, EnvironmentID: row.EnvironmentID, Name: row.Name,
		Version: next, BuildID: row.ID, Placement: row.Placement, ModuleKey: deref(row.ModuleKey),
		ModuleSha256: row.ModuleSha256, AbiVersion: derefInt(row.AbiVersion), InputSchema: info.InputSchema,
		OutputSchema: info.OutputSchema, Provenance: row.Provenance, Statement: row.Statement, Signature: row.Signature,
		KeyID: deref(row.KeyID), TestsPassed: passed, CreatedBy: createdBy(p),
	})
	if err != nil {
		return Version{}, failure(err, "function version")
	}
	detail := fmt.Sprintf("function %s, version %d, build %s, module %x", row.Name, next, storage.ID(row.ID), row.ModuleSha256)
	if alias != "" {
		if _, err := q.UpsertFunctionAlias(ctx, aliasParams(p, v, alias)); err != nil {
			return Version{}, failure(err, "function alias")
		}
		detail += ", alias " + alias
	}
	target := storage.ID(row.EnvironmentID) + "/" + row.Name
	if err := s.record(ctx, tx, p, audit.FunctionDeployed, "function", target, detail); err != nil {
		return Version{}, err
	}
	return versionWithBuild(ctx, q, v)
}

// lockScope names a lock that serialises the work of one kind on one
// function: its deploys and promotions, which number versions per app, or
// the writes of its key-value store, which is per environment. scope is
// the app or the environment.
func lockScope(kind string, scope pgtype.UUID, name string) string {
	return "function:" + kind + ":" + storage.ID(scope) + ":" + name
}

// aliasParams builds the row of an alias that points at a version.
func aliasParams(p auth.Principal, v dbgen.FunctionVersion, alias string) dbgen.UpsertFunctionAliasParams {
	return dbgen.UpsertFunctionAliasParams{
		OrganizationID: v.OrganizationID, AppID: v.AppID, EnvironmentID: v.EnvironmentID, Name: v.Name,
		Alias: alias, Version: v.Version, UpdatedBy: createdBy(p),
	}
}

// manifestEntry reads the schemas of the build's own function from the
// manifest the builder reported.
func manifestEntry(row dbgen.FunctionBuild) (Export, error) {
	var fns []Export
	if err := json.Unmarshal(row.FunctionsManifest, &fns); err != nil {
		return Export{}, fmt.Errorf("function: read the function manifest of a build: %w", err)
	}
	info, ok := infoOf(fns, row.Name)
	if !ok {
		return Export{}, plxerr.New(plxerr.FunctionDeclarationMismatch, "the build does not export the function %s", row.Name)
	}
	return info, nil
}

// Promote copies a version of a function from one environment of an app to
// another without rebuilding it, so the target holds the same number, the
// same module and the same signature (FN-006). It needs the permission a
// deploy needs in the target, because the copy changes what the target can
// release. A production target refuses a version whose tests failed
// (FN-031). Promoting a version the target already holds is a no-op, and
// alias, when set, is pointed at the version either way.
func (s *Service) Promote(ctx context.Context, p auth.Principal, appID, fromEnvID, toEnvID, name string, version int32, alias string) (Version, error) {
	if err := validatePromotion(name, alias, version); err != nil {
		return Version{}, err
	}
	var out Version
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = s.promoteTx(ctx, tx, p, appID, fromEnvID, toEnvID, name, version, alias)
		return err
	})
	if err != nil {
		return out, err
	}
	s.prepare(ctx, p.OrganizationID, out)
	return out, nil
}

// validatePromotion checks the names and the number a promotion is asked for.
func validatePromotion(name, alias string, version int32) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	if alias != "" {
		if err := ValidateAlias(alias); err != nil {
			return err
		}
	}
	if version < 1 {
		return plxerr.New(plxerr.InvalidFormat, "a function version is a positive number")
	}
	return nil
}

// promoteTx checks the permission, the tests and the approval policy of a
// promotion and copies the version, in the transaction of the request.
func (s *Service) promoteTx(ctx context.Context, tx pgx.Tx, p auth.Principal, appID, fromEnvID, toEnvID, name string, version int32, alias string) (Version, error) {
	q := dbgen.New(tx)
	from, err := environmentOf(ctx, q, appID, fromEnvID)
	if err != nil {
		return Version{}, err
	}
	to, err := environmentOf(ctx, q, appID, toEnvID)
	if err != nil {
		return Version{}, err
	}
	if from.ID == to.ID {
		return Version{}, plxerr.New(plxerr.InvalidFormat, "the source and the target of a promotion are the same environment")
	}
	if err := authorize(p, deployPermission(to.Production), appID); err != nil {
		return Version{}, err
	}
	src, err := q.GetFunctionVersion(ctx, dbgen.GetFunctionVersionParams{EnvironmentID: from.ID, Name: name, Version: version})
	if err != nil {
		return Version{}, versionMissing(err, name, version)
	}
	if !src.TestsPassed && to.Production {
		return Version{}, plxerr.New(plxerr.FunctionTestsFailed, "version %d of function %s failed its tests and cannot be promoted to a production environment", version, name)
	}
	if err := s.requireDeployment(ctx, tx, p, deployment{
		app: appID, env: to, name: name, subject: "function:" + name + "@" + strconv.Itoa(int(version)),
		content: deployContent(name, version, src.ModuleSha256, src.Placement, alias),
		summary: "version " + strconv.Itoa(int(version)) + " of function " + name + " is promoted to " + to.Key,
	}); err != nil {
		return Version{}, err
	}
	return s.copyVersion(ctx, tx, p, src, to, alias)
}

// copyVersion stores a copy of a version in the target environment under
// the function's lock, or finds the copy a promotion made earlier, then
// points the alias at it and audits what changed.
func (s *Service) copyVersion(ctx context.Context, tx pgx.Tx, p auth.Principal, src dbgen.FunctionVersion, to dbgen.Environment, alias string) (Version, error) {
	q := dbgen.New(tx)
	if err := q.LockFunction(ctx, lockScope("deploy", src.AppID, src.Name)); err != nil {
		return Version{}, fmt.Errorf("function: lock a function: %w", err)
	}
	target := storage.ID(to.ID) + "/" + src.Name
	v, err := q.GetFunctionVersion(ctx, dbgen.GetFunctionVersionParams{EnvironmentID: to.ID, Name: src.Name, Version: src.Version})
	held := err == nil
	switch {
	case held:
		if !bytes.Equal(v.ModuleSha256, src.ModuleSha256) {
			return Version{}, plxerr.New(plxerr.PreconditionFailed, "the target environment holds version %d of function %s with a different module", src.Version, src.Name)
		}
		if alias == "" {
			return versionWithBuild(ctx, q, v)
		}
	case errors.Is(err, pgx.ErrNoRows):
		v, err = q.InsertFunctionVersion(ctx, dbgen.InsertFunctionVersionParams{
			OrganizationID: src.OrganizationID, AppID: src.AppID, EnvironmentID: to.ID, Name: src.Name,
			Version: src.Version, BuildID: src.BuildID, Placement: src.Placement, ModuleKey: src.ModuleKey,
			ModuleSha256: src.ModuleSha256, AbiVersion: src.AbiVersion, InputSchema: src.InputSchema,
			OutputSchema: src.OutputSchema, Provenance: src.Provenance, Statement: src.Statement, Signature: src.Signature,
			KeyID: src.KeyID, TestsPassed: src.TestsPassed, CreatedBy: createdBy(p),
		})
		if err != nil {
			return Version{}, failure(err, "function version")
		}
		detail := fmt.Sprintf("function %s, version %d, from environment %s, module %x", src.Name, src.Version, storage.ID(src.EnvironmentID), src.ModuleSha256)
		if alias != "" {
			detail += ", alias " + alias
		}
		if err := s.record(ctx, tx, p, audit.FunctionPromoted, "function", target, detail); err != nil {
			return Version{}, err
		}
	default:
		return Version{}, fmt.Errorf("function: read a version: %w", err)
	}
	if alias != "" {
		if _, err := q.UpsertFunctionAlias(ctx, aliasParams(p, v, alias)); err != nil {
			return Version{}, failure(err, "function alias")
		}
		if held {
			detail := fmt.Sprintf("function %s, alias %s, version %d", src.Name, alias, src.Version)
			if err := s.record(ctx, tx, p, audit.FunctionAliasSet, "function", target, detail); err != nil {
				return Version{}, err
			}
		}
	}
	return versionWithBuild(ctx, q, v)
}

// SetAlias points an alias at an existing version and returns the
// function's aliases (FN-006). It needs the permission a deploy needs in
// that environment, because moving an alias changes what runs.
func (s *Service) SetAlias(ctx context.Context, p auth.Principal, appID, environmentID, name, alias string, version int32) ([]Alias, error) {
	if err := ValidateName(name); err != nil {
		return nil, err
	}
	if err := ValidateAlias(alias); err != nil {
		return nil, err
	}
	var out []Alias
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		env, err := environmentOf(ctx, q, appID, environmentID)
		if err != nil {
			return err
		}
		if err := authorize(p, deployPermission(env.Production), appID); err != nil {
			return err
		}
		v, err := q.GetFunctionVersion(ctx, dbgen.GetFunctionVersionParams{EnvironmentID: env.ID, Name: name, Version: version})
		if err != nil {
			return versionMissing(err, name, version)
		}
		if err := s.requireDeployment(ctx, tx, p, deployment{
			app: appID, env: env, name: name, subject: "function:" + name + "#alias:" + alias,
			content: deployContent(name, version, v.ModuleSha256, v.Placement, alias),
			summary: "alias " + alias + " of function " + name + " moves to version " + strconv.Itoa(int(version)) + " in " + env.Key,
		}); err != nil {
			return err
		}
		if _, err := q.UpsertFunctionAlias(ctx, aliasParams(p, v, alias)); err != nil {
			return failure(err, "function alias")
		}
		detail := fmt.Sprintf("function %s, alias %s, version %d", name, alias, version)
		if err := s.record(ctx, tx, p, audit.FunctionAliasSet, "function", storage.ID(env.ID)+"/"+name, detail); err != nil {
			return err
		}
		out, err = aliasesOf(ctx, q, env.ID, name)
		return err
	})
	return out, err
}

// versionMissing turns a missing row into PLX-7012.
func versionMissing(err error, name string, version int32) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return plxerr.New(plxerr.FunctionNotFound, "the function %s has no version %d", name, version)
	}
	return fmt.Errorf("function: read a version: %w", err)
}

// aliasesOf lists the aliases of a function.
func aliasesOf(ctx context.Context, q *dbgen.Queries, environment pgtype.UUID, name string) ([]Alias, error) {
	rows, err := q.ListFunctionAliases(ctx, dbgen.ListFunctionAliasesParams{EnvironmentID: environment, Name: name})
	if err != nil {
		return nil, fmt.Errorf("function: list aliases: %w", err)
	}
	out := make([]Alias, 0, len(rows))
	for _, r := range rows {
		out = append(out, Alias{Name: r.Alias, Version: r.Version})
	}
	return out, nil
}

// GetFunction returns a function's latest version with its aliases.
func (s *Service) GetFunction(ctx context.Context, p auth.Principal, appID, environmentID, name string) (Function, error) {
	if err := authorize(p, auth.FunctionRead, appID); err != nil {
		return Function{}, err
	}
	var out Function
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		env, err := environmentOf(ctx, q, appID, environmentID)
		if err != nil {
			return err
		}
		row, err := q.GetLatestFunctionVersion(ctx, dbgen.GetLatestFunctionVersionParams{EnvironmentID: env.ID, Name: name})
		if errors.Is(err, pgx.ErrNoRows) {
			return plxerr.New(plxerr.FunctionNotFound, "no function %s is deployed in this environment", name)
		}
		if err != nil {
			return fmt.Errorf("function: read a function: %w", err)
		}
		if out.Latest, err = versionWithBuild(ctx, q, row); err != nil {
			return err
		}
		out.Aliases, err = aliasesOf(ctx, q, env.ID, name)
		return err
	})
	return out, err
}

// ListFunctions lists the functions of an environment by name, each with
// its latest version, after the name a previous page ended with.
func (s *Service) ListFunctions(ctx context.Context, p auth.Principal, appID, environmentID, afterName string, size int32) ([]Version, error) {
	if err := authorize(p, auth.FunctionRead, appID); err != nil {
		return nil, err
	}
	var out []Version
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		env, err := environmentOf(ctx, q, appID, environmentID)
		if err != nil {
			return err
		}
		rows, err := q.ListLatestFunctionVersions(ctx, dbgen.ListLatestFunctionVersionsParams{EnvironmentID: env.ID, AfterName: afterName, PageSize: size})
		if err != nil {
			return fmt.Errorf("function: list functions: %w", err)
		}
		return appendVersions(ctx, q, &out, rows)
	})
	return out, err
}

// ListVersions lists a function's versions, newest first, below the
// version a previous page ended with (zero for the first page).
func (s *Service) ListVersions(ctx context.Context, p auth.Principal, appID, environmentID, name string, before int32, size int32) ([]Version, error) {
	if err := authorize(p, auth.FunctionRead, appID); err != nil {
		return nil, err
	}
	var out []Version
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		env, err := environmentOf(ctx, q, appID, environmentID)
		if err != nil {
			return err
		}
		rows, err := q.ListFunctionVersions(ctx, dbgen.ListFunctionVersionsParams{EnvironmentID: env.ID, Name: name, BeforeVersion: before, PageSize: size})
		if err != nil {
			return fmt.Errorf("function: list versions: %w", err)
		}
		return appendVersions(ctx, q, &out, rows)
	})
	return out, err
}

// appendVersions converts rows onto a list, with the source and the
// declaration of their builds.
func appendVersions(ctx context.Context, q *dbgen.Queries, out *[]Version, rows []dbgen.FunctionVersion) error {
	page := make([]Version, 0, len(rows))
	for _, r := range rows {
		v, err := versionOf(r)
		if err != nil {
			return err
		}
		page = append(page, v)
	}
	if err := attachBuilds(ctx, q, page); err != nil {
		return err
	}
	*out = append(*out, page...)
	return nil
}

// versionWithBuild converts a row like versionOf and adds the source and
// the declaration of its build.
func versionWithBuild(ctx context.Context, q *dbgen.Queries, row dbgen.FunctionVersion) (Version, error) {
	v, err := versionOf(row)
	if err != nil {
		return Version{}, err
	}
	one := []Version{v}
	if err := attachBuilds(ctx, q, one); err != nil {
		return Version{}, err
	}
	return one[0], nil
}

// attachBuilds sets SourceID and Declared on each version from its build,
// in one query. A promoted version keeps the build of the version it was
// copied from, so it reports that build's source and declaration; nothing
// is stored on the version, which stays as immutable as its module
// (FN-006, FN-012). A build that recorded no declaration leaves Declared
// nil.
func attachBuilds(ctx context.Context, q *dbgen.Queries, vs []Version) error {
	if len(vs) == 0 {
		return nil
	}
	ids := make([]pgtype.UUID, 0, len(vs))
	for _, v := range vs {
		id, err := storage.UUID(v.BuildID)
		if err != nil {
			return fmt.Errorf("function: the build of version %d of %s has no valid identifier: %w", v.Version, v.Name, err)
		}
		ids = append(ids, id)
	}
	rows, err := q.ListFunctionBuildLinks(ctx, ids)
	if err != nil {
		return fmt.Errorf("function: read the builds of versions: %w", err)
	}
	links := make(map[string]dbgen.ListFunctionBuildLinksRow, len(rows))
	for _, r := range rows {
		links[storage.ID(r.BuildID)] = r
	}
	for i := range vs {
		link, ok := links[vs[i].BuildID]
		if !ok {
			return fmt.Errorf("function: the build of version %d of %s is missing", vs[i].Version, vs[i].Name)
		}
		vs[i].SourceID = storage.ID(link.SourceID)
		if link.Declaration == nil {
			continue
		}
		var d Declaration
		if err := json.Unmarshal(link.Declaration, &d); err != nil {
			return fmt.Errorf("function: read the declaration of version %d of %s: %w", vs[i].Version, vs[i].Name, err)
		}
		vs[i].Declared = &d
	}
	return nil
}

// ResolveVersion returns the version an alias stands for, or the latest
// version when alias is empty, with the SHA-256 of its module and its
// placement: what a release records of a function it references (FN-006).
// It is for the server's own callers, which have authorised the request;
// it fails with PLX-7012 when the function has no version and PLX-7013
// when the alias does not exist.
func (s *Service) ResolveVersion(ctx context.Context, organizationID, appID, environmentID, name, alias string) (Resolved, error) {
	var out Resolved
	err := s.inTenant(ctx, storage.Tenant{OrganizationID: organizationID}, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = ResolveVersionTx(ctx, tx, appID, environmentID, name, alias)
		return err
	})
	return out, err
}

// ResolveVersionTx is ResolveVersion inside the caller's transaction, so
// that a release resolves every function it pins in the transaction that
// records it.
func ResolveVersionTx(ctx context.Context, tx pgx.Tx, appID, environmentID, name, alias string) (Resolved, error) {
	q := dbgen.New(tx)
	env, err := environmentOf(ctx, q, appID, environmentID)
	if err != nil {
		return Resolved{}, err
	}
	var row dbgen.FunctionVersion
	if alias == "" {
		row, err = q.GetLatestFunctionVersion(ctx, dbgen.GetLatestFunctionVersionParams{EnvironmentID: env.ID, Name: name})
		if errors.Is(err, pgx.ErrNoRows) {
			return Resolved{}, plxerr.New(plxerr.FunctionNotFound, "no function %s is deployed in this environment", name)
		}
	} else {
		row, err = resolveAlias(ctx, q, env.ID, name, alias)
	}
	if err != nil {
		return Resolved{}, err
	}
	return Resolved{Name: name, Version: row.Version, ModuleSHA256: row.ModuleSha256, Placement: Placement(row.Placement), TestsPassed: row.TestsPassed}, nil
}

// resolveAlias reads the version an alias points at.
func resolveAlias(ctx context.Context, q *dbgen.Queries, environment pgtype.UUID, name, alias string) (dbgen.FunctionVersion, error) {
	a, err := q.GetFunctionAlias(ctx, dbgen.GetFunctionAliasParams{EnvironmentID: environment, Name: name, Alias: alias})
	if errors.Is(err, pgx.ErrNoRows) {
		return dbgen.FunctionVersion{}, plxerr.New(plxerr.FunctionAliasNotFound, "the function %s has no alias %q", name, alias)
	}
	if err != nil {
		return dbgen.FunctionVersion{}, fmt.Errorf("function: read an alias: %w", err)
	}
	row, err := q.GetFunctionVersion(ctx, dbgen.GetFunctionVersionParams{EnvironmentID: environment, Name: name, Version: a.Version})
	if err != nil {
		return dbgen.FunctionVersion{}, fmt.Errorf("function: read the version of an alias: %w", err)
	}
	return row, nil
}

// versionOf converts a row.
func versionOf(row dbgen.FunctionVersion) (Version, error) {
	v := Version{
		AppID: storage.ID(row.AppID), EnvironmentID: storage.ID(row.EnvironmentID), Name: row.Name, Version: row.Version,
		Placement: Placement(row.Placement), ModuleKey: row.ModuleKey, ModuleSHA256: row.ModuleSha256, ABIVersion: row.AbiVersion,
		InputSchema: row.InputSchema, OutputSchema: row.OutputSchema, Statement: row.Statement, Signature: row.Signature,
		KeyID: row.KeyID, TestsPassed: row.TestsPassed, BuildID: storage.ID(row.BuildID), CreatedAt: storage.Time(row.CreatedAt),
	}
	if err := json.Unmarshal(row.Provenance, &v.Provenance); err != nil {
		return Version{}, fmt.Errorf("function: read the provenance of a version: %w", err)
	}
	return v, nil
}

// deref returns the string a pointer holds, or "".
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// derefInt returns the number a pointer holds, or zero.
func derefInt(n *int32) int32 {
	if n == nil {
		return 0
	}
	return *n
}

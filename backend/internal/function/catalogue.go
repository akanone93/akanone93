// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// cataloguePage is how many functions one query of a catalogue reads.
const cataloguePage = 200

// catalogue is the compiler's view of the functions of one app and
// environment, read once: the compiler asks synchronously and must get the
// same answer throughout one compilation (CMP-002). It is never changed
// after it is built, so it is safe for concurrent use.
type catalogue struct {
	deviceFunctions bool
	// latest maps a function name to its newest version.
	latest map[string]uint32
	// aliases maps a function name to its aliases and their versions.
	aliases map[string]map[string]uint32
	// versions holds every version an alias or "newest" can stand for.
	versions map[versionKey]compiler.FunctionInfo
}

// versionKey names a version of a function.
type versionKey struct {
	name    string
	version uint32
}

var _ compiler.FunctionCatalogue = (*catalogue)(nil)

// Function implements compiler.FunctionCatalogue: the version the alias
// selects, or the newest when alias is empty (FN-006).
func (c *catalogue) Function(name, alias string) (compiler.FunctionInfo, bool) {
	version, ok := c.latest[name]
	if alias != "" {
		version, ok = c.aliases[name][alias]
	}
	if !ok {
		return compiler.FunctionInfo{}, false
	}
	info, ok := c.versions[versionKey{name, version}]
	return cloneInfo(info), ok
}

// DeviceFunctionsEnabled implements compiler.FunctionCatalogue (FN-055).
func (c *catalogue) DeviceFunctionsEnabled() bool { return c.deviceFunctions }

// cloneInfo copies what the compiler could otherwise change in the shared
// snapshot.
func cloneInfo(in compiler.FunctionInfo) compiler.FunctionInfo {
	if c := in.Capabilities; c != nil {
		copied := *c
		copied.HTTP, copied.Secrets, copied.Functions = slices.Clone(c.HTTP), slices.Clone(c.Secrets), slices.Clone(c.Functions)
		in.Capabilities = &copied
	}
	if l := in.Limits; l != nil {
		copied := *l
		in.Limits = &copied
	}
	in.InputSchema, in.OutputSchema = slices.Clone(in.InputSchema), slices.Clone(in.OutputSchema)
	return in
}

// Catalogue reads every function deployed in an environment with its
// aliases, the declared capabilities and limits and the schemas derived
// from its types, and the environment's effective security setting
// deviceFunctions. It is for the server's own callers (publishing and
// creating a release), which have authorised the request. The result is an
// immutable snapshot of that moment (FN-004, FN-006, FN-008, FN-055,
// ACT-001).
func (s *Service) Catalogue(ctx context.Context, organizationID, appID, environmentID string) (compiler.FunctionCatalogue, error) {
	cat := &catalogue{
		latest:   map[string]uint32{},
		aliases:  map[string]map[string]uint32{},
		versions: map[versionKey]compiler.FunctionInfo{},
	}
	err := s.inTenant(ctx, storage.Tenant{OrganizationID: organizationID}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		env, err := environmentOf(ctx, q, appID, environmentID)
		if err != nil {
			return err
		}
		return cat.load(ctx, q, env.ID)
	})
	if err != nil {
		return nil, err
	}
	if s.o.DeviceFunctions != nil {
		if cat.deviceFunctions, err = s.o.DeviceFunctions(ctx, appID, environmentID); err != nil {
			return nil, fmt.Errorf("function.Catalogue: read the security setting deviceFunctions: %w", err)
		}
	}
	return cat, nil
}

// load reads the newest version of every function and the versions its
// aliases point at.
func (c *catalogue) load(ctx context.Context, q *dbgen.Queries, environment pgtype.UUID) error {
	after := ""
	for {
		rows, err := q.ListLatestFunctionVersions(ctx, dbgen.ListLatestFunctionVersionsParams{EnvironmentID: environment, AfterName: after, PageSize: cataloguePage})
		if err != nil {
			return fmt.Errorf("function.Catalogue: list functions: %w", err)
		}
		for _, row := range rows {
			if err := c.add(ctx, q, row); err != nil {
				return err
			}
		}
		if len(rows) < cataloguePage {
			return nil
		}
		after = rows[len(rows)-1].Name
	}
}

// add records one function: its newest version and its aliases.
func (c *catalogue) add(ctx context.Context, q *dbgen.Queries, newest dbgen.FunctionVersion) error {
	if err := c.addVersion(ctx, q, newest); err != nil {
		return err
	}
	c.latest[newest.Name] = versionNumber(newest)
	aliases, err := q.ListFunctionAliases(ctx, dbgen.ListFunctionAliasesParams{EnvironmentID: newest.EnvironmentID, Name: newest.Name})
	if err != nil {
		return fmt.Errorf("function.Catalogue: list the aliases of %s: %w", newest.Name, err)
	}
	for _, a := range aliases {
		row := newest
		if a.Version != newest.Version {
			if row, err = q.GetFunctionVersion(ctx, dbgen.GetFunctionVersionParams{EnvironmentID: newest.EnvironmentID, Name: newest.Name, Version: a.Version}); err != nil {
				return fmt.Errorf("function.Catalogue: read version %d of %s: %w", a.Version, newest.Name, err)
			}
			if err := c.addVersion(ctx, q, row); err != nil {
				return err
			}
		}
		if c.aliases[newest.Name] == nil {
			c.aliases[newest.Name] = map[string]uint32{}
		}
		c.aliases[newest.Name][a.Alias] = versionNumber(row)
	}
	return nil
}

// versionNumber is the version of a row; the database keeps it positive.
func versionNumber(row dbgen.FunctionVersion) uint32 {
	return uint32(max(row.Version, 0)) //nolint:gosec // G115: clamped to non-negative.
}

// addVersion describes a version for the compiler from its row and the
// declaration of its build.
func (c *catalogue) addVersion(ctx context.Context, q *dbgen.Queries, row dbgen.FunctionVersion) error {
	decl, err := q.GetFunctionDeclaration(ctx, row.BuildID)
	if err != nil {
		return fmt.Errorf("function.Catalogue: read the declaration of version %d of %s: %w", row.Version, row.Name, err)
	}
	var doc struct {
		Capabilities *schema.FunctionCapabilities `json:"capabilities"`
		Limits       *schema.FunctionLimits       `json:"limits"`
		Attestation  bool                         `json:"attestation"`
		SCA          bool                         `json:"sca"`
	}
	if err := json.Unmarshal(decl.Declaration, &doc); err != nil {
		return fmt.Errorf("function.Catalogue: read the declaration of version %d of %s: %w", row.Version, row.Name, err)
	}
	c.versions[versionKey{row.Name, versionNumber(row)}] = compiler.FunctionInfo{
		Placement: schema.Placement(row.Placement), Capabilities: doc.Capabilities, Limits: doc.Limits,
		InputSchema: json.RawMessage(row.InputSchema), OutputSchema: json.RawMessage(row.OutputSchema),
		Version: versionNumber(row),
		// FN-020: the grant tells the device in advance which evidence the
		// server will demand.
		RequiresAttestation: doc.Attestation, RequiresSCA: doc.SCA,
	}
	return nil
}

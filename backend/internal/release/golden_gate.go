// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/golden"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/security/settings"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// blocksOnGoldens is the app's goldens.blockPublish setting (TST-003).
func blocksOnGoldens(c compiled) bool {
	if c.result == nil || c.result.Project == nil || c.result.Project.App.Doc == nil {
		return false
	}
	g := c.result.Project.App.Doc.Goldens
	return g != nil && g.BlockPublish != nil && *g.BlockPublish
}

// gateGoldens applies goldens.blockPublish (TST-003, ADR-0070). When the
// setting is on, the publish needs a canonical golden run for the exact
// hashes of the bundle and of the app bundle it renders with: none is
// PLX-3061, one with differences nobody approved is PLX-3062. The
// compiler's determinism (CMP-002) makes the hashes the CLI computed
// locally the ones the publish produces. The returned diagnostics are a
// copy when they change.
//
// The security setting requiredTests asks the same of a publish to a
// production environment whatever the app document says, and is on under the
// maximum profile (TST-005): the Plux tests and the goldens run by plux
// golden run are the evidence.
func (s *Service) gateGoldens(ctx context.Context, system auth.Principal, row dbgen.PublishJob, c compiled, diags plxerr.Diagnostics) (plxerr.Diagnostics, error) {
	why := "goldens.blockPublish"
	if !blocksOnGoldens(c) {
		required, err := s.requiresTests(ctx, system, row)
		if err != nil || !required {
			return diags, err
		}
		why = "requiredTests"
	}
	b := pickBundle(c.result, c.key)
	if b == nil || c.result.App == nil {
		return diags, nil // the caller reports the missing bundle
	}
	var st golden.Status
	err := s.inOrg(ctx, system, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		st, err = golden.Lookup(ctx, tx, row.AppID, c.key, b.Hash[:], c.result.App.Hash[:])
		return err //nolint:wrapcheck // a domain error
	})
	if err != nil {
		return diags, err
	}
	switch {
	case !st.Found:
		return append(slices.Clone(diags), plxerr.NewDiagnostic(plxerr.GoldenRunMissing, plxerr.Location{},
			"%s is on and no canonical golden run exists for bundle %x with app bundle %x; run plux golden run --upload",
			why, b.Hash[:], c.result.App.Hash[:])), nil
	case st.Differences > 0:
		return append(slices.Clone(diags), plxerr.NewDiagnostic(plxerr.GoldenDifferenceUnapproved, plxerr.Location{},
			"golden run %s has %d of %d case(s) that differ from the approved baselines; approve them with plux golden approve", st.RunID, st.Differences, st.Cases)), nil
	}
	return diags, nil
}

// requiresTests reports whether the publish is to a production environment
// whose security configuration has requiredTests on (TST-005). Without a
// security configuration service nothing is required.
func (s *Service) requiresTests(ctx context.Context, system auth.Principal, row dbgen.PublishJob) (bool, error) {
	if s.o.SecurityConfig == nil {
		return false, nil
	}
	var production bool
	err := s.inOrg(ctx, system, func(ctx context.Context, tx pgx.Tx) error {
		env, err := dbgen.New(tx).GetEnvironment(ctx, row.EnvironmentID)
		production = env.Production
		return err //nolint:wrapcheck // a domain error
	})
	if err != nil || !production {
		return false, err
	}
	values, _, err := s.o.SecurityConfig.Effective(ctx, storage.ID(row.AppID), storage.ID(row.EnvironmentID))
	if err != nil {
		return false, fmt.Errorf("release: %w", err)
	}
	return values.Get(settings.RequiredTests).Bool(), nil
}

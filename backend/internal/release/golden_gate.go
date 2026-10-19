// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/golden"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
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
func (s *Service) gateGoldens(ctx context.Context, system auth.Principal, row dbgen.PublishJob, c compiled, diags plxerr.Diagnostics) (plxerr.Diagnostics, error) {
	if !blocksOnGoldens(c) {
		return diags, nil
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
			"goldens.blockPublish is on and no canonical golden run exists for bundle %x with app bundle %x; run plux golden run --upload",
			b.Hash[:], c.result.App.Hash[:])), nil
	case st.Differences > 0:
		return append(slices.Clone(diags), plxerr.NewDiagnostic(plxerr.GoldenDifferenceUnapproved, plxerr.Location{},
			"golden run %s has %d of %d case(s) that differ from the approved baselines; approve them with plux golden approve", st.RunID, st.Differences, st.Cases)), nil
	}
	return diags, nil
}

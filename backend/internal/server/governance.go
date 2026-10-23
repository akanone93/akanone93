// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/ext/governance"
	"github.com/nightCode42/plux3/backend/internal/approval"
	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/breakglass"
	"github.com/nightCode42/plux3/backend/internal/compliance"
	"github.com/nightCode42/plux3/backend/internal/config"
	"github.com/nightCode42/plux3/backend/internal/freeze"
	"github.com/nightCode42/plux3/backend/internal/notify"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
)

// governanceParts are the services of break-glass, change freezes and
// compliance reports (GOV-024 to GOV-026, ADR-0076).
type governanceParts struct {
	freezes    *freeze.Service
	breakGlass *breakglass.Service
	reports    *compliance.Service
}

// buildGovernance builds the break-glass, change-freeze and compliance
// services, and gives the approval engine the freeze guard that holds
// production changes back. The enterprise edition's calendar and report
// renderer come in through the extensions; without them the freeze calendar
// is the community's, which holds nothing, and reports have no PDF.
func buildGovernance(cfg *config.Config, db *storage.DB, log *audit.Log, ids interface{ New() (string, error) }, ten *tenancy.Service, notifier *notify.Service, approvals *approval.Service, work WorkDeps) (governanceParts, error) {
	effective := func(ctx context.Context, tx pgx.Tx, org string) (limits.Set, error) {
		return ten.Effective(ctx, tx, org, "", "") //nolint:wrapcheck // a domain error
	}
	ext := work.Governance.OrDefault()
	freezes, err := freeze.New(freeze.Options{DB: db, Audit: log, IDs: ids, Limits: effective, Calendar: ext.Freezes})
	if err != nil {
		return governanceParts{}, fmt.Errorf("server: %w", err)
	}
	approvals.UseFreezes(freezes)
	emergencies, err := breakglass.New(breakglass.Options{DB: db, Audit: log, IDs: ids, Limits: effective, Notifier: notifier, Log: work.Log})
	if err != nil {
		return governanceParts{}, fmt.Errorf("server: %w", err)
	}
	reports, err := compliance.New(compliance.Options{
		DB: db, Audit: log, IDs: ids, Limits: effective, Signer: work.Signer, SignerKey: cfg.Signing.Keys.Audit,
		Renderer: ext.Reports, Log: work.Log,
	})
	if err != nil {
		return governanceParts{}, fmt.Errorf("server: %w", err)
	}
	return governanceParts{freezes: freezes, breakGlass: emergencies, reports: reports}, nil
}

// WithGovernanceExtensions gives the process the enterprise edition's change
// freeze calendar and compliance report renderer (GOV-025, GOV-026); without
// it the process holds no change back and writes no PDF.
func WithGovernanceExtensions(e governance.Extensions) BuildOption {
	return func(o *buildOptions) { o.governance = e }
}

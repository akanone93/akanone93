// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package tenancy

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// AuditPage is one page of an audit log: Size entries after the sequence
// number From in order, or, NewestFirst, before it (From 0: from the
// newest entry).
type AuditPage struct {
	From        int64
	Size        int32
	NewestFirst bool
}

// read reads the page from one chain.
func (pg AuditPage) read(ctx context.Context, log *audit.Log, tx pgx.Tx, organizationID string) ([]audit.Entry, error) {
	if pg.NewestFirst {
		return log.ListNewestFirst(ctx, tx, organizationID, pg.From, pg.Size) //nolint:wrapcheck // the audit package wraps its own failures
	}
	return log.List(ctx, tx, organizationID, pg.From, pg.Size) //nolint:wrapcheck // the audit package wraps its own failures
}

// AuditEntries lists a page of an organisation's audit log (SEC-140).
func (s *Service) AuditEntries(ctx context.Context, p auth.Principal, page AuditPage) ([]audit.Entry, error) {
	if err := authorize(p, auth.AuditRead); err != nil {
		return nil, err
	}
	var out []audit.Entry
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = page.read(ctx, s.o.Audit, tx, p.OrganizationID)
		return err
	})
	return out, err
}

// InstallationAuditEntries lists a page of the installation's own chain —
// sign-ins, invitations, second factors — for an installation
// administrator.
func (s *Service) InstallationAuditEntries(ctx context.Context, id auth.Identity, page AuditPage) ([]audit.Entry, error) {
	if id.Kind != auth.KindUser || !id.InstallationAdmin {
		return nil, plxerr.New(plxerr.PermissionDenied, "only an installation administrator may read the installation's audit log")
	}
	var out []audit.Entry
	err := s.o.DB.InTx(ctx, storage.Tenant{Scope: storage.ScopeInstallation}, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = page.read(ctx, s.o.Audit, tx, "")
		return err
	})
	return out, err //nolint:wrapcheck // InTx wraps its own failures
}

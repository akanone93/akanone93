// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package tenancy

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// RecordInstallation appends an entry to the installation's own audit
// chain for an installation administrator's change, such as installing a
// licence (P11 §4.3 #23). Anyone else is refused.
func (s *Service) RecordInstallation(ctx context.Context, id auth.Identity, action audit.Action, kind, target string) error {
	if id.Kind != auth.KindUser || !id.InstallationAdmin {
		return plxerr.New(plxerr.PermissionDenied, "only an installation administrator may change the installation")
	}
	err := s.o.DB.InTx(ctx, storage.Tenant{Scope: storage.ScopeInstallation}, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := s.o.Audit.Append(ctx, tx, audit.Entry{Actor: id.Actor(), Action: action, TargetKind: kind, TargetID: target}); err != nil {
			return fmt.Errorf("tenancy: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("tenancy: record installation change: %w", err)
	}
	return nil
}

// InstallationOrganization is one row of the installation page: an
// organisation's identity and size, never its content.
type InstallationOrganization struct {
	ID          string
	Name        string
	CreatedAt   time.Time
	MemberCount int64
	AppCount    int64
}

// InstallationOrganizations lists every organisation of the installation
// with its member and app counts, in key order. It is the one read that
// crosses the tenant wall, so it needs an installation administrator and
// is recorded in the installation's own audit chain.
func (s *Service) InstallationOrganizations(ctx context.Context, id auth.Identity) ([]InstallationOrganization, error) {
	if id.Kind != auth.KindUser || !id.InstallationAdmin {
		return nil, plxerr.New(plxerr.PermissionDenied, "only an installation administrator may list every organisation")
	}
	var out []InstallationOrganization
	err := s.o.DB.InTx(ctx, storage.Tenant{Scope: storage.ScopeOverview}, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListInstallationOrganizations(ctx)
		if err != nil {
			return failure(err, "organisation")
		}
		for _, r := range rows {
			out = append(out, InstallationOrganization{
				ID: storage.ID(r.ID), Name: r.Name, CreatedAt: r.CreatedAt.Time, MemberCount: r.MemberCount, AppCount: r.AppCount,
			})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("tenancy: list installation organisations: %w", err)
	}
	if err := s.RecordInstallation(ctx, id, audit.InstallationOrganizationsViewed, "installation", "organizations"); err != nil {
		return nil, err
	}
	return out, nil
}

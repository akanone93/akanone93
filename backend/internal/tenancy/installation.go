// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package tenancy

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
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

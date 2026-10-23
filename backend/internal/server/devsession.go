// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/config"
	"github.com/nightCode42/plux3/backend/internal/devsession"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/seccfg"
	"github.com/nightCode42/plux3/backend/internal/security/settings"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
)

// buildDevSessions builds the sessions and pairing of dev sessions (DEV-002,
// DEV-003). Only a process that issues device tokens, which holds the token
// signer, serves them; elsewhere it returns nil.
func buildDevSessions(cfg *config.Config, db *storage.DB, log *audit.Log, ids devsession.IDs, ten *tenancy.Service, configs *seccfg.Service, signer signing.TokenSigner) (*devsession.Service, error) {
	if signer == nil {
		return nil, nil //nolint:nilnil // the role serves no dev sessions
	}
	o := devsession.Options{
		DB: db, Audit: log, IDs: ids, Tokens: signer,
		Environments: cfg.DevSession.EnabledEnvironments, PublicBaseURL: cfg.Server.PublicBaseURL,
		Limits: func(ctx context.Context, tx pgx.Tx, org, app string) (limits.Set, error) {
			return ten.Effective(ctx, tx, org, app, "") //nolint:wrapcheck // a domain error
		},
	}
	if configs != nil {
		o.StepUp = func(ctx context.Context, appID, environmentID string) (bool, error) {
			values, _, err := configs.Effective(ctx, appID, environmentID)
			if err != nil {
				return false, err //nolint:wrapcheck // a domain error
			}
			return values.Get(settings.DevSessionRequireStepUp).Bool(), nil
		}
	}
	s, err := devsession.New(o)
	if err != nil {
		return nil, fmt.Errorf("server: %w", err)
	}
	return s, nil
}

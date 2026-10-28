// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"fmt"

	"github.com/nightCode42/plux3/backend/internal/replay"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/seccfg"
	"github.com/nightCode42/plux3/backend/internal/security/settings"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// buildReplays builds the structural session replay service (ANL-050,
// ADR-0101). The environment's security configuration decides whether replay
// is allowed at all: the maximum profile turns it off.
func buildReplays(db *storage.DB, set limits.Set, configs *seccfg.Service) (*replay.Service, error) {
	svc, err := replay.NewService(replay.Options{
		DB: db, Limits: set,
		Allowed: func(ctx context.Context, appID, environmentID string) (bool, error) {
			values, _, err := configs.Effective(ctx, appID, environmentID)
			if err != nil {
				return false, fmt.Errorf("server: %w", err)
			}
			return values.Get(settings.AllowSessionReplay).Bool(), nil
		},
	})
	if err != nil {
		return nil, fmt.Errorf("server: %w", err)
	}
	return svc, nil
}

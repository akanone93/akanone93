// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"fmt"

	"github.com/nightCode42/plux3/backend/internal/approval"
	"github.com/nightCode42/plux3/backend/internal/seccfg"
	"github.com/nightCode42/plux3/backend/internal/security/settings"
)

// approvalProfiles answers the approval engine which security profile an
// environment has, which decides the default policies it starts with (Q16).
func approvalProfiles(configs *seccfg.Service) approval.ProfileFunc {
	return func(ctx context.Context, appID, environmentID string) (settings.Profile, error) {
		values, _, err := configs.Effective(ctx, appID, environmentID)
		if err != nil {
			return "", fmt.Errorf("server: %w", err)
		}
		return values.Profile(), nil
	}
}

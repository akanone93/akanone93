// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package serverapp

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/internal/config"
)

// openEdition returns the licence provider of the process: the enterprise
// verifier's when the binary registered one, otherwise the community
// provider that enables nothing (GOV-033). The core never reads the licence
// file itself.
func openEdition(ctx context.Context, cfg *config.Config, log *slog.Logger, opts Options) (edition.Provider, error) {
	if opts.Edition == nil {
		if cfg.License.File != "" {
			log.WarnContext(ctx, "license.file is set, but this binary is the open-source core and ignores it; enterprise features stay off (GOV-033)")
		}
		return edition.Community(), nil
	}
	p, err := opts.Edition.Open(ctx, edition.Settings{File: cfg.License.File, Log: log})
	if err != nil {
		return nil, fmt.Errorf("open the enterprise edition: %w", err)
	}
	return p, nil
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/approval"
	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/document"
	"github.com/nightCode42/plux3/backend/internal/experiment"
	"github.com/nightCode42/plux3/backend/internal/release"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
)

// buildExperiments builds the administration of feature flags and
// experiments (ABT-001, ABT-006). It writes the control documents through
// the release service and promotes winners through the document and release
// services; the approval engine guards a start (ABT-007).
func buildExperiments(db *storage.DB, log *audit.Log, ids experiment.IDs, ten *tenancy.Service, docs *document.Service, releases *release.Service, approvals approval.Gate) (*experiment.Service, error) {
	s, err := experiment.New(experiment.Options{
		DB: db, Audit: log, IDs: ids,
		Limits: func(ctx context.Context, tx pgx.Tx, org, app string) (limits.Set, error) {
			return ten.Effective(ctx, tx, org, app, "") //nolint:wrapcheck // a domain error
		},
		Control: releases, Documents: docs, Publisher: releases,
		Observations: experiment.PostgresObservations{DB: db},
		Approval:     experiment.EngineGate{Engine: approvals},
	})
	if err != nil {
		return nil, fmt.Errorf("server: %w", err)
	}
	return s, nil
}

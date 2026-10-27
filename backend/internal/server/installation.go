// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"fmt"
	"sort"

	"github.com/nightCode42/plux3/backend/internal/api"
	"github.com/nightCode42/plux3/backend/internal/config"
	"github.com/nightCode42/plux3/backend/internal/jobs"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// installationReader answers InstallationService from the pieces the
// server already owns (STU-116, P11 §4.3 #22). It reports names, sizes and
// counts only: never an error text, a connection string or a credential.
type installationReader struct {
	health  *Health
	db      *storage.DB
	jobs    *jobs.Client
	issuers []config.CIIssuer
}

var _ api.InstallationReader = installationReader{}

// Health runs the readiness checks once.
func (r installationReader) Health(ctx context.Context) []api.HealthResult {
	if r.health == nil {
		return nil
	}
	var out []api.HealthResult
	for _, c := range r.health.Results(ctx) {
		out = append(out, api.HealthResult{Name: c.Name, Healthy: c.Healthy})
	}
	return out
}

// DatabaseBytes is the size of the installation's database.
func (r installationReader) DatabaseBytes(ctx context.Context) (int64, error) {
	var size int64
	if err := r.db.Pool().QueryRow(ctx, `SELECT pg_database_size(current_database())`).Scan(&size); err != nil {
		return 0, fmt.Errorf("server: database size: %w", err)
	}
	return size, nil
}

// Jobs counts the jobs waiting in each queue, by queue name.
func (r installationReader) Jobs(ctx context.Context) ([]api.JobDepth, error) {
	if r.jobs == nil {
		return nil, nil
	}
	depths, err := r.jobs.QueueDepths(ctx)
	if err != nil {
		return nil, fmt.Errorf("server: job queues: %w", err)
	}
	out := make([]api.JobDepth, 0, len(depths))
	for queue, n := range depths {
		out = append(out, api.JobDepth{Queue: queue, Count: int64(n)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Queue < out[j].Queue })
	return out, nil
}

// Issuers lists the configured CI issuers.
func (r installationReader) Issuers() []api.TrustedIssuer {
	out := make([]api.TrustedIssuer, 0, len(r.issuers))
	for _, i := range r.issuers {
		out = append(out, api.TrustedIssuer{Issuer: i.Issuer, Audience: i.Audience, SubjectPattern: i.SubjectPattern})
	}
	return out
}

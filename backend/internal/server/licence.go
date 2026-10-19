// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/internal/api"
	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// licenceWatchInterval is how often the server compares the licence state
// with the one it last reported. The state itself is re-verified hourly by
// the enterprise provider; this only notices a change.
const licenceWatchInterval = time.Minute

// licenceActor is who records a licence change in the audit log.
var licenceActor = audit.Actor{Kind: "system", ID: "plux-server", Display: "licence check"}

// withEdition makes the licence provider available to every handler through
// the request context (edition.Enabled and friends).
func (s *Server) withEdition() api.Around {
	return func(ctx context.Context, _ api.Call, next func(context.Context) error) error {
		return next(edition.WithProvider(ctx, s.edition))
	}
}

// watchLicence reports each change of the licence state until the returned
// function is called: a log line, and for grace, expiry and invalidity an
// audit entry in the installation's chain (GOV-030). It never blocks or
// stops anything: a licence problem is a warning, not an outage (GOV-032).
func (s *Server) watchLicence(ctx context.Context) func() {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		var last edition.State
		tick := time.NewTicker(licenceWatchInterval)
		defer tick.Stop()
		for {
			if st := s.edition.Status(); st.State != last {
				last = st.State
				s.reportLicence(ctx, st)
			}
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
		}
	}()
	return func() { cancel(); <-done }
}

// reportLicence logs a licence state and audits the states that need an
// administrator's attention.
func (s *Server) reportLicence(ctx context.Context, st edition.Status) {
	attrs := []any{slog.String("state", string(st.State)), slog.Int("daysRemaining", st.DaysRemaining)}
	switch st.State {
	case edition.StateValid, edition.StateAbsent:
		s.log.InfoContext(ctx, "licence: "+st.Message, attrs...)
		return
	case edition.StateGrace, edition.StateExpired, edition.StateInvalid:
		s.log.WarnContext(ctx, "licence: "+st.Message, attrs...)
	}
	if s.db == nil {
		return
	}
	detail := fmt.Sprintf("state=%s daysRemaining=%d", st.State, st.DaysRemaining)
	err := s.db.InTx(ctx, storage.Tenant{Scope: storage.ScopeInstallation}, func(ctx context.Context, tx pgx.Tx) error {
		_, err := audit.NewLog(NewIDs(), nil).Append(ctx, tx, audit.Entry{
			Actor: licenceActor, Action: audit.LicenseStateChanged,
			TargetKind: "license", TargetID: st.LicenseID, Detail: detail,
		})
		return err //nolint:wrapcheck // reported below
	})
	if err != nil {
		s.log.ErrorContext(ctx, "licence: the state change could not be audited", slog.Any("error", err))
	}
}

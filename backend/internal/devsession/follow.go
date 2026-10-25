// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package devsession

import (
	"context"
	"slices"
	"testing/fstest"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/document"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

var _ document.DraftFollower = (*Service)(nil)

// DraftWritten compiles the stored draft of an app after an accepted write
// and pushes it to every live Studio session of the app, through the
// sessions' frame streams, so that a device on any replica receives it
// (L-14, contract #17, DEV-010). Studio never sends the edit itself. A
// session whose draft does not compile, or that cannot be pushed to, is
// left as it was; the write never fails for it.
func (s *Service) DraftWritten(_ context.Context, p auth.Principal, appID, pluginID string, paths []string) {
	if !s.CanPush() || len(paths) == 0 {
		return
	}
	s.drafter.submit(p, appID, pluginID, paths)
}

// draftWorkers reads the number of draft compiles that may run at once.
func (s *Service) draftWorkers(ctx context.Context, p auth.Principal, appID string) int {
	n := 1
	_ = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		set, err := s.o.Limits(ctx, tx, p.OrganizationID, appID)
		if err != nil {
			return err //nolint:wrapcheck // the conservative default stands
		}
		n = int(set.Get(limits.DevSessionDraftCompileWorkers))
		return nil
	})
	return max(n, 1)
}

// followApp compiles and pushes the paths written to an app to its sessions.
func (s *Service) followApp(ctx context.Context, p auth.Principal, appID string, paths []string) {
	app, err := storage.UUID(appID)
	if err != nil {
		return
	}
	var plans []pushPlan
	_ = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListFollowingDevSessions(ctx, dbgen.ListFollowingDevSessionsParams{AppID: app, Now: storage.Timestamp(s.now())})
		if err != nil {
			return err //nolint:wrapcheck // dropped: the write stands
		}
		for _, r := range rows {
			if plan, err := s.planOf(ctx, tx, p, sessionOf(r)); err == nil {
				plans = append(plans, plan)
			}
		}
		return nil
	})
	ids := slices.Clone(paths)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	for _, plan := range plans {
		s.followDraft(ctx, p, plan, ids)
	}
}

// followDraft pushes the stored draft to one Studio session.
func (s *Service) followDraft(ctx context.Context, p auth.Principal, plan pushPlan, ids []string) {
	st := s.enter(plan)
	defer st.mu.Unlock()
	var files fstest.MapFS
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		files, err = s.reload(ctx, tx, st, plan)
		return err
	})
	if err != nil {
		return
	}
	_, _ = s.compileAndPush(ctx, p, plan, st, files, st.revs, ids, nil) //nolint:errcheck // the write stands; the next one pushes again
}

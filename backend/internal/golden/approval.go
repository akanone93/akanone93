// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package golden

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/approval"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// requireApproval asks the approval engine whether the differences about to
// become baselines may (GOV-020, ADR-0069). The artifact is the run, the
// plugin and the image hash of every case chosen, so an approval is spent on
// exactly the images the approver saw. It runs before the baselines are
// written and before the audit entry (approval.Require).
func (s *Service) requireApproval(ctx context.Context, tx pgx.Tx, p auth.Principal, ref runRef, pluginKey string, diffs []dbgen.ListGoldenDifferencesRow, wanted map[string]bool) error {
	parts := [][]byte{[]byte(ref.runID), []byte(pluginKey)}
	chosen := 0
	for _, d := range diffs {
		if len(wanted) > 0 && !wanted[d.CaseID] {
			continue
		}
		parts = append(parts, []byte(d.CaseID), d.ImageSha256)
		chosen++
	}
	if chosen == 0 {
		return nil
	}
	err := approval.Require(ctx, s.o.Approvals, tx, approval.Artifact{
		Resource: schema.PolicyResourceGoldenBaseline, OrganizationID: p.OrganizationID, AppID: ref.appID,
		Action: approval.ActionApproveGoldens, Subject: "goldens:" + ref.runID + ":" + pluginKey, Hash: approval.HashOf(parts...),
		Summary: fmt.Sprintf("%d golden image(s) of plugin %q become baselines", chosen, pluginKey), Requester: p.Actor(), RequesterUser: p.UserID,
	})
	if err != nil {
		return fmt.Errorf("golden.requireApproval: %w", err)
	}
	return nil
}

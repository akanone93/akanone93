// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package compliance

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/ext/governance"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/prodchange"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// build reads the production changes of a period, with their decisions and
// break-glass invocations, into a report (GOV-026). It refuses a period that
// holds more changes than governance.reportChanges allows rather than list
// part of it.
func (s *Service) build(ctx context.Context, tx pgx.Tx, p auth.Principal, from, to time.Time) (governance.Report, error) {
	set, err := s.o.Limits(ctx, tx, p.OrganizationID)
	if err != nil {
		return governance.Report{}, fmt.Errorf("compliance: read the limits: %w", err)
	}
	most := set.Get(limits.GovernanceReportChanges)
	rows, err := prodchange.Between(ctx, tx, from, to, int32(min(most+1, 1<<30))) //nolint:gosec // bounded by the registry's maximum
	if err != nil {
		return governance.Report{}, fmt.Errorf("compliance: %w", err)
	}
	if int64(len(rows)) > most {
		return governance.Report{}, plxerr.New(plxerr.LimitExceeded,
			"the period holds more than %d production changes (governance.reportChanges); ask for a shorter period", most)
	}
	ev, err := s.evidence(ctx, tx, rows)
	if err != nil {
		return governance.Report{}, err
	}
	report := governance.Report{
		Version: governance.ReportVersion, OrganizationID: p.OrganizationID, From: from, To: to,
		GeneratedAt: s.now().UTC().Truncate(time.Microsecond), GeneratedBy: p.Actor().ID, Changes: make([]governance.Change, len(rows)),
	}
	for i, r := range rows {
		report.Changes[i] = changeOf(r, ev)
	}
	return report, nil
}

// evidence is what the changes of a period point to.
type evidence struct {
	decisions  map[string][]dbgen.ApprovalDecision
	breakGlass map[string]dbgen.BreakGlassInvocation
}

// evidence reads the decisions of the requests and the break-glass
// invocations that the changes name.
func (*Service) evidence(ctx context.Context, tx pgx.Tx, rows []dbgen.ProductionChange) (evidence, error) {
	var requests, invocations []pgtype.UUID
	for _, r := range rows {
		if r.RequestID.Valid {
			requests = append(requests, r.RequestID)
		}
		if r.BreakGlassID.Valid {
			invocations = append(invocations, r.BreakGlassID)
		}
	}
	q := dbgen.New(tx)
	ev := evidence{decisions: map[string][]dbgen.ApprovalDecision{}, breakGlass: map[string]dbgen.BreakGlassInvocation{}}
	decisions, err := q.ListDecisionsOfRequests(ctx, requests)
	if err != nil {
		return evidence{}, fmt.Errorf("compliance: read the decisions: %w", err)
	}
	for _, d := range decisions {
		ev.decisions[storage.ID(d.RequestID)] = append(ev.decisions[storage.ID(d.RequestID)], d)
	}
	bg, err := q.ListBreakGlassByIDs(ctx, invocations)
	if err != nil {
		return evidence{}, fmt.Errorf("compliance: read the break-glass invocations: %w", err)
	}
	for _, b := range bg {
		ev.breakGlass[storage.ID(b.ID)] = b
	}
	return ev, nil
}

// changeOf describes one ledger row with its evidence.
func changeOf(r dbgen.ProductionChange, ev evidence) governance.Change {
	c := governance.Change{
		ID: storage.ID(r.ID), AppID: storage.ID(r.AppID), Environment: r.EnvironmentKey, Resource: r.Resource, Action: r.Action,
		Subject: r.Subject, ContentHash: r.ContentHash, Summary: r.Summary, Author: r.AuthorID, ChangedBy: r.ChangedByID,
		ChangedAt: r.ChangedAt.Time.UTC(),
	}
	if request := storage.ID(r.RequestID); request != "" {
		c.Evidence = append(c.Evidence, governance.Evidence{Kind: "approvalRequest", Ref: request, Link: approvalLink(c.AppID, request)})
		for _, d := range ev.decisions[request] {
			c.Approvals = append(c.Approvals, approvalOf(d))
			c.Evidence = append(c.Evidence, governance.Evidence{Kind: "decisionRecord", Ref: storage.ID(d.ID)}, governance.Evidence{Kind: "auditEntry", Ref: strconv.FormatInt(d.AuditSequence, 10)})
		}
	}
	if id := storage.ID(r.BreakGlassID); id != "" {
		c.Evidence = append(c.Evidence, governance.Evidence{Kind: "breakGlass", Ref: id, Link: "/breakglass/" + id})
		if b, ok := ev.breakGlass[id]; ok {
			c.BreakGlass = breakGlassOf(b)
		}
	}
	return c
}

// approvalOf describes one decision, with the audit key's signature over its
// record when the worker has signed it.
func approvalOf(d dbgen.ApprovalDecision) governance.Approval {
	a := governance.Approval{
		DeciderID: d.DeciderID, Decider: d.Decider, Decision: d.Decision, Comment: d.Comment,
		DecidedAt: d.DecidedAt.Time.UTC(), AuditSequence: d.AuditSequence,
	}
	if d.KeyID != nil {
		a.KeyID, a.Signature = *d.KeyID, base64.StdEncoding.EncodeToString(d.Signature)
	}
	return a
}

// breakGlassOf describes an invocation and the state of its review.
func breakGlassOf(b dbgen.BreakGlassInvocation) *governance.BreakGlass {
	bg := &governance.BreakGlass{
		Justification: b.Justification, ReviewDueAt: b.ReviewDueAt.Time.UTC(), ReviewStatus: b.ReviewStatus,
		ReviewedBy: b.ReviewedByID,
	}
	if b.ReviewedAt.Valid {
		bg.ReviewedAt = b.ReviewedAt.Time.UTC()
	}
	return bg
}

// approvalLink is where Studio shows an approval request: a path.
func approvalLink(appID, requestID string) string {
	if appID == "" {
		return "/approvals/" + requestID
	}
	return "/apps/" + appID + "/approvals/" + requestID
}

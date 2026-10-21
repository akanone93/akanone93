// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package approval

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// Reviewer supplies the evidence that belongs to one resource type: what a
// release changes, what a function version is. The release service is the
// reviewer of releases.
type Reviewer interface {
	// Evidence gathers the evidence of a request inside the reader's
	// transaction.
	Evidence(ctx context.Context, tx pgx.Tx, r Request) (Evidence, error)
}

// Evidence is what a Reviewer contributes to a review bundle (GOV-023).
type Evidence struct {
	// Diff is the JSON diff of the change.
	Diff json.RawMessage
	// Reports are the validation and accessibility reports.
	Reports []Report
	// GoldenDiffs are the cases whose golden image differs from the
	// baseline the server holds.
	GoldenDiffs []GoldenImage
	// Changelog lists the changes, one line each.
	Changelog []string
	// Notes are the author's release notes.
	Notes string
}

// Report is one report as JSON.
type Report struct {
	Kind    string
	Content json.RawMessage
}

// GoldenImage is a golden-image difference: the approved baseline, empty
// when the case has none, and the image the run produced, both PNG.
type GoldenImage struct {
	PluginKey string
	CaseID    string
	Baseline  []byte
	Actual    []byte
}

// InstallBaseEntry counts the devices of the environment on one release and
// host build; Channel names the channel whose current release it is.
type InstallBaseEntry struct {
	Channel         string
	ReleaseSequence int64
	HostBuild       string
	Devices         int64
}

// RiskSummary is the computed risk of a change.
type RiskSummary struct {
	// Level is "low", "medium" or "high".
	Level   string
	Reasons []string
}

// Review is the bundle an approver sees before deciding (GOV-023).
type Review struct {
	Request     Request
	Evidence    Evidence
	InstallBase []InstallBaseEntry
	Risk        RiskSummary
}

// Review assembles the review bundle of a request: the resource's evidence
// from its Reviewer, the install base of the environment, and the computed
// risk summary.
func (s *Service) Review(ctx context.Context, p auth.Principal, appID, id string) (Review, error) {
	if err := authorizeScope(p, auth.AppRead, appID); err != nil {
		return Review{}, fmt.Errorf("approval: %w", err)
	}
	var out Review
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		req, err := s.loadRequest(ctx, q, appID, id)
		if err != nil {
			return err
		}
		out.Request = req
		if out.Evidence, err = s.evidence(ctx, tx, req); err != nil {
			return err
		}
		if out.InstallBase, err = installBase(ctx, q, req); err != nil {
			return err
		}
		out.Risk = riskOf(req, out.InstallBase)
		return nil
	})
	return out, err
}

// evidence asks the resource's reviewer, or falls back to what the request
// itself holds.
func (s *Service) evidence(ctx context.Context, tx pgx.Tx, req Request) (Evidence, error) {
	if r, ok := s.reviewers[req.Resource]; ok {
		return r.Evidence(ctx, tx, req)
	}
	return Evidence{Diff: req.Payload, Notes: req.Summary}, nil
}

// installBase counts the devices of the request's environment by the
// release they hold and their host build.
func installBase(ctx context.Context, q *dbgen.Queries, req Request) ([]InstallBaseEntry, error) {
	if req.EnvironmentID == "" {
		return nil, nil
	}
	env := storage.MustUUID(req.EnvironmentID)
	groups, err := q.ApprovalInstallBase(ctx, env)
	if err != nil {
		return nil, fmt.Errorf("approval: count the install base: %w", err)
	}
	channels, err := q.ApprovalChannels(ctx, env)
	if err != nil {
		return nil, fmt.Errorf("approval: list the channels: %w", err)
	}
	var out []InstallBaseEntry
	for _, g := range groups {
		entry := InstallBaseEntry{ReleaseSequence: g.InstalledSequence, HostBuild: g.HostBuild, Devices: g.Devices}
		named := false
		for _, c := range channels {
			if c.ReleaseSequence == g.InstalledSequence {
				e := entry
				e.Channel = c.Key
				out = append(out, e)
				named = true
			}
		}
		if !named {
			out = append(out, entry)
		}
	}
	return out, nil
}

// riskOf computes the risk summary of a request from its risk attributes,
// its resource, its environment and how many devices run the current release.
func riskOf(req Request, base []InstallBaseEntry) RiskSummary {
	out := RiskSummary{Level: "low"}
	raise := func(level, reason string) {
		out.Reasons = append(out.Reasons, reason)
		if level == "high" || out.Level == "low" {
			out.Level = level
		}
	}
	for _, r := range req.Risk {
		raise("high", riskReason(r))
	}
	switch req.Resource {
	case schema.PolicyResourceSecurityConfiguration, schema.PolicyResourceCapabilitySet, schema.PolicyResourceLimit, schema.PolicyResourcePolicy:
		raise("high", fmt.Sprintf("it changes %s, which governs other changes", req.Resource))
	}
	if req.Production {
		raise("medium", "it is made in a production environment")
	}
	var devices int64
	for _, e := range base {
		if e.Channel != "" {
			devices += e.Devices
		}
	}
	if devices > 0 {
		raise("medium", fmt.Sprintf("%d device(s) run a channel's current release", devices))
	}
	return out
}

// riskReason says in words why a risk attribute matters.
func riskReason(r schema.PolicyRisk) string {
	switch r {
	case schema.PolicyRiskTouchesSecurePage:
		return "it touches a page marked secure"
	case schema.PolicyRiskTouchesMoneyFlow:
		return "it touches a flow with a transaction confirmation"
	case schema.PolicyRiskChangesFunctionUsedBy:
		return "it changes a function that a money flow calls"
	}
	return string(r)
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package approval

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// Requirement is what one matching policy asks of the approvers of a change.
type Requirement struct {
	// PolicyKey is the key of the policy.
	PolicyKey string
	// Draft is true for the draft being tried and false for a policy in
	// force.
	Draft bool
	// Policy is the requirement as the policy document states it.
	Policy schema.PolicyRequire
}

// Simulation is the answer to "who would need to approve this change?".
type Simulation struct {
	// Matches is whether the draft matches the sample change.
	Matches bool
	// Requirements are the matching policies in key order, the draft
	// included when it matches.
	Requirements []Requirement
	// Required is the most approvals any matching policy asks for.
	Required int
	// RequiresEnterprise is true when the draft uses a feature of the
	// enterprise edition.
	RequiresEnterprise bool
}

// SimulatePolicy tries a draft policy against a sample change and says which
// policies would match and what they would ask of the approvers. The draft
// stands in for the policy in force under its key. Nothing is written: no
// version, no request, no audit record. It needs policies.manage, as the
// editor does (GOV-020).
func (s *Service) SimulatePolicy(ctx context.Context, p auth.Principal, appID string, raw []byte, sample Artifact) (Simulation, error) {
	if err := authorizeScope(p, auth.PoliciesManage, appID); err != nil {
		return Simulation{}, fmt.Errorf("approval: %w", err)
	}
	doc, canonical, err := s.ParsePolicy(raw)
	if err != nil {
		return Simulation{}, err
	}
	var out Simulation
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		if err := checkEnvironments(ctx, q, appID, doc); err != nil {
			return err
		}
		change, err := s.sampleChange(ctx, q, p, appID, sample)
		if err != nil {
			return err
		}
		profile, err := s.profileOf(ctx, q, Artifact{AppID: appID})
		if err != nil {
			return err
		}
		inForce, err := s.policiesIn(ctx, tx, p.OrganizationID, appID, profile)
		if err != nil {
			return err
		}
		out = simulate(inForce, Policy{Doc: doc, Hash: HashOf(canonical)}, change)
		return nil
	})
	return out, err
}

// sampleChange completes a sample with the organisation, the app and the
// environment it names, so that it is matched as a real change would be.
func (*Service) sampleChange(ctx context.Context, q *dbgen.Queries, p auth.Principal, appID string, sample Artifact) (Artifact, error) {
	sample.OrganizationID = p.OrganizationID
	sample.AppID = appID
	key := sample.Environment.Key
	sample.Environment = Environment{}
	if key == "" {
		return sample, nil
	}
	app, err := scopeUUID(appID)
	if err != nil {
		return Artifact{}, err
	}
	env, err := q.GetEnvironmentByKey(ctx, dbgen.GetEnvironmentByKeyParams{AppID: app, Key: key})
	if errors.Is(err, pgx.ErrNoRows) {
		return Artifact{}, plxerr.New(plxerr.PolicyDocumentInvalid, "the sample change names no environment of the app: %q", key)
	}
	if err != nil {
		return Artifact{}, fmt.Errorf("approval: read an environment: %w", err)
	}
	sample.Environment = Environment{ID: storage.ID(env.ID), Key: env.Key, Production: env.Production}
	return sample, nil
}

// simulate matches a change against the policies in force with the draft in
// place of the policy of its key.
func simulate(inForce []Policy, draft Policy, change Artifact) Simulation {
	policies := slices.DeleteFunc(slices.Clone(inForce), func(x Policy) bool { return x.Doc.Key == draft.Doc.Key })
	policies = append(policies, draft)
	out := Simulation{RequiresEnterprise: needsEnterprise(draft.Doc)}
	for _, m := range matching(policies, change) {
		isDraft := m.Doc.Key == draft.Doc.Key
		out.Matches = out.Matches || isDraft
		out.Required = max(out.Required, int(m.Doc.Require.Approvers))
		out.Requirements = append(out.Requirements, Requirement{PolicyKey: m.Doc.Key, Draft: isDraft, Policy: m.Doc.Require})
	}
	slices.SortStableFunc(out.Requirements, func(a, b Requirement) int { return strings.Compare(a.PolicyKey, b.PolicyKey) })
	return out
}

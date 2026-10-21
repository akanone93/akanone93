// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/approval"
	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// capabilitiesSubject names the app's approved capability set to the
// approval engine.
const capabilitiesSubject = "capabilities:app"

// gateCapabilities asks the approval engine whether the app's approved
// capability set may change (SEC-080, GOV-020): when an app publish carries
// a set that differs from the one the previous app publish carried, the
// change needs the approval a policy on resource "capabilitySet" asks for.
// A refusal is a diagnostic that fails the publish, naming the request. The
// approval is spent when the gate passes.
func (s *Service) gateCapabilities(ctx context.Context, system auth.Principal, row dbgen.PublishJob, c compiled, diags plxerr.Diagnostics) (plxerr.Diagnostics, error) {
	if s.o.Approvals == nil || row.PluginID.Valid || c.result == nil || c.result.Project == nil || c.result.Project.App.Doc == nil {
		return diags, nil
	}
	proposed := c.result.Project.App.Doc.Capabilities
	var refusal error
	err := s.inOrg(ctx, system, func(ctx context.Context, tx pgx.Tx) error {
		changed, err := s.capabilitiesChanged(ctx, tx, row, proposed)
		if err != nil || !changed {
			return err
		}
		art, err := s.capabilityArtifact(ctx, tx, row, proposed)
		if err != nil {
			return err
		}
		refusal = approval.Require(ctx, s.o.Approvals, tx, art)
		return nil
	})
	if err != nil {
		return diags, err
	}
	if refusal == nil {
		return diags, nil
	}
	var pe *plxerr.Error
	if !errors.As(refusal, &pe) {
		return diags, fmt.Errorf("release: %w", refusal)
	}
	return append(slices.Clone(diags), plxerr.NewDiagnostic(pe.Code, plxerr.Location{File: "app.json", Path: "/capabilities"}, "%s", pe.Message)), nil
}

// capabilityArtifact describes the proposed capability set.
func (*Service) capabilityArtifact(ctx context.Context, tx pgx.Tx, row dbgen.PublishJob, proposed *schema.ApprovedCapabilities) (approval.Artifact, error) {
	env, err := dbgen.New(tx).GetEnvironment(ctx, row.EnvironmentID)
	if err != nil {
		return approval.Artifact{}, failure(err, "environment")
	}
	hash, err := approval.HashJSON(proposed)
	if err != nil {
		return approval.Artifact{}, err //nolint:wrapcheck // wrapped by approval
	}
	raw, err := json.Marshal(proposed)
	if err != nil {
		return approval.Artifact{}, fmt.Errorf("release: encode the capabilities: %w", err)
	}
	who := audit.Actor{Kind: row.ActorKind, ID: row.ActorID, Display: row.ActorDisplay}
	return approval.Artifact{
		Resource: schema.PolicyResourceCapabilitySet, OrganizationID: storage.ID(row.OrganizationID), AppID: storage.ID(row.AppID),
		Environment: approval.Environment{ID: storage.ID(env.ID), Key: env.Key, Production: env.Production},
		Action:      approval.ActionSetCapabilities, Subject: capabilitiesSubject, Hash: hash, Payload: raw, Requester: who,
		Author:  authorOf(who),
		Summary: "the approved capability set of the app changes",
	}, nil
}

// authorOf is who to treat as the author of a change: a person.
func authorOf(a audit.Actor) string {
	if a.Kind == auth.KindUser {
		return a.ID
	}
	return ""
}

// capabilitiesChanged reports whether the proposed set differs from the one
// the previous app publish carried. The first app publish of an app changes
// nothing: there was no set before it.
func (s *Service) capabilitiesChanged(ctx context.Context, tx pgx.Tx, row dbgen.PublishJob, proposed *schema.ApprovedCapabilities) (bool, error) {
	previous, found, err := s.previousCapabilities(ctx, tx, row)
	if err != nil || !found {
		return false, err
	}
	was, err := approval.HashJSON(previous)
	if err != nil {
		return false, err //nolint:wrapcheck // wrapped by approval
	}
	now, err := approval.HashJSON(proposed)
	if err != nil {
		return false, err //nolint:wrapcheck // wrapped by approval
	}
	return was != now, nil
}

// previousCapabilities reads the capability set of the app document of the
// newest published app version.
func (s *Service) previousCapabilities(ctx context.Context, tx pgx.Tx, row dbgen.PublishJob) (*schema.ApprovedCapabilities, bool, error) {
	v, err := dbgen.New(tx).LatestVersion(ctx, dbgen.LatestVersionParams{AppID: row.AppID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, failure(err, "version")
	}
	fsys, err := s.o.Documents.SourceFiles(ctx, tx, storage.ID(row.AppID), []string{storage.ID(v.SourceSnapshotID)})
	if err != nil {
		return nil, false, err //nolint:wrapcheck // a domain error
	}
	raw, err := fs.ReadFile(fsys, "app.json")
	if err != nil {
		return nil, false, fmt.Errorf("release: read the previous app document: %w", err)
	}
	var doc schema.AppDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, false, fmt.Errorf("release: decode the previous app document: %w", err)
	}
	return doc.Capabilities, true, nil
}

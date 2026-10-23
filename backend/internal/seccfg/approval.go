// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package seccfg

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/approval"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/security/settings"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// requireApproval asks the approval engine whether an environment's security
// configuration may become want (GOV-020, SEC-182). Turning device-placed
// functions on is a second change with an action of its own, so that a
// policy can ask more of it (FN-055). The approval is about the profile and
// overrides asked for, whatever version they land in; the engine decides by
// the profile the environment has now, so tightening a profile is not
// judged by the profile it tightens to. A change that leaves the
// configuration as it is needs none.
//
// It runs before the environment is locked and before the audit entry
// (approval.Require).
func (s *Service) requireApproval(ctx context.Context, tx pgx.Tx, p auth.Principal, env dbgen.Environment, want Values) error {
	if s.o.Approvals == nil {
		return nil
	}
	current, err := latest(ctx, dbgen.New(tx), env.ID)
	if err != nil {
		return err
	}
	if current.Version > 0 && same(current.Values, want) {
		return nil
	}
	overrides, err := want.OverridesJSON()
	if err != nil {
		return err
	}
	art := approval.Artifact{
		Resource: schema.PolicyResourceSecurityConfiguration, OrganizationID: p.OrganizationID, AppID: storage.ID(env.AppID),
		Environment: approval.Environment{ID: storage.ID(env.ID), Key: env.Key, Production: env.Production},
		Action:      approval.ActionSet, Subject: "securityConfiguration:" + storage.ID(env.ID),
		Hash:    approval.HashOf([]byte(want.Profile()), overrides),
		Summary: fmt.Sprintf("security configuration of %s: profile %s", env.Key, want.Profile()),
		Payload: summary(want, overrides), Requester: p.Actor(), RequesterUser: p.UserID, Profile: string(current.Values.Profile()),
	}
	arts := []approval.Artifact{art}
	if !current.Values.Get(settings.DeviceFunctions).Bool() && want.Get(settings.DeviceFunctions).Bool() {
		enable := art
		enable.Action = approval.ActionEnableDeviceFunctions
		enable.Summary = fmt.Sprintf("device-placed functions are enabled in %s", env.Key)
		arts = append(arts, enable)
	}
	return approval.Require(ctx, s.o.Approvals, tx, arts...) //nolint:wrapcheck // a refusal for the caller
}

// summary is the content an approver sees of a proposed configuration: the
// profile and the overrides, which name settings and never hold secrets.
func summary(want Values, overrides []byte) []byte {
	return fmt.Appendf(nil, `{"profile":%q,"overrides":%s}`, want.Profile(), overrides)
}

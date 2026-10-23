// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package experiment

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/approval"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/schema"
)

// Launch describes an experiment that is about to start, for the approval
// gate.
type Launch struct {
	Principal auth.Principal
	// Tx is the transaction the start runs in; a gate that spends an
	// approval does so in it.
	Tx pgx.Tx
	// EnvironmentID and Environment are the identifier and the key of the
	// environment the experiment runs in, and Production whether it is a
	// production one.
	EnvironmentID string
	Environment   string
	Production    bool
	Experiment    Experiment
	// Risk lists the risk attributes the experiment carries, computed from
	// the draft: touchesSecurePage when a page marked secure shows a variant
	// of it, touchesMoneyFlow when the page belongs to a plugin or app that
	// declares a data source with a transaction mapping.
	Risk []schema.PolicyRisk
}

// ApprovalGate decides whether an experiment may start (ABT-007). The
// approval engine implements it: an experiment on a page marked secure or on
// a flow involving money needs an approval under the maximum security
// profile, and the gate refuses the start with PLX-8001 until it has one.
// The service calls it inside the start's transaction, before anything is
// audited.
type ApprovalGate interface {
	RequireLaunch(ctx context.Context, l Launch) error
}

// AllowAll is the gate used where no approval engine is wired: it allows
// every start.
type AllowAll struct{}

// RequireLaunch allows the start.
func (AllowAll) RequireLaunch(context.Context, Launch) error { return nil }

// EngineGate is the approval gate backed by the approval engine: it asks the
// engine whether the policies in force for the experiment's app and
// environment allow the start, and spends the approval it finds.
type EngineGate struct {
	// Engine is the approval engine; nil allows every start.
	Engine approval.Gate
}

// RequireLaunch asks the engine about the start. The approval is bound to
// the experiment's document, so editing the experiment after it was approved
// makes the approval stop counting (GOV-022).
func (g EngineGate) RequireLaunch(ctx context.Context, l Launch) error {
	e := l.Experiment
	art := approval.Artifact{
		Resource: schema.PolicyResourceExperiment, OrganizationID: l.Principal.OrganizationID, AppID: e.AppID,
		Environment: approval.Environment{ID: l.EnvironmentID, Key: l.Environment, Production: l.Production},
		Action:      approval.ActionLaunch, Subject: "experiment:" + e.ID,
		Hash:    approval.HashOf([]byte(e.ID), e.Document),
		Summary: fmt.Sprintf("Start experiment %q in %s", e.Key, l.Environment),
		Risk:    l.Risk, Requester: l.Principal.Actor(), RequesterUser: l.Principal.UserID,
	}
	if e.CreatedBy.Kind == auth.KindUser {
		art.Author = e.CreatedBy.ID
	}
	if err := approval.Require(ctx, g.Engine, l.Tx, art); err != nil {
		return fmt.Errorf("experiment.Approve: %w", err)
	}
	return nil
}

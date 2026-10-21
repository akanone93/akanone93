// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package experiment

import (
	"context"

	"github.com/nightCode42/plux3/backend/internal/auth"
)

// Launch describes an experiment that is about to start, for the approval
// gate.
type Launch struct {
	Principal auth.Principal
	// Environment is the key of the environment the experiment runs in.
	Environment string
	Experiment  Experiment
}

// ApprovalGate decides whether an experiment may start (ABT-007). The
// approval engine of milestone E2 implements it: an experiment on a page
// marked secure or on a flow involving money needs an approval under the
// maximum security profile, and the gate refuses the start with PLX-8001
// until it has one. The service calls it inside the start's transaction,
// before anything is written.
type ApprovalGate interface {
	RequireLaunch(ctx context.Context, l Launch) error
}

// AllowAll is the gate used until the approval engine is wired: it allows
// every start.
type AllowAll struct{}

// RequireLaunch allows the start.
func (AllowAll) RequireLaunch(context.Context, Launch) error { return nil }

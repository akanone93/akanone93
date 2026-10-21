// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/approval"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// deployment is a change of what runs in an environment that the approval
// engine may guard: a function version deployed, copied by a promotion, or
// made the target of an alias (FN-007, GOV-020).
type deployment struct {
	app     string
	env     dbgen.Environment
	name    string
	subject string
	// content is what the change makes live: the module and what is done with
	// it. The hash of it binds the approval (GOV-022).
	content [][]byte
	summary string
}

// requireDeployment asks the approval engine whether a deployment may
// happen. The permission and its step-up (function.deploy.production) stay
// what they were: the engine adds the second person a policy asks for. It
// runs before the function's lock and before the audit entry
// (approval.Require).
func (s *Service) requireDeployment(ctx context.Context, tx pgx.Tx, p auth.Principal, d deployment) error {
	return approval.Require(ctx, s.o.Approvals, tx, approval.Artifact{
		Resource: schema.PolicyResourceFunctionVersion, OrganizationID: p.OrganizationID, AppID: d.app,
		Environment: approval.Environment{ID: storage.ID(d.env.ID), Key: d.env.Key, Production: d.env.Production},
		Action:      approval.ActionDeploy, Subject: d.subject + "@" + storage.ID(d.env.ID),
		Hash: approval.HashOf(d.content...), Summary: d.summary, Requester: p.Actor(),
	})
}

// deployContent lists the parts of a deployment's hash: the function, its
// module and the placement and alias that go with it.
func deployContent(name string, version int32, module []byte, placement, alias string) [][]byte {
	return [][]byte{[]byte(name), []byte(strconv.Itoa(int(version))), module, []byte(placement), []byte(alias)}
}

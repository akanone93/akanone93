// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package approval

import (
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// orgScope is the app identifier under which the policies and requests of the
// whole organisation are stored: an app identifier of "" in the API and in an
// Artifact. An organisation's policies govern every app and the changes that
// belong to none, such as raising a limit (LIM-006).
const orgScope = "00000000-0000-7000-8000-000000000000"

// scopeUUID parses an app identifier into the value stored; "" is the
// organisation.
func scopeUUID(appID string) (pgtype.UUID, error) {
	if appID == "" {
		appID = orgScope
	}
	id, err := storage.UUID(appID)
	if err != nil {
		return pgtype.UUID{}, plxerr.New(plxerr.InvalidFormat, "the app identifier is not valid")
	}
	return id, nil
}

// scopeOf is scopeUUID for an identifier already validated by the caller.
func scopeOf(appID string) pgtype.UUID {
	id, err := scopeUUID(appID)
	if err != nil {
		return pgtype.UUID{}
	}
	return id
}

// appOf is the app identifier a stored value stands for; "" for the
// organisation.
func appOf(stored pgtype.UUID) string {
	if id := storage.ID(stored); id != orgScope {
		return id
	}
	return ""
}

// authorizeScope checks a permission on an app, or across the organisation
// when appID is empty.
func authorizeScope(p auth.Principal, want auth.Permission, appID string) error {
	if appID == "" {
		return p.Authorize(want)
	}
	return p.AuthorizeApp(want, appID)
}

// authorizeSomeEnvironment is the first check of a decision, made before the
// request and so its environment is known.
func authorizeSomeEnvironment(p auth.Principal, appID string) error {
	if appID == "" {
		return p.Authorize(auth.ApprovalsDecide)
	}
	return p.AuthorizeSomeEnvironment(auth.ApprovalsDecide, appID)
}

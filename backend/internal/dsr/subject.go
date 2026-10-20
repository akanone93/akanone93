// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package dsr

import (
	"crypto/sha256"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// Kind says what a subject identifier names.
type Kind string

// The kinds of data subject.
const (
	// KindDevice is a device identifier.
	KindDevice Kind = "device"
	// KindUser is a pseudonymous user identifier.
	KindUser Kind = "user"
)

// Subject is the device or pseudonymous user a request concerns, in one
// organisation.
type Subject struct {
	Kind Kind
	// ID is the device or user identifier.
	ID string
}

// resolved is a validated subject with the values every store needs.
type resolved struct {
	kind Kind
	// id is the canonical identifier, org the caller's organisation.
	id  pgtype.UUID
	org pgtype.UUID
	// hash identifies the subject in tombstones, requests and audit entries.
	hash []byte
}

// resolve validates a subject and computes its hash within an organisation.
func resolve(organizationID string, s Subject) (resolved, error) {
	if s.Kind != KindDevice && s.Kind != KindUser {
		return resolved{}, plxerr.New(plxerr.InvalidFormat, "the subject kind must be device or user")
	}
	id, err := storage.UUID(s.ID)
	if err != nil {
		return resolved{}, plxerr.New(plxerr.InvalidFormat, "the subject identifier is not a valid identifier")
	}
	org, err := storage.UUID(organizationID)
	if err != nil {
		return resolved{}, plxerr.New(plxerr.InvalidFormat, "the organisation identifier is not valid")
	}
	h := sha256.New()
	for _, part := range []string{"plux-dsr-v1", storage.ID(org), string(s.Kind), storage.ID(id)} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return resolved{kind: s.Kind, id: id, org: org, hash: h.Sum(nil)}, nil
}

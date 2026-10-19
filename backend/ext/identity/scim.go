// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/nightCode42/plux3/backend/edition"
)

// Errors a Directory returns, wrapped with %w; the SCIM server renders each
// as the status and scimType RFC 7644 §3.12 assigns.
var (
	// ErrUnauthorized reports a bearer token that is unknown, expired or
	// revoked (401).
	ErrUnauthorized = errors.New("identity: the token is not valid")
	// ErrNotFound reports a resource that does not exist in the
	// organisation (404).
	ErrNotFound = errors.New("identity: no such resource")
	// ErrConflict reports a resource that already exists (409, uniqueness).
	ErrConflict = errors.New("identity: the resource already exists")
	// ErrVersionMismatch reports an If-Match that no longer matches the
	// resource's version (412).
	ErrVersionMismatch = errors.New("identity: the resource changed")
	// ErrInvalid reports a value the directory cannot accept (400,
	// invalidValue).
	ErrInvalid = errors.New("identity: the value is not valid")
	// ErrRefused reports a change the organisation's own rules forbid, such
	// as removing the last owner or an installation administrator (409).
	ErrRefused = errors.New("identity: the change is refused")
)

// UserSpec is the desired state of a SCIM user.
type UserSpec struct {
	// ExternalID is the identity provider's identifier of the person.
	ExternalID string
	// UserName is the person's email address: SCIM's userName is the key of
	// the account.
	UserName    string
	DisplayName string
	// Active is false for a person who has left.
	Active bool
}

// GroupRef names a group a user belongs to.
type GroupRef struct {
	ID      string
	Display string
}

// User is a SCIM user as the directory holds it.
type User struct {
	ID          string
	ExternalID  string
	UserName    string
	DisplayName string
	Active      bool
	Groups      []GroupRef
	// Version changes with every change; the server renders it as the ETag.
	Version  int64
	Created  time.Time
	Modified time.Time
}

// GroupSpec is the desired state of a SCIM group.
type GroupSpec struct {
	ExternalID  string
	DisplayName string
	// MemberIDs are the user IDs of the members.
	MemberIDs []string
}

// Member is a member of a group.
type Member struct {
	UserID  string
	Display string
}

// Group is a SCIM group, which the core stores as a team of the
// organisation.
type Group struct {
	ID          string
	ExternalID  string
	DisplayName string
	Members     []Member
	Version     int64
	Created     time.Time
	Modified    time.Time
}

// ListQuery selects a page of resources. The directory supports a single
// equality filter, which is what SCIM clients use to find a resource they
// provisioned (RFC 7644 §3.4.2.2).
type ListQuery struct {
	// Attribute is "userName", "externalId" or "displayName"; empty lists
	// everything. Value is compared for equality, ignoring case for userName.
	Attribute string
	Value     string
	// StartIndex is 1-based; Count is the page size, which the directory
	// clamps to Limits().MaxResults.
	StartIndex int
	Count      int
}

// UserPage is a page of users and the number of users that match.
type UserPage struct {
	Total int
	Users []User
}

// GroupPage is a page of groups and the number of groups that match.
type GroupPage struct {
	Total  int
	Groups []Group
}

// DirectoryLimits bound what a SCIM client may ask for.
type DirectoryLimits struct {
	// MaxResults is the largest page the directory returns.
	MaxResults int
}

// Directory is the core's side of SCIM provisioning (GOV-004). Every method
// acts within one organisation, which Authenticate returned; the SCIM server
// never names another. Deprovisioning through ReplaceUser (Active false) or
// DeleteUser removes the person's access to the organisation and revokes
// their tokens at once, and their sessions when they belong to no other
// organisation.
type Directory interface {
	// Authenticate resolves a bearer token to its organisation, or returns
	// ErrUnauthorized.
	Authenticate(ctx context.Context, token string) (organizationID string, err error)
	// Limits reports what a client may ask for.
	Limits() DirectoryLimits

	CreateUser(ctx context.Context, org string, spec UserSpec) (User, error)
	GetUser(ctx context.Context, org, id string) (User, error)
	ListUsers(ctx context.Context, org string, q ListQuery) (UserPage, error)
	// ReplaceUser sets a user to spec. A non-zero ifVersion must equal the
	// user's version, or the call returns ErrVersionMismatch.
	ReplaceUser(ctx context.Context, org, id string, spec UserSpec, ifVersion int64) (User, error)
	DeleteUser(ctx context.Context, org, id string) error

	CreateGroup(ctx context.Context, org string, spec GroupSpec) (Group, error)
	GetGroup(ctx context.Context, org, id string) (Group, error)
	ListGroups(ctx context.Context, org string, q ListQuery) (GroupPage, error)
	// ReplaceGroup sets a group to spec; its members become exactly
	// spec.MemberIDs. A non-zero ifVersion must equal the group's version.
	ReplaceGroup(ctx context.Context, org, id string, spec GroupSpec, ifVersion int64) (Group, error)
	DeleteGroup(ctx context.Context, org, id string) error
}

// SCIMService is SCIM 2.0, the enterprise part of GOV-004. The core mounts
// the handler at /scim/v2/ behind its licence check and passes each request
// through with the edition's provider in the context, so an implementation
// gates its writes with edition.EnabledForWrite.
type SCIMService interface {
	// Handler returns the SCIM server over a directory. Its routes are
	// relative to /scim/v2/ (the core strips the prefix).
	Handler(d Directory) http.Handler
}

// NoSCIM is the community edition's SCIM: its handler answers every request
// with a SCIM error that carries PLX-8600.
type NoSCIM struct{}

var _ SCIMService = NoSCIM{}

// Handler returns a handler that refuses every request as not licensed.
func (NoSCIM) Handler(Directory) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		WriteSCIMError(w, http.StatusForbidden, "", notLicensed(edition.FeatureSCIM))
	})
}

// Extensions are the enterprise implementations a server is built with. A
// nil field selects the community default.
type Extensions struct {
	SAML SAMLProvider
	SCIM SCIMService
}

// OrDefault returns e with the community default in every nil field.
func (e Extensions) OrDefault() Extensions {
	if e.SAML == nil {
		e.SAML = NoSAML{}
	}
	if e.SCIM == nil {
		e.SCIM = NoSCIM{}
	}
	return e
}

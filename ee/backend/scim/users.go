// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package scim

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/nightCode42/plux3/backend/ext/identity"
)

// patchAttempts is how often a PATCH that did not name a version is retried
// when another change lands between its read and its write.
const patchAttempts = 3

// meta is the meta attribute of a resource.
type meta struct {
	ResourceType string `json:"resourceType"`
	Created      string `json:"created"`
	LastModified string `json:"lastModified"`
	Location     string `json:"location"`
	Version      string `json:"version"`
}

// newMeta builds the meta of a resource.
func newMeta(kind, id string, version int64, created, modified time.Time) meta {
	return meta{
		ResourceType: kind, Created: created.UTC().Format(time.RFC3339), LastModified: modified.UTC().Format(time.RFC3339),
		Location: location(kind+"s", id), Version: etag(version),
	}
}

// userResource is a User as the server renders it.
type userResource struct {
	Schemas     []string        `json:"schemas"`
	ID          string          `json:"id"`
	ExternalID  string          `json:"externalId,omitempty"`
	UserName    string          `json:"userName"`
	DisplayName string          `json:"displayName,omitempty"`
	Name        *userName       `json:"name,omitempty"`
	Emails      []userEmail     `json:"emails"`
	Active      bool            `json:"active"`
	Groups      []userGroupLink `json:"groups"`
	Meta        meta            `json:"meta"`
}

// userName is the name attribute: the display name, formatted.
type userName struct {
	Formatted string `json:"formatted"`
}

// userEmail is an email address of a user: the one that is the user name.
type userEmail struct {
	Value   string `json:"value"`
	Type    string `json:"type"`
	Primary bool   `json:"primary"`
}

// userGroupLink is a group a user belongs to.
type userGroupLink struct {
	Value   string `json:"value"`
	Ref     string `json:"$ref"`
	Display string `json:"display,omitempty"`
	Type    string `json:"type"`
}

// userOf renders a user.
func userOf(u identity.User) userResource {
	r := userResource{
		Schemas: []string{schemaUser}, ID: u.ID, ExternalID: u.ExternalID, UserName: u.UserName, DisplayName: u.DisplayName,
		Emails: []userEmail{{Value: u.UserName, Type: "work", Primary: true}}, Active: u.Active,
		Groups: make([]userGroupLink, 0, len(u.Groups)), Meta: newMeta("User", u.ID, u.Version, u.Created, u.Modified),
	}
	if u.DisplayName != "" {
		r.Name = &userName{Formatted: u.DisplayName}
	}
	for _, g := range u.Groups {
		r.Groups = append(r.Groups, userGroupLink{Value: g.ID, Ref: location("Groups", g.ID), Display: g.Display, Type: "direct"})
	}
	return r
}

// userRequest is the body of a create or a replace.
type userRequest struct {
	Schemas     []string `json:"schemas"`
	ExternalID  string   `json:"externalId"`
	UserName    string   `json:"userName"`
	DisplayName string   `json:"displayName"`
	Name        struct {
		Formatted  string `json:"formatted"`
		GivenName  string `json:"givenName"`
		FamilyName string `json:"familyName"`
	} `json:"name"`
	Active *flexBool `json:"active"`
}

// spec turns the request into the state the user is to have. A user is active
// unless the request says otherwise. The display name is the one the request
// gives, else its formatted name, else the given and family names.
func (q userRequest) spec() (identity.UserSpec, error) {
	if !hasSchema(q.Schemas, schemaUser) {
		return identity.UserSpec{}, reject(http.StatusBadRequest, "invalidSyntax", "schemas must name "+schemaUser)
	}
	if strings.TrimSpace(q.UserName) == "" {
		return identity.UserSpec{}, invalid("userName is required")
	}
	display := q.DisplayName
	if display == "" {
		display = q.Name.Formatted
	}
	if display == "" {
		display = strings.TrimSpace(q.Name.GivenName + " " + q.Name.FamilyName)
	}
	active := q.Active == nil || bool(*q.Active)
	return identity.UserSpec{ExternalID: q.ExternalID, UserName: q.UserName, DisplayName: display, Active: active}, nil
}

// readUser reads the body of a create or a replace.
func readUser(r *http.Request) (identity.UserSpec, error) {
	var q userRequest
	if err := decode(r, &q); err != nil {
		return identity.UserSpec{}, err
	}
	return q.spec()
}

// listUsers serves GET /Users.
func (s *server) listUsers(c call, _ string) {
	q, totalOnly, err := s.parseList(c.r, "userName", "externalId", "displayName")
	if err != nil {
		s.fail(c.w, c.r, err)
		return
	}
	found, err := s.dir.ListUsers(c.ctx(), c.org, q)
	if err != nil {
		s.fail(c.w, c.r, err)
		return
	}
	resources := make([]userResource, 0, len(found.Users))
	for _, u := range found.Users {
		resources = append(resources, userOf(u))
	}
	if totalOnly {
		resources = resources[:0]
	}
	s.respond(c, http.StatusOK, page(found.Total, q.StartIndex, resources, len(resources)), 0, "")
}

// createUser serves POST /Users.
func (s *server) createUser(c call, _ string) {
	spec, err := readUser(c.r)
	if err != nil {
		s.fail(c.w, c.r, err)
		return
	}
	u, err := s.dir.CreateUser(c.ctx(), c.org, spec)
	if err != nil {
		s.fail(c.w, c.r, err)
		return
	}
	s.respond(c, http.StatusCreated, userOf(u), u.Version, location("Users", u.ID))
}

// getUser serves GET /Users/{id}.
func (s *server) getUser(c call, id string) {
	u, err := s.dir.GetUser(c.ctx(), c.org, id)
	if err != nil {
		s.fail(c.w, c.r, err)
		return
	}
	if s.unchanged(c, u.Version) {
		return
	}
	s.respond(c, http.StatusOK, userOf(u), u.Version, "")
}

// replaceUser serves PUT /Users/{id}.
func (s *server) replaceUser(c call, id string) {
	spec, err := readUser(c.r)
	if err != nil {
		s.fail(c.w, c.r, err)
		return
	}
	current, err := s.dir.GetUser(c.ctx(), c.org, id)
	if err != nil {
		s.fail(c.w, c.r, err)
		return
	}
	if err := precondition(c.r, current.Version); err != nil {
		s.fail(c.w, c.r, err)
		return
	}
	version := int64(0)
	if c.r.Header.Get("If-Match") != "" {
		version = current.Version
	}
	u, err := s.dir.ReplaceUser(c.ctx(), c.org, id, spec, version)
	if err != nil {
		s.fail(c.w, c.r, err)
		return
	}
	s.respond(c, http.StatusOK, userOf(u), u.Version, "")
}

// patchUser serves PATCH /Users/{id}: read, apply, write against the version
// that was read. A client that named a version gets 412 if it moved; one that
// did not is retried against the new one.
func (s *server) patchUser(c call, id string) {
	ops, err := readPatch(c.r)
	if err != nil {
		s.fail(c.w, c.r, err)
		return
	}
	u, err := retryPatch(c.r, func() (identity.User, error) { return s.patchUserOnce(c, id, ops) })
	if err != nil {
		s.fail(c.w, c.r, err)
		return
	}
	s.respond(c, http.StatusOK, userOf(u), u.Version, "")
}

// retryPatch runs a read-apply-write until it lands. A request that named a
// version gets the mismatch; one that did not is tried again against the
// version that won, a few times.
func retryPatch[T any](r *http.Request, once func() (T, error)) (T, error) {
	named := r.Header.Get("If-Match") != ""
	for attempt := 1; ; attempt++ {
		v, err := once()
		if errors.Is(err, identity.ErrVersionMismatch) && !named && attempt < patchAttempts {
			continue
		}
		return v, err
	}
}

// patchUserOnce reads a user, applies the operations and writes the result
// against the version read.
func (s *server) patchUserOnce(c call, id string, ops []patchOp) (identity.User, error) {
	current, err := s.dir.GetUser(c.ctx(), c.org, id)
	if err != nil {
		return identity.User{}, err //nolint:wrapcheck // examined by fail
	}
	if err := precondition(c.r, current.Version); err != nil {
		return identity.User{}, err
	}
	spec := identity.UserSpec{
		ExternalID: current.ExternalID, UserName: current.UserName, DisplayName: current.DisplayName, Active: current.Active,
	}
	if err := applyUserOps(&spec, ops); err != nil {
		return identity.User{}, err
	}
	return s.dir.ReplaceUser(c.ctx(), c.org, id, spec, current.Version) //nolint:wrapcheck // examined by fail
}

// deleteUser serves DELETE /Users/{id}.
func (s *server) deleteUser(c call, id string) {
	current, err := s.dir.GetUser(c.ctx(), c.org, id)
	if err == nil {
		err = precondition(c.r, current.Version)
	}
	if err == nil {
		err = s.dir.DeleteUser(c.ctx(), c.org, id)
	}
	if err != nil {
		s.fail(c.w, c.r, err)
		return
	}
	c.w.WriteHeader(http.StatusNoContent)
}

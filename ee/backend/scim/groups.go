// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package scim

import (
	"net/http"
	"strings"

	"github.com/nightCode42/plux3/backend/ext/identity"
)

// groupResource is a Group as the server renders it.
type groupResource struct {
	Schemas     []string      `json:"schemas"`
	ID          string        `json:"id"`
	ExternalID  string        `json:"externalId,omitempty"`
	DisplayName string        `json:"displayName"`
	Members     []groupMember `json:"members"`
	Meta        meta          `json:"meta"`
}

// groupMember is a member of a group.
type groupMember struct {
	Value   string `json:"value"`
	Ref     string `json:"$ref"`
	Display string `json:"display,omitempty"`
	Type    string `json:"type"`
}

// groupOf renders a group.
func groupOf(g identity.Group) groupResource {
	r := groupResource{
		Schemas: []string{schemaGroup}, ID: g.ID, ExternalID: g.ExternalID, DisplayName: g.DisplayName,
		Members: make([]groupMember, 0, len(g.Members)), Meta: newMeta("Group", g.ID, g.Version, g.Created, g.Modified),
	}
	for _, m := range g.Members {
		r.Members = append(r.Members, groupMember{Value: m.UserID, Ref: location("Users", m.UserID), Display: m.Display, Type: "User"})
	}
	return r
}

// groupRequest is the body of a create or a replace.
type groupRequest struct {
	Schemas     []string    `json:"schemas"`
	ExternalID  string      `json:"externalId"`
	DisplayName string      `json:"displayName"`
	Members     []memberRef `json:"members"`
}

// spec turns the request into the state the group is to have.
func (q groupRequest) spec() (identity.GroupSpec, error) {
	if !hasSchema(q.Schemas, schemaGroup) {
		return identity.GroupSpec{}, reject(http.StatusBadRequest, "invalidSyntax", "schemas must name "+schemaGroup)
	}
	if strings.TrimSpace(q.DisplayName) == "" {
		return identity.GroupSpec{}, invalid("displayName is required")
	}
	spec := identity.GroupSpec{ExternalID: q.ExternalID, DisplayName: q.DisplayName}
	for _, m := range q.Members {
		if m.Value == "" {
			return identity.GroupSpec{}, invalid("a member needs a value")
		}
	}
	for _, m := range q.Members {
		spec.MemberIDs = appendMissing(spec.MemberIDs, []string{m.Value})
	}
	return spec, nil
}

// readGroup reads the body of a create or a replace.
func readGroup(r *http.Request) (identity.GroupSpec, error) {
	var q groupRequest
	if err := decode(r, &q); err != nil {
		return identity.GroupSpec{}, err
	}
	return q.spec()
}

// listGroups serves GET /Groups.
func (s *server) listGroups(c call, _ string) {
	q, totalOnly, err := s.parseList(c.r, "externalId", "displayName")
	if err != nil {
		s.fail(c.w, c.r, err)
		return
	}
	found, err := s.dir.ListGroups(c.ctx(), c.org, q)
	if err != nil {
		s.fail(c.w, c.r, err)
		return
	}
	resources := make([]groupResource, 0, len(found.Groups))
	for _, g := range found.Groups {
		resources = append(resources, groupOf(g))
	}
	if totalOnly {
		resources = resources[:0]
	}
	s.respond(c, http.StatusOK, page(found.Total, q.StartIndex, resources, len(resources)), 0, "")
}

// createGroup serves POST /Groups.
func (s *server) createGroup(c call, _ string) {
	spec, err := readGroup(c.r)
	if err != nil {
		s.fail(c.w, c.r, err)
		return
	}
	g, err := s.dir.CreateGroup(c.ctx(), c.org, spec)
	if err != nil {
		s.fail(c.w, c.r, err)
		return
	}
	s.respond(c, http.StatusCreated, groupOf(g), g.Version, location("Groups", g.ID))
}

// getGroup serves GET /Groups/{id}.
func (s *server) getGroup(c call, id string) {
	g, err := s.dir.GetGroup(c.ctx(), c.org, id)
	if err != nil {
		s.fail(c.w, c.r, err)
		return
	}
	if s.unchanged(c, g.Version) {
		return
	}
	s.respond(c, http.StatusOK, groupOf(g), g.Version, "")
}

// replaceGroup serves PUT /Groups/{id}.
func (s *server) replaceGroup(c call, id string) {
	spec, err := readGroup(c.r)
	if err != nil {
		s.fail(c.w, c.r, err)
		return
	}
	current, err := s.dir.GetGroup(c.ctx(), c.org, id)
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
	g, err := s.dir.ReplaceGroup(c.ctx(), c.org, id, spec, version)
	if err != nil {
		s.fail(c.w, c.r, err)
		return
	}
	s.respond(c, http.StatusOK, groupOf(g), g.Version, "")
}

// patchGroup serves PATCH /Groups/{id}, as patchUser does for a user.
func (s *server) patchGroup(c call, id string) {
	ops, err := readPatch(c.r)
	if err != nil {
		s.fail(c.w, c.r, err)
		return
	}
	g, err := retryPatch(c.r, func() (identity.Group, error) { return s.patchGroupOnce(c, id, ops) })
	if err != nil {
		s.fail(c.w, c.r, err)
		return
	}
	s.respond(c, http.StatusOK, groupOf(g), g.Version, "")
}

// patchGroupOnce reads a group, applies the operations and writes the result
// against the version read.
func (s *server) patchGroupOnce(c call, id string, ops []patchOp) (identity.Group, error) {
	current, err := s.dir.GetGroup(c.ctx(), c.org, id)
	if err != nil {
		return identity.Group{}, err //nolint:wrapcheck // examined by fail
	}
	if err := precondition(c.r, current.Version); err != nil {
		return identity.Group{}, err
	}
	spec := identity.GroupSpec{ExternalID: current.ExternalID, DisplayName: current.DisplayName}
	for _, m := range current.Members {
		spec.MemberIDs = append(spec.MemberIDs, m.UserID)
	}
	if err := applyGroupOps(&spec, ops); err != nil {
		return identity.Group{}, err
	}
	return s.dir.ReplaceGroup(c.ctx(), c.org, id, spec, current.Version) //nolint:wrapcheck // examined by fail
}

// deleteGroup serves DELETE /Groups/{id}.
func (s *server) deleteGroup(c call, id string) {
	current, err := s.dir.GetGroup(c.ctx(), c.org, id)
	if err == nil {
		err = precondition(c.r, current.Version)
	}
	if err == nil {
		err = s.dir.DeleteGroup(c.ctx(), c.org, id)
	}
	if err != nil {
		s.fail(c.w, c.r, err)
		return
	}
	c.w.WriteHeader(http.StatusNoContent)
}

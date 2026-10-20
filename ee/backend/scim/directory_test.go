// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package scim_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/nightCode42/plux3/backend/ext/identity"
)

// fakeDirectory is an in-memory identity.Directory that keeps the rules the
// core's keeps and the server relies on: a resource is in one organisation,
// userName is unique, a version changes with every change, and a person set
// inactive loses their tokens and sessions at once.
type fakeDirectory struct {
	mu     sync.Mutex
	clock  time.Time
	next   int
	tokens map[string]string // SCIM bearer token -> organisation
	users  map[string]*userRow
	groups map[string]*groupRow
	// userTokens and sessions are what deprovisioning must end, by user.
	userTokens map[string][]string
	sessions   map[string]int
	// failWith, when set, is returned by every read of a user.
	failWith error
}

type userRow struct {
	org string
	identity.User
}

type groupRow struct {
	org string
	identity.Group
}

// newDirectory returns a directory with a SCIM token for each organisation.
func newDirectory(orgs ...string) *fakeDirectory {
	d := &fakeDirectory{
		clock: time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC), tokens: map[string]string{},
		users: map[string]*userRow{}, groups: map[string]*groupRow{}, userTokens: map[string][]string{}, sessions: map[string]int{},
	}
	for _, o := range orgs {
		d.tokens["token-"+o] = o
	}
	return d
}

func (d *fakeDirectory) id(kind string) string {
	d.next++
	return fmt.Sprintf("%s-%04d", kind, d.next)
}

func (d *fakeDirectory) tick() time.Time {
	d.clock = d.clock.Add(time.Minute)
	return d.clock
}

// Authenticate resolves a token.
func (d *fakeDirectory) Authenticate(_ context.Context, token string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if org, ok := d.tokens[token]; ok {
		return org, nil
	}
	return "", identity.ErrUnauthorized
}

// Limits reports a small page so that paging is easy to reach.
func (d *fakeDirectory) Limits() identity.DirectoryLimits {
	return identity.DirectoryLimits{MaxResults: 3}
}

func (d *fakeDirectory) user(org, id string) (*userRow, error) {
	if d.failWith != nil {
		return nil, d.failWith
	}
	u, ok := d.users[id]
	if !ok || u.org != org {
		return nil, identity.ErrNotFound
	}
	return u, nil
}

// view returns a user with the groups they are in.
func (d *fakeDirectory) view(u *userRow) identity.User {
	out := u.User
	out.Groups = nil
	for _, g := range d.groups {
		for _, m := range g.Members {
			if g.org == u.org && m.UserID == u.ID {
				out.Groups = append(out.Groups, identity.GroupRef{ID: g.ID, Display: g.DisplayName})
			}
		}
	}
	return out
}

// CreateUser makes a user; a userName is unique within the organisation.
func (d *fakeDirectory) CreateUser(_ context.Context, org string, spec identity.UserSpec) (identity.User, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, u := range d.users {
		if u.org == org && strings.EqualFold(u.UserName, spec.UserName) {
			return identity.User{}, identity.ErrConflict
		}
	}
	if !strings.Contains(spec.UserName, "@") {
		return identity.User{}, fmt.Errorf("%w: the user name is not an email address", identity.ErrInvalid)
	}
	now := d.tick()
	u := &userRow{org: org, User: identity.User{
		ID: d.id("user"), ExternalID: spec.ExternalID, UserName: spec.UserName, DisplayName: spec.DisplayName,
		Active: spec.Active, Version: 1, Created: now, Modified: now,
	}}
	d.users[u.ID] = u
	return d.view(u), nil
}

// GetUser returns a user.
func (d *fakeDirectory) GetUser(_ context.Context, org, id string) (identity.User, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	u, err := d.user(org, id)
	if err != nil {
		return identity.User{}, err
	}
	return d.view(u), nil
}

// matches reports whether an attribute equals a value.
func matches(attr, value string, fields map[string]string) bool {
	if attr == "" {
		return true
	}
	if attr == "userName" {
		return strings.EqualFold(fields[attr], value)
	}
	return fields[attr] == value
}

// window cuts a sorted list to the page asked for.
func window[T any](all []T, q identity.ListQuery) []T {
	start := min(max(q.StartIndex-1, 0), len(all))
	end := min(start+q.Count, len(all))
	return all[start:end]
}

// ListUsers lists users by equality on one attribute.
func (d *fakeDirectory) ListUsers(_ context.Context, org string, q identity.ListQuery) (identity.UserPage, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var all []identity.User
	for _, u := range d.users {
		fields := map[string]string{"userName": u.UserName, "externalId": u.ExternalID, "displayName": u.DisplayName}
		if u.org == org && matches(q.Attribute, q.Value, fields) {
			all = append(all, d.view(u))
		}
	}
	slices.SortFunc(all, func(a, b identity.User) int { return strings.Compare(a.ID, b.ID) })
	return identity.UserPage{Total: len(all), Users: window(all, q)}, nil
}

// ReplaceUser sets a user to spec. Setting a user inactive revokes their
// tokens and ends their sessions.
func (d *fakeDirectory) ReplaceUser(_ context.Context, org, id string, spec identity.UserSpec, ifVersion int64) (identity.User, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	u, err := d.user(org, id)
	if err != nil {
		return identity.User{}, err
	}
	if ifVersion != 0 && ifVersion != u.Version {
		return identity.User{}, identity.ErrVersionMismatch
	}
	if u.Active && !spec.Active {
		d.deprovision(id)
	}
	u.ExternalID, u.UserName, u.DisplayName, u.Active = spec.ExternalID, spec.UserName, spec.DisplayName, spec.Active
	u.Version++
	u.Modified = d.tick()
	return d.view(u), nil
}

// deprovision ends a person's access.
func (d *fakeDirectory) deprovision(id string) {
	for _, t := range d.userTokens[id] {
		delete(d.tokens, t)
	}
	delete(d.userTokens, id)
	delete(d.sessions, id)
}

// DeleteUser removes a user, deprovisioning them first.
func (d *fakeDirectory) DeleteUser(_ context.Context, org, id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, err := d.user(org, id); err != nil {
		return err
	}
	d.deprovision(id)
	delete(d.users, id)
	return nil
}

// members resolves user identifiers to members of the organisation.
func (d *fakeDirectory) members(org string, ids []string) ([]identity.Member, error) {
	var out []identity.Member
	for _, id := range ids {
		u, ok := d.users[id]
		if !ok || u.org != org {
			return nil, fmt.Errorf("%w: %q is not a user of the organisation", identity.ErrInvalid, id)
		}
		out = append(out, identity.Member{UserID: id, Display: u.DisplayName})
	}
	return out, nil
}

// CreateGroup makes a team.
func (d *fakeDirectory) CreateGroup(_ context.Context, org string, spec identity.GroupSpec) (identity.Group, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, g := range d.groups {
		if g.org == org && g.DisplayName == spec.DisplayName {
			return identity.Group{}, identity.ErrConflict
		}
	}
	members, err := d.members(org, spec.MemberIDs)
	if err != nil {
		return identity.Group{}, err
	}
	now := d.tick()
	g := &groupRow{org: org, Group: identity.Group{
		ID: d.id("group"), ExternalID: spec.ExternalID, DisplayName: spec.DisplayName, Members: members,
		Version: 1, Created: now, Modified: now,
	}}
	d.groups[g.ID] = g
	return g.Group, nil
}

func (d *fakeDirectory) group(org, id string) (*groupRow, error) {
	g, ok := d.groups[id]
	if !ok || g.org != org {
		return nil, identity.ErrNotFound
	}
	return g, nil
}

// GetGroup returns a group.
func (d *fakeDirectory) GetGroup(_ context.Context, org, id string) (identity.Group, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	g, err := d.group(org, id)
	if err != nil {
		return identity.Group{}, err
	}
	return g.Group, nil
}

// ListGroups lists groups by equality on one attribute.
func (d *fakeDirectory) ListGroups(_ context.Context, org string, q identity.ListQuery) (identity.GroupPage, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var all []identity.Group
	for _, g := range d.groups {
		if g.org == org && matches(q.Attribute, q.Value, map[string]string{"externalId": g.ExternalID, "displayName": g.DisplayName}) {
			all = append(all, g.Group)
		}
	}
	slices.SortFunc(all, func(a, b identity.Group) int { return strings.Compare(a.ID, b.ID) })
	return identity.GroupPage{Total: len(all), Groups: window(all, q)}, nil
}

// ReplaceGroup sets a group; its members become exactly the ones given.
func (d *fakeDirectory) ReplaceGroup(_ context.Context, org, id string, spec identity.GroupSpec, ifVersion int64) (identity.Group, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	g, err := d.group(org, id)
	if err != nil {
		return identity.Group{}, err
	}
	if ifVersion != 0 && ifVersion != g.Version {
		return identity.Group{}, identity.ErrVersionMismatch
	}
	members, err := d.members(org, spec.MemberIDs)
	if err != nil {
		return identity.Group{}, err
	}
	g.ExternalID, g.DisplayName, g.Members = spec.ExternalID, spec.DisplayName, members
	g.Version++
	g.Modified = d.tick()
	return g.Group, nil
}

// DeleteGroup removes a group.
func (d *fakeDirectory) DeleteGroup(_ context.Context, org, id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, err := d.group(org, id); err != nil {
		return err
	}
	delete(d.groups, id)
	return nil
}

// signIn gives a user a token and a session of their own, which
// deprovisioning must end.
func (d *fakeDirectory) signIn(id, token string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.tokens[token] = "personal"
	d.userTokens[id] = append(d.userTokens[id], token)
	d.sessions[id]++
}

// access reports whether a user still has a token and a session.
func (d *fakeDirectory) access(id string) (tokens, sessions int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.userTokens[id]), d.sessions[id]
}

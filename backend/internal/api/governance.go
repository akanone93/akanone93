// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// Governance serves GovernanceService: the permission catalogue, roles
// and grants (GOV-002, GOV-003, ADR-0073).
type Governance struct{ h *Handlers }

var _ pluxv1connect.GovernanceServiceHandler = Governance{}

// Governance returns the GovernanceService handler.
func (h *Handlers) Governance() Governance { return Governance{h: h} }

// ListPermissions returns the permission catalogue.
func (s Governance) ListPermissions(ctx context.Context, req *connect.Request[pluxv1.ListPermissionsRequest]) (*connect.Response[pluxv1.ListPermissionsResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListPermissionsResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		if err := p.Authorize(auth.OrganizationRead); err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListPermissionsResponse{}
		for _, e := range auth.Catalogue() {
			out.Permissions = append(out.Permissions, &pluxv1.PermissionEntry{
				Name: string(e.Name), Description: e.Description, Risk: string(e.Risk),
				StepUp: e.StepUp, Scope: string(e.Scope), ReadOnly: e.ReadOnly,
			})
		}
		return out, nil
	})
}

// ListRoles lists the predefined and custom roles.
func (s Governance) ListRoles(ctx context.Context, req *connect.Request[pluxv1.ListRolesRequest]) (*connect.Response[pluxv1.ListRolesResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListRolesResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		q := Query{OrderBy: "key", Filters: map[string]func(Item) string{
			"key": func(m Item) string { return m.(*pluxv1.RoleDefinition).GetKey() },
		}}
		page := req.Msg.GetPage()
		clauses, err := q.Parse(page)
		if err != nil {
			return nil, err
		}
		after, size, err := s.h.Pages.Request(req.Spec().Procedure, p.OrganizationID, page)
		if err != nil {
			return nil, err
		}
		roles, err := s.h.Auth.ListRoles(ctx, p, after.Key, size)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListRolesResponse{Page: &pluxv1.PageResult{}}
		var last storage.Cursor
		for _, r := range roles {
			last = storage.Cursor{Key: r.Key}
			if item := roleProto(r); q.Keep(clauses, item) {
				out.Roles = append(out.Roles, item)
			}
		}
		out.Page.NextPageToken = s.h.Pages.Next(req.Spec().Procedure, p.OrganizationID, page, len(roles), size, last)
		return out, Mask(page.GetReadMask(), out.Roles)
	})
}

// GetRole returns one role.
func (s Governance) GetRole(ctx context.Context, req *connect.Request[pluxv1.GetRoleRequest]) (*connect.Response[pluxv1.GetRoleResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.GetRoleResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		r, err := s.h.Auth.GetRole(ctx, p, req.Msg.GetKey())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.GetRoleResponse{Role: roleProto(r)}, nil
	})
}

// CreateRole creates a custom role.
func (s Governance) CreateRole(ctx context.Context, req *connect.Request[pluxv1.CreateRoleRequest]) (*connect.Response[pluxv1.CreateRoleResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.CreateRoleResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		r, err := s.h.Auth.CreateRole(ctx, p, m.GetKey(), m.GetName(), m.GetDescription(), m.GetPermissions())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.CreateRoleResponse{Role: roleProto(r)}, nil
	})
}

// UpdateRole replaces a custom role's name, description and permissions.
func (s Governance) UpdateRole(ctx context.Context, req *connect.Request[pluxv1.UpdateRoleRequest]) (*connect.Response[pluxv1.UpdateRoleResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.UpdateRoleResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		r, err := s.h.Auth.UpdateRole(ctx, p, m.GetKey(), m.GetName(), m.GetDescription(), m.GetPermissions())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.UpdateRoleResponse{Role: roleProto(r)}, nil
	})
}

// DeleteRole deletes a custom role no grant uses.
func (s Governance) DeleteRole(ctx context.Context, req *connect.Request[pluxv1.DeleteRoleRequest]) (*connect.Response[pluxv1.DeleteRoleResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.DeleteRoleResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		if err := s.h.Auth.DeleteRole(ctx, p, req.Msg.GetKey()); err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.DeleteRoleResponse{}, nil
	})
}

// ListGrants lists the grants made through this service.
func (s Governance) ListGrants(ctx context.Context, req *connect.Request[pluxv1.ListGrantsRequest]) (*connect.Response[pluxv1.ListGrantsResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListGrantsResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		q := Query{OrderBy: "id", Filters: map[string]func(Item) string{
			"role":  func(m Item) string { return m.(*pluxv1.RoleGrant).GetRole() },
			"scope": func(m Item) string { return m.(*pluxv1.RoleGrant).GetScope() },
		}}
		page := req.Msg.GetPage()
		clauses, err := q.Parse(page)
		if err != nil {
			return nil, err
		}
		after, size, err := s.h.Pages.Request(req.Spec().Procedure, p.OrganizationID, page)
		if err != nil {
			return nil, err
		}
		filter := auth.GrantFilter{UserID: req.Msg.GetUserId(), TeamID: req.Msg.GetTeamId(), AppID: req.Msg.GetAppId()}
		grants, err := s.h.Auth.ListGrants(ctx, p, filter, after, size)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListGrantsResponse{Page: &pluxv1.PageResult{}}
		var last storage.Cursor
		for _, g := range grants {
			last = storage.Cursor{ID: g.ID}
			if item := roleGrantProto(p.OrganizationID, g); q.Keep(clauses, item) {
				out.Grants = append(out.Grants, item)
			}
		}
		out.Page.NextPageToken = s.h.Pages.Next(req.Spec().Procedure, p.OrganizationID, page, len(grants), size, last)
		return out, Mask(page.GetReadMask(), out.Grants)
	})
}

// CreateGrant binds a role to a user or a team at a scope.
func (s Governance) CreateGrant(ctx context.Context, req *connect.Request[pluxv1.CreateGrantRequest]) (*connect.Response[pluxv1.CreateGrantResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.CreateGrantResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		g, err := s.h.Auth.CreateGrant(ctx, p, auth.GrantSpec{
			UserID: m.GetUserId(), TeamID: m.GetTeamId(), Role: m.GetRole(), Scope: auth.Scope(m.GetScope()),
			AppID: m.GetAppId(), EnvironmentID: m.GetEnvironmentId(),
		})
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.CreateGrantResponse{Grant: roleGrantProto(p.OrganizationID, g)}, nil
	})
}

// UpdateGrant changes the role of a grant.
func (s Governance) UpdateGrant(ctx context.Context, req *connect.Request[pluxv1.UpdateGrantRequest]) (*connect.Response[pluxv1.UpdateGrantResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.UpdateGrantResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		g, err := s.h.Auth.UpdateGrant(ctx, p, req.Msg.GetId(), req.Msg.GetRole())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.UpdateGrantResponse{Grant: roleGrantProto(p.OrganizationID, g)}, nil
	})
}

// DeleteGrant removes a grant.
func (s Governance) DeleteGrant(ctx context.Context, req *connect.Request[pluxv1.DeleteGrantRequest]) (*connect.Response[pluxv1.DeleteGrantResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.DeleteGrantResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		if err := s.h.Auth.DeleteGrant(ctx, p, req.Msg.GetId()); err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.DeleteGrantResponse{}, nil
	})
}

// roleProto converts a role.
func roleProto(r auth.RoleDefinition) *pluxv1.RoleDefinition {
	return &pluxv1.RoleDefinition{
		Key: r.Key, Name: r.Name, Description: r.Description, Predefined: r.Predefined,
		Permissions: r.Permissions, CreatedAt: ts(r.CreatedAt), UpdatedAt: ts(r.UpdatedAt),
	}
}

// roleGrantProto converts a grant.
func roleGrantProto(org string, g auth.RoleGrantRecord) *pluxv1.RoleGrant {
	return &pluxv1.RoleGrant{
		Id: g.ID, OrganizationId: org, UserId: g.UserID, TeamId: g.TeamID, Role: g.Role,
		Scope: string(g.Scope), AppId: g.AppID, EnvironmentId: g.EnvironmentID, CreatedAt: ts(g.CreatedAt),
	}
}

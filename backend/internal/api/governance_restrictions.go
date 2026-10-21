// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// CreateRestriction creates a restriction: a ceiling that removes
// permissions from its subject whatever their grants say (GOV-006).
func (s Governance) CreateRestriction(ctx context.Context, req *connect.Request[pluxv1.CreateRestrictionRequest]) (*connect.Response[pluxv1.CreateRestrictionResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.CreateRestrictionResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		r, err := s.h.Auth.CreateRestriction(ctx, p, restrictionSpec(req.Msg))
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.CreateRestrictionResponse{Restriction: restrictionProto(r)}, nil
	})
}

// ListRestrictions lists the organisation's restrictions.
func (s Governance) ListRestrictions(ctx context.Context, req *connect.Request[pluxv1.ListRestrictionsRequest]) (*connect.Response[pluxv1.ListRestrictionsResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListRestrictionsResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		q := Query{OrderBy: "id", Filters: map[string]func(Item) string{
			"subject_kind": func(m Item) string { return m.(*pluxv1.Restriction).GetSubjectKind() },
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
		filter := auth.RestrictionFilter{SubjectID: req.Msg.GetSubjectId(), AppID: req.Msg.GetAppId()}
		found, err := s.h.Auth.ListRestrictions(ctx, p, filter, after, size)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListRestrictionsResponse{Page: &pluxv1.PageResult{}}
		var last storage.Cursor
		for _, r := range found {
			last = storage.Cursor{ID: r.ID}
			if item := restrictionProto(r); q.Keep(clauses, item) {
				out.Restrictions = append(out.Restrictions, item)
			}
		}
		out.Page.NextPageToken = s.h.Pages.Next(req.Spec().Procedure, p.OrganizationID, page, len(found), size, last)
		return out, Mask(page.GetReadMask(), out.Restrictions)
	})
}

// DeleteRestriction removes a restriction.
func (s Governance) DeleteRestriction(ctx context.Context, req *connect.Request[pluxv1.DeleteRestrictionRequest]) (*connect.Response[pluxv1.DeleteRestrictionResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.DeleteRestrictionResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		if err := s.h.Auth.DeleteRestriction(ctx, p, req.Msg.GetId()); err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.DeleteRestrictionResponse{}, nil
	})
}

// GetEffectivePermissions returns the caller's permissions in a scope as
// one sorted set with the organisation's permissions version (GOV-007).
func (s Governance) GetEffectivePermissions(ctx context.Context, req *connect.Request[pluxv1.GetEffectivePermissionsRequest]) (*connect.Response[pluxv1.GetEffectivePermissionsResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.GetEffectivePermissionsResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), req.Msg.GetOrganizationId())
		if err != nil {
			return nil, err
		}
		set, err := p.Effective(auth.AccessScope{AppID: req.Msg.GetAppId(), Environment: req.Msg.GetEnvironment()})
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.GetEffectivePermissionsResponse{Version: set.Version}
		for _, perm := range set.Permissions {
			out.Permissions = append(out.Permissions, string(perm))
		}
		return out, nil
	})
}

// WatchPermissions streams the organisation's permissions version: the
// current one, then each newer one as it is announced, so that a client
// refetches without polling (GOV-007).
func (s Governance) WatchPermissions(ctx context.Context, req *connect.Request[pluxv1.WatchPermissionsRequest], stream *connect.ServerStream[pluxv1.WatchPermissionsResponse]) error {
	p, err := s.h.principal(ctx, req.Header(), req.Msg.GetOrganizationId())
	if err != nil {
		return err
	}
	return s.h.Auth.WatchPermissions(ctx, p, func(version int64) error { //nolint:wrapcheck // a domain error
		return stream.Send(&pluxv1.WatchPermissionsResponse{OrganizationId: p.OrganizationID, Version: version}) //nolint:wrapcheck // the stream's own error
	})
}

// ExplainAccess says why a person can or cannot perform an action, naming
// the grants that allow it and the restrictions that remove it; with a
// proposal it evaluates the change without saving it (GOV-007). Anyone may
// explain their own access; another person's needs members.manage.
func (s Governance) ExplainAccess(ctx context.Context, req *connect.Request[pluxv1.ExplainAccessRequest]) (*connect.Response[pluxv1.ExplainAccessResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ExplainAccessResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), req.Msg.GetOrganizationId())
		if err != nil {
			return nil, err
		}
		query := accessQuery(req.Msg)
		if err := s.h.explainRate(ctx, p, query.Grant != nil || query.Restriction != nil); err != nil {
			return nil, err
		}
		d, err := s.h.Auth.ExplainAccess(ctx, p, query)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return explainProto(d), nil
	})
}

// explainRate counts an explain call against the principal's allowance: a
// separate, tighter one when it evaluates a proposal, because that runs the
// resolver on a state that does not exist (GOV-007). Without a limiter
// there is nothing to count.
func (h *Handlers) explainRate(ctx context.Context, p auth.Principal, simulating bool) error {
	if h.Limiter.Count == nil {
		return nil
	}
	key, limit := "rate:explain:", h.Limits.Get(limits.AuthExplainPerMinute)
	if simulating {
		key, limit = "rate:simulate:", h.Limits.Get(limits.AuthSimulatePerMinute)
	}
	_, err := h.Limiter.Allow(ctx, key+p.OrganizationID+":"+p.Kind+":"+p.ID, limit)
	return err
}

// restrictionSpec converts the request that creates a restriction.
func restrictionSpec(m *pluxv1.CreateRestrictionRequest) auth.RestrictionSpec {
	return auth.RestrictionSpec{
		SubjectKind: auth.SubjectKind(m.GetSubjectKind()), SubjectID: m.GetSubjectId(), Patterns: m.GetPatterns(),
		AppID: m.GetAppId(), Environment: m.GetEnvironment(), Reason: m.GetReason(),
	}
}

// accessQuery converts an explain request, with its proposal if it has one.
func accessQuery(m *pluxv1.ExplainAccessRequest) auth.AccessQuery {
	q := auth.AccessQuery{
		UserID: m.GetUserId(), Permission: auth.Permission(m.GetPermission()),
		Scope: auth.AccessScope{AppID: m.GetAppId(), Environment: m.GetEnvironment()},
	}
	if g := m.GetProposedGrant(); g != nil {
		q.Grant = &auth.ProposedGrant{
			UserID: g.GetUserId(), TeamID: g.GetTeamId(), Role: g.GetRole(),
			Scope: auth.Scope(g.GetScope()), AppID: g.GetAppId(), Environment: g.GetEnvironment(),
		}
	}
	if r := m.GetProposedRestriction(); r != nil {
		spec := restrictionSpec(r)
		q.Restriction = &spec
	}
	return q
}

// explainProto converts the answer to an explain call.
func explainProto(d auth.AccessDecision) *pluxv1.ExplainAccessResponse {
	decision := "deny"
	if d.Allowed {
		decision = "allow"
	}
	return &pluxv1.ExplainAccessResponse{
		Decision: decision, AllowingGrantIds: d.AllowingGrants,
		RemovingRestrictionIds: d.RemovingRestrictions, OwnerFloor: d.OwnerFloor,
	}
}

// restrictionProto converts a restriction.
func restrictionProto(r auth.Restriction) *pluxv1.Restriction {
	return &pluxv1.Restriction{
		Id: r.ID, SubjectKind: string(r.SubjectKind), SubjectId: r.SubjectID, Patterns: r.Patterns,
		AppId: r.AppID, Environment: r.Environment, Reason: r.Reason, CreatedBy: r.CreatedBy, CreatedAt: ts(r.CreatedAt),
	}
}

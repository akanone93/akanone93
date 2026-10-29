// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"slices"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/nightCode42/plux3/backend/internal/approval"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// Approval serves ApprovalService: policies, requests and decisions
// (GOV-020 to GOV-023, ADR-0076).
type Approval struct{ h *Handlers }

var _ pluxv1connect.ApprovalServiceHandler = Approval{}

// Approval returns the ApprovalService handler.
func (h *Handlers) Approval() Approval { return Approval{h: h} }

// PutPolicy writes a policy document as the next version of its key.
func (s Approval) PutPolicy(ctx context.Context, req *connect.Request[pluxv1.PutPolicyRequest]) (*connect.Response[pluxv1.PutPolicyResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.PutPolicyResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		rec, err := s.h.Approvals.PutPolicy(ctx, p, req.Msg.GetAppId(), req.Msg.GetDocument())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.PutPolicyResponse{Policy: policyProto(rec)}, nil
	})
}

// GetPolicy returns the policy in force for a key.
func (s Approval) GetPolicy(ctx context.Context, req *connect.Request[pluxv1.GetPolicyRequest]) (*connect.Response[pluxv1.GetPolicyResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.GetPolicyResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		rec, err := s.h.Approvals.GetPolicy(ctx, p, req.Msg.GetAppId(), req.Msg.GetKey())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.GetPolicyResponse{Policy: policyProto(rec)}, nil
	})
}

// ListPolicies lists the policies in force for an app.
func (s Approval) ListPolicies(ctx context.Context, req *connect.Request[pluxv1.ListPoliciesRequest]) (*connect.Response[pluxv1.ListPoliciesResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListPoliciesResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		recs, err := s.h.Approvals.ListPolicies(ctx, p, req.Msg.GetAppId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListPoliciesResponse{}
		for _, r := range recs {
			out.Policies = append(out.Policies, policyProto(r))
		}
		return out, nil
	})
}

// DeletePolicy withdraws a policy.
func (s Approval) DeletePolicy(ctx context.Context, req *connect.Request[pluxv1.DeletePolicyRequest]) (*connect.Response[pluxv1.DeletePolicyResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.DeletePolicyResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		if err := s.h.Approvals.DeletePolicy(ctx, p, req.Msg.GetAppId(), req.Msg.GetKey()); err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.DeletePolicyResponse{}, nil
	})
}

// ExportPolicies returns the policies as the files of the Git layout.
func (s Approval) ExportPolicies(ctx context.Context, req *connect.Request[pluxv1.ExportPoliciesRequest]) (*connect.Response[pluxv1.ExportPoliciesResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ExportPoliciesResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		files, err := s.h.Approvals.ExportPolicies(ctx, p, req.Msg.GetAppId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ExportPoliciesResponse{}
		for path, content := range files {
			out.Files = append(out.Files, &pluxv1.PolicyFile{Path: path, Content: content})
		}
		slices.SortFunc(out.Files, func(a, b *pluxv1.PolicyFile) int { return strings.Compare(a.GetPath(), b.GetPath()) })
		return out, nil
	})
}

// SimulatePolicy tries a draft policy against a sample change without saving
// anything.
func (s Approval) SimulatePolicy(ctx context.Context, req *connect.Request[pluxv1.SimulatePolicyRequest]) (*connect.Response[pluxv1.SimulatePolicyResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.SimulatePolicyResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		sample := approval.Artifact{
			Resource:    schema.PolicyResource(m.GetChange().GetResource()),
			Action:      m.GetChange().GetAction(),
			Environment: approval.Environment{Key: m.GetChange().GetEnvironment()},
		}
		for _, r := range m.GetChange().GetRisk() {
			sample.Risk = append(sample.Risk, schema.PolicyRisk(r))
		}
		sim, err := s.h.Approvals.SimulatePolicy(ctx, p, m.GetAppId(), m.GetDocument(), sample)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return simulationProto(sim), nil
	})
}

// ListApprovals lists the approval requests of an app, newest first.
func (s Approval) ListApprovals(ctx context.Context, req *connect.Request[pluxv1.ListApprovalsRequest]) (*connect.Response[pluxv1.ListApprovalsResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListApprovalsResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		q := Query{OrderBy: "created", Filters: map[string]func(Item) string{
			"resource": func(m Item) string { return m.(*pluxv1.ApprovalRequest).GetResource() },
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
		reqs, err := s.h.Approvals.ListApprovals(ctx, p, req.Msg.GetAppId(), req.Msg.GetStatus(), after, size)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListApprovalsResponse{Page: &pluxv1.PageResult{}}
		var last storage.Cursor
		for _, r := range reqs {
			last = storage.Cursor{Time: r.CreatedAt, ID: r.ID}
			if item := approvalProto(r); q.Keep(clauses, item) {
				out.Approvals = append(out.Approvals, item)
			}
		}
		out.Page.NextPageToken = s.h.Pages.Next(req.Spec().Procedure, p.OrganizationID, page, len(reqs), size, last)
		return out, Mask(page.GetReadMask(), out.Approvals)
	})
}

// GetApproval returns one request with its decisions.
func (s Approval) GetApproval(ctx context.Context, req *connect.Request[pluxv1.GetApprovalRequest]) (*connect.Response[pluxv1.GetApprovalResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.GetApprovalResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		r, err := s.h.Approvals.GetApproval(ctx, p, req.Msg.GetAppId(), req.Msg.GetId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.GetApprovalResponse{Approval: approvalProto(r)}, nil
	})
}

// GetApprovalReview returns the review bundle of a request.
func (s Approval) GetApprovalReview(ctx context.Context, req *connect.Request[pluxv1.GetApprovalReviewRequest]) (*connect.Response[pluxv1.GetApprovalReviewResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.GetApprovalReviewResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		r, err := s.h.Approvals.Review(ctx, p, req.Msg.GetAppId(), req.Msg.GetId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return reviewProto(r), nil
	})
}

// DecideApproval approves or rejects a request.
func (s Approval) DecideApproval(ctx context.Context, req *connect.Request[pluxv1.DecideApprovalRequest]) (*connect.Response[pluxv1.DecideApprovalResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.DecideApprovalResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		r, err := s.h.Approvals.Decide(ctx, p, m.GetAppId(), m.GetId(), m.GetDecision(), m.GetComment())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.DecideApprovalResponse{Approval: approvalProto(r)}, nil
	})
}

// policyProto converts a stored policy version.
func policyProto(r approval.PolicyRecord) *pluxv1.PolicyRecord {
	return &pluxv1.PolicyRecord{
		AppId: r.AppID, Key: r.Key, Version: r.Version, Document: r.Document, DocumentHash: r.Hash,
		Deleted: r.Deleted, CreatedBy: r.CreatedBy, CreatedAt: timestamppb.New(r.CreatedAt),
	}
}

// simulationProto converts the answer of a policy simulation.
func simulationProto(sim approval.Simulation) *pluxv1.SimulatePolicyResponse {
	out := &pluxv1.SimulatePolicyResponse{
		Matches: sim.Matches, RequiresEnterprise: sim.RequiresEnterprise,
		ApprovalsRequired: int32(sim.Required), //nolint:gosec // at most the schema's maximum of 10
	}
	for _, r := range sim.Requirements {
		req := r.Policy
		out.Requirements = append(out.Requirements, &pluxv1.SimulatedRequirement{
			PolicyKey: r.PolicyKey, Draft: r.Draft, Approvers: int32(req.Approvers), //nolint:gosec // at most the schema's maximum of 10
			Roles: req.Roles, Users: req.Users, Stages: string(req.Stages),
			DistinctTeams:      req.DistinctTeams != nil && *req.DistinctTeams,
			DistinctFromAuthor: req.DistinctFromAuthor != nil && *req.DistinctFromAuthor,
		})
	}
	return out
}

// approvalProto converts a request with its decisions.
func approvalProto(r approval.Request) *pluxv1.ApprovalRequest {
	out := &pluxv1.ApprovalRequest{
		Id: r.ID, AppId: r.AppID, Environment: r.Environment, Resource: string(r.Resource), Action: r.Action, Subject: r.Subject,
		ContentHash: r.ContentHash, Summary: r.Summary, Status: r.Status, RequestedBy: r.RequestedBy, PolicyKeys: r.PolicyKeys,
		ApprovalsRequired: int32(r.ApprovalsNeeded), //nolint:gosec // at most the schema's maximum of 10
		CreatedAt:         timestamppb.New(r.CreatedAt), ExpiresAt: timestamppb.New(r.ExpiresAt),
	}
	for _, risk := range r.Risk {
		out.Risk = append(out.Risk, string(risk))
	}
	for _, d := range r.Decisions {
		out.Decisions = append(out.Decisions, &pluxv1.ApprovalDecision{
			Id: d.ID, DeciderId: d.DeciderID, Decider: d.Decider, Decision: d.Decision, Comment: d.Comment, ContentHash: d.ContentHash,
			DecidedAt: timestamppb.New(d.DecidedAt), Payload: d.Payload, KeyId: d.KeyID, Signature: d.Signature, AuditSequence: d.AuditSequence,
		})
	}
	return out
}

// reviewProto converts a review bundle.
func reviewProto(r approval.Review) *pluxv1.GetApprovalReviewResponse {
	out := &pluxv1.GetApprovalReviewResponse{
		Approval: approvalProto(r.Request), Diff: r.Evidence.Diff, Changelog: r.Evidence.Changelog, Notes: r.Evidence.Notes,
		Risk: &pluxv1.ReviewRisk{Level: r.Risk.Level, Reasons: r.Risk.Reasons},
	}
	for _, rep := range r.Evidence.Reports {
		out.Reports = append(out.Reports, &pluxv1.ReviewReport{Kind: rep.Kind, Content: rep.Content})
	}
	for _, g := range r.Evidence.GoldenDiffs {
		out.GoldenDiffs = append(out.GoldenDiffs, &pluxv1.ReviewImage{PluginKey: g.PluginKey, CaseId: g.CaseID, Baseline: g.Baseline, Actual: g.Actual})
	}
	for _, e := range r.InstallBase {
		out.InstallBase = append(out.InstallBase, &pluxv1.InstallBaseEntry{
			Channel: e.Channel, ReleaseSequence: e.ReleaseSequence, HostBuild: e.HostBuild, Devices: e.Devices,
		})
	}
	return out
}

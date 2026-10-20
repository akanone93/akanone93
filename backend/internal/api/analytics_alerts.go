// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/analytics"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
)

// CreateAlertRule defines an alert rule (ANL-030).
func (s AnalyticsHandler) CreateAlertRule(ctx context.Context, req *connect.Request[pluxv1.CreateAlertRuleRequest]) (*connect.Response[pluxv1.CreateAlertRuleResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.CreateAlertRuleResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		r, err := s.h.Analytics.CreateAlertRule(ctx, p, ruleOfProto(req.Msg.GetRule()))
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.CreateAlertRuleResponse{Rule: ruleProto(r)}, nil
	})
}

// UpdateAlertRule replaces a rule's definition.
func (s AnalyticsHandler) UpdateAlertRule(ctx context.Context, req *connect.Request[pluxv1.UpdateAlertRuleRequest]) (*connect.Response[pluxv1.UpdateAlertRuleResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.UpdateAlertRuleResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		r, err := s.h.Analytics.UpdateAlertRule(ctx, p, ruleOfProto(req.Msg.GetRule()))
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.UpdateAlertRuleResponse{Rule: ruleProto(r)}, nil
	})
}

// DeleteAlertRule removes a rule.
func (s AnalyticsHandler) DeleteAlertRule(ctx context.Context, req *connect.Request[pluxv1.DeleteAlertRuleRequest]) (*connect.Response[pluxv1.DeleteAlertRuleResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.DeleteAlertRuleResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		if err := s.h.Analytics.DeleteAlertRule(ctx, p, req.Msg.GetId()); err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.DeleteAlertRuleResponse{}, nil
	})
}

// ListAlertRules lists rules.
func (s AnalyticsHandler) ListAlertRules(ctx context.Context, req *connect.Request[pluxv1.ListAlertRulesRequest]) (*connect.Response[pluxv1.ListAlertRulesResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListAlertRulesResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		rules, err := s.h.Analytics.ListAlertRules(ctx, p, req.Msg.GetAppId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListAlertRulesResponse{}
		for _, r := range rules {
			out.Rules = append(out.Rules, ruleProto(r))
		}
		return out, nil
	})
}

// ListAlertStates lists where rules stand.
func (s AnalyticsHandler) ListAlertStates(ctx context.Context, req *connect.Request[pluxv1.ListAlertStatesRequest]) (*connect.Response[pluxv1.ListAlertStatesResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListAlertStatesResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		states, err := s.h.Analytics.ListAlertStates(ctx, p, req.Msg.GetAppId(), analytics.AlertPhase(req.Msg.GetState()))
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListAlertStatesResponse{}
		for _, st := range states {
			out.States = append(out.States, alertStateProto(st))
		}
		return out, nil
	})
}

// ListErrorGroups lists error groups (ANL-040).
func (s AnalyticsHandler) ListErrorGroups(ctx context.Context, req *connect.Request[pluxv1.ListErrorGroupsRequest]) (*connect.Response[pluxv1.ListErrorGroupsResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListErrorGroupsResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		groups, err := s.h.Analytics.ListErrorGroups(ctx, p, m.GetAppId(), m.GetReleaseSequence(), int(m.GetDays()), int(m.GetLimit()))
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListErrorGroupsResponse{}
		for _, g := range groups {
			out.Groups = append(out.Groups, errorGroupProto(g))
		}
		return out, nil
	})
}

// GetErrorGroup returns one error group with samples.
func (s AnalyticsHandler) GetErrorGroup(ctx context.Context, req *connect.Request[pluxv1.GetErrorGroupRequest]) (*connect.Response[pluxv1.GetErrorGroupResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.GetErrorGroupResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		g, samples, err := s.h.Analytics.GetErrorGroup(ctx, p, req.Msg.GetId(), int(req.Msg.GetDays()))
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.GetErrorGroupResponse{Group: errorGroupProto(g)}
		for _, sm := range samples {
			out.Samples = append(out.Samples, &pluxv1.ErrorSample{EventId: sm.EventID, OccurredAt: ts(sm.OccurredAt), FieldsJson: string(sm.Fields)})
		}
		return out, nil
	})
}

// ruleOfProto reads a rule definition; a nil message is an empty rule that
// validation refuses.
func ruleOfProto(m *pluxv1.AlertRule) analytics.Rule {
	r := analytics.Rule{
		ID: m.GetId(), AppID: m.GetAppId(), Name: m.GetName(), Metric: m.GetMetric(), Environment: m.GetEnvironment(),
		Aggregation: analytics.Aggregation(m.GetAggregation()), Window: time.Duration(m.GetWindowSeconds()) * time.Second,
		Comparison: analytics.Comparison(m.GetComparison()), Threshold: m.GetThreshold(),
		For: time.Duration(m.GetForSeconds()) * time.Second, Severity: analytics.Severity(m.GetSeverity()), Enabled: !m.GetDisabled(),
	}
	if f := m.GetFilters(); len(f) > 0 {
		r.Filters = make(map[analytics.Dimension]string, len(f))
		for k, v := range f {
			r.Filters[analytics.Dimension(k)] = v
		}
	}
	return r
}

func ruleProto(r analytics.Rule) *pluxv1.AlertRule {
	out := &pluxv1.AlertRule{
		Id: r.ID, AppId: r.AppID, Name: r.Name, Metric: r.Metric, Environment: r.Environment,
		Aggregation: string(r.Aggregation), WindowSeconds: int32(r.Window / time.Second), //nolint:gosec // G115: bounded by the 30-day maximum window.
		Comparison: string(r.Comparison), Threshold: r.Threshold,
		ForSeconds: int32(r.For / time.Second), //nolint:gosec // G115: bounded by the one-day maximum.
		Severity:   string(r.Severity), Disabled: !r.Enabled, Revision: r.Revision, UpdatedBy: r.UpdatedBy, UpdatedAt: ts(r.UpdatedAt),
	}
	if len(r.Filters) > 0 {
		out.Filters = make(map[string]string, len(r.Filters))
		for k, v := range r.Filters {
			out.Filters[string(k)] = v
		}
	}
	return out
}

func alertStateProto(st analytics.AlertState) *pluxv1.AlertState {
	return &pluxv1.AlertState{
		RuleId: st.RuleID, AppId: st.AppID, RuleName: st.RuleName, State: string(st.Phase), Since: ts(st.Since),
		EvaluatedAt: ts(st.EvaluatedAt), Value: st.Value, FiredAt: ts(st.FiredAt), ResolvedAt: ts(st.ResolvedAt),
	}
}

func errorGroupProto(g analytics.ErrorGroup) *pluxv1.ErrorGroup {
	out := &pluxv1.ErrorGroup{
		Id: g.ID, AppId: g.AppID, ReleaseSequence: g.ReleaseSequence, Fingerprint: g.Fingerprint, Code: g.Code,
		PluginKey: g.PluginKey, Route: g.Route, NodePath: g.NodePath, JsonPath: g.JSONPath, SourceFile: g.SourceFile,
		Frames: g.Frames, Fatal: g.Fatal, FirstSeen: ts(g.FirstSeen), LastSeen: ts(g.LastSeen), Events: g.Events, Devices: g.Devices,
	}
	for _, d := range g.Trend {
		out.Trend = append(out.Trend, &pluxv1.ErrorDay{Day: d.Day.UTC().Format(time.DateOnly), Events: d.Events})
	}
	return out
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/testrun"
)

// TestRun serves TestRunService (TST-001, TST-002).
type TestRun struct{ h *Handlers }

var _ pluxv1connect.TestRunServiceHandler = TestRun{}

// TestRun returns the TestRunService handler.
func (h *Handlers) TestRun() TestRun { return TestRun{h: h} }

func testRunProto(r testrun.Run) *pluxv1.TestRun {
	out := &pluxv1.TestRun{
		Id: r.ID, AppId: r.AppID, ReportKey: r.Key, Source: r.Source, Label: r.Label,
		Passed: r.Passed, Failed: r.Failed, Skipped: r.Skipped, DurationMs: r.DurationMs,
		CreatedBy: r.CreatedBy, CreatedAt: timestamppb.New(r.CreatedAt),
	}
	for _, sc := range r.Scenarios {
		s := &pluxv1.TestScenarioResult{Id: sc.ID, Name: sc.Name, Status: sc.Status, DurationMs: sc.DurationMs}
		for _, st := range sc.Steps {
			s.Steps = append(s.Steps, &pluxv1.TestStepResult{Index: st.Index, Description: st.Description, Status: st.Status, Message: st.Message})
		}
		out.Scenarios = append(out.Scenarios, s)
	}
	return out
}

func testScenarios(in []*pluxv1.TestScenarioResult) []testrun.Scenario {
	out := make([]testrun.Scenario, 0, len(in))
	for _, sc := range in {
		s := testrun.Scenario{ID: sc.GetId(), Name: sc.GetName(), Status: sc.GetStatus(), DurationMs: sc.GetDurationMs(), Steps: []testrun.Step{}}
		for _, st := range sc.GetSteps() {
			s.Steps = append(s.Steps, testrun.Step{Index: st.GetIndex(), Description: st.GetDescription(), Status: st.GetStatus(), Message: st.GetMessage()})
		}
		out = append(out, s)
	}
	return out
}

// ReportRun records a run reported by the CLI or CI.
func (s TestRun) ReportRun(ctx context.Context, req *connect.Request[pluxv1.ReportRunRequest]) (*connect.Response[pluxv1.ReportRunResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.ReportRunResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		run, err := s.h.TestRuns.Report(ctx, p, testrun.Report{
			AppID: m.GetAppId(), Key: m.GetReportKey(), Source: m.GetSource(), Label: m.GetLabel(), Scenarios: testScenarios(m.GetScenarios()),
		})
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.ReportRunResponse{Run: testRunProto(run)}, nil
	})
}

// ListRuns lists the runs of an app, newest first.
func (s TestRun) ListRuns(ctx context.Context, req *connect.Request[pluxv1.ListRunsRequest]) (*connect.Response[pluxv1.ListRunsResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListRunsResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		page := req.Msg.GetPage()
		after, size, err := s.h.Pages.Request(req.Spec().Procedure, p.OrganizationID, page)
		if err != nil {
			return nil, err
		}
		runs, err := s.h.TestRuns.List(ctx, p, req.Msg.GetAppId(), after, size)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListRunsResponse{Page: &pluxv1.PageResult{}}
		var last storage.Cursor
		for _, r := range runs {
			last = storage.Cursor{Time: r.CreatedAt, ID: r.ID}
			out.Runs = append(out.Runs, testRunProto(r))
		}
		out.Page.NextPageToken = s.h.Pages.Next(req.Spec().Procedure, p.OrganizationID, page, len(runs), size, last)
		return out, nil
	})
}

// GetRun returns one run with its results.
func (s TestRun) GetRun(ctx context.Context, req *connect.Request[pluxv1.GetRunRequest]) (*connect.Response[pluxv1.GetRunResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.GetRunResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		run, err := s.h.TestRuns.Get(ctx, p, req.Msg.GetAppId(), req.Msg.GetRunId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.GetRunResponse{Run: testRunProto(run)}, nil
	})
}

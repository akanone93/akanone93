// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

// solid is a PNG of one colour.
func solid(t *testing.T, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := range 4 {
		for x := range 4 {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// fakeApprovals answers the approval service from memory.
type fakeApprovals struct {
	pluxv1connect.UnimplementedApprovalServiceHandler
	calls    *[]string
	baseline []byte
	actual   []byte
}

func (f fakeApprovals) record(s string) { *f.calls = append(*f.calls, s) }

func request() *pluxv1.ApprovalRequest {
	return &pluxv1.ApprovalRequest{
		Id: "r1", AppId: "a1", Environment: "production", Resource: "release", Action: "promote", Subject: "release:3@production/production",
		ContentHash: "abc", Summary: "release 3 <script>", Status: "open", RequestedBy: "ada", PolicyKeys: []string{"p"},
		ApprovalsRequired: 2, ExpiresAt: timestamppb.New(time.Unix(1_800_000_000, 0)), CreatedAt: timestamppb.New(time.Unix(1_700_000_000, 0)),
		Decisions: []*pluxv1.ApprovalDecision{{Decider: "bob", Decision: "approve", DecidedAt: timestamppb.New(time.Unix(1_700_000_100, 0))}},
	}
}

func (f fakeApprovals) ListPolicies(_ context.Context, r *connect.Request[pluxv1.ListPoliciesRequest]) (*connect.Response[pluxv1.ListPoliciesResponse], error) {
	f.record("list policies app=" + r.Msg.GetAppId())
	return connect.NewResponse(&pluxv1.ListPoliciesResponse{Policies: []*pluxv1.PolicyRecord{{AppId: "a1", Key: "p", Version: 2, DocumentHash: "0123456789abcdef", CreatedBy: "ada", Document: []byte(`{"key":"p"}`)}}}), nil
}

func (f fakeApprovals) PutPolicy(_ context.Context, r *connect.Request[pluxv1.PutPolicyRequest]) (*connect.Response[pluxv1.PutPolicyResponse], error) {
	f.record("put " + string(r.Msg.GetDocument()))
	return connect.NewResponse(&pluxv1.PutPolicyResponse{Policy: &pluxv1.PolicyRecord{Key: "p", Version: 3, DocumentHash: "ff"}}), nil
}

func (f fakeApprovals) DeletePolicy(_ context.Context, r *connect.Request[pluxv1.DeletePolicyRequest]) (*connect.Response[pluxv1.DeletePolicyResponse], error) {
	f.record("delete policy " + r.Msg.GetKey() + " app=" + r.Msg.GetAppId())
	return connect.NewResponse(&pluxv1.DeletePolicyResponse{}), nil
}

func (f fakeApprovals) ExportPolicies(context.Context, *connect.Request[pluxv1.ExportPoliciesRequest]) (*connect.Response[pluxv1.ExportPoliciesResponse], error) {
	return connect.NewResponse(&pluxv1.ExportPoliciesResponse{Files: []*pluxv1.PolicyFile{
		{Path: "policies/p.policy.json", Content: []byte("{}\n")}, {Path: "../escape.json", Content: []byte("x")},
	}}), nil
}

func (f fakeApprovals) ListApprovals(_ context.Context, r *connect.Request[pluxv1.ListApprovalsRequest]) (*connect.Response[pluxv1.ListApprovalsResponse], error) {
	f.record("list approvals status=" + r.Msg.GetStatus())
	return connect.NewResponse(&pluxv1.ListApprovalsResponse{Approvals: []*pluxv1.ApprovalRequest{request()}}), nil
}

func (f fakeApprovals) GetApprovalReview(context.Context, *connect.Request[pluxv1.GetApprovalReviewRequest]) (*connect.Response[pluxv1.GetApprovalReviewResponse], error) {
	return connect.NewResponse(&pluxv1.GetApprovalReviewResponse{
		Approval: request(), Diff: []byte(`[{"kind":"page"}]`), Changelog: []string{"changed page main/home"}, Notes: "n",
		Risk:        &pluxv1.ReviewRisk{Level: "high", Reasons: []string{"it touches a page marked secure"}},
		Reports:     []*pluxv1.ReviewReport{{Kind: "../accessibility", Content: []byte(`{}`)}},
		InstallBase: []*pluxv1.InstallBaseEntry{{Channel: "production", ReleaseSequence: 2, HostBuild: "1.0", Devices: 40}},
		GoldenDiffs: []*pluxv1.ReviewImage{{PluginKey: "main", CaseId: "home/dark", Baseline: f.baseline, Actual: f.actual}},
	}), nil
}

func (f fakeApprovals) DecideApproval(_ context.Context, r *connect.Request[pluxv1.DecideApprovalRequest]) (*connect.Response[pluxv1.DecideApprovalResponse], error) {
	f.record("decide " + r.Msg.GetId() + " " + r.Msg.GetDecision() + " " + r.Msg.GetComment())
	return connect.NewResponse(&pluxv1.DecideApprovalResponse{Approval: request()}), nil
}

// Verifies: GOV-020, GOV-021, GOV-023.
// plux policies and plux approvals call the approval service, and
// approvals show writes the diff, the reports, the golden images with a
// pixel diff and an escaped HTML summary, never outside the directory.
func TestApprovalCommands_GOV_023(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	t.Setenv(tokenEnv, "plux_pat_gov")
	t.Setenv(serverEnv, "")
	t.Setenv(orgEnv, "")
	var calls []string
	fake := fakeApprovals{calls: &calls, baseline: solid(t, color.White), actual: solid(t, color.Black)}
	mux := http.NewServeMux()
	mux.Handle(pluxv1connect.NewApprovalServiceHandler(fake))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	run := func(cmd func(env, []string) int, args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		e := newEnv(&out, &errOut)
		e.creds = credentials{dir: t.TempDir()}
		args = append([]string{args[0], "--server", srv.URL, "--org", "org-1", "--app", "a1"}, args[1:]...)
		code := cmd(e, args)
		return code, out.String(), errOut.String()
	}

	if code, out, stderr := run(env.policies, "list"); code != exitOK || !strings.Contains(out, "p\tv2\t0123456789ab\tada") {
		t.Errorf("policies list: %d %q %s", code, out, stderr)
	}
	if code, _, _ := run(env.policies, "list", "--org-wide"); code != exitOK || calls[len(calls)-1] != "list policies app=" {
		t.Errorf("policies list --org-wide: %d %v", code, calls)
	}
	file := filepath.Join(t.TempDir(), "p.json")
	if err := os.WriteFile(file, []byte(`{"key":"p"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, out, _ := run(env.policies, "put", file); code != exitOK || !strings.Contains(out, "v3") || calls[len(calls)-1] != `put {"key":"p"}` {
		t.Errorf("policies put: %d %q %v", code, out, calls)
	}
	if code, _, _ := run(env.policies, "delete", "p"); code != exitOK || calls[len(calls)-1] != "delete policy p app=a1" {
		t.Errorf("policies delete: %d %v", code, calls)
	}
	dir := t.TempDir()
	if code, _, _ := run(env.policies, "export", "--out", dir); code == exitOK {
		t.Error("an export with a path outside the directory succeeded")
	}
	if _, err := os.Stat(filepath.Join(dir, "policies", "p.policy.json")); err != nil {
		t.Errorf("the file before the bad path was not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "escape.json")); err == nil {
		t.Error("a file escaped the export directory")
	}

	if code, out, _ := run(env.approvals, "list", "--status", "open"); code != exitOK || !strings.Contains(out, "r1\topen\t1/2\trelease:3@production/production") {
		t.Errorf("approvals list: %d %q", code, out)
	}
	for _, verb := range []string{"approve", "reject"} {
		if code, _, _ := run(env.approvals, verb, "--comment", "ok", "r1"); code != exitOK || calls[len(calls)-1] != "decide r1 "+verb+" ok" {
			t.Errorf("approvals %s: %d %v", verb, code, calls)
		}
	}
	out := filepath.Join(t.TempDir(), "review")
	code, text, stderr := run(env.approvals, "show", "--out", out, "r1")
	if code != exitOK || !strings.Contains(text, "risk: high") || !strings.Contains(text, "golden differences: 1") {
		t.Fatalf("approvals show: %d %q %s", code, text, stderr)
	}
	for _, f := range []string{"diff.json", "reports/.._accessibility.json", "golden/main/home_dark-baseline.png", "golden/main/home_dark-actual.png", "golden/main/home_dark-diff.png", "review.html"} {
		if _, err := os.Stat(filepath.Join(out, f)); err != nil {
			t.Errorf("approvals show did not write %s: %v", f, err)
		}
	}
	page, err := os.ReadFile(filepath.Join(out, "review.html")) //nolint:gosec // a test file
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(page), "<script>") || !strings.Contains(string(page), "&lt;script&gt;") {
		t.Errorf("review.html does not escape what the server sent: %s", page)
	}
	delta, err := os.ReadFile(filepath.Join(out, "golden/main/home_dark-diff.png")) //nolint:gosec // a test file
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(delta))
	if err != nil {
		t.Fatal(err)
	}
	if r, _, _, _ := img.At(1, 1).RGBA(); r>>8 != 0xd3 {
		t.Errorf("a differing pixel is not red: %v", img.At(1, 1))
	}
	if code, text, _ := run(env.approvals, "show", "--json", "--out", out, "r1"); code != exitOK || !json.Valid([]byte(text)) {
		t.Errorf("approvals show --json: %d %q", code, text)
	}
	if code, _, _ := run(env.approvals, "frobnicate"); code != exitUsage {
		t.Errorf("an unknown subcommand: %d", code)
	}
}

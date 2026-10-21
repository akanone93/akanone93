// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

// fakeAudit answers the reconstruction calls of the audit service from
// memory.
type fakeAudit struct {
	pluxv1connect.UnimplementedAuditServiceHandler
	last *pluxv1.ReconstructRequest
}

func (f *fakeAudit) Reconstruct(_ context.Context, r *connect.Request[pluxv1.ReconstructRequest]) (*connect.Response[pluxv1.ReconstructResponse], error) {
	if r.Header().Get("Authorization") != "Bearer plux_pat_audit" {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}
	f.last = r.Msg
	if r.Msg.GetDeviceId() == "old" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, nil)
	}
	return connect.NewResponse(&pluxv1.ReconstructResponse{Reconstruction: &pluxv1.Reconstruction{
		Id: "rec-1", State: "pending", Moment: r.Msg.GetAt(), OpenUri: "plux-viewer://reconstruction/rec-1?server=x",
	}}), nil
}

func (f *fakeAudit) GetReconstruction(_ context.Context, r *connect.Request[pluxv1.GetReconstructionRequest]) (*connect.Response[pluxv1.GetReconstructionResponse], error) {
	return connect.NewResponse(&pluxv1.GetReconstructionResponse{
		Reconstruction: &pluxv1.Reconstruction{Id: r.Msg.GetId(), State: "signed", Moment: timestamppb.Now()},
		Manifest:       []byte(`{"type":"reconstruction"}`),
		Signatures:     []*pluxv1.ReconstructionSignature{{KeyId: "k"}},
		Bundles:        []*pluxv1.ReconstructionBundle{{Sha256: "sha256:ab", Url: "https://x/b"}},
	}), nil
}

func (f *fakeAudit) ListReconstructions(_ context.Context, r *connect.Request[pluxv1.ListReconstructionsRequest]) (*connect.Response[pluxv1.ListReconstructionsResponse], error) {
	if r.Msg.GetPage().GetPageToken() == "" {
		return connect.NewResponse(&pluxv1.ListReconstructionsResponse{
			Reconstructions: []*pluxv1.Reconstruction{{Id: "rec-2", State: "signed", Moment: timestamppb.Now()}},
			Page:            &pluxv1.PageResult{NextPageToken: "p2"},
		}), nil
	}
	return connect.NewResponse(&pluxv1.ListReconstructionsResponse{
		Reconstructions: []*pluxv1.Reconstruction{{Id: "rec-1", State: "pending", Moment: timestamppb.Now()}},
	}), nil
}

// Verifies: SEC-142.
// plux audit reconstruct sends the subject and the moment and prints how to
// open the release, refuses a missing or ambiguous subject and a malformed
// time, reports a retention failure, opens one release with its manifest
// and follows pages when listing.
func TestAuditCommand_SEC_142(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	t.Setenv(tokenEnv, "plux_pat_audit")
	t.Setenv(serverEnv, "")
	t.Setenv(orgEnv, "")
	fake := &fakeAudit{}
	mux := http.NewServeMux()
	mux.Handle(pluxv1connect.NewAuditServiceHandler(fake))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	run := func(args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		e := newEnv(&out, &errOut)
		e.creds = credentials{dir: t.TempDir()}
		args = append([]string{args[0], "--server", srv.URL, "--org", "org-1"}, args[1:]...)
		code := e.audit(args)
		return code, out.String(), errOut.String()
	}

	code, out, stderr := run("reconstruct", "--app", "app-1", "--device", "d1", "--at", "2026-03-01T12:00:00Z")
	if code != exitOK || !strings.Contains(out, "rec-1") || !strings.Contains(out, "plux-viewer://reconstruction/rec-1") {
		t.Errorf("reconstruct: %d %q %s", code, out, stderr)
	}
	if fake.last.GetDeviceId() != "d1" || fake.last.GetAt().AsTime().Year() != 2026 {
		t.Errorf("request: %v", fake.last)
	}
	if code, _, _ := run("reconstruct", "--app", "app-1", "--user", "u1", "--at", "2026-03-01T12:00:00Z"); code != exitOK || fake.last.GetUserRef() != "u1" {
		t.Errorf("reconstruct by user: %d %v", code, fake.last)
	}
	for _, args := range [][]string{
		{"reconstruct", "--app", "app-1", "--at", "2026-03-01T12:00:00Z"},
		{"reconstruct", "--app", "app-1", "--device", "d", "--user", "u", "--at", "2026-03-01T12:00:00Z"},
		{"reconstruct", "--app", "app-1", "--device", "d", "--at", "yesterday"},
		{"reconstruct", "--device", "d", "--at", "2026-03-01T12:00:00Z"},
	} {
		if code, _, _ := run(args...); code != exitUsage {
			t.Errorf("%v should be a usage error, got %d", args, code)
		}
	}
	if code, _, _ := run("reconstruct", "--app", "app-1", "--device", "old", "--at", "2020-01-01T00:00:00Z"); code != exitFailed {
		t.Errorf("a refused moment: %d", code)
	}
	if code, out, _ := run("reconstructions", "--app", "app-1", "rec-1"); code != exitOK || !strings.Contains(out, `{"type":"reconstruction"}`) {
		t.Errorf("open one: %d %q", code, out)
	}
	code, out, _ = run("reconstructions", "--app", "app-1", "--json")
	var listed struct{ Reconstructions []map[string]any }
	if code != exitOK || json.Unmarshal([]byte(out), &listed) != nil || len(listed.Reconstructions) != 2 {
		t.Errorf("list should follow pages: %d %q", code, out)
	}
	if code, _, _ := run("reconstructions"); code != exitUsage {
		t.Errorf("list without an app: %d", code)
	}
}

// Verifies: SEC-142.
// plux audit without a subcommand prints usage and an unknown one is
// refused.
func TestAuditCommandUsage_SEC_142(t *testing.T) {
	t.Parallel()
	var out, errOut bytes.Buffer
	e := newEnv(&out, &errOut)
	if code := e.audit(nil); code != exitUsage || !strings.Contains(errOut.String(), "plux audit") {
		t.Errorf("no subcommand: %d %q", code, errOut.String())
	}
	if code := e.audit([]string{"purge"}); code != exitUsage {
		t.Errorf("unknown subcommand: %d", code)
	}
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

// fakeDSR answers the data subject service from memory.
type fakeDSR struct {
	pluxv1connect.UnimplementedDataSubjectServiceHandler
}

func (fakeDSR) ExportSubject(_ context.Context, r *connect.Request[pluxv1.ExportSubjectRequest]) (*connect.Response[pluxv1.ExportSubjectResponse], error) {
	if r.Header().Get("Authorization") != "Bearer plux_pat_dsr" {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}
	if r.Msg.GetSubjectId() == "none" {
		return nil, connect.NewError(connect.CodeNotFound, nil)
	}
	return connect.NewResponse(&pluxv1.ExportSubjectResponse{RequestId: "r1", Archive: []byte(`{"subject":"` + r.Msg.GetSubjectId() + `"}`), RowCount: 3}), nil
}

func (fakeDSR) EraseSubject(_ context.Context, r *connect.Request[pluxv1.EraseSubjectRequest]) (*connect.Response[pluxv1.EraseSubjectResponse], error) {
	if r.Msg.GetSubjectId() == "again" {
		return connect.NewResponse(&pluxv1.EraseSubjectResponse{AlreadyErased: true}), nil
	}
	return connect.NewResponse(&pluxv1.EraseSubjectResponse{RequestId: "r2", Erased: []*pluxv1.StoreCount{{Store: "devices", Rows: 1}}}), nil
}

func (fakeDSR) GetRequest(_ context.Context, r *connect.Request[pluxv1.GetRequestRequest]) (*connect.Response[pluxv1.GetRequestResponse], error) {
	return connect.NewResponse(&pluxv1.GetRequestResponse{Request: &pluxv1.DataSubjectRequest{
		Id: r.Msg.GetId(), Action: "erase", Kind: pluxv1.SubjectKind_SUBJECT_KIND_DEVICE, RowCount: 1, RequestedAt: timestamppb.Now(),
		Tombstones: []*pluxv1.StoreCount{{Store: "devices", Rows: 1}},
	}}), nil
}

func (fakeDSR) ListRequests(_ context.Context, r *connect.Request[pluxv1.ListRequestsRequest]) (*connect.Response[pluxv1.ListRequestsResponse], error) {
	if r.Msg.GetPage().GetPageToken() == "" {
		return connect.NewResponse(&pluxv1.ListRequestsResponse{
			Requests: []*pluxv1.DataSubjectRequest{{Id: "r1", Action: "export", RequestedAt: timestamppb.Now()}},
			Page:     &pluxv1.PageResult{NextPageToken: "p2"},
		}), nil
	}
	return connect.NewResponse(&pluxv1.ListRequestsResponse{Requests: []*pluxv1.DataSubjectRequest{{Id: "r2", Action: "erase", RequestedAt: timestamppb.Now()}}}), nil
}

func (fakeDSR) GetPersonalDataMap(context.Context, *connect.Request[pluxv1.GetPersonalDataMapRequest]) (*connect.Response[pluxv1.GetPersonalDataMapResponse], error) {
	return connect.NewResponse(&pluxv1.GetPersonalDataMapResponse{Stores: []*pluxv1.PersonalDataStore{
		{Name: "devices", Erase: "delete", Retention: "until erased", PersonalFields: []string{"locale"}},
	}}), nil
}

// Verifies: SEC-162.
// plux dsr exports to a file or standard output, erases, reports an erasure
// that was already done, lists and shows requests, prints the map, and
// refuses a missing or unknown --kind.
func TestDSRCommand_SEC_162(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	t.Setenv(tokenEnv, "plux_pat_dsr")
	t.Setenv(serverEnv, "")
	t.Setenv(orgEnv, "")
	mux := http.NewServeMux()
	mux.Handle(pluxv1connect.NewDataSubjectServiceHandler(fakeDSR{}))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	run := func(args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		e := newEnv(&out, &errOut)
		e.creds = credentials{dir: t.TempDir()}
		args = append([]string{args[0], "--server", srv.URL, "--org", "org-1"}, args[1:]...)
		code := e.dsr(args)
		return code, out.String(), errOut.String()
	}

	if code, out, stderr := run("export", "--kind", "device", "d1"); code != exitOK || !strings.Contains(out, `"subject":"d1"`) {
		t.Errorf("export to stdout: %d %q %s", code, out, stderr)
	}
	file := filepath.Join(t.TempDir(), "archive.json")
	if code, _, stderr := run("export", "--kind", "user", "--out", file, "u1"); code != exitOK || !strings.Contains(stderr, "Wrote 3 rows") {
		t.Errorf("export to a file: %d %s", code, stderr)
	}
	if raw, err := os.ReadFile(file); err != nil || !strings.Contains(string(raw), `"subject":"u1"`) {
		t.Errorf("archive file: %v %q", err, raw)
	}
	if info, err := os.Stat(file); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("an archive holds personal data and must be private: %v %v", err, info)
	}
	if code, _, _ := run("export", "--kind", "device", "none"); code != exitFailed {
		t.Errorf("a subject without data: %d", code)
	}
	if code, _, stderr := run("export", "d1"); code != exitUsage || !strings.Contains(stderr, "--kind") {
		t.Errorf("export without a kind: %d %s", code, stderr)
	}
	if code, out, _ := run("erase", "--kind", "device", "d1"); code != exitOK || !strings.Contains(out, "devices\t1 rows") {
		t.Errorf("erase: %d %q", code, out)
	}
	if code, out, _ := run("erase", "--kind", "device", "again"); code != exitOK || !strings.Contains(out, "Already erased") {
		t.Errorf("a second erase: %d %q", code, out)
	}
	code, out, _ := run("status", "--json")
	var listed struct{ Requests []map[string]any }
	if code != exitOK || json.Unmarshal([]byte(out), &listed) != nil || len(listed.Requests) != 2 {
		t.Errorf("status list should follow pages: %d %q", code, out)
	}
	if code, out, _ := run("status", "r2"); code != exitOK || !strings.Contains(out, "devices\t1 rows") {
		t.Errorf("status of one request: %d %q", code, out)
	}
	if code, out, _ := run("map"); code != exitOK || !strings.Contains(out, "devices\tdelete\tuntil erased") {
		t.Errorf("map: %d %q", code, out)
	}
	if code, _, _ := run("status", "a", "b"); code != exitUsage {
		t.Errorf("two request identifiers: %d", code)
	}
}

// Verifies: SEC-162.
// plux dsr without a subcommand prints usage and an unknown one is refused.
func TestDSRCommandUsage_SEC_162(t *testing.T) {
	t.Parallel()
	var out, errOut bytes.Buffer
	e := newEnv(&out, &errOut)
	if code := e.dsr(nil); code != exitUsage || !strings.Contains(errOut.String(), "plux dsr") {
		t.Errorf("no subcommand: %d %q", code, errOut.String())
	}
	if code := e.dsr([]string{"purge"}); code != exitUsage {
		t.Errorf("unknown subcommand: %d", code)
	}
}

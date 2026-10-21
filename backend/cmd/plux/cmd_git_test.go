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
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

// fakeSearchGit serves the search, Git export and document services. It
// records what the CLI asked and answers from memory.
type fakeSearchGit struct {
	pluxv1connect.UnimplementedSearchServiceHandler
	pluxv1connect.UnimplementedGitExportServiceHandler
	pluxv1connect.UnimplementedDocumentServiceHandler
	calls *[]string
}

// fakeCredential is the token every fake call must present.
const fakeCredential = "Bearer plux_pat_search"

// checkCredential refuses a call that lacks the fake credential or the
// organisation the tests use.
func checkCredential(h http.Header) error {
	if h.Get("Authorization") != fakeCredential || h.Get("X-Plux-Organization") != "org-1" {
		return connect.NewError(connect.CodeUnauthenticated, nil)
	}
	return nil
}

func (f fakeSearchGit) record(s string) { *f.calls = append(*f.calls, s) }

func (f fakeSearchGit) Search(_ context.Context, r *connect.Request[pluxv1.SearchRequest]) (*connect.Response[pluxv1.SearchResponse], error) {
	if err := checkCredential(r.Header()); err != nil {
		return nil, err
	}
	f.record("search " + r.Msg.GetQuery() + " kinds=" + strings.Join(r.Msg.GetKinds(), ","))
	switch r.Msg.GetQuery() {
	case "refused":
		return nil, connect.NewError(connect.CodeInvalidArgument, nil)
	case "none":
		return connect.NewResponse(&pluxv1.SearchResponse{}), nil
	}
	return connect.NewResponse(&pluxv1.SearchResponse{Hits: []*pluxv1.SearchHit{{
		Kind: "app", Id: "a1", AppId: "a1", Name: "Shop", Path: "apps/shop", Snippet: "a «shop» app", Rank: 0.5,
	}}}), nil
}

func (f fakeSearchGit) ConfigureExport(_ context.Context, r *connect.Request[pluxv1.ConfigureExportRequest]) (*connect.Response[pluxv1.ConfigureExportResponse], error) {
	if err := checkCredential(r.Header()); err != nil {
		return nil, err
	}
	s := r.Msg.GetSettings()
	f.record("configure " + s.GetAppId() + " " + s.GetRemoteUrl() + " " + s.GetBranch() + " " + s.GetCredentialKey() + " " + s.GetCredentialEnvironmentId())
	if !strings.HasPrefix(s.GetRemoteUrl(), "https://") {
		return nil, connect.NewError(connect.CodeInvalidArgument, nil)
	}
	return connect.NewResponse(&pluxv1.ConfigureExportResponse{Settings: s}), nil
}

func (f fakeSearchGit) GetSettings(_ context.Context, r *connect.Request[pluxv1.GetSettingsRequest]) (*connect.Response[pluxv1.GetSettingsResponse], error) {
	if err := checkCredential(r.Header()); err != nil {
		return nil, err
	}
	if r.Msg.GetAppId() == "unset" {
		return nil, connect.NewError(connect.CodeNotFound, nil)
	}
	return connect.NewResponse(&pluxv1.GetSettingsResponse{Settings: &pluxv1.GitSettings{
		AppId: r.Msg.GetAppId(), Enabled: true, RemoteUrl: "https://example.com/r.git", Branch: "main", CredentialKey: "GIT_TOKEN",
	}}), nil
}

func (f fakeSearchGit) ListExports(_ context.Context, r *connect.Request[pluxv1.ListExportsRequest]) (*connect.Response[pluxv1.ListExportsResponse], error) {
	if err := checkCredential(r.Header()); err != nil {
		return nil, err
	}
	return connect.NewResponse(&pluxv1.ListExportsResponse{Exports: []*pluxv1.GitExport{
		{Id: "x1", AppId: r.Msg.GetAppId(), VersionId: "v1", State: "pushed", Commit: "abc"},
	}}), nil
}

func (f fakeSearchGit) RunExport(_ context.Context, r *connect.Request[pluxv1.RunExportRequest]) (*connect.Response[pluxv1.RunExportResponse], error) {
	if err := checkCredential(r.Header()); err != nil {
		return nil, err
	}
	f.record("run " + r.Msg.GetAppId())
	return connect.NewResponse(&pluxv1.RunExportResponse{Export: &pluxv1.GitExport{Id: "x2", AppId: r.Msg.GetAppId(), VersionId: "v2", State: "pending"}}), nil
}

// ImportDraft records the app and the paths the CLI sent, as the drafts
// endpoint does for plux import.
func (f fakeSearchGit) ImportDraft(_ context.Context, s *connect.ClientStream[pluxv1.ImportDraftRequest]) (*connect.Response[pluxv1.ImportDraftResponse], error) {
	if err := checkCredential(s.RequestHeader()); err != nil {
		return nil, err
	}
	var paths []string
	app := ""
	for s.Receive() {
		if m := s.Msg(); m.GetAppId() != "" {
			app = m.GetAppId()
		}
		paths = append(paths, s.Msg().GetPath())
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	f.record("import " + app + " " + strings.Join(paths, ","))
	return connect.NewResponse(&pluxv1.ImportDraftResponse{Revision: 3}), nil
}

// startFakeServer serves the fake services and records calls into calls.
func startFakeServer(t *testing.T, calls *[]string) *httptest.Server {
	t.Helper()
	fake := fakeSearchGit{calls: calls}
	mux := http.NewServeMux()
	mux.Handle(pluxv1connect.NewSearchServiceHandler(fake))
	mux.Handle(pluxv1connect.NewGitExportServiceHandler(fake))
	mux.Handle(pluxv1connect.NewDocumentServiceHandler(fake))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// Verifies: GOV-011.
// plux git export configures, shows and runs an app's Git export, and plux
// git import brings a repository back as the app's drafts through the
// server's validation. The credential is never printed.
func TestGitCommands_GOV_011(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	var calls []string
	srv := startFakeServer(t, &calls)
	t.Setenv(tokenEnv, "plux_pat_search")
	t.Setenv(serverEnv, srv.URL)
	t.Setenv(orgEnv, "org-1")
	repo := t.TempDir()
	writeFixture(t, repo, "apps/shop/app.json", "{}")
	writeFixture(t, repo, "apps/shop/page.json", "{}")
	run := func(args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		e := newEnv(&out, &errOut)
		e.creds = credentials{dir: t.TempDir()}
		e.clone = func(_ context.Context, address, dir string) error {
			calls = append(calls, "clone "+address)
			writeFixture(t, dir, "apps/shop/app.json", "{}")
			return nil
		}
		code := e.git(args)
		return code, out.String(), errOut.String()
	}

	code, out, stderr := run("export", "configure", "--app", "a1", "--remote", "https://example.com/r.git", "--branch", "main", "--credential", "GIT_TOKEN", "--environment", "env-1")
	if code != exitOK || !strings.Contains(out, "enabled\thttps://example.com/r.git\tmain\tGIT_TOKEN") || !slices.Contains(calls, "configure a1 https://example.com/r.git main GIT_TOKEN env-1") {
		t.Errorf("export configure: %d %q %s", code, out, stderr)
	}
	code, out, _ = run("export", "configure", "--json", "--app", "a1", "--remote", "https://example.com/r.git", "--credential", "GIT_TOKEN", "--environment", "env-1")
	var configured map[string]any
	if err := json.Unmarshal([]byte(out), &configured); code != exitOK || err != nil || configured["credential_key"] != "GIT_TOKEN" || strings.Contains(out, fakeCredential) {
		t.Errorf("export configure --json: %d %v %q", code, err, out)
	}
	if code, _, _ = run("export", "configure", "--app", "a1", "--remote", "http://example.com/r.git", "--credential", "GIT_TOKEN"); code != exitFailed {
		t.Errorf("a refused remote: %d", code)
	}

	if code, out, _ = run("export", "status", "--app", "a1"); code != exitOK || !strings.Contains(out, "enabled\thttps://example.com/r.git") || !strings.Contains(out, "x1\tv1\tpushed\tabc") {
		t.Errorf("export status: %d %q", code, out)
	}
	if code, _, _ = run("export", "status", "--app", "unset"); code != exitFailed {
		t.Errorf("status of an app with no export: %d", code)
	}

	if code, out, _ = run("export", "run", "--app", "a1"); code != exitOK || !strings.Contains(out, "x2\tv2\tpending") || !slices.Contains(calls, "run a1") {
		t.Errorf("export run: %d %q", code, out)
	}

	if code, out, _ = run("import", repo, "--app", "a1"); code != exitOK || !strings.Contains(out, "Imported 2 files.") || !slices.Contains(calls, "import a1 apps/shop/app.json,apps/shop/page.json") {
		t.Errorf("git import of a directory: %d %q", code, out)
	}
	if code, _, _ = run("import", "https://example.com/r.git", "--app", "a1"); code != exitOK || !slices.Contains(calls, "clone https://example.com/r.git") {
		t.Errorf("git import of an https url: %d", code)
	}
	clones := len(calls)
	if code, _, _ = run("import", "http://example.com/r.git", "--app", "a1"); code != exitUsage || len(calls) != clones {
		t.Errorf("an import from a non-https url: %d", code)
	}
	if code, _, _ = run("import", filepath.Join(repo, "missing"), "--app", "a1"); code != exitFailed {
		t.Errorf("an import from a missing directory: %d", code)
	}
	if code, _, _ = run("import", repo); code != exitUsage {
		t.Errorf("an import with no app: %d", code)
	}

	if code, _, stderr = run(); code != exitUsage || !strings.Contains(stderr, "Usage: plux git") {
		t.Errorf("git without a subcommand: %d %s", code, stderr)
	}
	if code, _, stderr = run("export", "unknown"); code != exitUsage || !strings.Contains(stderr, "unknown subcommand") {
		t.Errorf("an unknown export subcommand: %d %s", code, stderr)
	}
}

// writeFixture writes a file under dir, creating its parents.
func writeFixture(t *testing.T, dir, path, content string) {
	t.Helper()
	full := filepath.Join(dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

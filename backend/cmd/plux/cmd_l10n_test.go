// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

// fakeLocalisation is LocalisationService with one imported file recorded.
type fakeLocalisation struct {
	pluxv1connect.UnimplementedLocalisationServiceHandler
	mu       sync.Mutex
	imported []*pluxv1.ImportTranslationsRequest
}

func (*fakeLocalisation) ExportTranslations(_ context.Context, r *connect.Request[pluxv1.ExportTranslationsRequest]) (*connect.Response[pluxv1.ExportTranslationsResponse], error) {
	return connect.NewResponse(&pluxv1.ExportTranslationsResponse{
		Content: []byte(r.Msg.GetFormat() + ":" + r.Msg.GetLocale()), SourceLocale: "en",
	}), nil
}

func (f *fakeLocalisation) ImportTranslations(_ context.Context, r *connect.Request[pluxv1.ImportTranslationsRequest]) (*connect.Response[pluxv1.ImportTranslationsResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.imported = append(f.imported, r.Msg)
	res := &pluxv1.ImportTranslationsResponse{ChangedKeys: []string{"k1", "k2"}, Saved: !r.Msg.GetDryRun()}
	if strings.Contains(string(r.Msg.GetContent()), "ghost") {
		res.Diagnostics = []*pluxv1.Diagnostic{{
			Code: "PLX-1621", Severity: pluxv1.Severity_SEVERITY_ERROR, Message: "the file translates ghost",
			Location: &pluxv1.Location{Path: "/messages/ghost"},
		}}
	}
	return connect.NewResponse(res), nil
}

func (*fakeLocalisation) GetStatus(context.Context, *connect.Request[pluxv1.GetStatusRequest]) (*connect.Response[pluxv1.GetStatusResponse], error) {
	return connect.NewResponse(&pluxv1.GetStatusResponse{SourceLocale: "en", Locales: []*pluxv1.LocaleStatus{
		{Locale: "de", Total: 10, New: 1, Translated: 2, Reviewed: 3, Approved: 3, Missing: 1},
	}}), nil
}

// Verifies: CLI-003, I18N-009.
// plux l10n export, import, status, pull and push against a fake server.
func TestL10n(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	t.Setenv(tokenEnv, "plux_pat_l10n")
	t.Setenv(serverEnv, "")
	t.Setenv(orgEnv, "")
	drafts := &fake{drafts: map[string][]byte{
		"app.json":                  []byte(`{}`),
		"translations/keys.json":    []byte(`{"keys":[]}`),
		"translations/de.json":      []byte(`{"locale":"de"}`),
		"translations/pt-BR.json":   []byte(`{"locale":"pt-BR"}`),
		"translations/../evil.json": []byte(`x`),
	}}
	loc := &fakeLocalisation{}
	mux := http.NewServeMux()
	mux.Handle(pluxv1connect.NewDocumentServiceHandler(drafts))
	mux.Handle(pluxv1connect.NewLocalisationServiceHandler(loc))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	base := []string{"--server", srv.URL, "--app", "a1"}
	run := func(dir string, args ...string) (int, string, string) {
		return cli(t, t.TempDir(), append([]string{"l10n"}, append(args, append(base, "-C", dir)...)...)...)
	}
	dir := t.TempDir()

	if code, out, _ := run(dir, "status"); code != exitOK || !strings.Contains(out, "source locale en") ||
		!strings.Contains(out, "de      10     1    2           3         3         1") {
		t.Errorf("status: %d\n%s", code, out)
	}

	if code, out, _ := run(dir, "export", "--locale", "de", "--format", "xliff"); code != exitOK || out != "xliff:de" {
		t.Errorf("export to stdout: %d %q", code, out)
	}
	file := filepath.Join(dir, "de.csv")
	if code, _, stderr := run(dir, "export", "--locale", "de", "--format", "csv", "--out", file); code != exitOK {
		t.Errorf("export to file: %d %s", code, stderr)
	}
	if got, _ := os.ReadFile(file); string(got) != "csv:de" {
		t.Errorf("exported file = %q", got)
	}
	if code, _, _ := run(dir, "export", "--locale", "de", "--format", "docx"); code != exitUsage {
		t.Errorf("unknown format: %d, want usage", code)
	}
	if code, _, _ := run(dir, "export", "--format", "csv"); code != exitUsage {
		t.Errorf("no locale: %d, want usage", code)
	}

	in := filepath.Join(dir, "de.xlf")
	if err := os.WriteFile(in, []byte("<xliff/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, stderr := run(dir, "import", "--locale", "de", "--dry-run", in)
	if code != exitOK || !strings.Contains(out, "de: 2 keys changed, unchanged or not saved.") {
		t.Errorf("dry-run import: %d\n%s%s", code, out, stderr)
	}
	loc.mu.Lock()
	last := loc.imported[len(loc.imported)-1]
	loc.mu.Unlock()
	if last.GetFormat() != "xliff" || !last.GetDryRun() || last.GetLocale() != "de" || string(last.GetContent()) != "<xliff/>" {
		t.Errorf("import request = %v", last)
	}
	bad := filepath.Join(dir, "bad.csv")
	if err := os.WriteFile(bad, []byte("ghost"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, out, _ := run(dir, "import", "--locale", "de", bad); code != exitFailed || !strings.Contains(out, "PLX-1621") {
		t.Errorf("import with an error diagnostic: %d\n%s", code, out)
	}
	if code, _, _ := run(dir, "import", "--locale", "de", filepath.Join(dir, "missing.csv")); code != exitFailed {
		t.Errorf("missing file: %d", code)
	}
	if code, _, _ := run(dir, "import", "--locale", "de", filepath.Join(dir, "x.txt")); code != exitUsage {
		t.Errorf("unknown extension: %d", code)
	}

	pulled := t.TempDir()
	if code, out, stderr := run(pulled, "pull"); code != exitOK || !strings.Contains(out, "Wrote 2 translation files") {
		t.Fatalf("pull: %d\n%s%s", code, out, stderr)
	}
	for _, name := range []string{"de.json", "pt-BR.json"} {
		if _, err := os.Stat(filepath.Join(pulled, "translations", name)); err != nil {
			t.Errorf("pull did not write %s: %v", name, err)
		}
	}
	for _, name := range []string{"keys.json", "../evil.json"} {
		if _, err := os.Stat(filepath.Join(pulled, "translations", name)); err == nil {
			t.Errorf("pull wrote %s", name)
		}
	}

	loc.mu.Lock()
	loc.imported = nil
	loc.mu.Unlock()
	if code, out, stderr := run(pulled, "push"); code != exitOK {
		t.Fatalf("push: %d\n%s%s", code, out, stderr)
	}
	loc.mu.Lock()
	defer loc.mu.Unlock()
	if len(loc.imported) != 2 || loc.imported[0].GetLocale() != "de" || loc.imported[1].GetLocale() != "pt-BR" || loc.imported[0].GetFormat() != "json" {
		t.Errorf("push sent %v", loc.imported)
	}
}

// Verifies: CLI-003. plux l10n without a subcommand, or with an unknown one,
// is a usage error.
func TestL10nUsage(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"l10n"}, {"l10n", "extract-all"}} {
		if code, _, _ := cli(t, t.TempDir(), args...); code != exitUsage {
			t.Errorf("%v: %d, want usage", args, code)
		}
	}
}

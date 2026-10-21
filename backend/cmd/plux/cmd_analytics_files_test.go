// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

// Verifies: ANL-011.
// plux analytics files shows the exports with the secret's hint only, sends
// the settings with the webhook secret read from a file or standard input,
// and refuses a missing or empty secret file before it calls the server.
func TestAnalyticsFiles_ANL_011(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	t.Setenv(tokenEnv, "plux_pat_anl")
	t.Setenv(serverEnv, "")
	t.Setenv(orgEnv, "")
	var calls []string
	mux := http.NewServeMux()
	mux.Handle(pluxv1connect.NewAnalyticsServiceHandler(fakeAnalytics{calls: &calls}))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	run := func(stdin string, args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		e := newEnv(&out, &errOut)
		e.creds = credentials{dir: t.TempDir()}
		e.stdin = strings.NewReader(stdin)
		full := append([]string{"files", args[0], "--server", srv.URL, "--org", "org-1"}, args[1:]...)
		code := e.analytics(full)
		return code, out.String(), errOut.String()
	}

	code, out, stderr := run("", "show")
	if code != exitOK || !strings.Contains(out, "parquet\tenabled\t1h") ||
		!strings.Contains(out, "webhook\tenabled\thttps://hooks.example/plux\tsecret ending a1b2") ||
		!strings.Contains(out, "last error: PLX-8303") {
		t.Errorf("files show: %d %q %s", code, out, stderr)
	}
	code, out, _ = run("", "show", "--json")
	var shown map[string]any
	if err := json.Unmarshal([]byte(out), &shown); code != exitOK || err != nil || shown["secretHint"] != "a1b2" || shown["webhookUrl"] != "https://hooks.example/plux" {
		t.Errorf("files show --json: %d %v %s", code, err, out)
	}

	secretFile := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(secretFile, []byte("whsec_c2VjcmV0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, stderr = run("", "set", "--parquet-enabled", "--parquet-events", "--parquet-granularity", "1h",
		"--webhook-enabled", "--webhook-url", "https://hooks.example/plux", "--webhook-secret-file", secretFile)
	if code != exitOK || !strings.Contains(out, "secret ending cmV0") || strings.Contains(out, "c2VjcmV0") {
		t.Errorf("files set from a file: %d %q %s", code, out, stderr)
	}
	if code, out, _ = run("whsec_c3RkaW4\n", "set", "--webhook-secret-file", "-"); code != exitOK || strings.Contains(out, "c3RkaW4") {
		t.Errorf("files set from stdin: %d %q", code, out)
	}
	if code, _, _ = run("", "set"); code != exitOK {
		t.Errorf("files set without a secret: %d", code)
	}

	missing := filepath.Join(t.TempDir(), "absent")
	if code, _, stderr = run("", "set", "--webhook-secret-file", missing); code != exitFailed || !strings.Contains(stderr, "read the webhook secret") {
		t.Errorf("files set with a missing secret file: %d %s", code, stderr)
	}
	empty := filepath.Join(t.TempDir(), "empty")
	if err := os.WriteFile(empty, []byte(" \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr = run("", "set", "--webhook-secret-file", empty); code != exitUsage || !strings.Contains(stderr, "webhook secret is empty") {
		t.Errorf("files set with an empty secret file: %d %s", code, stderr)
	}

	want := []string{
		"files set parquet=yes events=yes rollups=no granularity=1h webhook=yes url=https://hooks.example/plux secretlen=14",
		"files set parquet=no events=no rollups=no granularity= webhook=no url= secretlen=13",
		"files set parquet=no events=no rollups=no granularity= webhook=no url= secretlen=0",
	}
	if !slices.Equal(calls, want) {
		t.Errorf("calls = %q\nwant    %q", calls, want)
	}
}

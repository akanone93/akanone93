// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// Verifies: SRV-061.
// plux search sends the query and the kinds to the search service with the
// caller's credential, prints the hits as text or JSON, and refuses a
// missing query or a query the server refuses.
func TestSearchCommand_SRV_061(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	var calls []string
	srv := startFakeServer(t, &calls)
	t.Setenv(tokenEnv, "plux_pat_search")
	t.Setenv(serverEnv, srv.URL)
	t.Setenv(orgEnv, "org-1")
	run := func(args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		e := newEnv(&out, &errOut)
		e.creds = credentials{dir: t.TempDir()}
		code := e.search(args)
		return code, out.String(), errOut.String()
	}

	code, out, stderr := run("shop")
	if code != exitOK || !strings.Contains(out, "app\tShop\tapps/shop\ta «shop» app") || !slices.Contains(calls, "search shop kinds=") {
		t.Errorf("search: %d %q %s", code, out, stderr)
	}
	code, out, _ = run("--json", "--kind", "app, plugin,", "shop")
	var found struct{ Hits []map[string]any }
	if err := json.Unmarshal([]byte(out), &found); code != exitOK || err != nil || len(found.Hits) != 1 || found.Hits[0]["kind"] != "app" {
		t.Errorf("search --json: %d %v %q", code, err, out)
	}
	if !slices.Contains(calls, "search shop kinds=app,plugin") {
		t.Errorf("the kinds were not sent: %v", calls)
	}
	if code, _, _ = run("shop", "--kind", "page"); code != exitOK || !slices.Contains(calls, "search shop kinds=page") {
		t.Errorf("flags after the query: %d %v", code, calls)
	}
	if code, out, _ = run("none"); code != exitOK || !strings.Contains(out, "No matches.") {
		t.Errorf("search with no hits: %d %q", code, out)
	}
	if code, _, _ = run("refused"); code != exitFailed {
		t.Errorf("a refused search: %d", code)
	}
	if code, _, stderr = run(); code != exitUsage || !strings.Contains(stderr, "Usage: plux search") {
		t.Errorf("search with no query: %d %s", code, stderr)
	}
}

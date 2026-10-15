// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package fnrunner

import (
	"strings"
	"testing"
)

// TestEnvironmentAndLogsCrossTheRunner checks that the declared environment
// variables reach env_get and that the lines a function logs come back in
// the response, bounded by the log limit.
// Verifies: FN-002, FN-032.
func TestEnvironmentAndLogsCrossTheRunner_FN_032(t *testing.T) {
	h := newHarness(t, harnessOptions{})
	msg := h.request("REGION")
	msg.Env = map[string]string{"REGION": "eu-west"}
	resp, err := h.call(6, msg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(resp.GetOutput()), `"env":"eu-west"`) {
		t.Errorf("env_get did not read the declared variable: %s", resp.GetOutput())
	}

	resp, err = h.call(7, h.request("3"))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(resp.GetLogs()); n != 3 {
		t.Fatalf("got %d log lines, want 3", n)
	}
	if l := resp.GetLogs()[0]; l.GetLevel() != 1 || l.GetMessage() != strings.Repeat("x", 64) || l.GetTimeUnixNano() == 0 {
		t.Errorf("unexpected log line %+v", l)
	}
}

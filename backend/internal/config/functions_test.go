// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package config

import "testing"

// internal is a server section that serves the internal listener over
// mutual TLS, as the api role does for the runner.
const internal = "  internalListen: \":9443\"\n  internalTLS:\n    caFile: /etc/plux/ca.pem\n    certFile: /etc/plux/api.pem\n    keyFile: /etc/plux/api.key\n"

// Verifies: FN-020, SEC-043.
// The api role reaches the runner at functions.runnerURL over https with
// its internal identity; anything else is refused when the configuration
// loads.
func TestFunctionsRunnerURL_FN_020(t *testing.T) {
	t.Parallel()
	c := parse(t, withServer(internal)+"functions:\n  runnerURL: \"https://fnrunner:9444\"\n")
	if c.Functions.RunnerURL != "https://fnrunner:9444" {
		t.Errorf("runnerURL = %q", c.Functions.RunnerURL)
	}
	if got := parse(t, minimal).Functions.RunnerURL; got != "" {
		t.Errorf("runnerURL defaults to %q, want none", got)
	}
	mentions(t, []struct{ name, src, want string }{
		{"plain http", withServer(internal) + "functions:\n  runnerURL: \"http://fnrunner:9444\"\n", "https"},
		{"not a URL", withServer(internal) + "functions:\n  runnerURL: \"fnrunner\"\n", "functions.runnerURL"},
		{"no identity", minimal + "functions:\n  runnerURL: \"https://fnrunner:9444\"\n", "server.internalTLS"},
		{"unknown key", withServer(internal) + "functions:\n  nonsense: 1\n", "nonsense"},
	})
}

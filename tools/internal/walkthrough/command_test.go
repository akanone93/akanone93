// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package walkthrough

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeScript writes an executable shell script and returns its path.
func writeScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o700); err != nil { //nolint:gosec // a test script
		t.Fatal(err)
	}
	return path
}

// The command line shown in the transcript never holds the credential.
func TestCommandStringHasNoCredential(t *testing.T) {
	t.Parallel()
	c := Command{Actor: "alice", Args: []string{"approvals", "approve", "--app", "a1", "r1"}}
	if got := c.String(); got != "alice$ plux approvals approve --app a1 r1" {
		t.Errorf("String = %q", got)
	}
}

// A command runs with the server, the organisation and its actor's token in
// the environment of the child, a private HOME, and nothing else of ours; a
// non-zero exit is a result, and an unknown actor is an error.
func TestExecCommanderRunsWithTheActorsCredential(t *testing.T) {
	bin := writeScript(t, `echo "args=$* server=$PLUX_SERVER org=$PLUX_ORGANIZATION token=$PLUX_TOKEN home=$HOME leak=${SECRET_FROM_PARENT:-none}"
echo "to stderr" >&2
[ "$1" = fail ] && exit 3
exit 0
`)
	t.Setenv("SECRET_FROM_PARENT", "must-not-reach-the-child")
	e := &ExecCommander{Binary: bin, Server: "https://s", Org: "o1", Home: "/home-for-test", Initial: map[string]string{"admin": "tok-admin"}}
	e.SetToken("alice", "tok-alice")

	res, err := e.Run(t.Context(), Command{Actor: "alice", Args: []string{"roles", "list"}})
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("Run = %+v, %v", res, err)
	}
	for _, want := range []string{"args=roles list", "server=https://s", "org=o1", "token=tok-alice", "home=/home-for-test", "leak=none"} {
		if !strings.Contains(res.Stdout, want) {
			t.Errorf("stdout %q lacks %q", res.Stdout, want)
		}
	}
	if strings.TrimSpace(res.Stderr) != "to stderr" {
		t.Errorf("stderr = %q", res.Stderr)
	}
	if res, err := e.Run(t.Context(), Command{Actor: "admin", Args: []string{"fail"}}); err != nil || res.ExitCode != 3 {
		t.Errorf("a failing command: %+v, %v", res, err)
	}
	if _, err := e.Run(t.Context(), Command{Actor: "mallory", Args: []string{"x"}}); err == nil {
		t.Error("an actor without a token must be an error")
	}
	if _, err := (&ExecCommander{Binary: "/nonexistent/plux", Initial: map[string]string{"a": "t"}}).Run(t.Context(), Command{Actor: "a"}); err == nil {
		t.Error("a binary that cannot start must be an error")
	}
}

// A command that never ends is cut off at the timeout.
func TestExecCommanderTimesOut(t *testing.T) {
	t.Parallel()
	e := &ExecCommander{Binary: writeScript(t, "sleep 30\n"), Initial: map[string]string{"a": "t"}, Timeout: 200 * time.Millisecond}
	start := time.Now()
	res, err := e.Run(t.Context(), Command{Actor: "a"})
	if (err == nil && res.ExitCode == 0) || time.Since(start) > 10*time.Second {
		t.Errorf("Run = %+v, %v after %s", res, err, time.Since(start))
	}
}

// When the CLI asks for a step-up, the hook is run once with the URL and the
// actor, and the CLI goes on after it has finished.
func TestExecCommanderCompletesAStepUp(t *testing.T) {
	t.Parallel()
	flag := filepath.Join(t.TempDir(), "approved")
	bin := writeScript(t, `echo "This action needs a step-up. Open https://plux.example/stepup/abc and confirm with your security key." >&2
for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20; do [ -e "`+flag+`" ] && echo approved && exit 0; sleep 0.25; done
exit 9
`)
	var gotActor, gotURL string
	calls := 0
	e := &ExecCommander{Binary: bin, Initial: map[string]string{"bob": "t"}, StepUp: func(_ context.Context, actor, url string) error {
		calls++
		gotActor, gotURL = actor, url
		return os.WriteFile(flag, nil, 0o600)
	}}
	res, err := e.Run(t.Context(), Command{Actor: "bob", Args: []string{"approvals", "approve"}})
	if err != nil || res.ExitCode != 0 || strings.TrimSpace(res.Stdout) != "approved" {
		t.Fatalf("Run = %+v, %v", res, err)
	}
	if calls != 1 || gotActor != "bob" || gotURL != "https://plux.example/stepup/abc" {
		t.Errorf("hook calls=%d actor=%q url=%q", calls, gotActor, gotURL)
	}
}

// A failing step-up hook fails the command rather than leaving the CLI waiting.
func TestExecCommanderReportsAFailedStepUp(t *testing.T) {
	t.Parallel()
	bin := writeScript(t, `echo "Open https://x/y and confirm with your security key." >&2
sleep 1
`)
	boom := errors.New("no authenticator")
	e := &ExecCommander{Binary: bin, Initial: map[string]string{"bob": "t"}, StepUp: func(context.Context, string, string) error { return boom }}
	if _, err := e.Run(t.Context(), Command{Actor: "bob"}); !errors.Is(err, boom) {
		t.Errorf("err = %v, want the hook's error", err)
	}
}

// A shell hook returns its output and carries its extra environment; a
// failure carries the command's own message.
func TestShellHook(t *testing.T) {
	t.Parallel()
	out, err := ShellHook(t.Context(), `echo "$WALKTHROUGH_ACTOR"`, "WALKTHROUGH_ACTOR=carol")
	if err != nil || strings.TrimSpace(out) != "carol" {
		t.Errorf("ShellHook = %q, %v", out, err)
	}
	if _, err := ShellHook(t.Context(), `echo "it broke" >&2; exit 4`); err == nil || !strings.Contains(err.Error(), "it broke") {
		t.Errorf("err = %v", err)
	}
}

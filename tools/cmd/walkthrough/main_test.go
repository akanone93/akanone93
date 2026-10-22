// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nightCode42/plux3/tools/internal/walkthrough"
)

// sealedRun writes the evidence of a one-step run and its signed bundle to a
// directory the way `walkthrough run` does, and returns the directory.
func sealedRun(t *testing.T, fail bool) string {
	t.Helper()
	dir := t.TempDir()
	store, err := walkthrough.NewStore(filepath.Join(dir, evidenceDir))
	if err != nil {
		t.Fatal(err)
	}
	step := walkthrough.Step{ID: "01-a", Title: "a", Verifies: []string{"GOV-006"}, Run: func(_ context.Context, r *walkthrough.Run) error {
		if err := r.Evidence("proof.txt", []byte("proof")); err != nil {
			return err
		}
		if fail {
			return errors.New("refused")
		}
		return nil
	}}
	st, _ := (&walkthrough.Runner{Steps: []walkthrough.Step{step}, Store: store, Now: time.Now}).Run(t.Context())
	if err := seal(dir, st); err != nil {
		t.Fatal(err)
	}
	return dir
}

// The dry run prints the plan of every step and starts nothing: it needs no
// server, no token and no binaries.
func TestDryRunPrintsThePlan(t *testing.T) {
	t.Setenv(adminTokenEnv, "")
	var stdout, stderr bytes.Buffer
	if code := run(t.Context(), []string{"run", "-dry-run"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	for _, want := range []string{"01-sso-roles-scim", "02-secure-release-approval", "03-staged-rollout", "04-kill-switch-break-glass", "05-mandatory-update-forced-rollback", "06-audit-export-compliance-report", "07-what-did-the-user-see", "07b-viewer-on-emulator", "08-rolling-upgrade", "09-data-subject-request", "GOV-006"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("the plan lacks %q", want)
		}
	}
}

// Usage errors exit 2: no command, an unknown command, a missing flag and a
// missing administrator token; a token is never taken from a flag.
func TestUsageErrors(t *testing.T) {
	t.Setenv(adminTokenEnv, "")
	var out, errOut bytes.Buffer
	for name, args := range map[string][]string{
		"no command":      {},
		"unknown command": {"nonsense"},
		"missing flags":   {"run"},
		"unknown flag":    {"run", "-token", "secret"},
	} {
		if code := run(t.Context(), args, &out, &errOut); code != exitUsage {
			t.Errorf("%s: exit %d, want %d", name, code, exitUsage)
		}
	}
	full := []string{"run", "-server", "https://s", "-org", "o", "-app", "a", "-advertise", "172.18.0.1", "-token-cmd", "x", "-audit-key-cmd", "x", "-upgrade-cmd", "x"}
	if code := run(t.Context(), full, &out, &errOut); code != exitUsage || !strings.Contains(errOut.String(), adminTokenEnv) {
		t.Errorf("without the token: exit %d: %s", code, errOut.String())
	}
	if code := run(t.Context(), []string{"run", "-h"}, &out, &errOut); code != exitOK {
		t.Errorf("-h: exit %d", code)
	}
}

// A sealed run verifies, and the verifier reports its steps.
func TestVerifyAcceptsASealedRun(t *testing.T) {
	t.Parallel()
	dir := sealedRun(t, false)
	var stdout, stderr bytes.Buffer
	if code := run(t.Context(), []string{"verify", "-dir", dir}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "2 steps, passed=true") {
		t.Errorf("stdout = %s", stdout.String())
	}
}

// A run that failed verifies as a bundle but exits 1: a signed failure is not
// a pass.
func TestVerifyFailsAFailedRun(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	if code := run(t.Context(), []string{"verify", "-dir", sealedRun(t, true)}, &stdout, &stderr); code != exitFailed {
		t.Errorf("exit %d, want %d", code, exitFailed)
	}
	if !strings.Contains(stdout.String(), "passed=false") {
		t.Errorf("stdout = %s", stdout.String())
	}
}

// Verification refuses a changed evidence file, a changed bundle, another
// key and missing files, each with exit 1.
func TestVerifyRefusesTampering(t *testing.T) {
	t.Parallel()
	edit := func(dir, name string, f func([]byte) []byte) {
		t.Helper()
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path) //nolint:gosec // a test path
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, f(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	otherKey := sealedRun(t, false)
	for name, tamper := range map[string]func(dir string){
		"evidence changed": func(dir string) {
			edit(dir, filepath.Join(evidenceDir, "01-a", "proof.txt"), func([]byte) []byte { return []byte("forged") })
		},
		"evidence deleted": func(dir string) { _ = os.Remove(filepath.Join(dir, evidenceDir, "run", "transcript.txt")) },
		"payload changed": func(dir string) {
			edit(dir, bundleFile, func(b []byte) []byte {
				return bytes.Replace(b, []byte("payload\": \""), []byte("payload\": \"AAAA"), 1)
			})
		},
		"another key": func(dir string) {
			edit(dir, keyFile, func([]byte) []byte { d, _ := os.ReadFile(filepath.Join(otherKey, keyFile)); return d }) //nolint:gosec // a test path
		},
		"no bundle":     func(dir string) { _ = os.Remove(filepath.Join(dir, bundleFile)) },
		"no key":        func(dir string) { _ = os.Remove(filepath.Join(dir, keyFile)) },
		"key not valid": func(dir string) { edit(dir, keyFile, func([]byte) []byte { return []byte("!!!") }) },
		"bundle not JSON": func(dir string) {
			edit(dir, bundleFile, func([]byte) []byte { return []byte("{") })
		},
	} {
		dir := sealedRun(t, false)
		tamper(dir)
		var stdout, stderr bytes.Buffer
		if code := run(t.Context(), []string{"verify", "-dir", dir}, &stdout, &stderr); code != exitFailed {
			t.Errorf("%s: exit %d, want %d (%s)", name, code, exitFailed, stderr.String())
		}
	}
}

// The key can be pinned from outside the directory, which is how an auditor
// who got the public key by another route should verify.
func TestVerifyWithAPinnedKey(t *testing.T) {
	t.Parallel()
	dir, other := sealedRun(t, false), sealedRun(t, false)
	var stdout, stderr bytes.Buffer
	if code := run(t.Context(), []string{"verify", "-dir", dir, "-key", filepath.Join(dir, keyFile)}, &stdout, &stderr); code != exitOK {
		t.Errorf("the right key: exit %d: %s", code, stderr.String())
	}
	if code := run(t.Context(), []string{"verify", "-dir", dir, "-key", filepath.Join(other, keyFile)}, &stdout, &stderr); code != exitFailed {
		t.Errorf("another run's key: exit %d, want %d", code, exitFailed)
	}
	if code := run(t.Context(), []string{"verify", "-nonsense"}, &stdout, &stderr); code != exitUsage {
		t.Errorf("a bad flag: exit %d", code)
	}
}

// A run against a server that fails the first call still leaves a sealed,
// verifiable bundle: the failure is evidence, the process reports it, and the
// services it started are stopped.
func TestExecuteSealsAFailedRun(t *testing.T) {
	t.Setenv(adminTokenEnv, "admin-token")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "app.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "out")
	var stdout bytes.Buffer
	o := options{
		server: srv.URL, org: "o", app: "a", out: out, project: project, plux: "plux", pluxLoad: "plux-load",
		listen: "127.0.0.1", advertise: "127.0.0.1", idpPort: freePort(t), sinkPort: freePort(t), devices: 10,
		stage: time.Minute, propagation: time.Minute, upgradeWindow: time.Minute, fleetEnv: "staging",
	}
	err := execute(t.Context(), o, &stdout)
	if err == nil || !strings.Contains(err.Error(), "01-sso-roles-scim") {
		t.Fatalf("err = %v", err)
	}
	st, verr := verifyDir(out, "")
	if verr != nil || st.Passed || st.Steps[0].Status != walkthrough.StatusFailed || st.Steps[1].Status != walkthrough.StatusSkipped {
		t.Errorf("verifyDir = %+v, %v", st, verr)
	}
	if _, err := os.Stat(filepath.Join(out, "work", "project", "app.json")); err != nil {
		t.Errorf("the project is not copied: %v", err)
	}
	if strings.Contains(stdout.String(), "admin-token") {
		t.Error("the administrator's token reached the transcript")
	}
}

// freePort returns a TCP port that was free a moment ago.
func freePort(t *testing.T) string {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return port
}

// The step list the runner is given enables only the optional steps asked for.
func TestEnabledSteps(t *testing.T) {
	t.Parallel()
	got := enabledSteps(" 07b-viewer-on-emulator, ,x ")
	if len(got) != 2 || !got["07b-viewer-on-emulator"] || !got["x"] {
		t.Errorf("enabledSteps = %v", got)
	}
	if len(enabledSteps("")) != 0 {
		t.Error("no steps are enabled by default")
	}
}

// A CA file that is missing or holds no certificate is an error.
func TestClientFor(t *testing.T) {
	t.Parallel()
	if _, err := clientFor(""); err != nil {
		t.Error(err)
	}
	if _, err := clientFor(filepath.Join(t.TempDir(), "missing.pem")); err == nil {
		t.Error("a missing CA file must be an error")
	}
	bad := filepath.Join(t.TempDir(), "bad.pem")
	if err := os.WriteFile(bad, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := clientFor(bad); err == nil {
		t.Error("a CA file without a certificate must be an error")
	}
}

// The hooks run the operator's commands with the documented environment.
func TestHooks(t *testing.T) {
	t.Parallel()
	o := options{ //nolint:gosec // shell commands, not credentials
		tokenCmd:    `echo "token-for-$WALKTHROUGH_ACTOR-$WALKTHROUGH_EMAIL"`,
		auditKeyCmd: `echo "audit-1=a2V5"`, upgradeCmd: `true`,
		viewerCmd: `test "$WALKTHROUGH_DEVICE" = d1 && test "$WALKTHROUGH_RECONSTRUCTION" = r1`,
	}
	h := hooksFor(o)
	if tok, err := h.IssueToken(t.Context(), "bob", "bob@x"); err != nil || tok != "token-for-bob-bob@x" {
		t.Errorf("IssueToken = %q, %v", tok, err)
	}
	if key, err := h.AuditKey(t.Context()); err != nil || strings.TrimSpace(key) != "audit-1=a2V5" {
		t.Errorf("AuditKey = %q, %v", key, err)
	}
	if err := h.Upgrade(t.Context()); err != nil {
		t.Error(err)
	}
	if err := h.Viewer(t.Context(), "d1", "r1"); err != nil {
		t.Error(err)
	}
	if err := h.Viewer(t.Context(), "d2", "r1"); err == nil {
		t.Error("the viewer command must see its environment")
	}
	if hooksFor(options{}).Viewer != nil {
		t.Error("without a viewer command there is no viewer hook")
	}
	if shellStepUp("") != nil {
		t.Error("without a step-up command there is no step-up function")
	}
	if err := shellStepUp(`test "$WALKTHROUGH_ACTOR" = bob && test "$WALKTHROUGH_STEPUP_URL" = https://u`)(t.Context(), "bob", "https://u"); err != nil {
		t.Error(err)
	}
}

// The person-facing hooks (token and step-up) get the run's shared environment
// beside the person: the installation, the organisation and the test provider.
func TestHooksSharedEnvironment(t *testing.T) {
	t.Parallel()
	shared := []string{"WALKTHROUGH_SERVER=https://s", "WALKTHROUGH_IDP_ISSUER=https://idp"}
	o := options{tokenCmd: `test "$WALKTHROUGH_SERVER" = https://s && echo "$WALKTHROUGH_IDP_ISSUER $WALKTHROUGH_ACTOR"`}
	if tok, err := hooksFor(o, shared...).IssueToken(t.Context(), "carol", "carol@x"); err != nil || tok != "https://idp carol" {
		t.Errorf("IssueToken = %q, %v", tok, err)
	}
	hook := shellStepUp(`test "$WALKTHROUGH_SERVER" = https://s && test "$WALKTHROUGH_ACTOR" = carol`, shared...)
	if err := hook(t.Context(), "carol", "https://s/stepup?request=1"); err != nil {
		t.Error(err)
	}
	if err := hook(t.Context(), "dave", "https://s/stepup?request=1"); err == nil {
		t.Error("the hook must see the actor it is run for")
	}
}

// hookEnvironment tells the hooks where the installation and the test
// provider are, and the server's CA only when there is one.
func TestHookEnvironment(t *testing.T) {
	t.Parallel()
	port := "0"
	idp, err := walkthrough.NewIDP("127.0.0.1:"+port, "127.0.0.1:"+port, []string{"127.0.0.1"}, "c", "s", time.Now, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = idp.Close() })
	env := hookEnvironment(options{server: "https://s", org: "org-1"}, idp)
	for _, want := range []string{"WALKTHROUGH_SERVER=https://s", "WALKTHROUGH_ORG=org-1", "WALKTHROUGH_IDP_ISSUER=" + idp.Issuer(), "WALKTHROUGH_IDP_CA_PEM=" + idp.CAPEM()} {
		if !slices.Contains(env, want) {
			t.Errorf("the environment lacks %.40s", want)
		}
	}
	if slices.ContainsFunc(env, func(s string) bool { return strings.HasPrefix(s, "WALKTHROUGH_CA_FILE=") }) {
		t.Error("no CA file was given")
	}
	if with := hookEnvironment(options{caFile: "/ca.pem"}, idp); !slices.Contains(with, "WALKTHROUGH_CA_FILE=/ca.pem") {
		t.Errorf("environment = %v", with)
	}
}

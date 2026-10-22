// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package walkthrough

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// The Connect services the walkthrough calls without the CLI.
const (
	identityService      = "/plux.v1.IdentityService/"
	federationService    = "/plux.v1.FederationService/"
	appService           = "/plux.v1.AppService/"
	securityAdminService = "/plux.v1.SecurityAdminService/"
	auditService         = "/plux.v1.AuditService/"
)

// Hooks are the parts of the run that only the environment can provide: the
// maintainer's script wires them to kubectl, helm, a browser with a virtual
// authenticator and the server's own binary.
type Hooks struct {
	// IssueToken returns an access token for a person who has an account
	// after step 1 (the stand-in for `plux login` and a person's browser).
	IssueToken func(ctx context.Context, actor, email string) (string, error)
	// AuditKey returns the audit public key as id=base64, as
	// `plux-server audit public-key` prints it.
	AuditKey func(ctx context.Context) (string, error)
	// Upgrade performs the chart's rolling upgrade and returns when it has
	// finished (step 8).
	Upgrade func(ctx context.Context) error
	// Viewer opens a reconstruction in the viewer host on an emulator and
	// checks that it shows the page (the optional step 7b).
	Viewer func(ctx context.Context, deviceID, reconstruction string) error
}

// Env is everything the steps need. The zero value is not usable; main and
// the tests fill it in.
type Env struct {
	Plux   Commander
	Creds  Credentials
	Admin  Caller
	Public Caller
	SCIM   SCIMClient
	Fleet  Fleet
	Hooks  Hooks
	IDP    *IDP
	Sink   *Sink
	// SinkAddress is the host:port the installation reaches the sink on.
	SinkAddress string

	// Server and Org are the installation's public URL and the organisation.
	Server, Org string
	App         string
	// Project is the directory of the writable copy of the Plux Bank
	// project the steps publish.
	Project string
	// FnSDK is the SDK's directory (sdk/fn): the plux-fn crate and the
	// example crates the project's functions are built from.
	FnSDK string
	// TempDir holds files the CLI writes before they become evidence.
	TempDir string
	// ProdEnv is the production environment, FleetEnv the environment the
	// synthetic devices belong to (devices cannot register in production).
	ProdEnv, FleetEnv string
	// Devices is the size of the synthetic fleet of one run.
	Devices int
	// StageDuration is how long each rollout stage lasts.
	StageDuration time.Duration
	// Propagation is the bound on how long the fleet may take to apply a
	// control document (NFR-024).
	Propagation time.Duration
	// UpgradeWindow is how long the fleet runs while the chart is upgraded.
	UpgradeWindow time.Duration

	// Now and Sleep are injected; Sleep returns early when ctx ends.
	Now   func() time.Time
	Sleep func(ctx context.Context, d time.Duration) error

	state walkState
}

// walkState is what earlier steps hand to later ones.
type walkState struct {
	started  time.Time
	teams    map[string]string // team name to ID
	users    map[string]string // actor to user ID
	deviceID string            // a device that was online during step 4
	killedAt time.Time
	// baseline is the release step 3 promoted before the rollout.
	baseline string
	// reconstruction is the reconstruction step 7 made.
	reconstruction string
	// dsrDevice is the device step 9 exports and erases.
	dsrDevice string
}

// SCIMClient makes the SCIM 2.0 calls of the provisioning in step 1.
type SCIMClient interface {
	// Post creates a resource at path, such as "/scim/v2/Users", with the
	// bearer token, and returns the created resource.
	Post(ctx context.Context, token, path string, body any) (map[string]any, error)
}

// Steps returns the nine steps of the walkthrough, in order, with the
// optional emulator step after the reconstruction.
func Steps(e *Env) []Step {
	return slices.Concat(e.accessSteps(), e.releaseSteps(), e.auditSteps())
}

// accessSteps are steps 1 and 2: who may do what.
func (e *Env) accessSteps() []Step {
	return []Step{
		{
			ID: "01-sso-roles-scim", Title: "SSO sign-in, a custom role, SCIM provisioning and a restriction policy",
			Verifies: []string{"SEC-100", "GOV-004", "GOV-002", "GOV-006", "GOV-007"},
			Plan: []string{
				"create an OIDC identity provider that points at the test provider and sign a provisioned person in through it",
				"create a SCIM token, provision four people and four teams through SCIM",
				"create the custom role release-approver and grant roles to the teams",
				"restrict the contractors team from production publishing and let `plux access explain` name the restriction",
			},
			Run: e.stepSSO,
		},
		{
			ID: "02-secure-release-approval", Title: "A release touching a secure page needs two approvers from distinct teams",
			Verifies: []string{"GOV-020", "GOV-021", "GOV-022", "GOV-023", "SEC-103"},
			Plan: []string{
				"install the maximum-profile policy; publish a release that touches the secure transfer page and try to promote it",
				"the author cannot approve; one approval is not enough",
				"a content change invalidates the approval; two approvers from distinct teams approve the new release and it is promoted",
			},
			Run: e.stepApproval,
		},
	}
}

// releaseSteps are steps 3 to 5: how a release reaches the fleet and how it
// is stopped.
func (e *Env) releaseSteps() []Step {
	return []Step{
		{
			ID: "03-staged-rollout", Title: "Staged rollout 1% to 10% to 50%; a regression trips the health gate",
			Verifies: []string{"REL-050", "REL-052", "REL-053"},
			Plan: []string{
				"promote a baseline, run the synthetic fleet, create a rollout 1% / 10% / 50% / 100% with a health gate that rolls back",
				"start a second fleet whose devices on the candidate report errors",
				"the gate trips, the rollout pauses and rolls back, and the channel returns to the baseline",
			},
			Run: e.stepRollout,
		},
		{
			ID: "04-kill-switch-break-glass", Title: "Kill switch through break-glass during a change freeze",
			Verifies: []string{"REL-060", "GOV-024", "GOV-025", "NFR-024", "SYN-060"},
			Plan: []string{
				"open a change freeze; a kill switch without break-glass is refused",
				"the kill switch through break-glass with a justification; the fleet applies it within the propagation bound",
				"the post-hoc review is opened by the action and closed by a second person",
			},
			Run: e.stepKillSwitch,
		},
		{
			ID: "05-mandatory-update-forced-rollback", Title: "A mandatory update and a forced rollback on the fleet",
			Verifies: []string{"SYN-060", "REL-006", "NFR-024"},
			Plan: []string{
				"make the newest release mandatory and force a rollback to the baseline, with the fleet running",
				"the fleet applies the control document within the propagation bound; both are withdrawn",
			},
			Run: e.stepMandatoryAndRollback,
		},
	}
}

// auditSteps are steps 6 to 9: what can be shown afterwards, the upgrade and
// the data-subject request.
func (e *Env) auditSteps() []Step {
	return []Step{
		{
			ID: "06-audit-export-compliance-report", Title: "Audit export to a syslog sink and a verified compliance report",
			Verifies: []string{"SEC-143", "GOV-026"},
			Plan: []string{
				"create a TLS syslog export destination and wait for RFC 5424 messages at the sink",
				"produce the compliance report for the period and verify its signature offline against the audit key",
			},
			Run: e.stepAuditExport,
		},
		{
			ID: "07-what-did-the-user-see", Title: "Reconstruct what a device showed at a moment",
			Verifies: []string{"SEC-142", "GOV-024"},
			Plan: []string{
				"reconstruct the release a device of the fleet was running during step 4 and open the signed, read-only reconstruction",
			},
			Run: e.stepReconstruct,
		},
		{
			ID: "07b-viewer-on-emulator", Title: "The viewer host on an emulator shows the reconstructed page (CI only)",
			Verifies: []string{"SEC-142"}, Optional: true,
			Plan: []string{"open the reconstruction in the viewer host on the emulator and check that the page is shown"},
			Run:  e.stepViewer,
		},
		{
			ID: "08-rolling-upgrade", Title: "Rolling upgrade of the chart with the fleet syncing",
			Verifies: []string{"DEP-004", "DEP-003"},
			Plan: []string{
				"run the synthetic fleet with a zero error budget, upgrade the chart in the middle of the run, and require not one failed request",
			},
			Run: e.stepUpgrade,
		},
		{
			ID: "09-data-subject-request", Title: "Data-subject export and erasure",
			Verifies: []string{"SEC-162"},
			Plan: []string{
				"list the stores of personal data, export everything held about a device of the fleet, erase it and show that the second export differs",
			},
			Run: e.stepDSR,
		},
	}
}

// walk is one step's view of the environment: it numbers and records every
// command and call, so that each leaves a piece of evidence.
type walk struct {
	e   *Env
	run *Run
	n   int
}

// newWalk returns the helper of one step.
func (e *Env) newWalk(run *Run) *walk { return &walk{e: e, run: run} }

// slug turns the first words of a command into a file-name part.
var slugUnsafe = regexp.MustCompile(`[^a-z0-9]+`)

func slug(words ...string) string {
	var keep []string
	for _, w := range words {
		if !strings.HasPrefix(w, "-") && len(keep) < 3 {
			keep = append(keep, w)
		}
	}
	return strings.Trim(slugUnsafe.ReplaceAllString(strings.ToLower(strings.Join(keep, "-")), "-"), "-")
}

// record stores a numbered evidence file.
func (w *walk) record(label string, data string) error {
	w.n++
	return w.run.Evidence(fmt.Sprintf("%02d-%s.txt", w.n, label), []byte(data))
}

// plux runs one command and records it. A non-zero exit is an error.
func (w *walk) plux(ctx context.Context, actor string, args ...string) (string, error) {
	res, err := w.do(ctx, actor, args)
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		return res.Stdout, fmt.Errorf("%s exited %d: %s", Command{Actor: actor, Args: args}, res.ExitCode, strings.TrimSpace(res.Stderr))
	}
	return res.Stdout, nil
}

// refused runs a command that must fail, with one of the wanted markers (a
// PLX code or a word) in what it printed.
func (w *walk) refused(ctx context.Context, actor string, want *regexp.Regexp, args ...string) error {
	res, err := w.do(ctx, actor, args)
	if err != nil {
		return err
	}
	out := res.Stdout + res.Stderr
	if res.ExitCode == 0 {
		return fmt.Errorf("%s succeeded, but must be refused", Command{Actor: actor, Args: args})
	}
	if !want.MatchString(out) {
		return fmt.Errorf("%s was refused with %q, want a match of %s", Command{Actor: actor, Args: args}, strings.TrimSpace(out), want)
	}
	return nil
}

// do runs a command and records its arguments, exit code and output.
func (w *walk) do(ctx context.Context, actor string, args []string) (Result, error) {
	cmd := Command{Actor: actor, Args: args}
	w.run.Logf("%s", cmd)
	res, err := w.e.Plux.Run(ctx, cmd)
	if err != nil {
		return res, fmt.Errorf("%s: %w", cmd, err)
	}
	body := fmt.Sprintf("%s\nexit %d\n--- stdout\n%s\n--- stderr\n%s\n", cmd, res.ExitCode, res.Stdout, res.Stderr)
	return res, w.record(slug(append([]string{actor}, args...)...), body)
}

// rpc makes one Connect call and records the request and the answer with
// every secret-bearing field replaced.
func (w *walk) rpc(ctx context.Context, c Caller, procedure string, req map[string]any) (map[string]any, error) {
	w.run.Logf("rpc %s", procedure)
	resp := map[string]any{}
	err := c.Call(ctx, procedure, req, &resp)
	text := "request " + redactedJSON(req) + "\nresponse " + redactedJSON(resp) + "\n"
	if err != nil {
		text += "error " + err.Error() + "\n"
	}
	if rerr := w.record(slug(procedure[strings.LastIndex(procedure, "/")+1:]), text); rerr != nil {
		return resp, rerr
	}
	if err != nil {
		return resp, fmt.Errorf("call %s: %w", procedure, err)
	}
	return resp, nil
}

// secretKey matches field names whose values are never written down.
var secretKey = regexp.MustCompile(`(?i)secret|token|password|credential|privatekey`)

// redactedJSON renders v with the value of every secret-bearing key replaced.
func redactedJSON(v any) string {
	data, err := json.Marshal(redact(v))
	if err != nil {
		return "{}"
	}
	return string(data)
}

// redact copies a decoded JSON value, replacing secret-bearing values.
func redact(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			if secretKey.MatchString(k) {
				out[k] = "[redacted]"
			} else {
				out[k] = redact(val)
			}
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = redact(val)
		}
		return out
	default:
		return v
	}
}

// decode parses the JSON a command printed.
func decode(out string) (any, error) {
	var v any
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		return nil, fmt.Errorf("the output is not JSON: %w", err)
	}
	return v, nil
}

// findString returns the first string or number stored under key, searching
// the value depth first in key order of appearance of arrays and sorted maps.
func findString(v any, key string) (string, bool) {
	switch x := v.(type) {
	case map[string]any:
		if val, ok := x[key]; ok {
			switch s := val.(type) {
			case string:
				return s, true
			case float64:
				return fmt.Sprintf("%.0f", s), true
			}
		}
		for _, k := range sortedKeys(x) {
			if s, ok := findString(x[k], key); ok {
				return s, true
			}
		}
	case []any:
		for _, item := range x {
			if s, ok := findString(item, key); ok {
				return s, true
			}
		}
	}
	return "", false
}

// findWhere returns the id of the first object whose field equals value.
func findWhere(v any, field, value string) (string, bool) {
	switch x := v.(type) {
	case map[string]any:
		if s, ok := x[field].(string); ok && s == value {
			if id, ok := x["id"].(string); ok {
				return id, true
			}
		}
		for _, k := range sortedKeys(x) {
			if id, ok := findWhere(x[k], field, value); ok {
				return id, true
			}
		}
	case []any:
		for _, item := range x {
			if id, ok := findWhere(item, field, value); ok {
				return id, true
			}
		}
	}
	return "", false
}

// jsonField runs a --json command's output through findString.
func jsonField(out, key string) (string, error) {
	v, err := decode(out)
	if err != nil {
		return "", err
	}
	s, ok := findString(v, key)
	if !ok {
		return "", fmt.Errorf("the output has no %q", key)
	}
	return s, nil
}

// poll calls check every interval until it reports done, fails, or the
// timeout passes.
func (e *Env) poll(ctx context.Context, timeout, interval time.Duration, what string, check func() (bool, error)) error {
	deadline := e.Now().Add(timeout)
	for {
		done, err := check()
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		if !e.Now().Before(deadline) {
			return fmt.Errorf("timed out after %s waiting for %s", timeout, what)
		}
		if err := e.Sleep(ctx, interval); err != nil {
			return err
		}
	}
}

// SleepContext waits for d or until ctx ends.
func SleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("walkthrough.SleepContext: %w", ctx.Err())
	case <-t.C:
		return nil
	}
}

// mutateProject changes the content of the project in a way the compiler
// sees: it appends a marker to the title of the secure transfer page.
func (e *Env) mutateProject(marker string) error {
	const titleID = "01a14bd7-5c1d-71fe-8376-851b7f675e47"
	path := filepath.Join(e.Project, "translations", "en.json")
	data, err := os.ReadFile(path) //nolint:gosec // the project directory is the walkthrough's own copy
	if err != nil {
		return fmt.Errorf("walkthrough.mutateProject: %w", err)
	}
	re := regexp.MustCompile(`("` + titleID + `"\s*:\s*")([^"]*)(")`)
	if !re.Match(data) {
		return errors.New("walkthrough.mutateProject: the secure page's title is not in translations/en.json")
	}
	out := re.ReplaceAll(data, []byte("${1}${2} "+marker+"${3}"))
	if err := os.WriteFile(path, out, 0o600); err != nil { //nolint:gosec // the project directory is the walkthrough's own copy
		return fmt.Errorf("walkthrough.mutateProject: %w", err)
	}
	return nil
}

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
	"strings"
	"testing"
	"time"
)

// killSwitchRules scripts step 4 against a fleet control document that
// starts at the given version and advances when the fleet's kill switch is
// thrown; production is frozen, so only its break-glass kill switch goes
// through.
func killSwitchRules(version *int) []rule {
	return []rule{
		{[]string{"freeze put"}, out(`{"id":"fz1"}`)},
		{[]string{"control kill", "--env production", "--break-glass"}, out("")},
		{[]string{"control kill", "--env staging"}, func(Command) Result { *version++; return Result{} }},
		{[]string{"control kill"}, refuse("PLX-8020 the environment is under a change freeze")},
		{[]string{"control get"}, func(Command) Result { return Result{Stdout: fmt.Sprintf(`{"version":%d}`, *version)} }},
		{[]string{"breakglass list", "open"}, out(`{"items":[{"id":"bg1"}]}`)},
		{[]string{"admin|breakglass review"}, refuse("PLX-8031 you cannot review your own emergency action")},
		{[]string{"bob|breakglass review"}, out("reviewed")},
		{[]string{"breakglass list", "reviewed"}, out(`{"items":[{"id":"bg1"}]}`)},
		{[]string{"freeze delete"}, out("deleted")},
	}
}

// Step 4: the production freeze refuses an ordinary kill switch, the
// break-glass one goes through, the fleet applies the switch on its own
// environment, the review is refused to the actor, closed by
// a second person, and the freeze is lifted.
func TestKillSwitchStep(t *testing.T) {
	t.Parallel()
	env, plux, fleet := testEnv(t)
	version := 1
	plux.rules = killSwitchRules(&version)
	fleet.report.ControlApplied = 20
	st, _, err := runOneStep(t, env, "04-kill-switch-break-glass")
	if err != nil || !st.Passed {
		t.Fatalf("step failed: %v", err)
	}
	if !plux.called("freeze put", "--env-key production") || !plux.called("breakglass review", "bob|") || !plux.called("freeze delete fz1") {
		t.Errorf("calls = %v", plux.calls)
	}
	// A page switch names the page by its document ID, as the server
	// requires; the page's key is refused (PLX-1005).
	if !plux.called("control kill", "--env production", "--break-glass", "page "+transferPageID) ||
		!plux.called("control kill", "--env staging", "page "+transferPageID) {
		t.Errorf("the kill switch does not name the transfer page's ID: calls = %v", plux.calls)
	}
	if env.state.deviceID != "dev-1" || env.state.killedAt.IsZero() {
		t.Errorf("state = %+v: step 7 needs a device and a time", env.state)
	}
	if got := fleet.specs[0]; got.Env != "staging" || got.Devices != 10 || got.Duration != 75*time.Second {
		t.Errorf("fleet spec = %+v: it must run past the propagation bound", got)
	}
}

// The propagation bound is a manual device check: a fleet that applies the
// kill switch late still passes step 4, and the transcript records its count.
func TestKillSwitchStepRecordsPropagation(t *testing.T) {
	t.Parallel()
	env, plux, fleet := testEnv(t)
	version := 1
	plux.rules = killSwitchRules(&version)
	fleet.report.ControlApplied = 12
	st, dir, err := runOneStep(t, env, "04-kill-switch-break-glass")
	if err != nil || !st.Passed {
		t.Fatalf("step failed: %v", err)
	}
	if got := allEvidence(t, dir); !strings.Contains(got, "12 control applications from") || !strings.Contains(got, "manual device check") {
		t.Errorf("the transcript does not record the propagation: %s", got)
	}
}

// Step 4 fails when the kill switch is not
// refused under the freeze, when the control document does not advance and
// when the actor may review their own action.
func TestKillSwitchStepFailures(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		applied int64
		tweak   func(*[]rule, *int)
		want    string
	}{
		"freeze ignored": {applied: 20, tweak: func(r *[]rule, _ *int) { *r = append([]rule{{[]string{"control kill"}, out("")}}, *r...) }, want: "must be refused"},
		"version stuck": {applied: 20, tweak: func(r *[]rule, v *int) {
			*r = append([]rule{{[]string{"control kill", "--env staging"}, out("")}}, *r...)
		}, want: "did not advance"},
		"self review ok": {applied: 20, tweak: func(r *[]rule, _ *int) { *r = append([]rule{{[]string{"admin|breakglass review"}, out("")}}, *r...) }, want: "must not review"},
		"no open review": {applied: 20, tweak: func(r *[]rule, _ *int) { *r = append([]rule{{[]string{"breakglass list", "open"}, out(`{}`)}}, *r...) }, want: "no open break-glass review"},
		"review not shown": {applied: 20, tweak: func(r *[]rule, _ *int) {
			*r = append([]rule{{[]string{"breakglass list", "reviewed"}, out(`{}`)}}, *r...)
		}, want: "not listed as reviewed"},
	} {
		env, plux, fleet := testEnv(t)
		version := 1
		plux.rules = killSwitchRules(&version)
		if tc.tweak != nil {
			tc.tweak(&plux.rules, &version)
		}
		fleet.report.ControlApplied = tc.applied
		if _, _, err := runOneStep(t, env, "04-kill-switch-break-glass"); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", name, err, tc.want)
		}
	}
}

// A channel with no control document yet expects one application per device,
// not two: there is nothing to apply when the fleet starts.
func TestPropagationWithoutAnEarlierDocument(t *testing.T) {
	t.Parallel()
	env, plux, fleet := testEnv(t)
	version := 0
	plux.rules = append([]rule{{[]string{"control get"}, func(Command) Result {
		if version == 0 {
			return Result{ExitCode: 1, Stderr: "no control document"}
		}
		return Result{Stdout: fmt.Sprintf(`{"version":%d}`, version)}
	}}}, killSwitchRules(&version)...)
	fleet.report.ControlApplied = 10
	if _, _, err := runOneStep(t, env, "04-kill-switch-break-glass"); err != nil {
		t.Errorf("err = %v", err)
	}
}

// Step 5 makes the newest release mandatory, forces the rollback to the
// baseline step 3 promoted, and withdraws both.
func TestMandatoryAndRollbackStep(t *testing.T) {
	t.Parallel()
	env, plux, fleet := testEnv(t)
	env.state.baseline = "4"
	version := 1
	plux.rules = []rule{
		{[]string{"release list"}, out(`{"releases":[{"sequence":9},{"sequence":8}]}`)},
		{[]string{"control mandatory", "--release 9"}, func(Command) Result { version++; return Result{} }},
		{[]string{"control rollback", "--release 4"}, func(Command) Result { version++; return Result{} }},
		{[]string{"control mandatory", "--clear"}, out("")},
		{[]string{"control rollback", "--clear"}, out("")},
		{[]string{"control get"}, func(Command) Result { return Result{Stdout: fmt.Sprintf(`{"version":%d}`, version)} }},
	}
	fleet.report.ControlApplied = 20
	if _, _, err := runOneStep(t, env, "05-mandatory-update-forced-rollback"); err != nil {
		t.Fatalf("err = %v", err)
	}
	env2, _, _ := testEnv(t)
	if _, _, err := runOneStep(t, env2, "05-mandatory-update-forced-rollback"); err == nil {
		t.Error("without the baseline of step 3 the step must fail instead of guessing a release")
	}
}

// Step 8 passes only when the upgrade ran and not one request failed.
func TestUpgradeStep(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		report  FleetReport
		waitErr error
		upgrade error
		want    string
	}{
		"clean":          {report: FleetReport{Requests: 5000}},
		"failed request": {report: FleetReport{Requests: 5000, Errors: 1}, want: "failed during the upgrade"},
		"fleet exit":     {report: FleetReport{Requests: 5000, Errors: 3}, waitErr: errors.New("exit status 1"), want: "failed requests"},
		"no requests":    {report: FleetReport{}, want: "failed during the upgrade"},
		"upgrade failed": {report: FleetReport{Requests: 5000}, upgrade: errors.New("helm: timed out"), want: "helm: timed out"},
	} {
		env, _, fleet := testEnv(t)
		fleet.report, fleet.waitErr = tc.report, tc.waitErr
		upgraded := false
		env.Hooks.Upgrade = func(context.Context) error { upgraded = true; return tc.upgrade }
		_, _, err := runOneStep(t, env, "08-rolling-upgrade")
		if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
			t.Errorf("%s: err = %v, want %q", name, err, tc.want)
		}
		if !upgraded {
			t.Errorf("%s: the chart was not upgraded", name)
		}
		if got := fleet.specs[0]; got.MaxErrorRatio != 0 || got.Duration != 5*time.Minute {
			t.Errorf("%s: spec = %+v: the error budget of an upgrade is zero", name, got)
		}
		if tc.upgrade == nil && env.state.dsrDevice != "dev-1" {
			t.Errorf("%s: step 9 needs a device from this fleet", name)
		}
	}
}

// Step 9 exports, erases and exports again; the archives must be real and
// must differ.
func TestDataSubjectStep(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		before, after string
		want          string
	}{
		"erased":       {before: `{"telemetry":[1,2],"device":{"id":"d"}}`, after: `{"telemetry":[]}`},
		"unchanged":    {before: `{"telemetry":[1]}`, after: `{"telemetry":[1]}`, want: "equals the export before"},
		"empty":        {before: `{}`, after: `{}`, want: "non-empty JSON archive"},
		"not an array": {before: `[1]`, after: `{}`, want: "non-empty JSON archive"},
	} {
		env, plux, _ := testEnv(t)
		env.state.dsrDevice = "dev-9"
		exports := 0
		plux.rules = []rule{
			{[]string{"dsr map"}, out("{}")},
			{[]string{"dsr export", "dev-9"}, func(c Command) Result {
				exports++
				body := tc.before
				if exports > 1 {
					body = tc.after
				}
				return writeOut(body)(c)
			}},
			{[]string{"dsr erase", "--kind device", "dev-9"}, out(`{"erased":"dev-9"}`)},
			{[]string{"dsr status"}, out("{}")},
		}
		_, dir, err := runOneStep(t, env, "09-data-subject-request")
		if tc.want == "" {
			if err != nil {
				t.Errorf("%s: %v", name, err)
			} else if !strings.Contains(allEvidence(t, dir), `"telemetry":[1,2]`) {
				t.Errorf("%s: the export before the erasure is not evidence", name)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", name, err, tc.want)
		}
	}
	env, _, _ := testEnv(t)
	if _, _, err := runOneStep(t, env, "09-data-subject-request"); err == nil {
		t.Error("without a device from step 8 the step must fail")
	}
}

// Step 7 reconstructs the device of step 4 at the moment of the kill switch
// and opens the reconstruction.
func TestReconstructStep(t *testing.T) {
	t.Parallel()
	env, plux, _ := testEnv(t)
	env.state.deviceID = "dev-1"
	env.state.killedAt = time.Date(2026, 3, 1, 12, 7, 0, 0, time.UTC)
	plux.rules = []rule{
		{[]string{"audit reconstruct", "--device dev-1", "--at 2026-03-01T12:07:00Z"}, out(`{"id":"rec1","state":"ready"}`)},
		{[]string{"audit reconstructions", "rec1"}, out(`{"id":"rec1","manifest":"{}"}`)},
		{[]string{"audit reconstructions"}, out(`{"reconstructions":[{"id":"rec1"}]}`)},
	}
	if _, _, err := runOneStep(t, env, "07-what-did-the-user-see"); err != nil || env.state.reconstruction != "rec1" {
		t.Fatalf("err = %v, state = %+v", err, env.state)
	}
	plux.rules[1].answer = out(`{"id":"rec1"}`)
	if _, _, err := runOneStep(t, env, "07-what-did-the-user-see"); err == nil || !strings.Contains(err.Error(), "no manifest") {
		t.Errorf("err = %v", err)
	}
	empty, _, _ := testEnv(t)
	if _, _, err := runOneStep(t, empty, "07-what-did-the-user-see"); err == nil {
		t.Error("without a device from step 4 the step must fail")
	}
}

// The viewer step is a hook; without it, or without a reconstruction, it fails
// rather than pretending the page was seen.
func TestViewerStep(t *testing.T) {
	t.Parallel()
	env, _, _ := testEnv(t)
	var gotDevice, gotRec string
	env.Hooks.Viewer = func(_ context.Context, d, r string) error { gotDevice, gotRec = d, r; return nil }
	env.state.deviceID, env.state.reconstruction = "dev-1", "rec1"
	store, _ := NewStore(t.TempDir())
	rn := &Runner{Steps: Steps(env)[7:8], Store: store, Now: newFakeClock().Now, Enabled: map[string]bool{"07b-viewer-on-emulator": true}}
	if _, err := rn.Run(t.Context()); err != nil || gotDevice != "dev-1" || gotRec != "rec1" {
		t.Errorf("err = %v, hook got %q %q", err, gotDevice, gotRec)
	}
	env.Hooks.Viewer = nil
	rn.Store, _ = NewStore(t.TempDir())
	if _, err := rn.Run(t.Context()); err == nil {
		t.Error("no hook must fail")
	}
}

// Step 6: the syslog destination is created with the sink's CA, messages that
// are not RFC 5424 fail it, and the compliance report must verify and list
// approvals.
func TestAuditExportStep(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		messages []string
		report   string
		verify   Result
		want     string
	}{
		"verified":     {messages: validSyslog(), report: `{"changes":[{"approvals":["bob","carol"]}]}`},
		"not syslog":   {messages: append(validSyslog()[:4], "plain text"), report: `{"approvals":[]}`, want: "not RFC 5424"},
		"verify fails": {messages: validSyslog(), report: `{"approvals":[]}`, verify: Result{ExitCode: 1, Stderr: "signature invalid"}, want: "does not verify"},
		"no approvals": {messages: validSyslog(), report: `{"changes":[]}`, want: "lists no approval"},
	} {
		env, plux, _ := testEnv(t)
		sink := newTestSink(t)
		sendSyslog(t, sink, tc.messages)
		env.Sink, env.SinkAddress = sink, sink.Address()
		admin := &fakeCaller{answers: map[string]map[string]any{}}
		env.Admin = admin
		env.state.started = env.Now()
		env.Hooks.AuditKey = func(context.Context) (string, error) { return "audit-1=a2V5\n", nil }
		plux.rules = []rule{
			{[]string{"compliance report"}, writeOut(tc.report)},
			{[]string{"compliance verify", "--key audit-1=a2V5"}, func(Command) Result { return tc.verify }},
		}
		_, dir, err := runOneStep(t, env, "06-audit-export-compliance-report")
		if tc.want == "" {
			if err != nil {
				t.Errorf("%s: %v", name, err)
			}
			req := admin.requests[auditService+"CreateExportDestination"]
			if len(req) != 1 || req[0]["kind"] != "syslog" || req[0]["caPem"] != sink.CAPEM() || req[0]["address"] != sink.Address() {
				t.Errorf("%s: export request = %v", name, req)
			}
			if !strings.Contains(allEvidence(t, dir), "compliance") {
				t.Errorf("%s: the report is not evidence", name)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", name, err, tc.want)
		}
	}
}

// Step 3: the rollout moves through its stages, the regression fleet starts
// at 10% for the candidate release only, and the gate rolls the rollout back.
func TestRolloutStep(t *testing.T) {
	t.Parallel()
	env, plux, fleet := testEnv(t)
	states := []string{`{"state":"active","basisPoints":100}`, `{"state":"active","basisPoints":1000}`, `{"state":"active","basisPoints":1000}`, `{"state":"rolled_back","basisPoints":0,"reverted":12}`}
	shown := 0
	releases := 0
	plux.rules = []rule{
		{[]string{"publish", "--promote staging"}, out("")},
		{[]string{"publish", "--release"}, out("")},
		{[]string{"release list"}, func(Command) Result {
			releases++
			return Result{Stdout: fmt.Sprintf(`{"releases":[{"sequence":%d}]}`, 10+releases)}
		}},
		{[]string{"rollout create"}, out(`{"id":"ro1"}`)},
		{[]string{"rollout show"}, func(Command) Result {
			s := states[min(shown, len(states)-1)]
			shown++
			return Result{Stdout: s}
		}},
	}
	st, _, err := runOneStep(t, env, "03-staged-rollout")
	if err != nil || !st.Passed {
		t.Fatalf("err = %v", err)
	}
	if env.state.baseline != "11" {
		t.Errorf("baseline = %q", env.state.baseline)
	}
	if len(fleet.specs) != 2 || fleet.specs[0].InjectErrors || !fleet.specs[1].InjectErrors || fleet.specs[1].ErrorRelease != 12 {
		t.Errorf("fleets = %+v: the regression must hit the candidate (12) only", fleet.specs)
	}
	var create Command
	for _, c := range plux.calls {
		if len(c.Args) > 1 && c.Args[1] == "create" {
			create = c
		}
	}
	line := strings.Join(create.Args, " ")
	for _, want := range []string{"--release 12", "--stages 1%:1m0s,10%:1m0s,50%:1m0s,100%", "--on-regression rollback", "--gate-errors 100"} {
		if !strings.Contains(line, want) {
			t.Errorf("rollout create %q lacks %q", line, want)
		}
	}
}

// Step 3 fails when the rollout completes without the gate tripping, and when
// the rollback names no reverting release.
func TestRolloutStepFailures(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		final string
		want  string
	}{
		"completed":    {final: `{"state":"completed","basisPoints":10000}`, want: "without tripping the gate"},
		"not reverted": {final: `{"state":"rolled_back","basisPoints":0,"reverted":0}`, want: "names no release"},
	} {
		env, plux, _ := testEnv(t)
		shown := 0
		plux.rules = []rule{
			{[]string{"publish"}, out("")},
			{[]string{"release list"}, out(`{"releases":[{"sequence":7}]}`)},
			{[]string{"rollout create"}, out(`{"id":"ro1"}`)},
			{[]string{"rollout show"}, func(Command) Result {
				shown++
				if shown == 1 {
					return Result{Stdout: `{"state":"active","basisPoints":1000}`}
				}
				return Result{Stdout: tc.final}
			}},
		}
		if _, _, err := runOneStep(t, env, "03-staged-rollout"); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", name, err, tc.want)
		}
	}
	env, plux, _ := testEnv(t)
	plux.rules = []rule{
		{[]string{"publish"}, out("")},
		{[]string{"release list"}, out(`{"releases":[{"sequence":7}]}`)},
		{[]string{"rollout create"}, out(`{"id":"ro1"}`)},
		{[]string{"rollout show"}, out(`{"state":"active","basisPoints":100}`)},
	}
	if _, _, err := runOneStep(t, env, "03-staged-rollout"); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Errorf("a rollout that never reaches 10%%: err = %v", err)
	}
}

// Step 2: the author cannot approve; one approval does not carry to changed
// content; two approvers from distinct teams approve the new request; only
// then does the promotion go through.
func TestApprovalStep(t *testing.T) {
	t.Parallel()
	env, plux, _ := testEnv(t)
	sequences, requests := 0, 0
	promoted := false
	plux.rules = []rule{
		{[]string{"policies put"}, out("")},
		{[]string{"alice|init"}, out("")},
		{[]string{"publish", "--release"}, out("")},
		{[]string{"release list"}, func(Command) Result {
			sequences++
			return Result{Stdout: fmt.Sprintf(`{"releases":[{"sequence":%d}]}`, sequences)}
		}},
		{[]string{"alice|approvals approve"}, refuse("PLX-8003 the author cannot approve their own change")},
		{[]string{"approvals approve"}, out("approved")},
		{[]string{"approvals list"}, func(Command) Result {
			return Result{Stdout: fmt.Sprintf(`{"requests":[{"id":"req%d"}]}`, requests)}
		}},
		{[]string{"release promote", "--wait"}, func(Command) Result { promoted = true; return Result{} }},
		{[]string{"release promote"}, func(Command) Result {
			requests++
			return Result{ExitCode: 1, Stderr: "PLX-8001 approval required"}
		}},
	}
	if err := os.MkdirAll(filepath.Join(env.Project, "policies"), 0o700); err != nil {
		t.Fatal(err)
	}
	st, dir, err := runOneStep(t, env, "02-secure-release-approval")
	if err != nil || !st.Passed || !promoted {
		t.Fatalf("err = %v, promoted = %v", err, promoted)
	}
	if !plux.called("bob|approvals approve", "req3") || !plux.called("carol|approvals approve", "req3") {
		t.Errorf("both approvers must approve the second request: %v", plux.calls)
	}
	data, _ := os.ReadFile(filepath.Join(env.Project, "translations", "en.json"))
	if !strings.Contains(string(data), "(revised)") {
		t.Errorf("the content was not changed: %s", data)
	}
	if !strings.Contains(allEvidence(t, dir), "PLX-8001") {
		t.Error("the refusals are not evidence")
	}
}

// Step 2 fails when the author can approve their own change, and when the
// promotion is not refused for want of approval.
func TestApprovalStepFailures(t *testing.T) {
	t.Parallel()
	base := func() []rule {
		return []rule{
			{[]string{"policies put"}, out("")},
			{[]string{"alice|init"}, out("")},
			{[]string{"publish"}, out("")},
			{[]string{"release list"}, out(`{"releases":[{"sequence":3}]}`)},
			{[]string{"approvals list"}, out(`{"requests":[{"id":"req1"}]}`)},
			{[]string{"alice|approvals approve"}, refuse("PLX-8003 not eligible")},
			{[]string{"approvals approve"}, out("")},
			{[]string{"release promote"}, refuse("PLX-8001 approval required")},
		}
	}
	for name, tc := range map[string]struct {
		prepend rule
		want    string
	}{
		"author approves": {rule{[]string{"alice|approvals approve"}, out("")}, "must be refused"},
		"promotion open":  {rule{[]string{"release promote"}, out("")}, "must be refused"},
		"wrong refusal":   {rule{[]string{"alice|approvals approve"}, refuse("network error")}, "want a match"},
	} {
		env, plux, _ := testEnv(t)
		if err := os.MkdirAll(filepath.Join(env.Project, "policies"), 0o700); err != nil {
			t.Fatal(err)
		}
		plux.rules = append([]rule{tc.prepend}, base()...)
		if _, _, err := runOneStep(t, env, "02-secure-release-approval"); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", name, err, tc.want)
		}
	}
}

// Step 1 provisions four people and teams through SCIM, signs a person in
// through the OIDC provider, hands out tokens, and shows the restriction in
// `plux access explain`. No secret reaches the evidence.
func TestSSOStep(t *testing.T) {
	t.Parallel()
	env, plux, _ := testEnv(t)
	idp := newTestIDP(t)
	env.IDP = idp
	scim := &fakeSCIM{}
	creds := &credentialsLog{}
	env.SCIM, env.Creds = scim, creds
	env.Admin = &fakeCaller{answers: map[string]map[string]any{
		federationService + "CreateIdentityProvider": {"provider": map[string]any{"id": "idp1"}},
		federationService + "CreateScimToken":        {"secret": "scim-secret-xyz", "token": map[string]any{"id": "t1"}},
	}}
	env.Public = &signInCaller{idp: idp}
	env.Hooks.IssueToken = func(_ context.Context, actor, _ string) (string, error) { return "token-of-" + actor, nil }
	restricted := false
	plux.rules = []rule{
		{[]string{"roles create"}, out("{}")},
		{[]string{"grants create"}, out("{}")},
		{[]string{"restrictions create"}, func(Command) Result { restricted = true; return Result{Stdout: `{"id":"restr-1"}`} }},
		{[]string{"access explain"}, func(Command) Result {
			if restricted {
				return Result{Stdout: `{"allowed":false,"reason":"removed by restriction restr-1"}`}
			}
			return Result{Stdout: `{"allowed":true}`}
		}},
		{[]string{"dave|whoami"}, out("{}")},
	}
	st, dir, err := runOneStep(t, env, "01-sso-roles-scim")
	if err != nil || !st.Passed {
		t.Fatalf("err = %v", err)
	}
	users, groups := 0, 0
	for _, p := range scim.made {
		if strings.HasSuffix(p, "Users") {
			users++
		} else {
			groups++
		}
	}
	if users != 4 || groups != 4 || scim.tokens[0] != "scim-secret-xyz" {
		t.Errorf("SCIM made %d users and %d groups with token %q", users, groups, scim.tokens[0])
	}
	if creds.tokens["bob"] != "token-of-bob" || len(creds.tokens) != 4 {
		t.Errorf("tokens = %v", creds.tokens)
	}
	if !plux.called("roles create", "release-approver") || !plux.called("grants create", "--role release-manager", "--team scim-8") {
		t.Errorf("calls = %v", plux.calls)
	}
	evidence := allEvidence(t, dir)
	for _, secret := range []string{"scim-secret-xyz", idp.ClientSecret(), "token-of-bob"} {
		if strings.Contains(evidence, secret) {
			t.Errorf("the evidence holds a secret: %s", secret)
		}
	}
}

// A restriction that the explanation does not name fails step 1.
func TestSSOStepNeedsTheExplanationToNameTheRestriction(t *testing.T) {
	t.Parallel()
	env, plux, _ := testEnv(t)
	idp := newTestIDP(t)
	env.IDP, env.SCIM, env.Creds = idp, &fakeSCIM{}, &credentialsLog{}
	env.Admin = &fakeCaller{answers: map[string]map[string]any{
		federationService + "CreateIdentityProvider": {"provider": map[string]any{"id": "idp1"}},
		federationService + "CreateScimToken":        {"secret": "s"},
	}}
	env.Public = &signInCaller{idp: idp}
	env.Hooks.IssueToken = func(context.Context, string, string) (string, error) { return "t", nil }
	plux.rules = []rule{
		{[]string{"roles create"}, out("{}")},
		{[]string{"grants create"}, out("{}")},
		{[]string{"restrictions create"}, out(`{"id":"restr-1"}`)},
		{[]string{"access explain"}, out(`{"allowed":true}`)},
	}
	if _, _, err := runOneStep(t, env, "01-sso-roles-scim"); err == nil || !strings.Contains(err.Error(), "does not name restriction") {
		t.Errorf("err = %v", err)
	}
}

// The helpers that read the CLI's JSON find a field wherever it is, redact
// secrets at any depth, and turn a command into a file-name part.
func TestJSONHelpers(t *testing.T) {
	t.Parallel()
	v, err := decode(`{"a":[{"x":1},{"id":"first","sequence":12,"name":"n"}],"id":"outer"}`)
	if err != nil {
		t.Fatal(err)
	}
	if s, ok := findString(v, "sequence"); !ok || s != "12" {
		t.Errorf("sequence = %q, %v", s, ok)
	}
	if s, ok := findString(v, "id"); !ok || s != "outer" {
		t.Errorf("id = %q, %v", s, ok)
	}
	if id, ok := findWhere(v, "name", "n"); !ok || id != "first" {
		t.Errorf("findWhere = %q, %v", id, ok)
	}
	if _, ok := findString(v, "nothing"); ok {
		t.Error("a missing key was found")
	}
	if _, err := decode("{"); err == nil {
		t.Error("invalid JSON must be an error")
	}
	if _, err := jsonField(`{"a":1}`, "b"); err == nil {
		t.Error("a missing field must be an error")
	}
	red := redactedJSON(map[string]any{"clientSecret": "s", "nested": []any{map[string]any{"accessToken": "t", "ok": "fine"}}, "key": "k"})
	if strings.Contains(red, `"s"`) || strings.Contains(red, `"t"`) || !strings.Contains(red, "fine") || !strings.Contains(red, `"k"`) {
		t.Errorf("redacted = %s", red)
	}
	if got := slug("bob", "approvals", "approve", "--app", "a1", "req1"); got != "bob-approvals-approve" {
		t.Errorf("slug = %q", got)
	}
}

// The project mutation changes the secure page's title and fails loudly when
// the fixture no longer has it.
func TestMutateProject(t *testing.T) {
	t.Parallel()
	env, _, _ := testEnv(t)
	if err := env.mutateProject("(x)"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(env.Project, "translations", "en.json"))
	var doc struct {
		Messages map[string]string `json:"messages"`
	}
	if err := json.Unmarshal(data, &doc); err != nil || doc.Messages["01a14bd7-5c1d-71fe-8376-851b7f675e47"] != "Transfer (x)" {
		t.Errorf("document = %s, %v", data, err)
	}
	if err := os.WriteFile(filepath.Join(env.Project, "translations", "en.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := env.mutateProject("(y)"); err == nil {
		t.Error("a project without the title must be an error")
	}
	env.Project = t.TempDir()
	if err := env.mutateProject("(z)"); err == nil {
		t.Error("a project without translations must be an error")
	}
}

// Polling stops when the check is done, fails on its error, and gives up at
// the timeout on the injected clock; sleeping obeys the context.
func TestPoll(t *testing.T) {
	t.Parallel()
	env, _, _ := testEnv(t)
	n := 0
	if err := env.poll(t.Context(), time.Minute, time.Second, "three", func() (bool, error) { n++; return n == 3, nil }); err != nil || n != 3 {
		t.Errorf("n = %d, err = %v", n, err)
	}
	if err := env.poll(t.Context(), 5*time.Second, time.Second, "never", func() (bool, error) { return false, nil }); err == nil || !strings.Contains(err.Error(), "timed out after 5s waiting for never") {
		t.Errorf("err = %v", err)
	}
	boom := errors.New("boom")
	if err := env.poll(t.Context(), time.Minute, time.Second, "x", func() (bool, error) { return false, boom }); !errors.Is(err, boom) {
		t.Errorf("err = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := SleepContext(ctx, time.Hour); err == nil {
		t.Error("a cancelled context must end the sleep")
	}
	if err := SleepContext(t.Context(), time.Millisecond); err != nil {
		t.Error(err)
	}
}

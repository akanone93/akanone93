// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/nightCode42/plux3/backend/ext/governance"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

// fakeControls answers the break-glass, freeze, compliance and control
// services from memory.
type fakeControls struct {
	pluxv1connect.UnimplementedBreakGlassServiceHandler
	pluxv1connect.UnimplementedFreezeServiceHandler
	pluxv1connect.UnimplementedComplianceServiceHandler
	pluxv1connect.UnimplementedControlServiceHandler
	calls *[]string
	// signed is whether GetComplianceReport answers with content.
	signed  bool
	content []byte
}

func (f fakeControls) record(s string) { *f.calls = append(*f.calls, s) }

func (f fakeControls) ListBreakGlass(_ context.Context, r *connect.Request[pluxv1.ListBreakGlassRequest]) (*connect.Response[pluxv1.ListBreakGlassResponse], error) {
	f.record("list breakglass status=" + r.Msg.GetStatus())
	at := timestamppb.New(time.Date(2026, 10, 9, 9, 30, 0, 0, time.UTC))
	return connect.NewResponse(&pluxv1.ListBreakGlassResponse{Invocations: []*pluxv1.BreakGlassInvocation{
		{Id: "b1", Action: "killSwitch", Environment: "production", InvokedBy: "ada", InvokedAt: at, ReviewDueAt: at, ReviewStatus: "open", Overdue: true},
		{Id: "b2", Action: "rollback", Environment: "production", InvokedBy: "bob", InvokedAt: at, ReviewDueAt: at, ReviewStatus: "reviewed", ReviewedBy: "cy"},
	}}), nil
}

func (f fakeControls) ReviewBreakGlass(_ context.Context, r *connect.Request[pluxv1.ReviewBreakGlassRequest]) (*connect.Response[pluxv1.ReviewBreakGlassResponse], error) {
	f.record("review " + r.Msg.GetId() + " " + r.Msg.GetComment())
	at := timestamppb.New(time.Date(2026, 10, 9, 9, 30, 0, 0, time.UTC))
	return connect.NewResponse(&pluxv1.ReviewBreakGlassResponse{Invocation: &pluxv1.BreakGlassInvocation{Id: r.Msg.GetId(), InvokedAt: at, ReviewDueAt: at}}), nil
}

func (f fakeControls) ListFreezes(context.Context, *connect.Request[pluxv1.ListFreezesRequest]) (*connect.Response[pluxv1.ListFreezesResponse], error) {
	return connect.NewResponse(&pluxv1.ListFreezesResponse{Freezes: []*pluxv1.ChangeFreeze{
		{Id: "f1", Name: "close", TimeZone: "Europe/Berlin", Start: "2026-12-30T18:00", DurationMinutes: 2160, Recurrence: "FREQ=WEEKLY;BYDAY=FR"},
	}}), nil
}

func (f fakeControls) PutFreeze(_ context.Context, r *connect.Request[pluxv1.PutFreezeRequest]) (*connect.Response[pluxv1.PutFreezeResponse], error) {
	w := r.Msg.GetFreeze()
	f.record("put freeze " + w.GetName() + " " + w.GetTimeZone() + " " + w.GetStart() + " " + w.GetRecurrence() + " " + w.GetEnvironmentKey())
	f.record("minutes " + string(rune('0'+w.GetDurationMinutes()/60)))
	return connect.NewResponse(&pluxv1.PutFreezeResponse{Freeze: &pluxv1.ChangeFreeze{Id: "f2", Name: w.GetName()}}), nil
}

func (f fakeControls) DeleteFreeze(_ context.Context, r *connect.Request[pluxv1.DeleteFreezeRequest]) (*connect.Response[pluxv1.DeleteFreezeResponse], error) {
	f.record("delete freeze " + r.Msg.GetId())
	return connect.NewResponse(&pluxv1.DeleteFreezeResponse{}), nil
}

func (f fakeControls) CreateComplianceReport(_ context.Context, r *connect.Request[pluxv1.CreateComplianceReportRequest]) (*connect.Response[pluxv1.CreateComplianceReportResponse], error) {
	f.record("create report " + r.Msg.GetFrom().AsTime().Format(time.RFC3339) + " " + r.Msg.GetTo().AsTime().Format(time.RFC3339))
	return connect.NewResponse(&pluxv1.CreateComplianceReportResponse{Report: &pluxv1.ComplianceReportInfo{Id: "r1"}}), nil
}

func (f fakeControls) GetComplianceReport(_ context.Context, r *connect.Request[pluxv1.GetComplianceReportRequest]) (*connect.Response[pluxv1.GetComplianceReportResponse], error) {
	f.record("get report " + r.Msg.GetId() + " " + r.Msg.GetFormat())
	info := &pluxv1.ComplianceReportInfo{Id: r.Msg.GetId(), Signed: f.signed, KeyId: "k1", Changes: 3}
	if !f.signed {
		return connect.NewResponse(&pluxv1.GetComplianceReportResponse{Report: info}), nil
	}
	return connect.NewResponse(&pluxv1.GetComplianceReportResponse{Report: info, Content: f.content}), nil
}

func (f fakeControls) SetKillSwitch(_ context.Context, r *connect.Request[pluxv1.SetKillSwitchRequest]) (*connect.Response[pluxv1.ControlDocumentResponse], error) {
	f.record("kill " + r.Msg.GetKillSwitch().GetTarget() + " bg=" + yesNo(r.Msg.GetBreakGlass()) + " " + r.Msg.GetJustification())
	return connect.NewResponse(&pluxv1.ControlDocumentResponse{Document: &pluxv1.ControlDocument{}}), nil
}

func (f fakeControls) ClearForcedRollback(_ context.Context, r *connect.Request[pluxv1.ClearForcedRollbackRequest]) (*connect.Response[pluxv1.ControlDocumentResponse], error) {
	f.record("unforce bg=" + yesNo(r.Msg.GetBreakGlass()) + " " + r.Msg.GetJustification())
	return connect.NewResponse(&pluxv1.ControlDocumentResponse{Document: &pluxv1.ControlDocument{}}), nil
}

// yesNo spells a bool for the call log.
func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// signedReport returns the JSON of a DSSE envelope over a small report and
// the public key that verifies it.
func signedReport(t *testing.T) ([]byte, ed25519.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	statement, err := json.Marshal(governance.Report{
		Version: governance.ReportVersion, OrganizationID: "org-1",
		From: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		Changes: []governance.Change{{ID: "c1"}, {ID: "c2"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(governance.NewEnvelope(statement, "k1", ed25519.Sign(priv, governance.PAE(governance.PayloadType, statement))))
	if err != nil {
		t.Fatal(err)
	}
	return raw, pub
}

// governanceCLI runs a command against a fake server.
func governanceCLI(t *testing.T, fake fakeControls) func(cmd func(env, []string) int, args ...string) (int, string, string) {
	t.Helper()
	t.Setenv(tokenEnv, "plux_pat_gov")
	t.Setenv(serverEnv, "")
	t.Setenv(orgEnv, "")
	mux := http.NewServeMux()
	mux.Handle(pluxv1connect.NewBreakGlassServiceHandler(fake))
	mux.Handle(pluxv1connect.NewFreezeServiceHandler(fake))
	mux.Handle(pluxv1connect.NewComplianceServiceHandler(fake))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return func(cmd func(env, []string) int, args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		e := newEnv(&out, &errOut)
		e.creds = credentials{dir: t.TempDir()}
		args = append([]string{args[0], "--server", srv.URL, "--org", "org-1"}, args[1:]...)
		code := cmd(e, args)
		return code, out.String(), errOut.String()
	}
}

// Verifies: GOV-024.
// plux breakglass lists the emergency actions, marking an overdue review with
// PLX-8006, and completes a review.
func TestBreakGlassCommands_GOV_024(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	var calls []string
	run := governanceCLI(t, fakeControls{calls: &calls})
	code, out, stderr := run(env.breakglass, "list", "--status", "open")
	if code != exitOK || !strings.Contains(out, "b1\t2026-10-09T09:30:00Z\tkillSwitch\tproduction\tOVERDUE (PLX-8006)") || !strings.Contains(out, "reviewed by cy") || calls[0] != "list breakglass status=open" {
		t.Errorf("breakglass list: %d %q %s %v", code, out, stderr, calls)
	}
	if code, _, _ := run(env.breakglass, "review", "--comment", "justified", "b1"); code != exitOK || calls[len(calls)-1] != "review b1 justified" {
		t.Errorf("breakglass review: %d %v", code, calls)
	}
	if code, _, _ := run(env.breakglass, "review"); code != exitUsage {
		t.Errorf("review without an identifier: %d", code)
	}
}

// Verifies: GOV-025.
// plux freeze lists, writes and removes windows, passing every field of the
// window to the server.
func TestFreezeCommands_GOV_025(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	var calls []string
	run := governanceCLI(t, fakeControls{calls: &calls})
	if code, out, _ := run(env.freeze, "list"); code != exitOK || !strings.Contains(out, "f1\tclose\t2026-12-30T18:00 Europe/Berlin\t2160m\tFREQ=WEEKLY;BYDAY=FR\tall production") {
		t.Errorf("freeze list: %d %q", code, out)
	}
	code, _, stderr := run(env.freeze, "put", "--name", "close", "--tz", "Europe/Berlin", "--start", "2026-12-30T18:00", "--duration", "180m", "--recurrence", "FREQ=DAILY", "--env-key", "production")
	if code != exitOK || calls[len(calls)-2] != "put freeze close Europe/Berlin 2026-12-30T18:00 FREQ=DAILY production" || calls[len(calls)-1] != "minutes 3" {
		t.Errorf("freeze put: %d %s %v", code, stderr, calls)
	}
	if code, _, _ := run(env.freeze, "put", "--name", "x", "--duration", "90s"); code == exitOK {
		t.Error("a duration of 90 seconds was accepted")
	}
	if code, _, _ := run(env.freeze, "delete", "f1"); code != exitOK || calls[len(calls)-1] != "delete freeze f1" {
		t.Errorf("freeze delete: %d %v", code, calls)
	}
}

// Verifies: GOV-026.
// plux compliance report creates the report for the period, waits for the
// signature and writes the envelope or the PDF to a file; a report that is
// not signed in time is an error that names it, and a PDF needs a file.
func TestComplianceReportCommand_GOV_026(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	var calls []string
	envelope, _ := signedReport(t)
	run := governanceCLI(t, fakeControls{calls: &calls, signed: true, content: envelope})
	out := filepath.Join(t.TempDir(), "report.json")
	code, text, stderr := run(env.compliance, "report", "--from", "2026-10-01", "--to", "2026-11-01T00:00:00Z", "--out", out)
	if code != exitOK || !strings.Contains(text, "Wrote report r1 (3 production changes, signed with key k1)") {
		t.Fatalf("compliance report: %d %q %s", code, text, stderr)
	}
	if calls[0] != "create report 2026-10-01T00:00:00Z 2026-11-01T00:00:00Z" || calls[1] != "get report r1 json" {
		t.Errorf("calls = %v", calls)
	}
	if got, err := os.ReadFile(out); err != nil || !bytes.Equal(got, envelope) { //nolint:gosec // a test file
		t.Errorf("the file = %q, %v", got, err)
	}
	if code, stdout, _ := run(env.compliance, "report", "--from", "2026-10-01", "--to", "2026-11-01"); code != exitOK || stdout != string(envelope) {
		t.Errorf("JSON to standard output: %d %q", code, stdout)
	}
	pdf := filepath.Join(t.TempDir(), "report.pdf")
	if code, _, _ := run(env.compliance, "report", "--from", "2026-10-01", "--to", "2026-11-01", "--format", "pdf", "--out", pdf); code != exitOK || calls[len(calls)-1] != "get report r1 pdf" {
		t.Errorf("pdf: %d %v", code, calls)
	}
	for name, args := range map[string][]string{
		"a PDF without a file": {"--from", "2026-10-01", "--to", "2026-11-01", "--format", "pdf"},
		"another format":       {"--from", "2026-10-01", "--to", "2026-11-01", "--format", "docx"},
		"no period":            {},
		"a bad start":          {"--from", "last month", "--to", "2026-11-01"},
	} {
		if code, _, _ := run(env.compliance, append([]string{"report"}, args...)...); code != exitUsage {
			t.Errorf("%s: %d, want a usage error", name, code)
		}
	}

	slow := governanceCLI(t, fakeControls{calls: &calls, signed: false})
	code, _, stderr = slow(env.compliance, "report", "--from", "2026-10-01", "--to", "2026-11-01", "--wait", "0s", "--out", out)
	if code == exitOK || !strings.Contains(stderr, "r1") || !strings.Contains(stderr, "not signed yet") {
		t.Errorf("an unsigned report: %d %q", code, stderr)
	}
}

// Verifies: GOV-026.
// plux compliance verify checks an envelope offline against the key it is
// given: it verifies the genuine report, and refuses a changed byte, another
// key, a missing key and a file that is not an envelope.
func TestComplianceVerifyCommand_GOV_026(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	envelope, pub := signedReport(t)
	dir := t.TempDir()
	write := func(name string, data []byte) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	verify := func(args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		code := newEnv(&out, &errOut).compliance(append([]string{"verify"}, args...))
		return code, out.String(), errOut.String()
	}
	key := "k1=" + base64.StdEncoding.EncodeToString(pub)

	code, out, stderr := verify("--key", key, write("report.json", envelope))
	if code != exitOK || !strings.Contains(out, "Verified: report of organisation org-1, 2026-10-01T00:00:00Z to 2026-11-01T00:00:00Z, 2 production changes.") {
		t.Fatalf("verify: %d %q %s", code, out, stderr)
	}
	if code, out, _ := verify("--json", "--key", key, write("again.json", envelope)); code != exitOK || !strings.Contains(out, `"verified": true`) || !strings.Contains(out, `"changes": 2`) {
		t.Errorf("verify --json: %d %q", code, out)
	}

	tampered := bytes.Replace(envelope, []byte(`"payload":"`), []byte(`"payload":"AAAA`), 1)
	if code, _, stderr := verify("--key", key, write("tampered.json", tampered)); code == exitOK || !strings.Contains(stderr, "does not verify") {
		t.Errorf("a changed payload: %d %q", code, stderr)
	}
	other, _, _ := ed25519.GenerateKey(nil)
	if code, _, stderr := verify("--key", "k1="+base64.StdEncoding.EncodeToString(other), write("report.json", envelope)); code == exitOK || !strings.Contains(stderr, "does not verify") {
		t.Errorf("another key: %d %q", code, stderr)
	}
	if code, _, _ := verify(write("report.json", envelope)); code != exitUsage {
		t.Errorf("no key: %d, want a usage error", code)
	}
	for _, bad := range []string{"k1", "=abc", "k1=!!!", "k1=" + base64.StdEncoding.EncodeToString([]byte("short"))} {
		if code, _, _ := verify("--key", bad, write("report.json", envelope)); code != exitUsage {
			t.Errorf("--key %q: %d, want a usage error", bad, code)
		}
	}
	if code, _, stderr := verify("--key", key, write("junk.json", []byte("not json"))); code == exitOK || !strings.Contains(stderr, "not a compliance report") {
		t.Errorf("a file that is not an envelope: %d %q", code, stderr)
	}
	if code, _, _ := verify("--key", key, filepath.Join(dir, "missing.json")); code == exitOK {
		t.Error("a missing file verified")
	}
}

// Verifies: GOV-024.
// The break-glass flags reach the server on kill, unforce and rollback calls;
// a justification without --break-glass, and --break-glass without one, are
// refused before anything is sent, and a promotion cannot be a break-glass
// action.
func TestBreakGlassFlags_GOV_024(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	var calls []string
	fake := fakeControls{calls: &calls}
	mux := http.NewServeMux()
	mux.Handle(pluxv1connect.NewControlServiceHandler(fake))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client := pluxv1connect.NewControlServiceClient(http.DefaultClient, srv.URL)
	target := controlTarget{app: "a1", env: "e1", channel: "production"}
	ctx := t.Context()

	if _, err := controlSwitch(ctx, client, target, breakGlassFlags{on: true, justification: "loans crashes"}, true, []string{"plugin", "loans"}); err != nil {
		t.Fatal(err)
	}
	if _, err := controlSwitch(ctx, client, target, breakGlassFlags{}, true, []string{"plugin", "loans"}); err != nil {
		t.Fatal(err)
	}
	if _, err := controlRelease(ctx, client, target, controlFlags{clear: true, bg: breakGlassFlags{on: true, justification: "safe again"}}, false); err != nil {
		t.Fatal(err)
	}
	want := []string{"kill loans bg=yes loans crashes", "kill loans bg=no ", "unforce bg=yes safe again"}
	if strings.Join(calls, "|") != strings.Join(want, "|") {
		t.Errorf("calls = %q, want %q", calls, want)
	}

	for name, bg := range map[string]breakGlassFlags{
		"no justification":      {on: true},
		"a blank justification": {on: true, justification: "  "},
		"a justification alone": {justification: "because"},
	} {
		if err := bg.check(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if err := (breakGlassFlags{on: true, justification: "x"}).check(); err != nil {
		t.Errorf("a complete set: %v", err)
	}
	if err := checkBreakGlass("promote", breakGlassFlags{on: true, justification: "x"}); err == nil {
		t.Error("a promotion as break-glass was accepted")
	}
	if err := checkBreakGlass("rollback", breakGlassFlags{on: true, justification: "x"}); err != nil {
		t.Errorf("a rollback as break-glass: %v", err)
	}
	if err := checkBreakGlass("promote", breakGlassFlags{}); err != nil {
		t.Errorf("a plain promotion: %v", err)
	}
}

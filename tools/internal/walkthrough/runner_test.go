// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package walkthrough

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fixedClock returns a clock that ticks one second per reading.
func fixedClock() func() time.Time {
	t := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	return func() time.Time {
		t = t.Add(time.Second)
		return t
	}
}

// newTestRunner returns a runner over steps with a store in a temp directory.
func newTestRunner(t *testing.T, steps ...Step) (*Runner, *bytes.Buffer) {
	t.Helper()
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	return &Runner{Steps: steps, Store: store, Out: &out, Now: fixedClock()}, &out
}

// okStep is a step that writes one evidence file.
func okStep(id string) Step {
	return Step{ID: id, Title: "title " + id, Verifies: []string{"GOV-006"}, Run: func(_ context.Context, r *Run) error {
		r.Logf("working on %s", id)
		return r.Evidence("proof.txt", []byte("proof of "+id))
	}}
}

// Steps run in order; each step's evidence lands in the statement under its
// own ID, and the transcript's hash is the hash of the stored transcript.
func TestRunnerRunsStepsInOrderAndRecordsEvidence(t *testing.T) {
	t.Parallel()
	rn, out := newTestRunner(t, okStep("01-a"), okStep("02-b"))
	st, err := rn.Run(t.Context())
	if err != nil || !st.Passed {
		t.Fatalf("Run = %+v, %v", st, err)
	}
	if len(st.Steps) != 3 || st.Steps[0].ID != "01-a" || st.Steps[1].ID != "02-b" || st.Steps[2].ID != "run" {
		t.Fatalf("steps = %+v", st.Steps)
	}
	for _, s := range st.Steps[:2] {
		if s.Status != StatusPassed || len(s.Evidence) != 1 || s.Evidence[0].Name != "proof.txt" {
			t.Errorf("step %s = %+v", s.ID, s)
		}
	}
	transcript, err := os.ReadFile(filepath.Join(rn.Store.Dir(), "run", "transcript.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if st.TranscriptSHA256 != st.Steps[2].Evidence[0].SHA256 || string(transcript) != out.String() {
		t.Errorf("the transcript's hash %s does not match what was streamed", st.TranscriptSHA256)
	}
	if !strings.Contains(out.String(), "working on 01-a") || strings.Index(out.String(), "01-a") > strings.Index(out.String(), "02-b") {
		t.Errorf("transcript = %s", out.String())
	}
	if err := CheckFiles(st, rn.Store.Dir()); err != nil {
		t.Errorf("CheckFiles: %v", err)
	}
}

// The first failed step stops the run: the later steps are recorded as
// skipped, the failure is in the statement, and the statement says not passed.
func TestRunnerStopsAtTheFirstFailure(t *testing.T) {
	t.Parallel()
	boom := errors.New("the server said no")
	failing := Step{ID: "02-b", Title: "b", Run: func(context.Context, *Run) error { return boom }}
	var ran bool
	last := Step{ID: "03-c", Title: "c", Run: func(context.Context, *Run) error { ran = true; return nil }}
	rn, _ := newTestRunner(t, okStep("01-a"), failing, last)
	st, err := rn.Run(t.Context())
	if err == nil || !strings.Contains(err.Error(), "02-b") || !strings.Contains(err.Error(), boom.Error()) {
		t.Fatalf("err = %v", err)
	}
	if st.Passed || ran {
		t.Errorf("passed=%v, last step ran=%v", st.Passed, ran)
	}
	want := []Status{StatusPassed, StatusFailed, StatusSkipped}
	for i, s := range st.Steps[:3] {
		if s.Status != want[i] {
			t.Errorf("step %s status = %s, want %s", s.ID, s.Status, want[i])
		}
	}
	if st.Steps[1].Error != boom.Error() {
		t.Errorf("recorded error = %q", st.Steps[1].Error)
	}
}

// An optional step is skipped unless the runner enables it, and a skipped
// optional step does not fail the walkthrough.
func TestRunnerRunsOptionalStepsOnlyWhenEnabled(t *testing.T) {
	t.Parallel()
	for _, enabled := range []bool{false, true} {
		optional := okStep("07b-viewer")
		optional.Optional = true
		rn, _ := newTestRunner(t, okStep("01-a"), optional)
		rn.Enabled = map[string]bool{"07b-viewer": enabled}
		st, err := rn.Run(t.Context())
		if err != nil || !st.Passed {
			t.Fatalf("enabled=%v: %+v, %v", enabled, st, err)
		}
		want := StatusSkipped
		if enabled {
			want = StatusPassed
		}
		if st.Steps[1].Status != want {
			t.Errorf("enabled=%v: status = %s, want %s", enabled, st.Steps[1].Status, want)
		}
	}
}

// A step left out by Skip runs only its setup, later steps still run, and
// the statement lists it; a failed setup fails the walkthrough.
func TestRunnerSkipsStepsToTheirSetup(t *testing.T) {
	t.Parallel()
	var ran, setup bool
	slow := Step{
		ID: "02-b", Title: "b",
		Run:   func(context.Context, *Run) error { ran = true; return nil },
		Setup: func(context.Context, *Run) error { setup = true; return nil },
	}
	rn, out := newTestRunner(t, okStep("01-a"), slow, okStep("03-c"))
	rn.Skip = map[string]bool{"02-b": true}
	st, err := rn.Run(t.Context())
	if err != nil || !st.Passed || ran || !setup {
		t.Fatalf("passed=%v ran=%v setup=%v err=%v", st.Passed, ran, setup, err)
	}
	want := []Status{StatusPassed, StatusSkipped, StatusPassed}
	for i, s := range st.Steps[:3] {
		if s.Status != want[i] {
			t.Errorf("step %s status = %s, want %s", s.ID, s.Status, want[i])
		}
	}
	if len(st.Skipped) != 1 || st.Skipped[0] != "02-b" || !strings.Contains(out.String(), "skipped by -skip") {
		t.Errorf("skipped = %v, transcript:\n%s", st.Skipped, out)
	}

	broken := slow
	broken.Setup = func(context.Context, *Run) error { return errors.New("no baseline") }
	rn, _ = newTestRunner(t, broken, okStep("03-c"))
	rn.Skip = map[string]bool{"02-b": true}
	st, err = rn.Run(t.Context())
	if err == nil || st.Passed || st.Steps[0].Status != StatusFailed || st.Steps[1].Status != StatusSkipped {
		t.Errorf("a failed setup: %+v, %v", st.Steps, err)
	}
}

// A run seals into a bundle that verifies, including when it failed: the
// failure is signed evidence too.
func TestRunnerStatementSeals(t *testing.T) {
	t.Parallel()
	failing := Step{ID: "01-a", Title: "a", Run: func(context.Context, *Run) error { return errors.New("no") }}
	rn, _ := newTestRunner(t, failing)
	st, _ := rn.Run(t.Context())
	pub, priv, _ := GenerateKey(rand.Reader)
	b, err := Seal(st, priv)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Verify(b, pub)
	if err != nil || got.Passed || got.Steps[0].Status != StatusFailed {
		t.Errorf("Verify = %+v, %v", got, err)
	}
}

// The dry-run plan lists every step with its requirement IDs and actions, and
// marks the optional ones.
func TestRunnerPlan(t *testing.T) {
	t.Parallel()
	optional := Step{ID: "07b", Title: "viewer", Verifies: []string{"SEC-142"}, Optional: true, Plan: []string{"open it"}}
	rn := &Runner{Steps: []Step{{ID: "01-a", Title: "first", Verifies: []string{"GOV-006", "GOV-007"}, Plan: []string{"do this", "do that"}}, optional}}
	var b bytes.Buffer
	rn.Plan(&b)
	for _, want := range []string{"01-a  first", "verifies: GOV-006, GOV-007", "- do this", "- do that", "07b  viewer (optional, enabled by -with 07b)", "- open it"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("plan lacks %q:\n%s", want, b.String())
		}
	}
}

// The real steps have unique IDs, name requirement IDs and carry a plan; the
// optional emulator step is the only optional one.
func TestStepsAreWellFormed(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for _, s := range Steps(&Env{}) {
		if seen[s.ID] || !plainName(s.ID) || s.Title == "" || len(s.Verifies) == 0 || len(s.Plan) == 0 || s.Run == nil {
			t.Errorf("step %+v is malformed or repeated", s.ID)
		}
		seen[s.ID] = true
		if s.Optional != (s.ID == "07b-viewer-on-emulator") {
			t.Errorf("step %s optional = %v", s.ID, s.Optional)
		}
	}
	if len(seen) != 10 {
		t.Errorf("%d steps, want the nine and the optional emulator step", len(seen))
	}
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package walkthrough

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// transcriptStep is the pseudo step the transcript is stored under.
const transcriptStep = "run"

// Step is one stage of the walkthrough.
type Step struct {
	// ID is a short plain name, such as "01-sso-and-roles"; it names the
	// evidence directory.
	ID    string
	Title string
	// Verifies lists the requirement IDs the step exercises.
	Verifies []string
	// Optional steps run only when the runner enables them by ID.
	Optional bool
	// Plan describes what the step does, one line per action, for the
	// dry run and for the auditors' document.
	Plan []string
	// Run performs the step; a non-nil error fails it.
	Run func(ctx context.Context, r *Run) error
	// Setup, when set, does only what later steps need of this one; it runs
	// in place of Run when the runner skips the step.
	Setup func(ctx context.Context, r *Run) error
}

// Run is what a step sees of the runner: the transcript and the evidence of
// its own step.
type Run struct {
	step *Step
	rn   *Runner
}

// Logf writes one line to the transcript.
func (r *Run) Logf(format string, args ...any) { r.rn.logf("   "+format, args...) }

// Evidence stores data as an evidence file of the step.
func (r *Run) Evidence(name string, data []byte) error {
	item, err := r.rn.Store.Add(r.step.ID, name, data)
	if err != nil {
		return err
	}
	r.rn.logf("   evidence %s/%s sha256 %s", item.Step, item.Name, item.SHA256[:12])
	return nil
}

// Runner runs steps in order. It stops at the first failed step, marks the
// rest skipped, and returns the statement of what happened. A Runner is not
// reused.
type Runner struct {
	Steps []Step
	Store *Store
	// Out receives the transcript as it grows.
	Out io.Writer
	// Now supplies the time of the transcript and of the statement.
	Now func() time.Time
	// Enabled names the optional steps to run.
	Enabled map[string]bool
	// Skip names the steps to leave out of a quicker, partial run; the
	// statement lists them.
	Skip map[string]bool

	mu         sync.Mutex
	transcript bytes.Buffer
}

// logf appends one line, prefixed with the time, to the transcript and to Out.
func (rn *Runner) logf(format string, args ...any) {
	line := fmt.Sprintf("%s %s\n", rn.Now().UTC().Format(time.RFC3339), fmt.Sprintf(format, args...))
	rn.mu.Lock()
	defer rn.mu.Unlock()
	rn.transcript.WriteString(line)
	if rn.Out != nil {
		_, _ = io.WriteString(rn.Out, line)
	}
}

// Plan prints what a run would do, without doing it.
func (rn *Runner) Plan(w io.Writer) {
	for i := range rn.Steps {
		s := &rn.Steps[i]
		mark := ""
		if s.Optional {
			mark = " (optional, enabled by -with " + s.ID + ")"
		}
		_, _ = fmt.Fprintf(w, "%s  %s%s\n    verifies: %s\n", s.ID, s.Title, mark, strings.Join(s.Verifies, ", "))
		for _, line := range s.Plan {
			_, _ = fmt.Fprintf(w, "    - %s\n", line)
		}
	}
}

// Run executes the steps. The returned statement is complete even when a step
// failed, so the failure is itself signed evidence; the error is the first
// step's failure.
func (rn *Runner) Run(ctx context.Context) (Statement, error) {
	st := Statement{StartedAt: rn.Now().UTC().Format(time.RFC3339)}
	var first error
	for i := range rn.Steps {
		rec := StepRecord{ID: rn.Steps[i].ID, Title: rn.Steps[i].Title, Verifies: rn.Steps[i].Verifies}
		switch {
		case first != nil || (rn.Steps[i].Optional && !rn.Enabled[rn.Steps[i].ID]):
			rec.Status = StatusSkipped
			rn.logf("== %s %s: skipped", rec.ID, rec.Title)
		case rn.Skip[rn.Steps[i].ID]:
			st.Skipped = append(st.Skipped, rec.ID)
			rec.Status, rec.Error = rn.skipStep(ctx, &rn.Steps[i])
			if rec.Status == StatusFailed {
				first = fmt.Errorf("walkthrough: step %s: %s", rec.ID, rec.Error)
			}
		default:
			rec.Status, rec.Error = rn.runStep(ctx, &rn.Steps[i])
			if rec.Status == StatusFailed {
				first = fmt.Errorf("walkthrough: step %s: %s", rec.ID, rec.Error)
			}
		}
		st.Steps = append(st.Steps, rec)
	}
	return rn.finish(st, first)
}

// runStep runs one step and reports how it ended.
func (rn *Runner) runStep(ctx context.Context, s *Step) (Status, string) {
	rn.logf("== %s %s", s.ID, s.Title)
	if err := s.Run(ctx, &Run{step: s, rn: rn}); err != nil {
		rn.logf("   FAILED: %v", err)
		return StatusFailed, err.Error()
	}
	rn.logf("   passed")
	return StatusPassed, ""
}

// skipStep leaves a step out, running only its setup when it has one.
func (rn *Runner) skipStep(ctx context.Context, s *Step) (Status, string) {
	rn.logf("== %s %s: skipped by -skip", s.ID, s.Title)
	if s.Setup == nil {
		return StatusSkipped, ""
	}
	if err := s.Setup(ctx, &Run{step: s, rn: rn}); err != nil {
		rn.logf("   FAILED in setup: %v", err)
		return StatusFailed, "setup: " + err.Error()
	}
	rn.logf("   setup done")
	return StatusSkipped, ""
}

// finish stores the transcript and fills the statement's evidence lists and
// hash.
func (rn *Runner) finish(st Statement, first error) (Statement, error) {
	rn.logf("== finished: %s", map[bool]string{true: "all steps passed", false: "the walkthrough failed"}[first == nil])
	rn.mu.Lock()
	data := append([]byte(nil), rn.transcript.Bytes()...)
	rn.mu.Unlock()
	item, err := rn.Store.Add(transcriptStep, "transcript.txt", data)
	if err != nil {
		return st, err
	}
	st.TranscriptSHA256 = item.SHA256
	st.FinishedAt = rn.Now().UTC().Format(time.RFC3339)
	st.Passed = first == nil
	byStep := map[string][]Item{}
	for _, it := range rn.Store.Items() {
		byStep[it.Step] = append(byStep[it.Step], it)
	}
	for i := range st.Steps {
		st.Steps[i].Evidence = byStep[st.Steps[i].ID]
		if st.Steps[i].Evidence == nil {
			st.Steps[i].Evidence = []Item{}
		}
	}
	st.Steps = append(st.Steps, StepRecord{
		ID: transcriptStep, Title: "transcript", Verifies: []string{}, Status: StatusPassed, Evidence: byStep[transcriptStep],
	})
	return st, first
}

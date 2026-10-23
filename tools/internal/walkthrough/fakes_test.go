// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package walkthrough

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeClock is a clock that moves only when someone sleeps.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock() *fakeClock { return &fakeClock{t: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Sleep(ctx context.Context, d time.Duration) error {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
	return ctx.Err()
}

// rule answers the commands whose actor and arguments contain every one of
// its fragments.
type rule struct {
	contains []string
	answer   func(Command) Result
}

// fakePlux is a Commander that answers from rules, in order, and records
// every command. A command no rule matches fails loudly.
type fakePlux struct {
	mu    sync.Mutex
	rules []rule
	calls []Command
}

// Run implements Commander.
func (f *fakePlux) Run(_ context.Context, c Command) (Result, error) {
	f.mu.Lock()
	f.calls = append(f.calls, c)
	f.mu.Unlock()
	line := c.Actor + "|" + strings.Join(c.Args, " ")
	for _, r := range f.rules {
		if matchesAll(line, r.contains) {
			return r.answer(c), nil
		}
	}
	return Result{ExitCode: 1, Stderr: "unscripted command: " + line}, nil
}

// matchesAll reports whether s contains every fragment.
func matchesAll(s string, fragments []string) bool {
	for _, f := range fragments {
		if !strings.Contains(s, f) {
			return false
		}
	}
	return true
}

// called reports whether a command containing every fragment was run.
func (f *fakePlux) called(fragments ...string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if matchesAll(c.Actor+"|"+strings.Join(c.Args, " "), fragments) {
			return true
		}
	}
	return false
}

// out answers with a successful command that printed s.
func out(s string) func(Command) Result { return func(Command) Result { return Result{Stdout: s} } }

// refuse answers with a failed command that printed s on standard error.
func refuse(s string) func(Command) Result {
	return func(Command) Result { return Result{ExitCode: 1, Stderr: s} }
}

// writeOut answers a command with --out by writing content to that file.
func writeOut(content string) func(Command) Result {
	return func(c Command) Result {
		for i, a := range c.Args {
			if a == "--out" && i+1 < len(c.Args) {
				if err := os.WriteFile(c.Args[i+1], []byte(content), 0o600); err != nil {
					return Result{ExitCode: 1, Stderr: err.Error()}
				}
			}
		}
		return Result{}
	}
}

// fakeFleet is a Fleet whose runs end at once with a canned report.
type fakeFleet struct {
	mu      sync.Mutex
	specs   []FleetSpec
	ids     []string
	report  FleetReport
	waitErr error
	// onStart runs when a fleet starts.
	onStart func(FleetSpec)
}

type fakeRun struct {
	ids     []string
	report  FleetReport
	waitErr error
}

// Start implements Fleet.
func (f *fakeFleet) Start(_ context.Context, spec FleetSpec) (FleetRun, error) {
	f.mu.Lock()
	f.specs = append(f.specs, spec)
	f.mu.Unlock()
	if f.onStart != nil {
		f.onStart(spec)
	}
	rep := f.report
	if rep.Raw == nil {
		rep.Raw = []byte(fmt.Sprintf(`{"registered":%d}`, rep.Registered))
	}
	return &fakeRun{ids: f.ids, report: rep, waitErr: f.waitErr}, nil
}

func (r *fakeRun) DeviceIDs(context.Context) ([]string, error) { return r.ids, nil }
func (r *fakeRun) Wait() (FleetReport, error)                  { return r.report, r.waitErr }

// fakeCaller is a Caller that answers by procedure and records requests.
type fakeCaller struct {
	mu       sync.Mutex
	answers  map[string]map[string]any
	requests map[string][]map[string]any
	err      map[string]error
}

// Call implements Caller.
func (f *fakeCaller) Call(_ context.Context, procedure string, req, resp any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.requests == nil {
		f.requests = map[string][]map[string]any{}
	}
	f.requests[procedure] = append(f.requests[procedure], req.(map[string]any))
	if err := f.err[procedure]; err != nil {
		return err
	}
	if out, ok := resp.(*map[string]any); ok {
		for k, v := range f.answers[procedure] {
			(*out)[k] = v
		}
	}
	return nil
}

// fakeSCIM is a SCIMClient that numbers what it creates.
type fakeSCIM struct {
	mu     sync.Mutex
	made   []string
	tokens []string
}

// Post implements SCIMClient.
func (f *fakeSCIM) Post(_ context.Context, token, path string, _ any) (map[string]any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.made = append(f.made, path)
	f.tokens = append(f.tokens, token)
	return map[string]any{"id": fmt.Sprintf("scim-%d", len(f.made))}, nil
}

// credentialsLog records the tokens the steps hand to the commander.
type credentialsLog struct{ tokens map[string]string }

// SetToken implements Credentials.
func (c *credentialsLog) SetToken(actor, token string) {
	if c.tokens == nil {
		c.tokens = map[string]string{}
	}
	c.tokens[actor] = token
}

// transferPageID is the id of the fake project's transfer page, which the
// kill switch of step 4 names.
const transferPageID = "01d0c450-6c00-7000-8000-000000000212"

// testEnv returns an environment with fakes, a fake clock and a project copy
// in a temporary directory.
func testEnv(t *testing.T) (*Env, *fakePlux, *fakeFleet) {
	t.Helper()
	clock := newFakeClock()
	plux, fleet := &fakePlux{}, &fakeFleet{ids: []string{"dev-1", "dev-2"}, report: FleetReport{Registered: 10}}
	dir := t.TempDir()
	project := filepath.Join(dir, "project")
	if err := os.MkdirAll(filepath.Join(project, "translations"), 0o700); err != nil {
		t.Fatal(err)
	}
	title := `{"messages":{"01a14bd7-5c1d-71fe-8376-851b7f675e47": "Transfer"}}`
	if err := os.WriteFile(filepath.Join(project, "translations", "en.json"), []byte(title), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(project, filepath.Dir(killPage)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, killPage), []byte(`{"id": "`+transferPageID+`", "key": "transfer"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return &Env{
		Plux: plux, Fleet: fleet, Server: "https://plux.example", Org: "org1", App: "app1", Project: project, TempDir: dir,
		ProdEnv: "production", FleetEnv: "staging", Devices: 10,
		StageDuration: time.Minute, Propagation: 60 * time.Second, UpgradeWindow: 5 * time.Minute,
		Now: clock.Now, Sleep: clock.Sleep,
	}, plux, fleet
}

// runOneStep runs the step with the given ID of env's steps and returns the
// statement, the error and the evidence directory.
func runOneStep(t *testing.T, env *Env, id string) (Statement, string, error) {
	t.Helper()
	var chosen []Step
	for _, s := range Steps(env) {
		if s.ID == id {
			chosen = append(chosen, s)
		}
	}
	if len(chosen) != 1 {
		t.Fatalf("no step %s", id)
	}
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rn := &Runner{Steps: chosen, Store: store, Now: newFakeClock().Now}
	st, err := rn.Run(t.Context())
	return st, store.Dir(), err
}

// allEvidence returns the contents of every evidence file below dir.
func allEvidence(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, rerr := os.ReadFile(path) //nolint:gosec // a path below the test's directory
		b.WriteString(string(data))
		return rerr
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}

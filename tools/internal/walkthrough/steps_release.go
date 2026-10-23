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
	"strconv"
	"strings"
	"time"
)

// killPage is the document of the page the kill switch of step 4 disables:
// the secure transfer page of the Plux Bank project. A page switch names the
// page by its document ID, so the walkthrough reads it from the project.
const killPage = "plugins/banking/pages/transfer.page.json"

// minRegressionDevices is the smallest second fleet that gives the health
// gate a sample to decide on.
const minRegressionDevices = 20

// gateMinSample is the number of sessions each release needs before the gate
// of step 3 decides.
const gateMinSample = "3"

// startFleet starts a fleet, writes its report as evidence when it ends and
// returns the handle.
func (e *Env) startFleet(ctx context.Context, spec FleetSpec) (FleetRun, error) {
	if spec.Devices == 0 {
		spec.Devices = e.Devices
	}
	if spec.Env == "" {
		spec.Env = e.FleetEnv
	}
	run, err := e.Fleet.Start(ctx, spec)
	if err != nil {
		return nil, fmt.Errorf("start the fleet: %w", err)
	}
	return run, nil
}

// finishFleet waits for a fleet and records its report.
func finishFleet(w *walk, label string, run FleetRun) (FleetReport, error) {
	rep, err := run.Wait()
	if rerr := w.run.Evidence("fleet-"+label+".json", rep.Raw); rerr != nil && err == nil {
		err = rerr
	}
	return rep, err
}

// stepRollout is step 3: the staged rollout and its health gate.
func (e *Env) stepRollout(ctx context.Context, run *Run) error {
	w := e.newWalk(run)
	if _, err := w.plux(ctx, "admin", "publish", "-C", e.Project, "--app", e.App, "--env", e.FleetEnv, "--promote", e.FleetEnv, "--notes", "baseline"); err != nil {
		return err
	}
	base, err := e.newestRelease(ctx, w)
	if err != nil {
		return err
	}
	e.state.baseline = base
	if err := e.mutateProject("(candidate)"); err != nil {
		return err
	}
	cand, err := e.publishRelease(ctx, w, "admin", "rollout candidate")
	if err != nil {
		return err
	}
	healthy, err := e.startFleet(ctx, FleetSpec{Duration: 5*e.StageDuration + 2*time.Minute, MaxErrorRatio: 0.05})
	if err != nil {
		return err
	}
	if _, err := healthy.DeviceIDs(ctx); err != nil {
		return fmt.Errorf("the healthy fleet: %w", err)
	}
	id, err := e.createRollout(ctx, w, cand)
	if err != nil {
		return err
	}
	if err := e.watchRollout(ctx, w, id, cand); err != nil {
		return err
	}
	_, err = finishFleet(w, "healthy", healthy)
	return err
}

// createRollout schedules 1% / 10% / 50% / 100% with a health gate that rolls
// back, and returns the rollout's ID.
func (e *Env) createRollout(ctx context.Context, w *walk, candidate string) (string, error) {
	d := e.StageDuration.String()
	out, err := w.plux(ctx, "admin", "rollout", "create", "--app", e.App, "--env", e.FleetEnv, "--release", candidate,
		"--stages", "1%:"+d+",10%:"+d+",50%:"+d+",100%", "--gate-errors", "100", "--gate-min-sample", gateMinSample,
		"--on-regression", "rollback", "--json")
	if err != nil {
		return "", err
	}
	return jsonField(out, "id")
}

// rolloutState shows a rollout and returns its state and share in basis
// points.
func (e *Env) rolloutState(ctx context.Context, w *walk, id string) (string, int, error) {
	out, err := w.plux(ctx, "admin", "rollout", "show", "--app", e.App, "--json", id)
	if err != nil {
		return "", 0, err
	}
	state, err := jsonField(out, "state")
	if err != nil {
		return "", 0, err
	}
	share, err := jsonField(out, "basisPoints")
	if err != nil {
		return "", 0, err
	}
	bp, err := strconv.Atoi(share)
	if err != nil {
		return "", 0, fmt.Errorf("basisPoints %q: %w", share, err)
	}
	return state, bp, nil
}

// watchRollout follows the rollout through its stages, starts the regression
// once the 10% stage is reached, and waits for the gate to roll it back.
func (e *Env) watchRollout(ctx context.Context, w *walk, id, candidate string) error {
	cand, err := strconv.ParseInt(candidate, 10, 64)
	if err != nil {
		return fmt.Errorf("release sequence %q: %w", candidate, err)
	}
	var regression FleetRun
	stage := 2*e.StageDuration + 3*time.Minute
	err = e.poll(ctx, stage, 3*time.Second, "the rollout to reach 10%", func() (bool, error) {
		state, bp, err := e.rolloutState(ctx, w, id)
		return bp >= 1000 && state == "active", err
	})
	if err != nil {
		return err
	}
	if regression, err = e.startFleet(ctx, FleetSpec{
		Devices: max(e.Devices/2, minRegressionDevices), Duration: 3*e.StageDuration + time.Minute,
		InjectErrors: true, ErrorRelease: cand, MaxErrorRatio: 0.05,
	}); err != nil {
		return err
	}
	err = e.poll(ctx, 4*e.StageDuration+3*time.Minute, 3*time.Second, "the health gate to roll the rollout back", func() (bool, error) {
		state, bp, err := e.rolloutState(ctx, w, id)
		w.run.Logf("rollout %s: %s at %.2f%%", id, state, float64(bp)/100)
		switch state {
		case "rolled_back":
			return true, nil
		case "completed", "ended", "aborted":
			return false, fmt.Errorf("the rollout %s without tripping the gate", state)
		}
		return false, err
	})
	if err != nil {
		return err
	}
	if _, err := finishFleet(w, "regression", regression); err != nil {
		return err
	}
	out, err := w.plux(ctx, "admin", "rollout", "show", "--app", e.App, "--json", id)
	if err != nil {
		return err
	}
	if reverted, err := jsonField(out, "reverted"); err != nil || reverted == "0" {
		return errors.New("the rolled-back rollout names no release that reverted it")
	}
	return nil
}

// newestRelease returns the sequence of the newest release of the app.
func (e *Env) newestRelease(ctx context.Context, w *walk) (string, error) {
	out, err := w.plux(ctx, "admin", "release", "list", "--app", e.App, "--json")
	if err != nil {
		return "", err
	}
	return jsonField(out, "sequence")
}

// freezeRefusal matches the refusal of a change under a freeze.
var freezeRefusal = regexp.MustCompile(`(?i)freeze|PLX-80\d\d`)

// reviewRefusal matches the refusal of a review by the person who acted.
var reviewRefusal = regexp.MustCompile(`(?i)cannot|PLX-80\d\d|not eligible`)

// stepKillSwitch is step 4.
func (e *Env) stepKillSwitch(ctx context.Context, run *Run) error {
	w := e.newWalk(run)
	out, err := w.plux(ctx, "admin", "freeze", "put", "--name", "walkthrough-close", "--tz", "UTC",
		"--start", e.Now().UTC().Add(-time.Minute).Format("2006-01-02T15:04"), "--duration", "60m",
		"--env-key", e.ProdEnv, "--reason", "walkthrough: month-end close", "--json")
	if err != nil {
		return err
	}
	freeze, err := jsonField(out, "id")
	if err != nil {
		return err
	}
	killTarget, err := e.documentID(killPage)
	if err != nil {
		return err
	}
	// A freeze holds back production changes only (GOV-025), and the fleet
	// runs outside production, so the freeze and break-glass are shown on
	// production and the fleet applies the same switch on its environment.
	if err := w.refused(ctx, "admin", freezeRefusal, "control", "kill", "--env", e.ProdEnv, "--app", e.App, "page", killTarget); err != nil {
		return fmt.Errorf("a change during the freeze must be refused: %w", err)
	}
	if _, err := w.plux(ctx, "admin", "control", "kill", "--env", e.ProdEnv, "--app", e.App,
		"--break-glass", "--justification", "walkthrough: the transfer page misbehaves", "page", killTarget); err != nil {
		return err
	}
	if err := e.propagate(ctx, w, "kill-switch", func() error {
		_, err := w.plux(ctx, "admin", "control", "kill", "--env", e.FleetEnv, "--app", e.App, "page", killTarget)
		return err
	}); err != nil {
		return err
	}
	if err := e.reviewBreakGlass(ctx, w); err != nil {
		return err
	}
	_, err = w.plux(ctx, "admin", "freeze", "delete", freeze)
	return err
}

// documentID returns the id of a document of the walkthrough's project, by
// its path in the project.
func (e *Env) documentID(path string) (string, error) {
	data, err := os.ReadFile(filepath.Join(e.Project, path)) //nolint:gosec // the project directory is the walkthrough's own copy
	if err != nil {
		return "", fmt.Errorf("walkthrough.documentID: %w", err)
	}
	var doc struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return "", fmt.Errorf("walkthrough.documentID: %s: %w", path, err)
	}
	if doc.ID == "" {
		return "", fmt.Errorf("walkthrough.documentID: %s names no id", path)
	}
	return doc.ID, nil
}

// reviewBreakGlass shows the open post-hoc review of the emergency action,
// refuses its closing by the person who acted, and closes it with a second
// person.
func (e *Env) reviewBreakGlass(ctx context.Context, w *walk) error {
	out, err := w.plux(ctx, "admin", "breakglass", "list", "--app", e.App, "--status", "open", "--json")
	if err != nil {
		return err
	}
	id, err := jsonField(out, "id")
	if err != nil {
		return fmt.Errorf("no open break-glass review after the emergency action: %w", err)
	}
	if err := w.refused(ctx, "admin", reviewRefusal, "breakglass", "review", "--app", e.App, id); err != nil {
		return fmt.Errorf("whoever acted must not review: %w", err)
	}
	if _, err := w.plux(ctx, "bob", "breakglass", "review", "--app", e.App, "--comment", "reviewed: justified, the page was misbehaving", id); err != nil {
		return err
	}
	closed, err := w.plux(ctx, "admin", "breakglass", "list", "--app", e.App, "--status", "reviewed", "--json")
	if err != nil {
		return err
	}
	if !strings.Contains(closed, id) {
		return fmt.Errorf("review %s is not listed as reviewed", id)
	}
	return nil
}

// controlVersion returns the version of the fleet environment's newest
// control document, or zero when there is none yet.
func (e *Env) controlVersion(ctx context.Context, w *walk) (int64, error) {
	res, err := w.do(ctx, "admin", []string{"control", "get", "--env", e.FleetEnv, "--app", e.App, "--json"})
	if err != nil {
		return 0, err
	}
	if res.ExitCode != 0 {
		return 0, nil
	}
	v, err := jsonField(res.Stdout, "version")
	if err != nil {
		return 0, nil //nolint:nilerr // a channel with no control document yet has no version
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("control version %q: %w", v, err)
	}
	return n, nil
}

// propagate runs a fleet, performs the action while it is online and requires
// that the fleet applied a control document within the propagation bound
// (NFR-024). A device applies the document that already exists when it
// starts, so a channel with one expects two applications per device.
func (e *Env) propagate(ctx context.Context, w *walk, label string, action func() error) error {
	before, err := e.controlVersion(ctx, w)
	if err != nil {
		return err
	}
	run, err := e.startFleet(ctx, FleetSpec{Duration: e.Propagation + 15*time.Second, MaxErrorRatio: 0.01})
	if err != nil {
		return err
	}
	ids, err := run.DeviceIDs(ctx)
	if err != nil {
		return fmt.Errorf("the %s fleet: %w", label, err)
	}
	if err := action(); err != nil {
		return err
	}
	e.state.killedAt, e.state.deviceID = e.Now(), ids[0]
	rep, err := finishFleet(w, label, run)
	if err != nil {
		return err
	}
	after, err := e.controlVersion(ctx, w)
	if err != nil {
		return err
	}
	if after <= before {
		return fmt.Errorf("the control document did not advance (version %d before, %d after)", before, after)
	}
	per := int64(1)
	if before == 0 {
		per = 0
	}
	need := ((per+1)*int64(rep.Registered)*95 + 99) / 100
	if rep.ControlApplied < need {
		return fmt.Errorf("%d control applications from %d devices within %s, want at least %d (95%% of the fleet)", rep.ControlApplied, rep.Registered, e.Propagation, need)
	}
	w.run.Logf("%d control applications from %d devices within %s", rep.ControlApplied, rep.Registered, e.Propagation)
	return nil
}

// stepMandatoryAndRollback is step 5.
func (e *Env) stepMandatoryAndRollback(ctx context.Context, run *Run) error {
	w := e.newWalk(run)
	newest, err := e.newestRelease(ctx, w)
	if err != nil {
		return err
	}
	if e.state.baseline == "" {
		return errors.New("step 3 recorded no baseline release to roll back to")
	}
	if err := e.propagate(ctx, w, "mandatory-and-rollback", func() error {
		if _, err := w.plux(ctx, "admin", "control", "mandatory", "--env", e.FleetEnv, "--app", e.App, "--release", newest); err != nil {
			return err
		}
		_, err := w.plux(ctx, "admin", "control", "rollback", "--env", e.FleetEnv, "--app", e.App, "--release", e.state.baseline)
		return err
	}); err != nil {
		return err
	}
	if _, err := w.plux(ctx, "admin", "control", "get", "--env", e.FleetEnv, "--app", e.App, "--json"); err != nil {
		return err
	}
	for _, sub := range []string{"mandatory", "rollback"} {
		if _, err := w.plux(ctx, "admin", "control", sub, "--env", e.FleetEnv, "--app", e.App, "--clear"); err != nil {
			return err
		}
	}
	return nil
}

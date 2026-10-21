// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package walkthrough

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// syslogMessages is the number of messages the sink must have received
// before the export is judged.
const syslogMessages = 5

// rfc5424 matches the start of an RFC 5424 message: priority and version 1.
var rfc5424 = regexp.MustCompile(`^<\d{1,3}>1 `)

// stepAuditExport is step 6.
func (e *Env) stepAuditExport(ctx context.Context, run *Run) error {
	w := e.newWalk(run)
	if _, err := w.rpc(ctx, e.Admin, auditService+"CreateExportDestination", map[string]any{
		"name": "walkthrough-sink", "kind": "syslog", "address": e.SinkAddress, "allowPrivate": true, "caPem": e.Sink.CAPEM(),
	}); err != nil {
		return err
	}
	if err := e.awaitSyslog(ctx, w); err != nil {
		return err
	}
	return e.complianceReport(ctx, w)
}

// awaitSyslog waits for messages at the sink and requires every one to be an
// RFC 5424 message (SEC-143). The messages are the evidence.
func (e *Env) awaitSyslog(ctx context.Context, w *walk) error {
	err := e.poll(ctx, 3*time.Minute, 2*time.Second, fmt.Sprintf("%d syslog messages at the sink", syslogMessages), func() (bool, error) {
		return len(e.Sink.Messages()) >= syslogMessages, nil
	})
	if err != nil {
		return err
	}
	msgs := e.Sink.Messages()
	for _, m := range msgs {
		if !rfc5424.MatchString(m) {
			return fmt.Errorf("a message at the sink is not RFC 5424: %.80q", m)
		}
	}
	return w.record("syslog-messages", strings.Join(msgs, "\n")+"\n")
}

// complianceReport produces the report for the period of the run and verifies
// its signature offline against the audit key (GOV-026).
func (e *Env) complianceReport(ctx context.Context, w *walk) error {
	file := filepath.Join(e.TempDir, "compliance-report.json")
	from := e.state.started.Add(-time.Minute).UTC().Format(time.RFC3339)
	to := e.Now().Add(time.Minute).UTC().Format(time.RFC3339)
	if _, err := w.plux(ctx, "bob", "compliance", "report", "--from", from, "--to", to, "--format", "json", "--out", file, "--wait", "3m"); err != nil {
		return err
	}
	key, err := e.Hooks.AuditKey(ctx)
	if err != nil {
		return fmt.Errorf("the audit public key: %w", err)
	}
	if _, err := w.plux(ctx, "bob", "compliance", "verify", "--key", strings.TrimSpace(key), file); err != nil {
		return fmt.Errorf("the compliance report does not verify: %w", err)
	}
	data, err := os.ReadFile(file) //nolint:gosec // the file is the one this step asked the CLI to write
	if err != nil {
		return fmt.Errorf("read the report: %w", err)
	}
	if !bytes.Contains(bytes.ToLower(data), []byte("approv")) {
		return errors.New("the compliance report lists no approval")
	}
	return w.run.Evidence("compliance-report.json", data)
}

// stepReconstruct is step 7: what a device of step 4 saw at the time.
func (e *Env) stepReconstruct(ctx context.Context, run *Run) error {
	w := e.newWalk(run)
	if e.state.deviceID == "" {
		return errors.New("step 4 recorded no device to reconstruct")
	}
	at := e.state.killedAt.UTC().Format(time.RFC3339)
	out, err := w.plux(ctx, "bob", "audit", "reconstruct", "--app", e.App, "--device", e.state.deviceID, "--at", at, "--json")
	if err != nil {
		return err
	}
	id, err := jsonField(out, "id")
	if err != nil {
		return err
	}
	e.state.reconstruction = id
	if _, err := w.plux(ctx, "bob", "audit", "reconstructions", "--app", e.App, "--json"); err != nil {
		return err
	}
	opened, err := w.plux(ctx, "bob", "audit", "reconstructions", "--app", e.App, "--json", id)
	if err != nil {
		return err
	}
	if !strings.Contains(opened, "manifest") {
		return errors.New("the opened reconstruction holds no manifest")
	}
	return nil
}

// stepViewer is the optional step 7b, which only an emulator can do.
func (e *Env) stepViewer(ctx context.Context, run *Run) error {
	if e.Hooks.Viewer == nil || e.state.reconstruction == "" {
		return errors.New("the viewer step needs the viewer hook and the reconstruction of step 7")
	}
	run.Logf("opening reconstruction %s for device %s in the viewer host", e.state.reconstruction, e.state.deviceID)
	return e.Hooks.Viewer(ctx, e.state.deviceID, e.state.reconstruction)
}

// stepUpgrade is step 8: the chart is upgraded while the fleet syncs, and the
// fleet's error budget is zero (DEP-004, X5).
func (e *Env) stepUpgrade(ctx context.Context, run *Run) error {
	w := e.newWalk(run)
	fleet, err := e.startFleet(ctx, FleetSpec{Duration: e.UpgradeWindow, MaxErrorRatio: 0})
	if err != nil {
		return err
	}
	ids, err := fleet.DeviceIDs(ctx)
	if err != nil {
		return fmt.Errorf("the upgrade fleet: %w", err)
	}
	e.state.dsrDevice = ids[0]
	// The upgrade starts once the fleet is up; its error budget is the whole
	// window, so a request refused at any point of the roll fails the step.
	run.Logf("upgrading the chart with %d devices syncing", len(ids))
	if err := e.Hooks.Upgrade(ctx); err != nil {
		_, _ = finishFleet(w, "upgrade", fleet)
		return fmt.Errorf("the upgrade: %w", err)
	}
	rep, err := finishFleet(w, "upgrade", fleet)
	if err != nil {
		return fmt.Errorf("the fleet saw failed requests during the upgrade: %w", err)
	}
	if rep.Requests == 0 || rep.Errors != 0 {
		return fmt.Errorf("%d of %d requests failed during the upgrade, want none", rep.Errors, rep.Requests)
	}
	return nil
}

// stepDSR is step 9: the export and the erasure of a device's data.
func (e *Env) stepDSR(ctx context.Context, run *Run) error {
	w := e.newWalk(run)
	id := e.state.dsrDevice
	if id == "" {
		return errors.New("step 8 recorded no device for the data-subject request")
	}
	if _, err := w.plux(ctx, "admin", "dsr", "map", "--json"); err != nil {
		return err
	}
	before, err := e.exportSubject(ctx, w, id, "before")
	if err != nil {
		return err
	}
	if _, err := w.plux(ctx, "admin", "dsr", "erase", "--kind", "device", "--json", id); err != nil {
		return err
	}
	after, err := e.exportSubject(ctx, w, id, "after")
	if err != nil {
		return err
	}
	if bytes.Equal(before, after) {
		return errors.New("the export after the erasure equals the export before it")
	}
	_, err = w.plux(ctx, "admin", "dsr", "status", "--json")
	return err
}

// exportSubject exports everything held about a device, requires a non-empty
// JSON archive and keeps it as evidence.
func (e *Env) exportSubject(ctx context.Context, w *walk, id, label string) ([]byte, error) {
	file := filepath.Join(e.TempDir, "dsr-"+label+".json")
	if _, err := w.plux(ctx, "admin", "dsr", "export", "--kind", "device", "--out", file, id); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(file) //nolint:gosec // the file is the one this step asked the CLI to write
	if err != nil {
		return nil, fmt.Errorf("read the %s export: %w", label, err)
	}
	var archive map[string]any
	if err := json.Unmarshal(data, &archive); err != nil || len(archive) == 0 {
		return nil, fmt.Errorf("the %s export is not a non-empty JSON archive", label)
	}
	return data, w.run.Evidence("dsr-export-"+label+".json", data)
}

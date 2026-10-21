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
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// FleetSpec describes one run of the synthetic device fleet.
type FleetSpec struct {
	Devices  int
	Duration time.Duration
	// Env is the environment the devices belong to; it must not be a
	// production one, because the devices register with development evidence.
	Env string
	// InjectErrors makes every device report an error event with each
	// minute of use: the synthetic regression of step 3.
	InjectErrors bool
	// ErrorRelease, when positive, limits the regression to the devices that
	// run this release sequence: the candidate regresses, the release it
	// replaces does not.
	ErrorRelease int64
	// MaxErrorRatio is the share of failed requests the run tolerates; the
	// upgrade step sets it to zero.
	MaxErrorRatio float64
}

// FleetReport is the part of plux-load's report the walkthrough reads.
type FleetReport struct {
	Registered     int
	Requests       int64
	Errors         int64
	ControlApplied int64
	// Raw is the report as plux-load printed it, kept as evidence.
	Raw []byte
}

// FleetRun is a fleet that is running; Wait returns its report.
type FleetRun interface {
	// DeviceIDs returns the IDs of the registered devices, once they have
	// registered.
	DeviceIDs(ctx context.Context) ([]string, error)
	// Wait blocks until the run ends and returns its report. The error is
	// non-nil when the run exceeded its error ratio.
	Wait() (FleetReport, error)
}

// Fleet starts synthetic device fleets.
type Fleet interface {
	Start(ctx context.Context, spec FleetSpec) (FleetRun, error)
}

// ExecFleet runs the plux-load binary against the installation.
type ExecFleet struct {
	// Binary is the path of plux-load.
	Binary string
	// Server is the public base URL the devices' DPoP proofs bind to.
	Server string
	App    string
	// CAFile is the PEM file of the CA that signed the server's certificate,
	// or empty.
	CAFile string
	// Dir is where the fleet writes its device list; it must exist.
	Dir string
	// Seed makes the fleets' pacing repeatable.
	Seed uint64

	mu sync.Mutex
	n  int
}

// Start launches plux-load and returns the running fleet.
func (f *ExecFleet) Start(ctx context.Context, spec FleetSpec) (FleetRun, error) {
	f.mu.Lock()
	f.n++
	idsFile := filepath.Join(f.Dir, "devices-"+strconv.Itoa(f.n)+".txt")
	f.mu.Unlock()
	args := []string{
		"-server", f.Server, "-app", f.App, "-env", spec.Env,
		"-devices", strconv.Itoa(spec.Devices), "-duration", spec.Duration.String(),
		"-max-error-ratio", strconv.FormatFloat(spec.MaxErrorRatio, 'f', -1, 64),
		"-seed", strconv.FormatUint(f.Seed, 10), "-forwarded-for", "-device-ids-file", idsFile,
	}
	if f.CAFile != "" {
		args = append(args, "-ca-file", f.CAFile)
	}
	if spec.InjectErrors {
		args = append(args, "-inject-errors", "-error-release", strconv.FormatInt(spec.ErrorRelease, 10))
	}
	cmd := exec.CommandContext(ctx, f.Binary, args...) //nolint:gosec // the binary and flags come from the walkthrough's own configuration
	run := &execRun{cmd: cmd, idsFile: idsFile, done: make(chan struct{})}
	cmd.Stdout, cmd.Stderr = &run.stdout, &run.stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("walkthrough.ExecFleet.Start: %w", err)
	}
	go run.wait()
	return run, nil
}

// execRun is one plux-load process.
type execRun struct {
	cmd            *exec.Cmd
	idsFile        string
	stdout, stderr bytes.Buffer
	done           chan struct{}
	err            error
}

// wait reaps the process and releases Wait.
func (r *execRun) wait() {
	r.err = r.cmd.Wait()
	close(r.done)
}

// DeviceIDs polls for the device list plux-load writes once its devices have
// registered.
func (r *execRun) DeviceIDs(ctx context.Context) ([]string, error) {
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		if ids := r.readIDs(); len(ids) > 0 {
			return ids, nil
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("walkthrough.execRun.DeviceIDs: %w", ctx.Err())
		case <-r.done:
			if ids := r.readIDs(); len(ids) > 0 {
				return ids, nil
			}
			return nil, errors.New("walkthrough.execRun.DeviceIDs: the fleet ended before its devices registered: " + strings.TrimSpace(r.stderr.String()))
		case <-tick.C:
		}
	}
}

// readIDs returns the device IDs plux-load has written, or none yet.
func (r *execRun) readIDs() []string {
	data, err := os.ReadFile(r.idsFile)
	if err != nil {
		return nil
	}
	return strings.Fields(string(data))
}

// Wait returns the report and, when plux-load exited non-zero, its message.
func (r *execRun) Wait() (FleetReport, error) {
	<-r.done
	rep, perr := ParseFleetReport(r.stdout.Bytes())
	if r.err != nil {
		return rep, fmt.Errorf("walkthrough.execRun.Wait: %w: %s", r.err, strings.TrimSpace(r.stderr.String()))
	}
	return rep, perr
}

// ParseFleetReport reads plux-load's JSON report.
func ParseFleetReport(data []byte) (FleetReport, error) {
	var in struct {
		Registered int `json:"registered"`
		Operations map[string]struct {
			Requests int64 `json:"requests"`
			Errors   int64 `json:"errors"`
		} `json:"operations"`
		ControlApplied int64 `json:"controlApplied"`
	}
	rep := FleetReport{Raw: data}
	if err := json.Unmarshal(data, &in); err != nil {
		return rep, fmt.Errorf("walkthrough.ParseFleetReport: %w", err)
	}
	rep.Registered, rep.ControlApplied = in.Registered, in.ControlApplied
	for _, op := range in.Operations {
		rep.Requests += op.Requests
		rep.Errors += op.Errors
	}
	return rep, nil
}

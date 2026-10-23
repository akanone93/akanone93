// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package loadgen

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Fleet is a set of synthetic devices of one app and environment.
type Fleet struct {
	cfg     Config
	rec     *recorder
	devices []*device
	// registered are the devices that registered; byPush finds them by
	// push token.
	registered []*device
	byPush     map[string]*device
}

// NewFleet validates the configuration and prepares the devices. It makes
// no request.
func NewFleet(cfg Config) (*Fleet, error) {
	cfg, err := cfg.withDefaults()
	if err != nil {
		return nil, err
	}
	f := &Fleet{cfg: cfg, rec: newRecorder(), byPush: map[string]*device{}}
	for i := range cfg.Devices {
		d, err := newDevice(&f.cfg, f.rec, i)
		if err != nil {
			return nil, err
		}
		f.devices = append(f.devices, d)
	}
	return f, nil
}

// Register registers every device, at most Concurrency at a time. A device
// that fails to register is counted in the report and left out of the run;
// Register fails only when none registered.
func (f *Fleet) Register(ctx context.Context) error {
	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)
	gate := make(chan struct{}, f.cfg.Concurrency)
	for _, d := range f.devices {
		gate <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-gate }()
			if d.register(ctx) != nil {
				return
			}
			mu.Lock()
			f.registered = append(f.registered, d)
			if d.pushToken != "" {
				f.byPush[d.pushToken] = d
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	if len(f.registered) == 0 {
		return errors.New("loadgen.Register: no device registered; see the report's register errors")
	}
	return nil
}

// Registered is the number of devices that registered.
func (f *Fleet) Registered() int { return len(f.registered) }

// DeviceIDs returns the identifiers the registered devices were given, in
// registration order of completion; they are synthetic and identify nobody.
func (f *Fleet) DeviceIDs() []string {
	ids := make([]string, 0, len(f.registered))
	for _, d := range f.registered {
		ids = append(ids, d.id)
	}
	return ids
}

// Nudge makes the device that holds the push token check its control
// document at once, as the host app does when a silent push arrives. It
// reports whether the token belongs to a device of the fleet.
func (f *Fleet) Nudge(pushToken string) bool {
	d, ok := f.byPush[pushToken]
	if ok {
		d.nudge()
	}
	return ok
}

// Run lets the registered devices keep to their pacing for the configured
// duration, or until ctx ends, and returns the report.
func (f *Fleet) Run(ctx context.Context) Report {
	start := f.cfg.Now()
	runCtx, stop := context.WithTimeout(ctx, f.cfg.Duration)
	defer stop()
	var wg sync.WaitGroup
	for _, d := range f.registered {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.run(runCtx)
		}()
	}
	wg.Wait()
	return f.rec.report(f.cfg, len(f.registered), f.cfg.Now().Sub(start))
}

// Report returns what the fleet has done so far, over the given time.
func (f *Fleet) Report(took time.Duration) Report {
	return f.rec.report(f.cfg, len(f.registered), took)
}

// Run registers a fleet and runs it for the configured duration.
func Run(ctx context.Context, cfg Config) (Report, error) {
	f, err := NewFleet(cfg)
	if err != nil {
		return Report{}, err
	}
	if err := f.Register(ctx); err != nil {
		return f.Report(0), fmt.Errorf("loadgen.Run: %w", err)
	}
	return f.Run(ctx), nil
}

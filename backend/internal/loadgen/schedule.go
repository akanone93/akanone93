// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package loadgen

import (
	"context"
	"time"
)

// task is something a device does again and again.
type task struct {
	every time.Duration
	due   time.Time
	run   func(context.Context)
}

// tasks returns what the device does besides starting up, each first due
// at a random moment within its interval so that the fleet spreads out.
func (d *device) tasks() []*task {
	p := d.cfg.Pacing
	ts := []*task{
		{every: p.Manifest, run: d.checkManifest},
		{every: p.Control, run: d.checkControl},
		{every: p.Telemetry, run: d.sendActivity},
	}
	if d.cfg.Function != nil && p.Function > 0 {
		ts = append(ts, &task{every: p.Function, run: d.callFunction})
	}
	now := d.cfg.Now()
	for _, t := range ts {
		t.due = now.Add(d.rng.within(t.every))
	}
	return ts
}

// sendActivity sends a batch of the events of a minute of use.
func (d *device) sendActivity(ctx context.Context) {
	d.activity()
	d.flush(ctx)
}

// start does what the runtime does when the app starts: check the manifest,
// report the session of the release it runs, and fetch the control document.
func (d *device) start(ctx context.Context) {
	d.checkManifest(ctx)
	d.sessionStart()
	d.checkControl(ctx)
	d.flush(ctx)
}

// run starts the device and then keeps to its pacing until ctx ends. A
// nudge makes it check its control document at once.
func (d *device) run(ctx context.Context) {
	d.start(ctx)
	ts := d.tasks()
	for {
		next := earliest(ts)
		timer := time.NewTimer(next.due.Sub(d.cfg.Now()))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-d.nudges:
			timer.Stop()
			d.renewToken(ctx)
			d.checkControl(ctx)
		case <-timer.C:
			d.renewToken(ctx)
			next.run(ctx)
			next.due = d.cfg.Now().Add(d.rng.jittered(next.every, d.cfg.Pacing.Jitter))
		}
	}
}

// earliest returns the task that is due first.
func earliest(ts []*task) *task {
	first := ts[0]
	for _, t := range ts[1:] {
		if t.due.Before(first.due) {
			first = t
		}
	}
	return first
}

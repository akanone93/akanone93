// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package rollout

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nightCode42/plux3/backend/internal/analytics"
)

// The runtime events the health gates count (Q13, Appendix G.2). The failure
// events of actions and functions are the rollup series E6 derives from the
// results of action_run and function_call.
const (
	EventSessionStart  = "session_start"
	EventFatalError    = "fatal_error"
	EventError         = "error"
	EventActionRun     = "action_run"
	EventActionError   = "action_error"
	EventFunctionCall  = "function_call"
	EventFunctionError = "function_error"
)

// rollupLimit bounds the rows one count reads: a window of a day at five
// minutes is 288 buckets per release.
const rollupLimit = 100000

// AnalyticsSignals counts events from the analytics rollups (ANL-010), which
// hold them per release.
type AnalyticsSignals struct {
	Store analytics.Store
}

var _ Signals = AnalyticsSignals{}

// Counts sums the events of one release over a window. A window longer than a
// day is read at hourly granularity.
func (a AnalyticsSignals) Counts(ctx context.Context, org string, q SignalQuery) (Counts, error) {
	if a.Store == nil {
		return Counts{}, errors.New("rollout: no analytics store")
	}
	var c Counts
	for _, f := range []struct {
		name string
		into *int64
	}{
		{EventSessionStart, &c.Sessions},
		{EventFatalError, &c.Crashed},
		{EventError, &c.Errors},
		{EventActionRun, &c.Actions},
		{EventActionError, &c.ActionErrors},
		{EventFunctionCall, &c.Calls},
		{EventFunctionError, &c.CallErrors},
	} {
		n, err := a.count(ctx, org, q, f.name)
		if err != nil {
			return Counts{}, err
		}
		*f.into = n
	}
	c.Custom = make(map[string]int64, len(q.Custom))
	for _, name := range q.Custom {
		n, err := a.count(ctx, org, q, name)
		if err != nil {
			return Counts{}, err
		}
		c.Custom[name] = n
	}
	return c, nil
}

// count sums one event's rollups for the query's release.
func (a AnalyticsSignals) count(ctx context.Context, org string, q SignalQuery, event string) (int64, error) {
	granularity := analytics.FiveMinutes
	if q.To.Sub(q.From) > 24*time.Hour {
		granularity = analytics.Hour
	}
	series, err := a.Store.Query(ctx, org, analytics.Query{
		AppID: q.AppID, EnvironmentID: q.EnvironmentID, Metric: event, Granularity: granularity,
		From: q.From, To: q.To, GroupBy: []analytics.Dimension{analytics.ByRelease}, Limit: rollupLimit,
	})
	if err != nil {
		return 0, fmt.Errorf("rollout: query %s: %w", event, err)
	}
	if series.Truncated {
		return 0, fmt.Errorf("rollout: the rollups of %s exceed %d rows; shorten the gate's window", event, rollupLimit)
	}
	var n int64
	for _, p := range series.Points {
		if p.Group.Release == q.Release {
			n += p.Events
		}
	}
	return n, nil
}

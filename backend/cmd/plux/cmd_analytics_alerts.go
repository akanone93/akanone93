// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

const (
	alertsUsage = "Usage: plux analytics alerts list|states|create|update|delete [flags]\n"
	errorsUsage = "Usage: plux analytics errors list|show [flags]\n"
)

// alertFlags are the flags that define an alert rule.
type alertFlags struct {
	app, name, metric, environment, filters, aggregation, comparison, severity, state string
	window, hold                                                                      time.Duration
	threshold                                                                         float64
	disabled                                                                          bool
}

// bind registers the flags on set.
func (f *alertFlags) bind(set *flag.FlagSet) {
	set.StringVar(&f.app, "app", "", "the app `id`")
	set.StringVar(&f.name, "name", "", "the rule's `name`")
	set.StringVar(&f.metric, "metric", "", "the event `name`, such as fatal_error")
	set.StringVar(&f.environment, "env", "", "restrict to one environment `key`")
	set.StringVar(&f.filters, "filter", "", "dimension filters, comma-separated `dimension=value` pairs")
	set.StringVar(&f.aggregation, "aggregation", "count", "count, sum, avg, min, max, p50, p90 or p99")
	set.DurationVar(&f.window, "window", 5*time.Minute, "the `duration` the aggregation covers")
	set.StringVar(&f.comparison, "comparison", ">", "the comparison: >, >=, < or <=")
	set.Float64Var(&f.threshold, "threshold", 0, "the `value` the aggregation is compared with")
	set.DurationVar(&f.hold, "for", 0, "how long the comparison must hold before the rule fires")
	set.StringVar(&f.severity, "severity", "warning", "info, warning or critical")
	set.BoolVar(&f.disabled, "disabled", false, "keep the rule without evaluating it")
	set.StringVar(&f.state, "state", "", "list only rules in this `state`: ok, pending, firing or resolved")
}

// rule builds the rule message the flags describe.
func (f alertFlags) rule(id string) (*pluxv1.AlertRule, error) {
	filters := map[string]string{}
	for _, pair := range splitList(f.filters) {
		k, v, ok := strings.Cut(pair, "=")
		if !ok || k == "" {
			return nil, usageError("--filter takes dimension=value pairs, got " + pair)
		}
		filters[k] = v
	}
	return &pluxv1.AlertRule{
		Id: id, AppId: f.app, Name: f.name, Metric: f.metric, Environment: f.environment, Filters: filters,
		Aggregation: f.aggregation, WindowSeconds: int32(f.window / time.Second), //nolint:gosec // G115: a flag value; the server bounds it
		Comparison: f.comparison, Threshold: f.threshold,
		ForSeconds: int32(f.hold / time.Second), //nolint:gosec // G115: a flag value; the server bounds it
		Severity:   f.severity, Disabled: f.disabled,
	}, nil
}

func ruleView(r *pluxv1.AlertRule) map[string]any {
	return map[string]any{
		"id": r.GetId(), "app": r.GetAppId(), "name": r.GetName(), "metric": r.GetMetric(), "environment": r.GetEnvironment(),
		"filters": r.GetFilters(), "aggregation": r.GetAggregation(), "windowSeconds": r.GetWindowSeconds(),
		"comparison": r.GetComparison(), "threshold": r.GetThreshold(), "forSeconds": r.GetForSeconds(),
		"severity": r.GetSeverity(), "disabled": r.GetDisabled(), "revision": r.GetRevision(),
	}
}

func ruleLine(r *pluxv1.AlertRule) string {
	state := ""
	if r.GetDisabled() {
		state = "\t(disabled)"
	}
	return fmt.Sprintf("%s\t%s\t%s(%s) over %ds %s %g for %ds\t%s%s\n", r.GetId(), r.GetName(), r.GetAggregation(), r.GetMetric(),
		r.GetWindowSeconds(), r.GetComparison(), r.GetThreshold(), r.GetForSeconds(), r.GetSeverity(), state)
}

func alertStateView(s *pluxv1.AlertState) map[string]any {
	v := map[string]any{"rule": s.GetRuleId(), "name": s.GetRuleName(), "state": s.GetState(), "since": s.GetSince().AsTime().Format(time.RFC3339)}
	if s.Value != nil {
		v["value"] = s.GetValue()
	}
	return v
}

func alertStateLine(s *pluxv1.AlertState) string {
	value := "no data"
	if s.Value != nil {
		value = fmt.Sprintf("%g", s.GetValue())
	}
	return fmt.Sprintf("%s\t%s\t%s\tsince %s\tvalue %s\n", s.GetRuleId(), s.GetRuleName(), s.GetState(), s.GetSince().AsTime().Format(time.RFC3339), value)
}

// analyticsAlerts manages alert rules and shows where they stand (ANL-030).
func (e env) analyticsAlerts(args []string) int {
	var f alertFlags
	c, rest, code, ok := e.governanceCmd("analytics alerts", alertsUsage, args, map[string]subUsage{
		"list":   {"Usage: plux analytics alerts list [--app id] [--json]\n\nLists alert rules.", 0},
		"states": {"Usage: plux analytics alerts states [--app id] [--state firing] [--json]\n\nShows where the rules stand: ok, pending, firing or resolved.", 0},
		"create": {"Usage: plux analytics alerts create --app id --name n --metric event [--aggregation count] [--window 5m] [--comparison >] --threshold v [--for 10m] [--severity warning] [--filter release=7] [--env key] [--json]\n\nDefines a rule. It fires when the aggregation of the metric over the window has been past the threshold for the for-duration, and notifies the subscribers of alert events.", 0},
		"update": {"Usage: plux analytics alerts update [flags as create] [--json] <id>\n\nReplaces a rule's definition; its state starts again.", 1},
		"delete": {"Usage: plux analytics alerts delete [--json] <id>\n\nRemoves a rule and its state.", 1},
	}, f.bind)
	if !ok {
		return code
	}
	client, err := e.analyticsClient(c)
	if err != nil {
		return e.fail("analytics alerts", err)
	}
	return e.alertsCall(context.Background(), client, c, args[0], rest, f)
}

// alertsCall runs one alerts subcommand.
func (e env) alertsCall(ctx context.Context, client pluxv1connect.AnalyticsServiceClient, c common, sub string, rest []string, f alertFlags) int {
	cmd := "analytics alerts " + sub
	switch sub {
	case "list":
		res, err := client.ListAlertRules(ctx, connect.NewRequest(&pluxv1.ListAlertRulesRequest{AppId: f.app}))
		if err != nil {
			return e.fail(cmd, err)
		}
		views, text := []map[string]any{}, strings.Builder{}
		for _, r := range res.Msg.GetRules() {
			views = append(views, ruleView(r))
			text.WriteString(ruleLine(r))
		}
		return e.emit(c.json, map[string]any{"rules": views}, text.String())
	case "states":
		res, err := client.ListAlertStates(ctx, connect.NewRequest(&pluxv1.ListAlertStatesRequest{AppId: f.app, State: f.state}))
		if err != nil {
			return e.fail(cmd, err)
		}
		views, text := []map[string]any{}, strings.Builder{}
		for _, s := range res.Msg.GetStates() {
			views = append(views, alertStateView(s))
			text.WriteString(alertStateLine(s))
		}
		return e.emit(c.json, map[string]any{"states": views}, text.String())
	case "delete":
		if _, err := client.DeleteAlertRule(ctx, connect.NewRequest(&pluxv1.DeleteAlertRuleRequest{Id: rest[0]})); err != nil {
			return e.fail(cmd, err)
		}
		return e.emit(c.json, map[string]any{"deleted": rest[0]}, "Removed the rule "+rest[0]+"\n")
	}
	return e.saveAlert(ctx, client, c, sub, rest, f)
}

// saveAlert creates or updates a rule from the flags.
func (e env) saveAlert(ctx context.Context, client pluxv1connect.AnalyticsServiceClient, c common, sub string, rest []string, f alertFlags) int {
	cmd := "analytics alerts " + sub
	id := ""
	if sub == "update" {
		id = rest[0]
	}
	rule, err := f.rule(id)
	if err != nil {
		return e.fail(cmd, err)
	}
	var saved *pluxv1.AlertRule
	if sub == "create" {
		res, err := client.CreateAlertRule(ctx, connect.NewRequest(&pluxv1.CreateAlertRuleRequest{Rule: rule}))
		if err != nil {
			return e.fail(cmd, err)
		}
		saved = res.Msg.GetRule()
	} else {
		res, err := client.UpdateAlertRule(ctx, connect.NewRequest(&pluxv1.UpdateAlertRuleRequest{Rule: rule}))
		if err != nil {
			return e.fail(cmd, err)
		}
		saved = res.Msg.GetRule()
	}
	return e.emit(c.json, ruleView(saved), ruleLine(saved))
}

// errorFlags are the flags of plux analytics errors.
type errorFlags struct {
	app     string
	release int64
	days    int
	limit   int
}

func errorGroupView(g *pluxv1.ErrorGroup) map[string]any {
	trend := []map[string]any{}
	for _, d := range g.GetTrend() {
		trend = append(trend, map[string]any{"day": d.GetDay(), "events": d.GetEvents()})
	}
	return map[string]any{
		"id": g.GetId(), "app": g.GetAppId(), "release": g.GetReleaseSequence(), "fingerprint": g.GetFingerprint(),
		"code": g.GetCode(), "plugin": g.GetPluginKey(), "route": g.GetRoute(), "nodePath": g.GetNodePath(),
		"jsonPath": g.GetJsonPath(), "sourceFile": g.GetSourceFile(), "frames": g.GetFrames(), "fatal": g.GetFatal(),
		"firstSeen": g.GetFirstSeen().AsTime().Format(time.RFC3339), "lastSeen": g.GetLastSeen().AsTime().Format(time.RFC3339),
		"events": g.GetEvents(), "devices": g.GetDevices(), "trend": trend,
	}
}

func errorGroupLine(g *pluxv1.ErrorGroup) string {
	where := g.GetJsonPath()
	if where == "" {
		where = g.GetNodePath()
	}
	return fmt.Sprintf("%s\t%s\trelease %d\t%d events\t%d devices\tlast %s\t%s\n", g.GetId(), g.GetCode(), g.GetReleaseSequence(),
		g.GetEvents(), g.GetDevices(), g.GetLastSeen().AsTime().Format(time.RFC3339), where)
}

// analyticsErrors lists error groups and shows one with its samples (ANL-040).
func (e env) analyticsErrors(args []string) int {
	var f errorFlags
	c, rest, code, ok := e.governanceCmd("analytics errors", errorsUsage, args, map[string]subUsage{
		"list": {"Usage: plux analytics errors list --app id [--release n] [--days 14] [--limit n] [--json]\n\nLists the error groups of an app, newest activity first.", 0},
		"show": {"Usage: plux analytics errors show [--days 14] [--json] <id>\n\nShows a group with its trend by day and its newest sample events. The first call resolves the node to its document path.", 1},
	}, func(set *flag.FlagSet) {
		set.StringVar(&f.app, "app", "", "the app `id`")
		set.Int64Var(&f.release, "release", 0, "restrict to one release `sequence`")
		set.IntVar(&f.days, "days", 0, "how many `days` back; 0 for the default")
		set.IntVar(&f.limit, "limit", 0, "the most groups (`n`)")
	})
	if !ok {
		return code
	}
	client, err := e.analyticsClient(c)
	if err != nil {
		return e.fail("analytics errors", err)
	}
	ctx := context.Background()
	if args[0] == "show" {
		res, err := client.GetErrorGroup(ctx, connect.NewRequest(&pluxv1.GetErrorGroupRequest{Id: rest[0], Days: int32(f.days)})) //nolint:gosec // G115: a flag value; the server bounds it
		if err != nil {
			return e.fail("analytics errors show", err)
		}
		return e.showErrorGroup(c.json, res.Msg)
	}
	if f.app == "" {
		return e.fail("analytics errors list", usageError("--app is required"))
	}
	res, err := client.ListErrorGroups(ctx, connect.NewRequest(&pluxv1.ListErrorGroupsRequest{
		AppId: f.app, ReleaseSequence: f.release, Days: int32(f.days), Limit: int32(f.limit), //nolint:gosec // G115: flag values; the server bounds them
	}))
	if err != nil {
		return e.fail("analytics errors list", err)
	}
	views, text := []map[string]any{}, strings.Builder{}
	for _, g := range res.Msg.GetGroups() {
		views = append(views, errorGroupView(g))
		text.WriteString(errorGroupLine(g))
	}
	return e.emit(c.json, map[string]any{"groups": views}, text.String())
}

// showErrorGroup prints one group, its trend and its samples.
func (e env) showErrorGroup(jsonOut bool, r *pluxv1.GetErrorGroupResponse) int {
	view := errorGroupView(r.GetGroup())
	samples := []map[string]any{}
	var text strings.Builder
	text.WriteString(errorGroupLine(r.GetGroup()))
	for _, d := range r.GetGroup().GetTrend() {
		fmt.Fprintf(&text, "  %s\t%d events\n", d.GetDay(), d.GetEvents())
	}
	for _, s := range r.GetSamples() {
		at := s.GetOccurredAt().AsTime().Format(time.RFC3339)
		samples = append(samples, map[string]any{"event": s.GetEventId(), "at": at, "fields": s.GetFieldsJson()})
		fmt.Fprintf(&text, "  sample %s\t%s\t%s\n", s.GetEventId(), at, s.GetFieldsJson())
	}
	view["samples"] = samples
	return e.emit(jsonOut, view, text.String())
}

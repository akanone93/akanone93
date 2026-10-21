// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"strconv"
	"strings"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

const (
	analyticsUsage = "Usage: plux analytics query|retention|export|files|alerts|errors [flags]\n"
	retentionUsage = "Usage: plux analytics retention show|set|list|cancel [flags]\n"
	exportUsage    = "Usage: plux analytics export show|set|delete [flags]\n"
)

// analytics queries the rollups of runtime events and manages how long
// events are kept and where rollups are exported (ANL-010, ANL-011).
func (e env) analytics(args []string) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(e.stderr, analyticsUsage)
		return exitUsage
	}
	switch args[0] {
	case "query":
		return e.analyticsQuery(args)
	case "retention":
		return e.analyticsRetention(args[1:])
	case "export":
		return e.analyticsExport(args[1:])
	case "files":
		return e.analyticsFiles(args[1:])
	case "alerts":
		return e.analyticsAlerts(args[1:])
	case "errors":
		return e.analyticsErrors(args[1:])
	}
	_, _ = fmt.Fprintf(e.stderr, "%s analytics: unknown subcommand %q\n", name, args[0])
	return exitUsage
}

// analyticsClient connects to the analytics service.
func (e env) analyticsClient(c common) (pluxv1connect.AnalyticsServiceClient, error) {
	opts, err := e.authOptions(c)
	if err != nil {
		return nil, err
	}
	return pluxv1connect.NewAnalyticsServiceClient(e.httpClient, c.server, opts), nil
}

// queryFlags are the flags of plux analytics query.
type queryFlags struct {
	app, metric, environment, granularity, from, to, groupBy string
	since                                                    time.Duration
	limit                                                    int
}

// window resolves the time range of a query: --from to --to, where --to
// defaults to now and --from to --since before it.
func (q queryFlags) window(now time.Time) (time.Time, time.Time, error) {
	to := now
	if q.to != "" {
		t, err := time.Parse(time.RFC3339, q.to)
		if err != nil {
			return time.Time{}, time.Time{}, usageError("--to is not an RFC 3339 time such as 2026-10-18T00:00:00Z")
		}
		to = t
	}
	from := to.Add(-q.since)
	if q.from != "" {
		t, err := time.Parse(time.RFC3339, q.from)
		if err != nil {
			return time.Time{}, time.Time{}, usageError("--from is not an RFC 3339 time such as 2026-10-17T00:00:00Z")
		}
		from = t
	}
	if !to.After(from) {
		return time.Time{}, time.Time{}, usageError("the time range is empty: --to must be after --from")
	}
	return from, to, nil
}

func pointView(p *pluxv1.RollupPoint) map[string]any {
	g := p.GetGroup()
	return map[string]any{
		"bucket": p.GetBucket().AsTime().Format(time.RFC3339), "events": p.GetEvents(), "valueCount": p.GetValueCount(),
		"sum": p.GetSum(), "min": p.GetMin(), "max": p.GetMax(), "p50": p.GetP50(), "p90": p.GetP90(), "p99": p.GetP99(),
		"environmentId": g.GetEnvironmentId(), "release": g.GetReleaseSequence(), "plugin": g.GetPluginKey(),
		"page": g.GetPage(), "deviceClass": g.GetDeviceClass(), "locale": g.GetLocale(),
	}
}

func pointLine(p *pluxv1.RollupPoint) string {
	return fmt.Sprintf("%s\t%d events\tp50 %.1f\tp90 %.1f\tp99 %.1f\t%s\n", p.GetBucket().AsTime().Format(time.RFC3339),
		p.GetEvents(), p.GetP50(), p.GetP90(), p.GetP99(), groupLabel(p.GetGroup()))
}

// groupLabel names the dimensions a point is kept apart by, skipping the
// empty ones.
func groupLabel(g *pluxv1.RollupGroup) string {
	var parts []string
	add := func(k, v string) {
		if v != "" {
			parts = append(parts, k+"="+v)
		}
	}
	add("environment", g.GetEnvironmentId())
	if g.GetReleaseSequence() != 0 {
		add("release", strconv.FormatInt(g.GetReleaseSequence(), 10))
	}
	add("plugin", g.GetPluginKey())
	add("page", g.GetPage())
	add("device_class", g.GetDeviceClass())
	add("locale", g.GetLocale())
	return strings.Join(parts, " ")
}

// analyticsQuery reads the rollups of one event name and follows the pages.
func (e env) analyticsQuery(args []string) int {
	var f queryFlags
	c, _, code, ok := e.governanceCmd("analytics", analyticsUsage, args, map[string]subUsage{
		"query": {"Usage: plux analytics query --metric name [--app id] [--env key] [--granularity 5m|1h|1d] [--from time] [--to time | --since 24h] [--group-by release,page] [--limit n] [--json]\n\nReads the rollups of an event name: counts, and p50, p90 and p99 of its durations in milliseconds.", 0},
	}, func(set *flag.FlagSet) {
		set.StringVar(&f.app, "app", "", "the app `id`")
		set.StringVar(&f.metric, "metric", "", "the event `name`, such as render_perf")
		set.StringVar(&f.environment, "env", "", "restrict to one environment `key`")
		set.StringVar(&f.granularity, "granularity", "1h", "the bucket width: 5m, 1h or 1d")
		set.StringVar(&f.from, "from", "", "the start `time` (RFC 3339)")
		set.StringVar(&f.to, "to", "", "the end `time` (RFC 3339), default now")
		set.DurationVar(&f.since, "since", 24*time.Hour, "the range before --to when --from is not given")
		set.StringVar(&f.groupBy, "group-by", "", "dimensions to keep apart, comma-separated: environment, release, plugin, page, device_class, locale")
		set.IntVar(&f.limit, "limit", 0, "the most points per request (`n`); further pages are followed")
	})
	if !ok {
		return code
	}
	if f.app == "" || f.metric == "" {
		return e.fail("analytics query", usageError("--app and --metric are required"))
	}
	from, to, err := f.window(time.Now())
	if err != nil {
		return e.fail("analytics query", err)
	}
	client, err := e.analyticsClient(c)
	if err != nil {
		return e.fail("analytics query", err)
	}
	views := []map[string]any{}
	var text strings.Builder
	if err := eachPage(func(token string) (string, error) {
		res, err := client.QueryRollups(context.Background(), connect.NewRequest(&pluxv1.QueryRollupsRequest{
			AppId: f.app, Environment: f.environment, Metric: f.metric, Granularity: f.granularity,
			From: timestamppb.New(from), To: timestamppb.New(to), GroupBy: splitList(f.groupBy),
			Limit: int32(f.limit), PageToken: token, //nolint:gosec // a flag value; the server bounds it
		}))
		if err != nil {
			return "", err //nolint:wrapcheck // e.fail names the command
		}
		for _, p := range res.Msg.GetPoints() {
			views = append(views, pointView(p))
			text.WriteString(pointLine(p))
		}
		return res.Msg.GetNextPageToken(), nil
	}); err != nil {
		return e.fail("analytics query", err)
	}
	return e.emit(c.json, map[string]any{"metric": f.metric, "points": views}, text.String())
}

func settingView(s *pluxv1.RetentionSetting) map[string]any {
	return map[string]any{
		"dataClass": s.GetDataClass(), "days": s.GetDays(), "default": s.GetIsDefault(), "enforced": s.GetEnforced(),
		"changedBy": s.GetChangedBy(),
	}
}

// daysText is a retention in words; 0 is indefinitely.
func daysText(days int32) string {
	if days == 0 {
		return "indefinitely"
	}
	return fmt.Sprintf("%d days", days)
}

func changeView(c *pluxv1.RetentionChange) map[string]any {
	return map[string]any{
		"id": c.GetId(), "dataClass": c.GetDataClass(), "days": c.GetDays(), "state": c.GetState(),
		"requestedBy": c.GetRequestedBy(), "applyAt": c.GetApplyAt().AsTime().Format(time.RFC3339),
	}
}

func changeLine(c *pluxv1.RetentionChange) string {
	return fmt.Sprintf("%s\t%s\t%s\t%s\t%s\n", c.GetId(), c.GetDataClass(), daysText(c.GetDays()), c.GetState(), c.GetApplyAt().AsTime().Format(time.RFC3339))
}

// retentionFlags are the flags of plux analytics retention.
type retentionFlags struct {
	class                string
	days                 int
	immediate, scheduled bool
}

// analyticsRetention shows and changes how long data is kept (Q17).
func (e env) analyticsRetention(args []string) int {
	var f retentionFlags
	c, rest, code, ok := e.governanceCmd("analytics retention", retentionUsage, args, map[string]subUsage{
		"show":   {"Usage: plux analytics retention show [--json]\n\nShows the retention of raw events, rollups and the production audit log, and the changes waiting for the maintenance window.", 0},
		"set":    {"Usage: plux analytics retention set --class raw_events|rollups|audit_production --days n [--immediate] [--json]\n\nChanges a retention; n = 0 keeps the data indefinitely. The change applies in the maintenance window unless --immediate is given.", 0},
		"list":   {"Usage: plux analytics retention list [--scheduled] [--json]\n\nLists retention changes, newest first.", 0},
		"cancel": {"Usage: plux analytics retention cancel [--json] <id>\n\nWithdraws a change that waits for the maintenance window.", 1},
	}, func(set *flag.FlagSet) {
		set.StringVar(&f.class, "class", "", "the data `class`: raw_events, rollups or audit_production")
		set.IntVar(&f.days, "days", -1, "the retention in `days`; 0 for indefinitely")
		set.BoolVar(&f.immediate, "immediate", false, "apply the change now instead of in the maintenance window")
		set.BoolVar(&f.scheduled, "scheduled", false, "list only the changes that still wait")
	})
	if !ok {
		return code
	}
	client, err := e.analyticsClient(c)
	if err != nil {
		return e.fail("analytics retention", err)
	}
	ctx := context.Background()
	switch args[0] {
	case "show":
		res, err := client.GetRetention(ctx, connect.NewRequest(&pluxv1.GetRetentionRequest{}))
		if err != nil {
			return e.fail("analytics retention show", err)
		}
		return e.showRetention(c.json, res.Msg)
	case "set":
		return e.setRetention(ctx, client, c, f)
	case "list":
		res, err := client.ListRetentionChanges(ctx, connect.NewRequest(&pluxv1.ListRetentionChangesRequest{OnlyScheduled: f.scheduled}))
		if err != nil {
			return e.fail("analytics retention list", err)
		}
		views := []map[string]any{}
		var text strings.Builder
		for _, ch := range res.Msg.GetChanges() {
			views = append(views, changeView(ch))
			text.WriteString(changeLine(ch))
		}
		return e.emit(c.json, map[string]any{"changes": views}, text.String())
	default: // cancel
		res, err := client.CancelRetentionChange(ctx, connect.NewRequest(&pluxv1.CancelRetentionChangeRequest{Id: rest[0]}))
		if err != nil {
			return e.fail("analytics retention cancel", err)
		}
		return e.emit(c.json, changeView(res.Msg.GetChange()), "Cancelled the change of "+res.Msg.GetChange().GetDataClass())
	}
}

// setRetention schedules or makes a retention change and prints what the
// server says about it, including PLX-8301.
func (e env) setRetention(ctx context.Context, client pluxv1connect.AnalyticsServiceClient, c common, f retentionFlags) int {
	if f.class == "" || f.days < 0 {
		return e.fail("analytics retention set", usageError("--class and --days are required"))
	}
	res, err := client.SetRetention(ctx, connect.NewRequest(&pluxv1.SetRetentionRequest{
		DataClass: f.class, Days: int32(f.days), Immediate: f.immediate, //nolint:gosec // the server bounds it
	}))
	if err != nil {
		return e.fail("analytics retention set", err)
	}
	ch := res.Msg.GetChange()
	text := fmt.Sprintf("Set %s to %s (%s)", ch.GetDataClass(), daysText(ch.GetDays()), ch.GetState())
	for _, d := range res.Msg.GetDiagnostics() {
		text += "\n" + d.GetMessage()
	}
	return e.emit(c.json, changeView(ch), text)
}

func (e env) showRetention(jsonOut bool, r *pluxv1.GetRetentionResponse) int {
	settings := []map[string]any{}
	scheduled := []map[string]any{}
	var text strings.Builder
	for _, s := range r.GetSettings() {
		settings = append(settings, settingView(s))
		note := ""
		switch {
		case !s.GetEnforced():
			note = "\t(recorded; nothing deletes it yet)"
		case s.GetIsDefault():
			note = "\t(default)"
		}
		_, _ = fmt.Fprintf(&text, "%s\t%s%s\n", s.GetDataClass(), daysText(s.GetDays()), note)
	}
	for _, ch := range r.GetScheduled() {
		scheduled = append(scheduled, changeView(ch))
		text.WriteString("scheduled\t" + changeLine(ch))
	}
	return e.emit(jsonOut, map[string]any{"settings": settings, "scheduled": scheduled}, text.String())
}

func exportView(x *pluxv1.Export) map[string]any {
	return map[string]any{
		"endpoint": x.GetEndpoint(), "enabled": x.GetEnabled(), "exportedUntil": x.GetExportedUntil().AsTime().Format(time.RFC3339),
		"lastError": x.GetLastError(),
	}
}

func exportLine(x *pluxv1.Export) string {
	state := "enabled"
	if !x.GetEnabled() {
		state = "disabled"
	}
	line := fmt.Sprintf("%s\t%s\texported until %s", x.GetEndpoint(), state, x.GetExportedUntil().AsTime().Format(time.RFC3339))
	if x.GetLastError() != "" {
		line += "\tlast error: " + x.GetLastError()
	}
	return line
}

// analyticsExport shows, sets and deletes the OTLP destination (ANL-011).
func (e env) analyticsExport(args []string) int {
	var endpoint string
	var disabled bool
	c, _, code, ok := e.governanceCmd("analytics export", exportUsage, args, map[string]subUsage{
		"show":   {"Usage: plux analytics export show [--json]\n\nShows the OTLP destination and how far the export got.", 0},
		"set":    {"Usage: plux analytics export set --endpoint url [--disabled] [--json]\n\nSets the OTLP/HTTP collector that receives the rollups as metrics. The server sends it an empty request first; a destination that does not answer is refused.", 0},
		"delete": {"Usage: plux analytics export delete\n\nRemoves the OTLP destination.", 0},
	}, func(set *flag.FlagSet) {
		set.StringVar(&endpoint, "endpoint", "", "the collector's `url` (https)")
		set.BoolVar(&disabled, "disabled", false, "save the destination without exporting")
	})
	if !ok {
		return code
	}
	client, err := e.analyticsClient(c)
	if err != nil {
		return e.fail("analytics export", err)
	}
	ctx := context.Background()
	switch args[0] {
	case "show":
		res, err := client.GetExport(ctx, connect.NewRequest(&pluxv1.GetExportRequest{}))
		if err != nil {
			return e.fail("analytics export show", err)
		}
		return e.emit(c.json, exportView(res.Msg.GetExport()), exportLine(res.Msg.GetExport()))
	case "set":
		if endpoint == "" {
			return e.fail("analytics export set", usageError("--endpoint is required"))
		}
		res, err := client.SetExport(ctx, connect.NewRequest(&pluxv1.SetExportRequest{Endpoint: endpoint, Enabled: !disabled}))
		if err != nil {
			return e.fail("analytics export set", err)
		}
		return e.emit(c.json, exportView(res.Msg.GetExport()), exportLine(res.Msg.GetExport()))
	default: // delete
		if _, err := client.DeleteExport(ctx, connect.NewRequest(&pluxv1.DeleteExportRequest{})); err != nil {
			return e.fail("analytics export delete", err)
		}
		return e.emit(c.json, map[string]any{"deleted": true}, "Removed the OTLP destination")
	}
}

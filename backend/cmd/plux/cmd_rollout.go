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

const rolloutUsage = `Usage: plux rollout create|show|pause|resume|abort|list [flags]

  create   Schedule a rollout of a release on a channel, in stages
  show     Show a rollout, its state and its newest health check
  pause    Stop a rollout from advancing
  resume   Continue a paused rollout
  abort    End a rollout and return the channel to its previous release
  list     List an app's rollouts
`

// rolloutFlags are the flags of the rollout subcommands beyond the common
// ones.
type rolloutFlags struct {
	env, channel, stages, start, end, rule, segments, allow, deny string
	release                                                       int64
	reason                                                        string
	open                                                          bool
	gate                                                          gateFlags
}

// gateFlags are the flags that make a health gate (REL-053).
type gateFlags struct {
	crashFree, errors, actions, functions int
	custom, onRegression                  string
	minSample                             int64
	window                                time.Duration
}

// rollout schedules and runs staged rollouts of a release (REL-050 to
// REL-053). It needs rollout.manage.
func (e env) rollout(args []string) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(e.stderr, rolloutUsage)
		return exitUsage
	}
	sub := args[0]
	usage, positional, known := rolloutSubcommand(sub)
	if !known {
		_, _ = fmt.Fprintf(e.stderr, "%s rollout: unknown subcommand %q\n\n%s", name, sub, rolloutUsage)
		return exitUsage
	}
	var (
		c common
		f rolloutFlags
	)
	set := flag.NewFlagSet("rollout "+sub, flag.ContinueOnError)
	c.register(set, true)
	registerRolloutFlags(set, &f)
	rest, code, ok := parse(set, args[1:], e.stderr, usage, positional)
	if !ok {
		return code
	}
	if err := c.resolve(); err != nil {
		return e.fail("rollout", err)
	}
	if err := needApp(c); err != nil {
		return e.fail("rollout", err)
	}
	return e.rolloutRun(c, f, sub, rest)
}

// registerRolloutFlags defines the flags of all rollout subcommands.
func registerRolloutFlags(set *flag.FlagSet, f *rolloutFlags) {
	set.StringVar(&f.env, "env", "", "the environment `key` (create)")
	set.StringVar(&f.channel, "channel", "", "the channel `key` (default production)")
	set.Int64Var(&f.release, "release", 0, "the `sequence` of the release to roll out")
	set.StringVar(&f.stages, "stages", "", "the schedule, as `share[:duration],...`, e.g. 1%:1h,10%:1d,100%")
	set.StringVar(&f.start, "start", "", "when to start, as an RFC 3339 `time` (default now)")
	set.StringVar(&f.end, "end", "", "when to end and revert, as an RFC 3339 `time`")
	set.StringVar(&f.rule, "rule", "", "a `file` holding a targeting rule as JSON")
	set.StringVar(&f.segments, "segment", "", "comma-separated segment document `files`")
	set.StringVar(&f.allow, "allow", "", "comma-separated `devices` that always receive the release")
	set.StringVar(&f.deny, "deny", "", "comma-separated `devices` that never receive it")
	set.StringVar(&f.reason, "reason", "", "why (pause, abort)")
	set.BoolVar(&f.open, "open", false, "list only rollouts in progress")
	set.IntVar(&f.gate.crashFree, "gate-crash-free", 0, "most crash-free sessions may drop, in basis points (0: off)")
	set.IntVar(&f.gate.errors, "gate-errors", 0, "most the Plux error rate may rise, in basis points (0: off)")
	set.IntVar(&f.gate.actions, "gate-actions", 0, "most the action error rate may rise, in basis points (0: off)")
	set.IntVar(&f.gate.functions, "gate-functions", 0, "most the function error rate may rise, in basis points (0: off)")
	set.StringVar(&f.gate.custom, "gate-custom", "", "custom metrics, as `event=basis-points,...`")
	set.StringVar(&f.gate.onRegression, "on-regression", "pause", "what a failed gate does: pause or rollback")
	set.Int64Var(&f.gate.minSample, "gate-min-sample", 0, "sessions each release needs before the gate decides (0: server default)")
	set.DurationVar(&f.gate.window, "gate-window", 0, "how far back the gate compares (0: server default)")
}

// rolloutSubcommand returns a subcommand's usage line and positional count.
func rolloutSubcommand(sub string) (usage string, positional int, known bool) {
	switch sub {
	case "create":
		return "Usage: plux rollout create --app <id> --env <key> --release <n> --stages <schedule> [flags]\n\nSchedules a rollout. Devices outside it keep the channel's release. Gate flags make a health gate.", 0, true
	case "show":
		return "Usage: plux rollout show [--json] <rollout-id>\n\nShows a rollout.", 1, true
	case "pause":
		return "Usage: plux rollout pause [--reason text] <rollout-id>\n\nStops a rollout from advancing.", 1, true
	case "resume":
		return "Usage: plux rollout resume <rollout-id>\n\nContinues a paused rollout with its current stage afresh.", 1, true
	case "abort":
		return "Usage: plux rollout abort [--reason text] <rollout-id>\n\nEnds a rollout and returns the channel to its previous release as a new release.", 1, true
	case "list":
		return "Usage: plux rollout list [--open] [--json]\n\nLists the app's rollouts, newest first.", 0, true
	}
	return "", 0, false
}

// rolloutRun connects and performs the subcommand.
func (e env) rolloutRun(c common, f rolloutFlags, sub string, rest []string) int {
	cl, err := e.connect(c, true)
	if err != nil {
		return e.fail("rollout", err)
	}
	client := pluxv1connect.NewRolloutServiceClient(e.httpClient, c.server, connect.WithInterceptors(headers{token: cl.token, org: c.org}))
	ctx := context.Background()
	if sub == "list" {
		return e.rolloutList(ctx, client, c, f)
	}
	var r *pluxv1.Rollout
	switch sub {
	case "create":
		r, err = rolloutCreate(ctx, cl, client, c, f)
	case "show":
		var res *connect.Response[pluxv1.GetRolloutResponse]
		if res, err = client.GetRollout(ctx, connect.NewRequest(&pluxv1.GetRolloutRequest{Id: rest[0]})); err == nil {
			r = res.Msg.GetRollout()
		}
	case "pause":
		var res *connect.Response[pluxv1.PauseRolloutResponse]
		if res, err = client.PauseRollout(ctx, connect.NewRequest(&pluxv1.PauseRolloutRequest{Id: rest[0], Reason: f.reason})); err == nil {
			r = res.Msg.GetRollout()
		}
	case "resume":
		var res *connect.Response[pluxv1.ResumeRolloutResponse]
		if res, err = client.ResumeRollout(ctx, connect.NewRequest(&pluxv1.ResumeRolloutRequest{Id: rest[0]})); err == nil {
			r = res.Msg.GetRollout()
		}
	default: // abort
		var res *connect.Response[pluxv1.AbortRolloutResponse]
		if res, err = client.AbortRollout(ctx, connect.NewRequest(&pluxv1.AbortRolloutRequest{Id: rest[0], Reason: f.reason})); err == nil {
			r = res.Msg.GetRollout()
		}
	}
	if err != nil {
		return e.fail("rollout "+sub, stepUp(err))
	}
	return e.emit(c.json, rolloutView(r), rolloutText(r))
}

// rolloutCreate builds and sends a create request.
func rolloutCreate(ctx context.Context, cl *clients, svc pluxv1connect.RolloutServiceClient, c common, f rolloutFlags) (*pluxv1.Rollout, error) {
	if f.env == "" || f.release <= 0 || f.stages == "" {
		return nil, usageError("--env, --release and --stages are required")
	}
	req, err := createRequest(f)
	if err != nil {
		return nil, err
	}
	envID, err := environmentID(ctx, cl, c.app, f.env)
	if err != nil {
		return nil, err
	}
	req.AppId, req.EnvironmentId = c.app, envID
	res, err := svc.CreateRollout(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, err //nolint:wrapcheck // reported as is
	}
	return res.Msg.GetRollout(), nil
}

// createRequest turns the flags into a request, reading the files they name.
func createRequest(f rolloutFlags) (*pluxv1.CreateRolloutRequest, error) {
	stages, err := parseStages(f.stages)
	if err != nil {
		return nil, err
	}
	req := &pluxv1.CreateRolloutRequest{
		Channel: f.channel, CandidateSequence: f.release, Stages: stages,
		AllowDevices: splitList(f.allow), DenyDevices: splitList(f.deny),
	}
	if req.Health, err = f.gate.build(); err != nil {
		return nil, err
	}
	if req.StartAt, err = parseTime("--start", f.start); err != nil {
		return nil, err
	}
	if req.EndAt, err = parseTime("--end", f.end); err != nil {
		return nil, err
	}
	if f.rule != "" {
		if req.Rule, err = readNamedFile(f.rule); err != nil {
			return nil, fmt.Errorf("read the rule: %w", err)
		}
	}
	for _, file := range splitList(f.segments) {
		doc, err := readNamedFile(file)
		if err != nil {
			return nil, fmt.Errorf("read a segment: %w", err)
		}
		req.Segments = append(req.Segments, doc)
	}
	return req, nil
}

// parseTime reads an optional RFC 3339 flag.
func parseTime(flagName, value string) (*timestamppb.Timestamp, error) {
	if value == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, usageError(flagName + " is an RFC 3339 time, such as 2026-11-01T09:00:00Z")
	}
	return timestamppb.New(t), nil
}

// build makes the gate of the flags, or nil when none is asked for.
func (g gateFlags) build() (*pluxv1.HealthGate, error) {
	custom, err := parseCustomGates(g.custom)
	if err != nil {
		return nil, err
	}
	if g.crashFree == 0 && g.errors == 0 && g.actions == 0 && g.functions == 0 && len(custom) == 0 {
		return nil, nil
	}
	out := &pluxv1.HealthGate{
		MinSample: g.minSample, WindowSeconds: int64(g.window / time.Second),
		MaxCrashFreeDropBasisPoints: int32(g.crashFree), MaxErrorRateIncreaseBasisPoints: int32(g.errors), //nolint:gosec // bounded by the server
		MaxActionErrorRateIncreaseBasisPoints: int32(g.actions), MaxFunctionErrorRateIncreaseBasisPoints: int32(g.functions), //nolint:gosec // bounded by the server
		Custom: custom,
	}
	switch g.onRegression {
	case "pause", "":
		out.Action = pluxv1.HealthAction_HEALTH_ACTION_PAUSE
	case "rollback":
		out.Action = pluxv1.HealthAction_HEALTH_ACTION_ROLLBACK
	default:
		return nil, usageError("--on-regression is pause or rollback")
	}
	return out, nil
}

// parseCustomGates reads event=basis-points pairs.
func parseCustomGates(v string) ([]*pluxv1.CustomMetricGate, error) {
	var out []*pluxv1.CustomMetricGate
	for _, item := range splitList(v) {
		event, bp, ok := strings.Cut(item, "=")
		n, err := strconv.ParseInt(bp, 10, 32)
		if !ok || event == "" || err != nil {
			return nil, usageError("--gate-custom takes event=basis-points pairs, such as checkout_failed=100")
		}
		out = append(out, &pluxv1.CustomMetricGate{Event: event, MaxIncreaseBasisPoints: int32(n)})
	}
	return out, nil
}

// parseStages reads a schedule such as "1%:1h,10%:1d,100%": each stage a share
// of devices, and how long it lasts; the last may omit the duration.
func parseStages(v string) ([]*pluxv1.Stage, error) {
	var out []*pluxv1.Stage
	for _, item := range splitList(v) {
		share, dur, hasDur := strings.Cut(item, ":")
		pct, err := strconv.ParseFloat(strings.TrimSuffix(share, "%"), 64)
		if err != nil || !strings.HasSuffix(share, "%") {
			return nil, usageError(fmt.Sprintf("stage %q: the share is a percentage such as 10%%", item))
		}
		st := &pluxv1.Stage{BasisPoints: int32(pct*100 + 0.5)} //nolint:gosec // validated by the server
		if hasDur {
			d, err := parseDuration(dur)
			if err != nil {
				return nil, usageError(fmt.Sprintf("stage %q: the duration is such as 90m, 12h or 2d", item))
			}
			st.DurationSeconds = int64(d / time.Second)
		}
		out = append(out, st)
	}
	if len(out) == 0 {
		return nil, usageError("--stages needs at least one stage")
	}
	return out, nil
}

// parseDuration reads a Go duration or a number of days such as 2d.
func parseDuration(v string) (time.Duration, error) {
	if days, ok := strings.CutSuffix(v, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("not a duration: %q", v)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("not a duration: %w", err)
	}
	return d, nil
}

// rolloutList lists the app's rollouts.
func (e env) rolloutList(ctx context.Context, client pluxv1connect.RolloutServiceClient, c common, f rolloutFlags) int {
	views := []map[string]any{}
	var text strings.Builder
	err := eachPage(func(token string) (string, error) {
		res, err := client.ListRollouts(ctx, connect.NewRequest(&pluxv1.ListRolloutsRequest{AppId: c.app, OnlyOpen: f.open, Page: &pluxv1.Page{PageSize: 100, PageToken: token}}))
		if err != nil {
			return "", err //nolint:wrapcheck // e.fail names the command
		}
		for _, r := range res.Msg.GetRollouts() {
			views = append(views, rolloutView(r))
			text.WriteString(rolloutLine(r) + "\n")
		}
		return res.Msg.GetPage().GetNextPageToken(), nil
	})
	if err != nil {
		return e.fail("rollout list", err)
	}
	return e.emit(c.json, map[string]any{"rollouts": views}, strings.TrimSuffix(text.String(), "\n"))
}

// rolloutLine is a rollout on one line.
func rolloutLine(r *pluxv1.Rollout) string {
	return fmt.Sprintf("%s  %s/%s  release %d  %s  %.2f%%", r.GetId(), r.GetEnvironmentKey(), r.GetChannel(), r.GetCandidateSequence(),
		strings.ToLower(strings.TrimPrefix(r.GetState().String(), "ROLLOUT_STATE_")), float64(r.GetBasisPoints())/100)
}

// rolloutText is a rollout in detail.
func rolloutText(r *pluxv1.Rollout) string {
	var b strings.Builder
	b.WriteString(rolloutLine(r))
	if r.GetPreviousSequence() > 0 {
		fmt.Fprintf(&b, "\n  devices outside the rollout keep release %d", r.GetPreviousSequence())
	}
	fmt.Fprintf(&b, "\n  stage %d of %d", r.GetStageIndex()+1, len(r.GetStages()))
	if r.GetStateReason() != "" {
		b.WriteString("\n  " + r.GetStateReason())
	}
	if r.GetRevertedSequence() > 0 {
		fmt.Fprintf(&b, "\n  reverted by release %d", r.GetRevertedSequence())
	}
	if len(r.GetLastCheck()) > 0 {
		fmt.Fprintf(&b, "\n  last health check %s: %s", r.GetLastCheckAt().AsTime().UTC().Format(time.RFC3339), r.GetLastCheck())
	}
	return b.String()
}

// rolloutView is a rollout for --json.
func rolloutView(r *pluxv1.Rollout) map[string]any {
	stages := make([]map[string]any, len(r.GetStages()))
	for i, s := range r.GetStages() {
		stages[i] = map[string]any{"basisPoints": s.GetBasisPoints(), "durationSeconds": s.GetDurationSeconds()}
	}
	v := map[string]any{
		"id": r.GetId(), "app": r.GetAppId(), "environment": r.GetEnvironmentKey(), "channel": r.GetChannel(),
		"candidate": r.GetCandidateSequence(), "previous": r.GetPreviousSequence(),
		"state": strings.ToLower(strings.TrimPrefix(r.GetState().String(), "ROLLOUT_STATE_")), "reason": r.GetStateReason(),
		"stages": stages, "stage": r.GetStageIndex(), "basisPoints": r.GetBasisPoints(), "reverted": r.GetRevertedSequence(),
	}
	if len(r.GetLastCheck()) > 0 {
		v["lastCheck"] = string(r.GetLastCheck())
	}
	return v
}

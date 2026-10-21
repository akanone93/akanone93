// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"regexp"
	"strings"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

const experimentUsage = `Usage: plux experiment create|list|get|delete|start|stop|results|holdout|promote [flags]

  create   Register a draft experiment from its experiment document
  list     List the app's experiments
  get      Show one experiment
  delete   Discard a draft
  start    Allocate the experiment to devices through the control document
  stop     Take it out of the control document
  results  Compare each variant with the control (valid at any time)
  holdout  Show or set the share of subjects kept out of every experiment
  promote  Make a variant the app and publish it
`

// experimentStates maps the spellings of a state to the wire form.
var experimentStates = map[string]pluxv1.ExperimentState{
	"draft": pluxv1.ExperimentState_EXPERIMENT_STATE_DRAFT, "running": pluxv1.ExperimentState_EXPERIMENT_STATE_RUNNING,
	"stopped": pluxv1.ExperimentState_EXPERIMENT_STATE_STOPPED, "concluded": pluxv1.ExperimentState_EXPERIMENT_STATE_CONCLUDED,
}

// experimentFlags are the flags of the experiment subcommands beyond the
// common ones.
type experimentFlags struct {
	env, state, label, notes string
	layerShare, percent      float64
	percentSet               bool
}

// uuidPattern is the form of an identifier, which tells it from a key.
var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// experiment administers and analyses experiments (ABT-001, ABT-003,
// ABT-004, ABT-005).
func (e env) experiment(args []string) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(e.stderr, experimentUsage)
		return exitUsage
	}
	sub := args[0]
	usage, positional, known := experimentSubcommand(sub)
	if !known {
		_, _ = fmt.Fprintf(e.stderr, "%s experiment: unknown subcommand %q\n\n%s", name, sub, experimentUsage)
		return exitUsage
	}
	var (
		c common
		f experimentFlags
	)
	set := flag.NewFlagSet("experiment "+sub, flag.ContinueOnError)
	c.register(set, true)
	set.StringVar(&f.env, "env", "", "the environment `key`")
	set.StringVar(&f.state, "state", "", "list only experiments in this `state`: draft, running, stopped or concluded")
	set.StringVar(&f.label, "label", "", "the release's label")
	set.StringVar(&f.notes, "notes", "", "the release's notes")
	set.Float64Var(&f.layerShare, "layer-share", 0, "the `percent` of the experiment's layer to take (default all of it)")
	set.Func("percent", "the holdout `percent`, 0 to 100", func(v string) error {
		f.percentSet = true
		if _, err := fmt.Sscanf(v, "%g", &f.percent); err != nil {
			return fmt.Errorf("parse the percent: %w", err)
		}
		return nil
	})
	rest, code, ok := parse(set, args[1:], e.stderr, usage, positional)
	if !ok {
		return code
	}
	if err := c.resolve(); err != nil {
		return e.fail("experiment", err)
	}
	if err := needApp(c); err != nil {
		return e.fail("experiment", err)
	}
	return e.experimentRun(c, f, sub, rest)
}

// experimentSubcommand returns a subcommand's usage line and positional
// count.
func experimentSubcommand(sub string) (usage string, positional int, known bool) {
	const ref = "<id|key>"
	switch sub {
	case "create":
		return "Usage: plux experiment create --env key [-C dir] [--json] <file>\n\nRegisters a draft from an experiment document (experiments/<key>.experiment.json). Nothing reaches a device until it is started.", 1, true
	case "list":
		return "Usage: plux experiment list [--env key] [--state s] [-C dir] [--json]\n\nLists the app's experiments.", 0, true
	case "get":
		return "Usage: plux experiment get [--env key] [-C dir] [--json] " + ref + "\n\nShows an experiment by identifier, or by key with --env.", 1, true
	case "delete":
		return "Usage: plux experiment delete [--env key] [-C dir] [--json] " + ref + "\n\nDiscards a draft.", 1, true
	case "start":
		return "Usage: plux experiment start [--env key] [--layer-share percent] [-C dir] [--json] " + ref + "\n\nWrites the allocation into the environment's control documents. Experiments of one layer are mutually exclusive (PLX-8200).", 1, true
	case "stop":
		return "Usage: plux experiment stop [--env key] [-C dir] [--json] " + ref + "\n\nTakes a running experiment out of the control documents.", 1, true
	case "results":
		return "Usage: plux experiment results [--env key] [-C dir] [--json] " + ref + "\n\nCompares each variant with the control with confidence sequences valid however often they are read; PLX-8201 until the minimum sample size is reached, and a sample ratio mismatch is PLX-8202.", 1, true
	case "holdout":
		return "Usage: plux experiment holdout --env key [--percent n] [-C dir] [--json]\n\nShows the share of subjects kept out of every experiment of the environment, or sets it with --percent (0 removes it).", 0, true
	case "promote":
		return "Usage: plux experiment promote --env key [--label text] [--notes text] [-C dir] [--json] " + ref + " <variant>\n\nMakes the variant the app: the project is rewritten with the winner as the default and the experiment's code paths removed, and a publish job makes the release through the normal path.", 2, true
	}
	return "", 0, false
}

// experimentRun connects and runs one subcommand.
func (e env) experimentRun(c common, f experimentFlags, sub string, rest []string) int {
	cl, err := e.connect(c, true)
	if err != nil {
		return e.fail("experiment", err)
	}
	ctx := context.Background()
	var envID string
	if f.env != "" {
		if envID, err = environmentID(ctx, cl, c.app, f.env); err != nil {
			return e.fail("experiment", err)
		}
	}
	client := pluxv1connect.NewExperimentServiceClient(e.httpClient, c.server, connect.WithInterceptors(headers{token: cl.token, org: c.org}))
	view, text, err := experimentCall(ctx, client, experimentTarget{app: c.app, env: envID}, f, sub, rest)
	if err != nil {
		return e.fail("experiment", stepUp(err))
	}
	return e.emit(c.json, view, text)
}

// experimentTarget names the app and, when given, the environment a call is
// about.
type experimentTarget struct{ app, env string }

// experimentCall performs the call a subcommand stands for.
func experimentCall(ctx context.Context, client pluxv1connect.ExperimentServiceClient, t experimentTarget, f experimentFlags, sub string, rest []string) (any, string, error) {
	switch sub {
	case "create":
		return experimentCreate(ctx, client, t, rest[0])
	case "list":
		return experimentList(ctx, client, t, f.state)
	case "holdout":
		return experimentHoldout(ctx, client, t, f)
	}
	id, err := experimentID(ctx, client, t, rest[0])
	if err != nil {
		return nil, "", err
	}
	switch sub {
	case "get":
		res, err := client.GetExperiment(ctx, connect.NewRequest(&pluxv1.GetExperimentRequest{Id: id}))
		return experimentOutcome(res, err)
	case "delete":
		if _, err := client.DeleteExperiment(ctx, connect.NewRequest(&pluxv1.DeleteExperimentRequest{Id: id})); err != nil {
			return nil, "", err //nolint:wrapcheck // reported as is
		}
		return map[string]any{"deleted": id}, "deleted " + rest[0], nil
	case "start":
		res, err := client.StartExperiment(ctx, connect.NewRequest(&pluxv1.StartExperimentRequest{Id: id, LayerSharePercent: f.layerShare}))
		return experimentOutcome(res, err)
	case "stop":
		res, err := client.StopExperiment(ctx, connect.NewRequest(&pluxv1.StopExperimentRequest{Id: id}))
		return experimentOutcome(res, err)
	case "results":
		return experimentResults(ctx, client, id)
	}
	return experimentPromote(ctx, client, t, f, id, rest[1])
}

// experimentOutcome renders an experiment or the error that came instead.
func experimentOutcome(res *connect.Response[pluxv1.ExperimentResponse], err error) (any, string, error) {
	if err != nil {
		return nil, "", err //nolint:wrapcheck // reported as is
	}
	e := res.Msg.GetExperiment()
	return experimentView(e), experimentText(e), nil
}

// experimentID resolves an identifier or, with an environment, a key.
func experimentID(ctx context.Context, client pluxv1connect.ExperimentServiceClient, t experimentTarget, arg string) (string, error) {
	if uuidPattern.MatchString(arg) {
		return arg, nil
	}
	if t.env == "" {
		return "", usageError(fmt.Sprintf("%q is not an identifier: pass --env to name an experiment by its key", arg))
	}
	res, err := client.ListExperiments(ctx, connect.NewRequest(&pluxv1.ListExperimentsRequest{AppId: t.app, EnvironmentId: t.env}))
	if err != nil {
		return "", err //nolint:wrapcheck // reported as is
	}
	for _, x := range res.Msg.GetExperiments() {
		if x.GetKey() == arg {
			return x.GetId(), nil
		}
	}
	return "", fmt.Errorf("no experiment %q in this environment", arg)
}

func experimentCreate(ctx context.Context, client pluxv1connect.ExperimentServiceClient, t experimentTarget, file string) (any, string, error) {
	if t.env == "" {
		return nil, "", usageError("--env names the environment")
	}
	doc, err := readNamedFile(file)
	if err != nil {
		return nil, "", fmt.Errorf("read the experiment document: %w", err)
	}
	res, err := client.CreateExperiment(ctx, connect.NewRequest(&pluxv1.CreateExperimentRequest{AppId: t.app, EnvironmentId: t.env, Document: doc}))
	if err != nil {
		return nil, "", err //nolint:wrapcheck // reported as is
	}
	return experimentOutcome(res, nil)
}

func experimentList(ctx context.Context, client pluxv1connect.ExperimentServiceClient, t experimentTarget, state string) (any, string, error) {
	req := &pluxv1.ListExperimentsRequest{AppId: t.app, EnvironmentId: t.env}
	if state != "" {
		s, ok := experimentStates[state]
		if !ok {
			return nil, "", usageError(fmt.Sprintf("%q is not a state: use draft, running, stopped or concluded", state))
		}
		req.State = s
	}
	res, err := client.ListExperiments(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, "", err //nolint:wrapcheck // reported as is
	}
	views := []map[string]any{}
	lines := []string{}
	for _, x := range res.Msg.GetExperiments() {
		views = append(views, experimentView(x))
		lines = append(lines, experimentText(x))
	}
	return views, strings.Join(lines, "\n"), nil
}

func experimentHoldout(ctx context.Context, client pluxv1connect.ExperimentServiceClient, t experimentTarget, f experimentFlags) (any, string, error) {
	if t.env == "" {
		return nil, "", usageError("--env names the environment")
	}
	var res *pluxv1.HoldoutResponse
	if f.percentSet {
		r, err := client.SetHoldout(ctx, connect.NewRequest(&pluxv1.SetHoldoutRequest{AppId: t.app, EnvironmentId: t.env, Percent: f.percent}))
		if err != nil {
			return nil, "", err //nolint:wrapcheck // reported as is
		}
		res = r.Msg
	} else {
		r, err := client.GetHoldout(ctx, connect.NewRequest(&pluxv1.GetHoldoutRequest{AppId: t.app, EnvironmentId: t.env}))
		if err != nil {
			return nil, "", err //nolint:wrapcheck // reported as is
		}
		res = r.Msg
	}
	return map[string]any{"percent": res.GetPercent()}, fmt.Sprintf("holdout %g%%", res.GetPercent()), nil
}

func experimentPromote(ctx context.Context, client pluxv1connect.ExperimentServiceClient, t experimentTarget, f experimentFlags, id, variant string) (any, string, error) {
	if t.env == "" {
		return nil, "", usageError("--env names the environment the release is made for")
	}
	res, err := client.PromoteExperiment(ctx, connect.NewRequest(&pluxv1.PromoteExperimentRequest{
		Id: id, Variant: variant, EnvironmentId: t.env, Label: f.label, Notes: f.notes,
	}))
	if err != nil {
		return nil, "", err //nolint:wrapcheck // reported as is
	}
	view := experimentView(res.Msg.GetExperiment())
	view["publishJob"] = res.Msg.GetPublishJobId()
	return view, fmt.Sprintf("%s\npublish job %s", experimentText(res.Msg.GetExperiment()), res.Msg.GetPublishJobId()), nil
}

func experimentResults(ctx context.Context, client pluxv1connect.ExperimentServiceClient, id string) (any, string, error) {
	res, err := client.GetResults(ctx, connect.NewRequest(&pluxv1.GetResultsRequest{Id: id}))
	if err != nil {
		return nil, "", err //nolint:wrapcheck // reported as is
	}
	r := res.Msg
	view := map[string]any{
		"experiment": r.GetKey(), "state": r.GetState().String(),
		"variants":    variantCountsView(r.GetVariants()),
		"sampleRatio": map[string]any{"pValue": r.GetSampleRatioPValue(), "mismatch": r.GetSampleRatioMismatch(), "code": r.GetSampleRatioCode()},
		"primary":     metricView(r.GetPrimary()),
	}
	guardrails := []map[string]any{}
	for _, g := range r.GetGuardrails() {
		guardrails = append(guardrails, metricView(g))
	}
	view["guardrails"] = guardrails
	return view, resultsText(r), nil
}

func variantCountsView(counts []*pluxv1.VariantCount) []map[string]any {
	out := []map[string]any{}
	for _, v := range counts {
		out = append(out, map[string]any{"name": v.GetName(), "control": v.GetControl(), "subjects": v.GetSubjects(), "share": v.GetShare()})
	}
	return out
}

func metricView(m *pluxv1.MetricResult) map[string]any {
	comparisons := []map[string]any{}
	for _, c := range m.GetComparisons() {
		comparisons = append(comparisons, map[string]any{
			"variant": c.GetVariant(), "diff": c.GetDiff(), "lower": c.GetLower(), "upper": c.GetUpper(), "pValue": c.GetPValue(),
			"relativeDiff": c.GetRelativeDiff(), "improved": c.GetImproved(), "worsened": c.GetWorsened(),
			"guardrailTripped": c.GetGuardrailTripped(), "cuped": c.GetCuped(), "varianceReduction": c.GetVarianceReduction(),
		})
	}
	return map[string]any{"event": m.GetEvent(), "kind": m.GetKind(), "direction": m.GetDirection(), "comparisons": comparisons}
}

func resultsText(r *pluxv1.GetResultsResponse) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%s)\n", r.GetKey(), strings.ToLower(strings.TrimPrefix(r.GetState().String(), "EXPERIMENT_STATE_")))
	for _, v := range r.GetVariants() {
		fmt.Fprintf(&b, "  %-12s %8d subjects (promised %.1f%%)\n", v.GetName(), v.GetSubjects(), v.GetShare()*100)
	}
	if r.GetSampleRatioMismatch() {
		fmt.Fprintf(&b, "WARNING PLX-%d: the split of subjects differs from the allocation (p = %.2g); do not trust the comparisons\n", r.GetSampleRatioCode(), r.GetSampleRatioPValue())
	}
	writeMetric(&b, "primary", r.GetPrimary())
	for _, g := range r.GetGuardrails() {
		writeMetric(&b, "guardrail", g)
	}
	return b.String()
}

func writeMetric(b *strings.Builder, role string, m *pluxv1.MetricResult) {
	fmt.Fprintf(b, "%s %s (%s, %s)\n", role, m.GetEvent(), m.GetKind(), m.GetDirection())
	for _, c := range m.GetComparisons() {
		verdict := "no clear difference"
		switch {
		case c.GetGuardrailTripped():
			verdict = "GUARDRAIL TRIPPED"
		case c.GetImproved():
			verdict = "better"
		case c.GetWorsened():
			verdict = "worse"
		}
		fmt.Fprintf(b, "  %-12s %+.4f [%+.4f, %+.4f] p %.3g  %s\n", c.GetVariant(), c.GetDiff(), c.GetLower(), c.GetUpper(), c.GetPValue(), verdict)
	}
}

func experimentView(e *pluxv1.Experiment) map[string]any {
	view := map[string]any{
		"id": e.GetId(), "key": e.GetKey(), "state": experimentStateName(e.GetState()), "layerStart": e.GetLayerStart(), "layerEnd": e.GetLayerEnd(),
	}
	if e.GetWinner() != "" {
		view["winner"] = e.GetWinner()
	}
	return view
}

func experimentText(e *pluxv1.Experiment) string {
	s := fmt.Sprintf("%s  %s  %s", e.GetId(), e.GetKey(), experimentStateName(e.GetState()))
	if e.GetLayerEnd() > 0 {
		s += fmt.Sprintf("  layer buckets %d-%d", e.GetLayerStart(), e.GetLayerEnd())
	}
	if e.GetWinner() != "" {
		s += "  winner " + e.GetWinner()
	}
	return s
}

func experimentStateName(s pluxv1.ExperimentState) string {
	for n, wire := range experimentStates {
		if wire == s {
			return n
		}
	}
	return "unknown"
}

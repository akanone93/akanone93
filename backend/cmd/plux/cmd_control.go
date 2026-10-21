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

	"github.com/nightCode42/plux3/backend/internal/controldoc"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

const controlUsage = `Usage: plux control get|kill|unkill|mandatory|rollback [flags]

  get         Show a channel's control document
  kill        Switch a plugin, page, actionGraph, dataSource or function off
  unkill      Switch it back on

kill, unkill and rollback take --break-glass --justification text to act as an emergency without approval.
  mandatory   Make a release mandatory, or withdraw it with --clear
  rollback    Force devices back to an earlier release, or withdraw it with --clear
`

// controlScopes maps the spellings of a kill switch scope to the wire form.
var controlScopes = map[controldoc.Scope]pluxv1.KillSwitchScope{
	controldoc.ScopePlugin:      pluxv1.KillSwitchScope_KILL_SWITCH_SCOPE_PLUGIN,
	controldoc.ScopePage:        pluxv1.KillSwitchScope_KILL_SWITCH_SCOPE_PAGE,
	controldoc.ScopeActionGraph: pluxv1.KillSwitchScope_KILL_SWITCH_SCOPE_ACTION_GRAPH,
	controldoc.ScopeDataSource:  pluxv1.KillSwitchScope_KILL_SWITCH_SCOPE_DATA_SOURCE,
	controldoc.ScopeFunction:    pluxv1.KillSwitchScope_KILL_SWITCH_SCOPE_FUNCTION,
}

// controlFlags are the flags of the control subcommands beyond the common
// ones.
type controlFlags struct {
	env, channel, message string
	release               int64
	clear                 bool
	bg                    breakGlassFlags
}

// control reads and changes a channel's signed control document: kill
// switches, the mandatory update and the forced rollback (REL-060,
// REL-070).
func (e env) control(args []string) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(e.stderr, controlUsage)
		return exitUsage
	}
	sub := args[0]
	usage, positional, known := controlSubcommand(sub)
	if !known {
		_, _ = fmt.Fprintf(e.stderr, "%s control: unknown subcommand %q\n\n%s", name, sub, controlUsage)
		return exitUsage
	}
	var (
		c common
		f controlFlags
	)
	set := flag.NewFlagSet("control "+sub, flag.ContinueOnError)
	c.register(set, true)
	set.StringVar(&f.env, "env", "", "the environment `key`")
	set.StringVar(&f.channel, "channel", "", "the channel `key` (default production)")
	set.StringVar(&f.message, "message", "", "the localisation `key` of the update screen's message")
	set.Int64Var(&f.release, "release", 0, "the release `sequence`")
	set.BoolVar(&f.clear, "clear", false, "withdraw instead of set")
	f.bg.register(set)
	rest, code, ok := parse(set, args[1:], e.stderr, usage, positional)
	if !ok {
		return code
	}
	if err := c.resolve(); err != nil {
		return e.fail("control", err)
	}
	if err := needApp(c); err != nil {
		return e.fail("control", err)
	}
	if f.env == "" {
		return e.fail("control", usageError("--env names the environment"))
	}
	if err := f.bg.check(); err != nil {
		return e.fail("control", err)
	}
	return e.controlRun(c, f, sub, rest)
}

// controlSubcommand returns a subcommand's usage line and positional count.
func controlSubcommand(sub string) (usage string, positional int, known bool) {
	switch sub {
	case "get":
		return "Usage: plux control get --env key [--channel key] [-C dir] [--json]\n\nShows the newest control document of a channel: its version, the kill switches, the mandatory update, the forced rollback and the expiry of the signed document.", 0, true
	case "kill":
		return "Usage: plux control kill --env key [--channel key] [--break-glass --justification text] [-C dir] [--json] <scope> <target>\n\nSwitches off a plugin, page, actionGraph, dataSource or function for a channel's devices. With --break-glass it is an emergency action: no approval and no change freeze, and a post-hoc review follows (GOV-024).", 2, true
	case "unkill":
		return "Usage: plux control unkill --env key [--channel key] [--break-glass --justification text] [-C dir] [--json] <scope> <target>\n\nSwitches it back on.", 2, true
	case "mandatory":
		return "Usage: plux control mandatory --env key [--channel key] (--release sequence [--message key] | --clear) [-C dir] [--json]\n\nMakes a release mandatory: devices activate it before they show another page. --clear withdraws it.", 0, true
	case "rollback":
		return "Usage: plux control rollback --env key [--channel key] (--release sequence | --clear) [--break-glass --justification text] [-C dir] [--json]\n\nForces devices back to an earlier release they retain. --clear withdraws it.", 0, true
	}
	return "", 0, false
}

// controlRun connects and runs one subcommand.
func (e env) controlRun(c common, f controlFlags, sub string, rest []string) int {
	cl, err := e.connect(c, true)
	if err != nil {
		return e.fail("control", err)
	}
	ctx := context.Background()
	envID, err := environmentID(ctx, cl, c.app, f.env)
	if err != nil {
		return e.fail("control", err)
	}
	client := pluxv1connect.NewControlServiceClient(e.httpClient, c.server, connect.WithInterceptors(e.headers(c.server, cl.token, c.org)))
	doc, err := controlCall(ctx, client, controlTarget{app: c.app, env: envID, channel: f.channel}, f, sub, rest)
	if err != nil {
		return e.fail("control", stepUp(err))
	}
	return e.emit(c.json, controlView(doc), controlText(doc))
}

// controlTarget names the channel a call is about.
type controlTarget struct{ app, env, channel string }

// controlCall performs the call a subcommand stands for.
func controlCall(ctx context.Context, client pluxv1connect.ControlServiceClient, t controlTarget, f controlFlags, sub string, rest []string) (*pluxv1.ControlDocument, error) {
	switch sub {
	case "get":
		res, err := client.GetControlDocument(ctx, connect.NewRequest(&pluxv1.GetControlDocumentRequest{AppId: t.app, EnvironmentId: t.env, ChannelKey: t.channel}))
		if err != nil {
			return nil, err //nolint:wrapcheck // reported as is
		}
		return res.Msg.GetDocument(), nil
	case "kill", "unkill":
		return controlSwitch(ctx, client, t, f.bg, sub == "kill", rest)
	}
	return controlRelease(ctx, client, t, f, sub == "mandatory")
}

// controlSwitch sets or clears a kill switch.
func controlSwitch(ctx context.Context, client pluxv1connect.ControlServiceClient, t controlTarget, bg breakGlassFlags, set bool, rest []string) (*pluxv1.ControlDocument, error) {
	scope, ok := controlScopes[controldoc.Scope(rest[0])]
	if !ok {
		return nil, usageError(fmt.Sprintf("%q is not a kill switch scope: use plugin, page, actionGraph, dataSource or function", rest[0]))
	}
	entry := &pluxv1.KillSwitchEntry{Scope: scope, Target: rest[1]}
	if set {
		return documentOf(client.SetKillSwitch(ctx, connect.NewRequest(&pluxv1.SetKillSwitchRequest{AppId: t.app, EnvironmentId: t.env, ChannelKey: t.channel, KillSwitch: entry, BreakGlass: bg.on, Justification: bg.justification})))
	}
	return documentOf(client.ClearKillSwitch(ctx, connect.NewRequest(&pluxv1.ClearKillSwitchRequest{AppId: t.app, EnvironmentId: t.env, ChannelKey: t.channel, KillSwitch: entry, BreakGlass: bg.on, Justification: bg.justification})))
}

// controlRelease sets or withdraws the mandatory update or the forced
// rollback.
func controlRelease(ctx context.Context, client pluxv1connect.ControlServiceClient, t controlTarget, f controlFlags, mandatory bool) (*pluxv1.ControlDocument, error) {
	if f.clear == (f.release != 0) {
		return nil, usageError("name the release with --release, or withdraw with --clear, not both and not neither")
	}
	switch {
	case mandatory && f.clear:
		return documentOf(client.ClearMandatoryUpdate(ctx, connect.NewRequest(&pluxv1.ClearMandatoryUpdateRequest{AppId: t.app, EnvironmentId: t.env, ChannelKey: t.channel})))
	case mandatory:
		return documentOf(client.SetMandatoryUpdate(ctx, connect.NewRequest(&pluxv1.SetMandatoryUpdateRequest{AppId: t.app, EnvironmentId: t.env, ChannelKey: t.channel, ReleaseSequence: f.release, MessageKey: f.message})))
	case f.clear:
		return documentOf(client.ClearForcedRollback(ctx, connect.NewRequest(&pluxv1.ClearForcedRollbackRequest{AppId: t.app, EnvironmentId: t.env, ChannelKey: t.channel, BreakGlass: f.bg.on, Justification: f.bg.justification})))
	default:
		return documentOf(client.SetForcedRollback(ctx, connect.NewRequest(&pluxv1.SetForcedRollbackRequest{AppId: t.app, EnvironmentId: t.env, ChannelKey: t.channel, ReleaseSequence: f.release, BreakGlass: f.bg.on, Justification: f.bg.justification})))
	}
}

// documentOf unwraps the document of a change's response.
func documentOf(res *connect.Response[pluxv1.ControlDocumentResponse], err error) (*pluxv1.ControlDocument, error) {
	if err != nil {
		return nil, err //nolint:wrapcheck // reported as is
	}
	return res.Msg.GetDocument(), nil
}

// controlView is the JSON form of a control document.
func controlView(d *pluxv1.ControlDocument) map[string]any {
	switches := []map[string]string{}
	for _, k := range d.GetKillSwitches() {
		switches = append(switches, map[string]string{"scope": scopeName(k.GetScope()), "target": k.GetTarget()})
	}
	view := map[string]any{
		"app": d.GetAppId(), "environment": d.GetEnvironmentId(), "channel": d.GetChannelKey(),
		"version": d.GetVersion(), "signedVersion": d.GetSignedVersion(), "killSwitches": switches,
	}
	if m := d.GetMandatoryUpdate(); m != nil {
		view["mandatoryUpdate"] = map[string]any{"releaseSequence": m.GetReleaseSequence(), "messageKey": m.GetMessageKey()}
	}
	if r := d.GetForcedRollback(); r != nil {
		view["forcedRollback"] = map[string]any{"releaseSequence": r.GetReleaseSequence()}
	}
	if d.GetExpiresAt() != nil {
		view["expiresAt"] = d.GetExpiresAt().AsTime().UTC().Format(time.RFC3339)
	}
	return view
}

// controlText is the human form of a control document.
func controlText(d *pluxv1.ControlDocument) string {
	var b strings.Builder
	fmt.Fprintf(&b, "version %d", d.GetVersion())
	switch {
	case d.GetSignedVersion() == 0:
		b.WriteString(" (waiting for its signature)")
	case d.GetSignedVersion() != d.GetVersion():
		fmt.Fprintf(&b, " (version %d is signed)", d.GetSignedVersion())
	}
	if d.GetExpiresAt() != nil {
		fmt.Fprintf(&b, ", signed document expires %s", d.GetExpiresAt().AsTime().UTC().Format(time.RFC3339))
	}
	b.WriteString("\n")
	for _, k := range d.GetKillSwitches() {
		fmt.Fprintf(&b, "kill  %s %s\n", scopeName(k.GetScope()), k.GetTarget())
	}
	if m := d.GetMandatoryUpdate(); m != nil {
		fmt.Fprintf(&b, "mandatory update to release %d\n", m.GetReleaseSequence())
	}
	if r := d.GetForcedRollback(); r != nil {
		fmt.Fprintf(&b, "forced rollback to release %d\n", r.GetReleaseSequence())
	}
	return b.String()
}

// scopeName is the spelling of a wire scope.
func scopeName(s pluxv1.KillSwitchScope) string {
	for scope, wire := range controlScopes {
		if wire == s {
			return string(scope)
		}
	}
	return "unknown"
}

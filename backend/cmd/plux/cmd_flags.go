// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

const flagsUsage = `Usage: plux flags list|get|set|delete --env key [flags]

  list    List an environment's feature flags
  get     Show one flag
  set     Create a flag, or change its default and per-segment values
  delete  Remove a flag; devices fall back to the default compiled into the app
`

// flagTypeNames maps the spellings of a flag type to the wire form.
var flagTypeNames = map[string]pluxv1.FlagType{
	"bool": pluxv1.FlagType_FLAG_TYPE_BOOL, "number": pluxv1.FlagType_FLAG_TYPE_NUMBER,
	"string": pluxv1.FlagType_FLAG_TYPE_STRING, "json": pluxv1.FlagType_FLAG_TYPE_JSON,
}

// flagTypeName is the spelling of a wire flag type.
func flagTypeName(t pluxv1.FlagType) string {
	for name, wire := range flagTypeNames {
		if wire == t {
			return name
		}
	}
	return "unknown"
}

// flagsFlags are the flags of the flags subcommands beyond the common ones.
type flagsFlags struct {
	env, typ, def, schemaFile, description string
	segments                               listFlag
	clearSegments                          bool
}

// flags administers the feature flags of an environment (ABT-006): typed
// values per segment that reach devices through the control document, with no
// release.
func (e env) flags(args []string) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(e.stderr, flagsUsage)
		return exitUsage
	}
	sub := args[0]
	usage, positional, known := flagsSubcommand(sub)
	if !known {
		_, _ = fmt.Fprintf(e.stderr, "%s flags: unknown subcommand %q\n\n%s", name, sub, flagsUsage)
		return exitUsage
	}
	var (
		c common
		f flagsFlags
	)
	set := flag.NewFlagSet("flags "+sub, flag.ContinueOnError)
	c.register(set, true)
	set.StringVar(&f.env, "env", "", "the environment `key`")
	set.StringVar(&f.typ, "type", "", "the flag's `type`: bool, number, string or json (required to create)")
	set.StringVar(&f.def, "default", "", "the default as `JSON`, such as false, 42, \"text\" or {\"a\":1}")
	set.StringVar(&f.schemaFile, "schema", "", "the `file` with the JSON Schema of a json flag's values")
	set.StringVar(&f.description, "description", "", "what the flag is for")
	set.Var(&f.segments, "segment", "a per-segment value `key=JSON`, in precedence order; repeat for more")
	set.BoolVar(&f.clearSegments, "clear-segments", false, "remove every per-segment value")
	rest, code, ok := parse(set, args[1:], e.stderr, usage, positional)
	if !ok {
		return code
	}
	if err := c.resolve(); err != nil {
		return e.fail("flags", err)
	}
	if err := needApp(c); err != nil {
		return e.fail("flags", err)
	}
	if f.env == "" {
		return e.fail("flags", usageError("--env names the environment"))
	}
	return e.flagsRun(c, f, sub, rest)
}

// flagsSubcommand returns a subcommand's usage line and positional count.
func flagsSubcommand(sub string) (usage string, positional int, known bool) {
	switch sub {
	case "list":
		return "Usage: plux flags list --env key [-C dir] [--json]\n\nLists the environment's flags with their types and defaults.", 0, true
	case "get":
		return "Usage: plux flags get --env key [-C dir] [--json] <flag>\n\nShows a flag with its per-segment values.", 1, true
	case "set":
		return "Usage: plux flags set --env key [--type t] [--default JSON] [--segment key=JSON]... [--schema file] [--description text] [-C dir] [--json] <flag>\n\n" +
			"Creates the flag, which needs --type and --default, or replaces what an existing flag says: an omitted --default or --segment keeps the current one. A flag's type cannot change (PLX-8203).", 1, true
	case "delete":
		return "Usage: plux flags delete --env key [-C dir] [--json] <flag>\n\nRemoves the flag from the environment.", 1, true
	}
	return "", 0, false
}

// flagsRun connects and runs one subcommand.
func (e env) flagsRun(c common, f flagsFlags, sub string, rest []string) int {
	cl, err := e.connect(c, true)
	if err != nil {
		return e.fail("flags", err)
	}
	ctx := context.Background()
	envID, err := environmentID(ctx, cl, c.app, f.env)
	if err != nil {
		return e.fail("flags", err)
	}
	client := pluxv1connect.NewFlagServiceClient(e.httpClient, c.server, connect.WithInterceptors(headers{token: cl.token, org: c.org}))
	t := flagTarget{app: c.app, env: envID}
	switch sub {
	case "list":
		res, err := client.ListFlags(ctx, connect.NewRequest(&pluxv1.ListFlagsRequest{AppId: t.app, EnvironmentId: t.env}))
		if err != nil {
			return e.fail("flags", stepUp(err))
		}
		return e.emit(c.json, flagsView(res.Msg.GetFlags()), flagsText(res.Msg.GetFlags()))
	case "delete":
		if _, err := client.DeleteFlag(ctx, connect.NewRequest(&pluxv1.DeleteFlagRequest{AppId: t.app, EnvironmentId: t.env, Key: rest[0]})); err != nil {
			return e.fail("flags", stepUp(err))
		}
		return e.emit(c.json, map[string]any{"deleted": rest[0]}, "deleted "+rest[0])
	}
	var flg *pluxv1.Flag
	if sub == "get" {
		res, err := client.GetFlag(ctx, connect.NewRequest(&pluxv1.GetFlagRequest{AppId: t.app, EnvironmentId: t.env, Key: rest[0]}))
		if err != nil {
			return e.fail("flags", stepUp(err))
		}
		flg = res.Msg.GetFlag()
	} else if flg, err = flagSet(ctx, client, t, f, rest[0]); err != nil {
		return e.fail("flags", stepUp(err))
	}
	return e.emit(c.json, flagView(flg), flagText(flg))
}

// flagTarget names the environment a call is about.
type flagTarget struct{ app, env string }

// flagSet creates the flag when it does not exist and updates it otherwise.
func flagSet(ctx context.Context, client pluxv1connect.FlagServiceClient, t flagTarget, f flagsFlags, key string) (*pluxv1.Flag, error) {
	current, err := client.GetFlag(ctx, connect.NewRequest(&pluxv1.GetFlagRequest{AppId: t.app, EnvironmentId: t.env, Key: key}))
	exists := err == nil
	if err != nil && connect.CodeOf(err) != connect.CodeNotFound {
		return nil, err //nolint:wrapcheck // reported as is
	}
	spec, err := flagSpecFrom(f)
	if err != nil {
		return nil, err
	}
	if !exists {
		if spec.GetType() == pluxv1.FlagType_FLAG_TYPE_UNSPECIFIED || spec.GetDefaultJson() == "" {
			return nil, usageError("a new flag needs --type and --default")
		}
		res, err := client.CreateFlag(ctx, connect.NewRequest(&pluxv1.CreateFlagRequest{AppId: t.app, EnvironmentId: t.env, Key: key, Spec: spec}))
		if err != nil {
			return nil, err //nolint:wrapcheck // reported as is
		}
		return res.Msg.GetFlag(), nil
	}
	have := current.Msg.GetFlag()
	if spec.GetDefaultJson() == "" {
		spec.DefaultJson = have.GetDefaultJson()
	}
	if len(spec.GetSegments()) == 0 && !f.clearSegments {
		spec.Segments = have.GetSegments()
	}
	if f.description == "" {
		spec.Description = have.GetDescription()
	}
	res, err := client.UpdateFlag(ctx, connect.NewRequest(&pluxv1.UpdateFlagRequest{AppId: t.app, EnvironmentId: t.env, Key: key, Spec: spec}))
	if err != nil {
		return nil, err //nolint:wrapcheck // reported as is
	}
	return res.Msg.GetFlag(), nil
}

// flagSpecFrom reads the spec the flags describe.
func flagSpecFrom(f flagsFlags) (*pluxv1.FlagSpec, error) {
	spec := &pluxv1.FlagSpec{Description: f.description, DefaultJson: f.def}
	if f.typ != "" {
		typ, ok := flagTypeNames[f.typ]
		if !ok {
			return nil, usageError(fmt.Sprintf("%q is not a flag type: use bool, number, string or json", f.typ))
		}
		spec.Type = typ
	}
	if f.schemaFile != "" {
		data, err := os.ReadFile(f.schemaFile)
		if err != nil {
			return nil, fmt.Errorf("read the schema: %w", err)
		}
		spec.SchemaJson = string(data)
	}
	for _, s := range f.segments {
		key, value, ok := strings.Cut(s, "=")
		if !ok || key == "" || value == "" {
			return nil, usageError(fmt.Sprintf("%q is not a segment value: write key=JSON", s))
		}
		spec.Segments = append(spec.Segments, &pluxv1.FlagSegmentValue{Segment: key, ValueJson: value})
	}
	return spec, nil
}

func flagView(f *pluxv1.Flag) map[string]any {
	segments := []map[string]string{}
	for _, s := range f.GetSegments() {
		segments = append(segments, map[string]string{"segment": s.GetSegment(), "value": s.GetValueJson()})
	}
	return map[string]any{
		"key": f.GetKey(), "type": flagTypeName(f.GetType()), "default": f.GetDefaultJson(),
		"description": f.GetDescription(), "segments": segments,
	}
}

func flagsView(flags []*pluxv1.Flag) []map[string]any {
	out := []map[string]any{}
	for _, f := range flags {
		out = append(out, flagView(f))
	}
	return out
}

func flagText(f *pluxv1.Flag) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s  %s  default %s", f.GetKey(), flagTypeName(f.GetType()), f.GetDefaultJson())
	for _, s := range f.GetSegments() {
		fmt.Fprintf(&b, "\n  %s: %s", s.GetSegment(), s.GetValueJson())
	}
	return b.String()
}

func flagsText(flags []*pluxv1.Flag) string {
	lines := make([]string, len(flags))
	for i, f := range flags {
		lines[i] = flagText(f)
	}
	return strings.Join(lines, "\n")
}

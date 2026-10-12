// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
)

const configUsage = "Usage: plux config set|get|list|delete [flags]\n"

// config runs plux config set|get|list|delete, the server-held
// configuration values of an environment (DAT-090).
func (e env) config(args []string) int {
	if len(args) < 1 {
		_, _ = fmt.Fprint(e.stderr, configUsage)
		return exitUsage
	}
	switch args[0] {
	case "set":
		return e.configSet(args[1:])
	case "get":
		return e.configGet(args[1:])
	case "list":
		return e.configList(args[1:])
	case "delete":
		return e.configDelete(args[1:])
	}
	_, _ = fmt.Fprint(e.stderr, configUsage)
	return exitUsage
}

var (
	configTierNames = map[string]pluxv1.ConfigTier{
		"server-bound": pluxv1.ConfigTier_CONFIG_TIER_SERVER_BOUND, "live": pluxv1.ConfigTier_CONFIG_TIER_LIVE,
	}
	configTypeNames = map[string]pluxv1.ConfigType{
		"string": pluxv1.ConfigType_CONFIG_TYPE_STRING, "integer": pluxv1.ConfigType_CONFIG_TYPE_INTEGER,
		"decimal": pluxv1.ConfigType_CONFIG_TYPE_DECIMAL, "boolean": pluxv1.ConfigType_CONFIG_TYPE_BOOLEAN,
	}
)

// tierName and typeName print a tier and a type the way the flags take
// them.
func tierName(t pluxv1.ConfigTier) string {
	for name, v := range configTierNames {
		if v == t {
			return name
		}
	}
	return "?"
}

func typeName(t pluxv1.ConfigType) string {
	for name, v := range configTypeNames {
		if v == t {
			return name
		}
	}
	return "?"
}

// configConnect resolves the common flags and the environment key.
func (e env) configConnect(c common, envKey string) (securityTarget, error) {
	return e.securityConnect(c, envKey)
}

// configSet creates or changes a value.
func (e env) configSet(args []string) int {
	var c common
	set := flag.NewFlagSet("config set", flag.ContinueOnError)
	c.register(set, true)
	envKey := set.String("env", "", "the environment `key` (default plux.json, then development)")
	tier := set.String("tier", "", "`server-bound` (bound into gateway requests, never sent to devices) or `live` (fetched by devices, held in memory)")
	typ := set.String("type", "string", "the value's `type`: string, integer, decimal or boolean")
	value := set.String("value", "", "the `value`; use --value-stdin for a sensitive one, so it stays out of the process list")
	fromStdin := set.Bool("value-stdin", false, "read the value from standard input")
	sensitive := set.Bool("sensitive", false, "seal the value at rest and never show it again (server-bound only)")
	strict := set.Bool("strict", false, "refuse gateway calls made with older live values, and fail closed offline (live only)")
	expected := set.Int64("expected-version", -1, "the `version` you read; 0 to create. Omit to set without the check")
	pos, code, ok := parseInterleaved(set, args, e.stderr, "Usage: plux config set <key> --tier server-bound|live [--type t] (--value v | --value-stdin) [--sensitive] [--strict] [--expected-version n] [--env key] [-C dir] [--app id] [--json]\n\nCreates or changes a server-held value. A live value is current when a device reads it; a server-bound one is current when the gateway uses it. Values in the project's environment variables may lag instead.")
	if !ok {
		return code
	}
	v, err := e.configValueFromFlags(pos, *tier, *typ, *value, *fromStdin, *sensitive, *strict)
	if err != nil {
		return e.fail("config set", err)
	}
	t, err := e.configConnect(c, *envKey)
	if err != nil {
		return e.fail("config set", err)
	}
	req := &pluxv1.SetConfigValueRequest{AppId: t.app, EnvironmentId: t.envID, Value: v}
	if *expected >= 0 {
		req.ExpectedVersion = expected
	}
	res, err := t.client.SetConfigValue(context.Background(), connect.NewRequest(req))
	if err != nil {
		return e.fail("config set", err)
	}
	return e.emit(c.json, configView(res.Msg.GetValue()), fmt.Sprintf("%s is now at version %d\n", v.GetKey(), res.Msg.GetValue().GetVersion()))
}

// configValueFromFlags checks the flags of config set and builds the value.
func (e env) configValueFromFlags(pos []string, tier, typ, value string, fromStdin, sensitive, strict bool) (*pluxv1.ConfigValue, error) {
	if len(pos) != 1 {
		return nil, usageError("name the value: plux config set <key> ...")
	}
	tierV, ok := configTierNames[tier]
	if !ok {
		return nil, usageError("--tier is server-bound or live")
	}
	typeV, ok := configTypeNames[typ]
	if !ok {
		return nil, usageError("--type is string, integer, decimal or boolean")
	}
	if fromStdin {
		raw, err := io.ReadAll(io.LimitReader(e.stdin, 1<<20))
		if err != nil {
			return nil, fmt.Errorf("read the value: %w", err)
		}
		value = strings.TrimSuffix(strings.TrimSuffix(string(raw), "\n"), "\r")
	} else if sensitive {
		return nil, usageError("give a sensitive value with --value-stdin, so it does not appear in the process list")
	}
	if value == "" {
		return nil, usageError("no value: --value or --value-stdin")
	}
	return &pluxv1.ConfigValue{Key: pos[0], Tier: tierV, Type: typeV, Value: value, Sensitive: sensitive, Strict: strict}, nil
}

// configGet shows one value.
func (e env) configGet(args []string) int {
	var c common
	set := flag.NewFlagSet("config get", flag.ContinueOnError)
	c.register(set, true)
	envKey := set.String("env", "", "the environment `key` (default plux.json, then development)")
	pos, code, ok := parseInterleaved(set, args, e.stderr, "Usage: plux config get <key> [--env key] [-C dir] [--app id] [--json]\n\nShows a value; a sensitive value's content is never shown.")
	if !ok {
		return code
	}
	if len(pos) != 1 {
		return e.fail("config get", usageError("name the value: plux config get <key>"))
	}
	t, err := e.configConnect(c, *envKey)
	if err != nil {
		return e.fail("config get", err)
	}
	res, err := t.client.GetConfigValue(context.Background(), connect.NewRequest(&pluxv1.GetConfigValueRequest{AppId: t.app, EnvironmentId: t.envID, Key: pos[0]}))
	if err != nil {
		return e.fail("config get", err)
	}
	v := res.Msg.GetValue()
	return e.emit(c.json, configView(v), configLine(v))
}

// configList lists an environment's values.
func (e env) configList(args []string) int {
	var c common
	set := flag.NewFlagSet("config list", flag.ContinueOnError)
	c.register(set, true)
	envKey := set.String("env", "", "the environment `key` (default plux.json, then development)")
	if _, code, ok := parse(set, args, e.stderr, "Usage: plux config list [--env key] [-C dir] [--app id] [--json]\n\nLists the values of an environment with their tier, type and version.", 0); !ok {
		return code
	}
	t, err := e.configConnect(c, *envKey)
	if err != nil {
		return e.fail("config list", err)
	}
	var all []*pluxv1.ConfigValue
	var epoch int64
	token := ""
	for {
		res, err := t.client.ListConfigValues(context.Background(), connect.NewRequest(&pluxv1.ListConfigValuesRequest{
			AppId: t.app, EnvironmentId: t.envID, Page: &pluxv1.Page{PageSize: 100, PageToken: token},
		}))
		if err != nil {
			return e.fail("config list", err)
		}
		all = append(all, res.Msg.GetValues()...)
		epoch = res.Msg.GetEpoch()
		if token = res.Msg.GetPage().GetNextPageToken(); token == "" {
			break
		}
	}
	views := make([]map[string]any, len(all))
	var text strings.Builder
	w := tabwriter.NewWriter(&text, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "KEY\tTIER\tTYPE\tFLAGS\tVERSION\tVALUE")
	for i, v := range all {
		views[i] = configView(v)
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\n", v.GetKey(), tierName(v.GetTier()), typeName(v.GetType()), configFlags(v), v.GetVersion(), shownValue(v))
	}
	_ = w.Flush()
	return e.emit(c.json, map[string]any{"epoch": epoch, "values": views}, text.String())
}

// configDelete removes a value.
func (e env) configDelete(args []string) int {
	var c common
	set := flag.NewFlagSet("config delete", flag.ContinueOnError)
	c.register(set, true)
	envKey := set.String("env", "", "the environment `key` (default plux.json, then development)")
	pos, code, ok := parseInterleaved(set, args, e.stderr, "Usage: plux config delete <key> [--env key] [-C dir] [--app id] [--json]\n\nRemoves a value. An operation that binds it is refused until it is set again.")
	if !ok {
		return code
	}
	if len(pos) != 1 {
		return e.fail("config delete", usageError("name the value: plux config delete <key>"))
	}
	t, err := e.configConnect(c, *envKey)
	if err != nil {
		return e.fail("config delete", err)
	}
	if _, err := t.client.DeleteConfigValue(context.Background(), connect.NewRequest(&pluxv1.DeleteConfigValueRequest{AppId: t.app, EnvironmentId: t.envID, Key: pos[0]})); err != nil {
		return e.fail("config delete", err)
	}
	return e.emit(c.json, map[string]any{"deleted": pos[0]}, "deleted "+pos[0]+"\n")
}

// configView is the JSON form of a value; a sensitive value has no content.
func configView(v *pluxv1.ConfigValue) map[string]any {
	out := map[string]any{
		"key": v.GetKey(), "tier": tierName(v.GetTier()), "type": typeName(v.GetType()), "sensitive": v.GetSensitive(),
		"strict": v.GetStrict(), "version": v.GetVersion(),
	}
	if !v.GetSensitive() {
		out["value"] = v.GetValue()
	}
	return out
}

func configFlags(v *pluxv1.ConfigValue) string {
	var f []string
	if v.GetSensitive() {
		f = append(f, "sensitive")
	}
	if v.GetStrict() {
		f = append(f, "strict")
	}
	return strings.Join(f, ",")
}

func shownValue(v *pluxv1.ConfigValue) string {
	if v.GetSensitive() {
		return "(sealed)"
	}
	return v.GetValue()
}

func configLine(v *pluxv1.ConfigValue) string {
	return fmt.Sprintf("%s = %s (%s, %s, version %d)\n", v.GetKey(), shownValue(v), tierName(v.GetTier()), typeName(v.GetType()), v.GetVersion())
}

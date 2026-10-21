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

const freezeUsage = "Usage: plux freeze list|put|delete [flags]\n"

// freezeFlags are the flags of `freeze put`.
type freezeFlags struct {
	name, envKey, zone, start, recurrence, reason string
	duration                                      time.Duration
}

// freeze lists, writes and removes change-freeze windows (GOV-025). Inside a
// window, changes to production are refused with PLX-8010 unless they are
// made through break-glass.
func (e env) freeze(args []string) int {
	var f freezeFlags
	c, rest, code, ok := e.approvalCmd("freeze", freezeUsage, args, map[string]subUsage{
		"list": {"Usage: plux freeze list [--json]\n\nLists the change-freeze windows of the organisation.", 0},
		"put": {"Usage: plux freeze put --name text --tz zone --start YYYY-MM-DDTHH:MM --duration 36h [--recurrence rule] [--env-key key] [--reason text] [--json]\n\n" +
			"Creates a window, or replaces the one of the same name. The start is wall-clock time in the zone. A recurrence is FREQ=DAILY or FREQ=WEEKLY;BYDAY=MO,TU,..., optionally with ;UNTIL=YYYYMMDD. Without --env-key it freezes every production environment.", 0},
		"delete": {"Usage: plux freeze delete <id>\n\nRemoves a window.", 1},
	}, func(set *flag.FlagSet) {
		set.StringVar(&f.name, "name", "", "the window's `name`")
		set.StringVar(&f.zone, "tz", "", "the IANA time `zone`, such as Europe/Berlin")
		set.StringVar(&f.start, "start", "", "the wall-clock start as `YYYY-MM-DDTHH:MM` in the zone")
		set.DurationVar(&f.duration, "duration", 0, "how long each occurrence lasts, in whole minutes")
		set.StringVar(&f.recurrence, "recurrence", "", "the `rule`; without it the window happens once")
		set.StringVar(&f.envKey, "env-key", "", "freeze this environment `key` only")
		set.StringVar(&f.reason, "reason", "", "why the window exists")
	})
	if !ok {
		return code
	}
	opts, err := e.authOptions(c)
	if err != nil {
		return e.fail("freeze", err)
	}
	client := pluxv1connect.NewFreezeServiceClient(e.httpClient, c.server, opts)
	switch args[0] {
	case "list":
		return e.freezeList(client, c)
	case "put":
		return e.freezePut(client, c, f)
	}
	if _, err := client.DeleteFreeze(context.Background(), connect.NewRequest(&pluxv1.DeleteFreezeRequest{Id: rest[0]})); err != nil {
		return e.fail("freeze delete", stepUp(err))
	}
	return e.emit(c.json, map[string]any{"deleted": rest[0]}, "Deleted freeze window "+rest[0])
}

// freezeList prints the windows.
func (e env) freezeList(client pluxv1connect.FreezeServiceClient, c common) int {
	res, err := client.ListFreezes(context.Background(), connect.NewRequest(&pluxv1.ListFreezesRequest{}))
	if err != nil {
		return e.fail("freeze list", err)
	}
	views, text := []map[string]any{}, strings.Builder{}
	for _, w := range res.Msg.GetFreezes() {
		views = append(views, freezeView(w))
		fmt.Fprintf(&text, "%s\t%s\t%s %s\t%dm\t%s\t%s\n", w.GetId(), w.GetName(), w.GetStart(), w.GetTimeZone(), w.GetDurationMinutes(), firstOf(w.GetRecurrence(), "once"), firstOf(w.GetEnvironmentKey(), "all production"))
	}
	return e.emit(c.json, map[string]any{"freezes": views}, text.String())
}

// freezePut writes a window.
func (e env) freezePut(client pluxv1connect.FreezeServiceClient, c common, f freezeFlags) int {
	if f.duration%time.Minute != 0 {
		return e.fail("freeze put", usageError("--duration is a whole number of minutes"))
	}
	res, err := client.PutFreeze(context.Background(), connect.NewRequest(&pluxv1.PutFreezeRequest{Freeze: &pluxv1.ChangeFreeze{
		Name: f.name, EnvironmentKey: f.envKey, TimeZone: f.zone, Start: f.start, DurationMinutes: int32(f.duration / time.Minute), //nolint:gosec // the server bounds it by governance.freezeWindowHours
		Recurrence: f.recurrence, Reason: f.reason,
	}}))
	if err != nil {
		return e.fail("freeze put", stepUp(err))
	}
	w := res.Msg.GetFreeze()
	return e.emit(c.json, freezeView(w), fmt.Sprintf("Wrote freeze window %q (%s).", w.GetName(), w.GetId()))
}

// freezeView is a window for --json.
func freezeView(w *pluxv1.ChangeFreeze) map[string]any {
	return map[string]any{
		"id": w.GetId(), "name": w.GetName(), "environmentKey": w.GetEnvironmentKey(), "timeZone": w.GetTimeZone(), "start": w.GetStart(),
		"durationMinutes": w.GetDurationMinutes(), "recurrence": w.GetRecurrence(), "reason": w.GetReason(),
	}
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

const (
	notificationsUsage = "Usage: plux notifications inbox|read|read-all|types|subscribe|unsubscribe|subscriptions|channels|channel-add|channel-delete|channel-test|deliveries [flags]\n"
	webhooksUsage      = "Usage: plux webhooks list|create|delete|rotate|deliveries [flags]\n"
	triggersUsage      = "Usage: plux fn triggers list|create|delete|hook-list|hook-create|hook-delete|hook-rotate [flags]\n"
)

// authOptions loads the caller's token for a server (SRV-062, SRV-063,
// FN-025).
func (e env) authOptions(c common) (connect.ClientOption, error) {
	token, err := e.creds.load(c.server)
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, authError("not signed in to " + c.server + ": run plux login or set " + tokenEnv)
	}
	return connect.WithInterceptors(e.headers(c.server, token, c.org)), nil
}

// secretFrom reads a credential from the environment variable a flag
// names, so that it never appears on a command line or in shell history.
func secretFrom(variable string) (string, error) {
	if variable == "" {
		return "", nil
	}
	v := os.Getenv(variable)
	if v == "" {
		return "", usageError("the environment variable " + variable + " is empty")
	}
	return v, nil
}

// settingsOf parses repeated key=value flags.
func settingsOf(pairs []string) (map[string]string, error) {
	out := map[string]string{}
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		if !ok || k == "" {
			return nil, usageError("a setting is key=value, not " + p)
		}
		out[k] = v
	}
	return out, nil
}

// listFlag collects a repeatable flag.
type listFlag []string

func (l *listFlag) String() string     { return strings.Join(*l, ",") }
func (l *listFlag) Set(v string) error { *l = append(*l, v); return nil }

func stamp(t *timestamppb.Timestamp) string {
	if t == nil {
		return "-"
	}
	return t.AsTime().UTC().Format(time.RFC3339)
}

func deliveryLine(d *pluxv1.Delivery) string {
	return fmt.Sprintf("%s\t%s\t%s\t%s\t%d attempts\t%s\n", d.GetId(), d.GetEventType(), d.GetState(), stamp(d.GetCreatedAt()), d.GetAttempts(), d.GetLastError())
}

func deliveryView(d *pluxv1.Delivery) map[string]any {
	return map[string]any{
		"id": d.GetId(), "kind": d.GetKind(), "targetId": d.GetTargetId(), "eventType": d.GetEventType(), "state": d.GetState(),
		"attempts": d.GetAttempts(), "lastStatus": d.GetLastStatus(), "lastError": d.GetLastError(),
		"createdAt": stamp(d.GetCreatedAt()), "deliveredAt": stamp(d.GetDeliveredAt()),
	}
}

// notifications reads the inbox and manages subscriptions and channels
// (SRV-062).
func (e env) notifications(args []string) int {
	var (
		unread, all                   bool
		event, channel, team, subject string
		chName, kind, secretEnv       string
		limit                         int
		settings                      listFlag
	)
	c, rest, code, ok := e.governanceCmd("notifications", notificationsUsage, args, map[string]subUsage{
		"inbox":          {"Usage: plux notifications inbox [--unread] [--limit n] [--json]\n\nLists your notifications, newest first.", 0},
		"read":           {"Usage: plux notifications read <id>\n\nMarks a notification read.", 1},
		"read-all":       {"Usage: plux notifications read-all\n\nMarks all your notifications read.", 0},
		"types":          {"Usage: plux notifications types [--json]\n\nLists the event types.", 0},
		"subscribe":      {"Usage: plux notifications subscribe --event type [--channel id] [--team id] [--user id] [--json]\n\nFollows an event type in your inbox, or through a channel. Subscribing a team or someone else needs notifications.manage.", 0},
		"unsubscribe":    {"Usage: plux notifications unsubscribe <id>\n\nRemoves a subscription.", 1},
		"subscriptions":  {"Usage: plux notifications subscriptions [--all] [--json]\n\nLists your subscriptions, or all with notifications.manage.", 0},
		"channels":       {"Usage: plux notifications channels [--json]\n\nLists the notification channels.", 0},
		"channel-add":    {"Usage: plux notifications channel-add --name text --kind smtp|slack|teams|telegram [--setting key=value]... [--secret-env VAR] [--json]\n\nAdds a channel. The credential (SMTP password, chat webhook URL or bot token) is read from the environment variable named, never from the command line.", 0},
		"channel-delete": {"Usage: plux notifications channel-delete <id>\n\nRemoves a channel.", 1},
		"channel-test":   {"Usage: plux notifications channel-test <id>\n\nSends a test message through a channel.", 1},
		"deliveries":     {"Usage: plux notifications deliveries [--channel id] [--limit n] [--json]\n\nReads the delivery log of channels.", 0},
	}, func(set *flag.FlagSet) {
		set.BoolVar(&unread, "unread", false, "list unread notifications only")
		set.BoolVar(&all, "all", false, "list the organisation's subscriptions")
		set.StringVar(&event, "event", "", "the event `type`")
		set.StringVar(&channel, "channel", "", "the channel `id`")
		set.StringVar(&team, "team", "", "the team `id` to subscribe")
		set.StringVar(&subject, "user", "", "the user `id` to subscribe")
		set.StringVar(&chName, "name", "", "the channel `name`")
		set.StringVar(&kind, "kind", "", "the channel `kind`")
		set.StringVar(&secretEnv, "secret-env", "", "the environment `variable` holding the credential")
		set.IntVar(&limit, "limit", 0, "the most rows to list")
		set.Var(&settings, "setting", "a channel setting as `key=value` (repeatable)")
	})
	if !ok {
		return code
	}
	opt, err := e.authOptions(c)
	if err != nil {
		return e.fail("notifications", err)
	}
	svc := pluxv1connect.NewNotificationServiceClient(e.httpClient, c.server, opt)
	ctx := context.Background()
	switch args[0] {
	case "inbox", "read", "read-all", "types":
		return e.notifyInbox(ctx, svc, c, args[0], rest, unread, int32(limit)) //nolint:gosec // a small row count
	case "subscribe", "unsubscribe", "subscriptions":
		return e.notifySubscriptions(ctx, svc, c, args[0], rest, &pluxv1.NotificationSubscription{
			EventType: event, ChannelId: channel, SubjectKind: kindOf(team, subject), SubjectId: team + subject,
		}, all)
	}
	return e.notifyChannels(ctx, svc, c, args[0], rest, channelInput{name: chName, kind: kind, secretEnv: secretEnv, settings: settings, filter: channel, limit: int32(limit)}) //nolint:gosec // a small row count
}

func kindOf(team, user string) string {
	switch {
	case team != "":
		return "team"
	case user != "":
		return "user"
	}
	return ""
}

func (e env) notifyInbox(ctx context.Context, svc pluxv1connect.NotificationServiceClient, c common, sub string, rest []string, unread bool, limit int32) int {
	switch sub {
	case "inbox":
		res, err := svc.ListInbox(ctx, connect.NewRequest(&pluxv1.ListInboxRequest{UnreadOnly: unread, Limit: limit}))
		if err != nil {
			return e.fail("notifications inbox", err)
		}
		views := []map[string]any{}
		var text strings.Builder
		for _, i := range res.Msg.GetItems() {
			mark := "*"
			if i.GetRead() {
				mark = " "
			}
			views = append(views, map[string]any{"id": i.GetId(), "eventType": i.GetEventType(), "title": i.GetTitle(), "body": i.GetBody(), "link": i.GetLink(), "read": i.GetRead(), "createdAt": stamp(i.GetCreatedAt())})
			fmt.Fprintf(&text, "%s %s\t%s\t%s\t%s\n", mark, i.GetId(), i.GetEventType(), stamp(i.GetCreatedAt()), i.GetTitle())
		}
		fmt.Fprintf(&text, "%d unread", res.Msg.GetUnread())
		return e.emit(c.json, map[string]any{"items": views, "unread": res.Msg.GetUnread()}, text.String())
	case "types":
		res, err := svc.ListEventTypes(ctx, connect.NewRequest(&pluxv1.ListEventTypesRequest{}))
		if err != nil {
			return e.fail("notifications types", err)
		}
		views := []map[string]any{}
		var text strings.Builder
		for _, t := range res.Msg.GetEventTypes() {
			views = append(views, map[string]any{"type": t.GetType(), "description": t.GetDescription(), "severity": t.GetSeverity()})
			fmt.Fprintf(&text, "%s\t%s\t%s\n", t.GetType(), t.GetSeverity(), t.GetDescription())
		}
		return e.emit(c.json, map[string]any{"eventTypes": views}, text.String())
	}
	req := &pluxv1.MarkReadRequest{}
	if sub == "read" {
		req.Ids = rest
	}
	res, err := svc.MarkRead(ctx, connect.NewRequest(req))
	if err != nil {
		return e.fail("notifications "+sub, err)
	}
	return e.emit(c.json, map[string]any{"marked": res.Msg.GetMarked()}, fmt.Sprintf("Marked %d read", res.Msg.GetMarked()))
}

func (e env) notifySubscriptions(ctx context.Context, svc pluxv1connect.NotificationServiceClient, c common, sub string, rest []string, in *pluxv1.NotificationSubscription, all bool) int {
	switch sub {
	case "subscribe":
		res, err := svc.Subscribe(ctx, connect.NewRequest(&pluxv1.SubscribeRequest{Subscription: in}))
		if err != nil {
			return e.fail("notifications subscribe", err)
		}
		s := res.Msg.GetSubscription()
		return e.emit(c.json, map[string]any{"id": s.GetId(), "eventType": s.GetEventType(), "channelId": s.GetChannelId()}, "Subscribed "+s.GetId())
	case "unsubscribe":
		if _, err := svc.Unsubscribe(ctx, connect.NewRequest(&pluxv1.UnsubscribeRequest{Id: rest[0]})); err != nil {
			return e.fail("notifications unsubscribe", err)
		}
		return e.emit(c.json, map[string]any{"deleted": rest[0]}, "Removed subscription "+rest[0])
	}
	res, err := svc.ListSubscriptions(ctx, connect.NewRequest(&pluxv1.ListSubscriptionsRequest{All: all}))
	if err != nil {
		return e.fail("notifications subscriptions", err)
	}
	views := []map[string]any{}
	var text strings.Builder
	for _, s := range res.Msg.GetSubscriptions() {
		views = append(views, map[string]any{"id": s.GetId(), "subjectKind": s.GetSubjectKind(), "subjectId": s.GetSubjectId(), "eventType": s.GetEventType(), "channelId": s.GetChannelId()})
		fmt.Fprintf(&text, "%s\t%s %s\t%s\t%s\n", s.GetId(), s.GetSubjectKind(), s.GetSubjectId(), s.GetEventType(), orInbox(s.GetChannelId()))
	}
	return e.emit(c.json, map[string]any{"subscriptions": views}, text.String())
}

func orInbox(channel string) string {
	if channel == "" {
		return "inbox"
	}
	return channel
}

// channelInput gathers the flags of the channel subcommands.
type channelInput struct {
	name, kind, secretEnv, filter string
	settings                      []string
	limit                         int32
}

func (e env) notifyChannels(ctx context.Context, svc pluxv1connect.NotificationServiceClient, c common, sub string, rest []string, in channelInput) int {
	switch sub {
	case "channel-add":
		settings, err := settingsOf(in.settings)
		if err != nil {
			return e.fail("notifications channel-add", err)
		}
		secret, err := secretFrom(in.secretEnv)
		if err != nil {
			return e.fail("notifications channel-add", err)
		}
		res, err := svc.CreateNotificationChannel(ctx, connect.NewRequest(&pluxv1.CreateNotificationChannelRequest{Name: in.name, Kind: in.kind, Settings: settings, Secret: secret}))
		if err != nil {
			return e.fail("notifications channel-add", err)
		}
		ch := res.Msg.GetChannel()
		return e.emit(c.json, map[string]any{"id": ch.GetId(), "name": ch.GetName(), "kind": ch.GetKind(), "hint": ch.GetHint()}, "Added channel "+ch.GetId())
	case "channel-delete":
		if _, err := svc.DeleteNotificationChannel(ctx, connect.NewRequest(&pluxv1.DeleteNotificationChannelRequest{Id: rest[0]})); err != nil {
			return e.fail("notifications channel-delete", err)
		}
		return e.emit(c.json, map[string]any{"deleted": rest[0]}, "Removed channel "+rest[0])
	case "channel-test":
		if _, err := svc.TestNotificationChannel(ctx, connect.NewRequest(&pluxv1.TestNotificationChannelRequest{Id: rest[0]})); err != nil {
			return e.fail("notifications channel-test", err)
		}
		return e.emit(c.json, map[string]any{"tested": rest[0]}, "Sent a test message through "+rest[0])
	case "deliveries":
		res, err := svc.ListDeliveries(ctx, connect.NewRequest(&pluxv1.ListDeliveriesRequest{TargetId: in.filter, Limit: in.limit}))
		if err != nil {
			return e.fail("notifications deliveries", err)
		}
		return e.emitDeliveries(c, res.Msg.GetDeliveries())
	}
	res, err := svc.ListNotificationChannels(ctx, connect.NewRequest(&pluxv1.ListNotificationChannelsRequest{}))
	if err != nil {
		return e.fail("notifications channels", err)
	}
	views := []map[string]any{}
	var text strings.Builder
	for _, ch := range res.Msg.GetChannels() {
		views = append(views, map[string]any{"id": ch.GetId(), "name": ch.GetName(), "kind": ch.GetKind(), "settings": ch.GetSettings(), "hint": ch.GetHint(), "enabled": ch.GetEnabled()})
		fmt.Fprintf(&text, "%s\t%s\t%s\t...%s\n", ch.GetId(), ch.GetName(), ch.GetKind(), ch.GetHint())
	}
	return e.emit(c.json, map[string]any{"channels": views}, text.String())
}

func (e env) emitDeliveries(c common, rows []*pluxv1.Delivery) int {
	views := []map[string]any{}
	var text strings.Builder
	for _, d := range rows {
		views = append(views, deliveryView(d))
		text.WriteString(deliveryLine(d))
	}
	return e.emit(c.json, map[string]any{"deliveries": views}, text.String())
}

// webhooks manages outbound webhooks (SRV-063).
func (e env) webhooks(args []string) int {
	var (
		hookName, url, types, id string
		overlap                  time.Duration
		limit                    int
	)
	c, rest, code, ok := e.governanceCmd("webhooks", webhooksUsage, args, map[string]subUsage{
		"list":       {"Usage: plux webhooks list [--json]\n\nLists the outbound webhooks.", 0},
		"create":     {"Usage: plux webhooks create --name text --url https://... [--events a,b] [--json]\n\nRegisters an endpoint. Its signing secret is printed once.", 0},
		"delete":     {"Usage: plux webhooks delete <id>\n\nRemoves an endpoint.", 1},
		"rotate":     {"Usage: plux webhooks rotate [--overlap 24h] [--json] <id>\n\nIssues a new secret; both sign until the overlap ends (a negative overlap ends the old secret at once).", 1},
		"deliveries": {"Usage: plux webhooks deliveries [--id endpoint] [--limit n] [--json]\n\nReads the delivery log of webhooks.", 0},
	}, func(set *flag.FlagSet) {
		set.StringVar(&hookName, "name", "", "the endpoint `name`")
		set.StringVar(&url, "url", "", "the endpoint `url`")
		set.StringVar(&types, "events", "", "the event types sent, comma-separated (default all)")
		set.StringVar(&id, "id", "", "an endpoint `id` to filter by")
		set.DurationVar(&overlap, "overlap", 0, "how long the old secret stays valid (default 24h)")
		set.IntVar(&limit, "limit", 0, "the most rows to list")
	})
	if !ok {
		return code
	}
	opt, err := e.authOptions(c)
	if err != nil {
		return e.fail("webhooks", err)
	}
	svc := pluxv1connect.NewWebhookServiceClient(e.httpClient, c.server, opt)
	ctx := context.Background()
	switch args[0] {
	case "create":
		res, err := svc.CreateWebhook(ctx, connect.NewRequest(&pluxv1.CreateWebhookRequest{Name: hookName, Url: url, EventTypes: splitList(types)}))
		if err != nil {
			return e.fail("webhooks create", err)
		}
		w := res.Msg.GetWebhook()
		return e.emit(c.json, map[string]any{"id": w.GetId(), "secret": res.Msg.GetSecret()}, fmt.Sprintf("Created webhook %s\nSecret (shown once): %s", w.GetId(), res.Msg.GetSecret()))
	case "delete":
		if _, err := svc.DeleteWebhook(ctx, connect.NewRequest(&pluxv1.DeleteWebhookRequest{Id: rest[0]})); err != nil {
			return e.fail("webhooks delete", err)
		}
		return e.emit(c.json, map[string]any{"deleted": rest[0]}, "Removed webhook "+rest[0])
	case "rotate":
		res, err := svc.RotateWebhookSecret(ctx, connect.NewRequest(&pluxv1.RotateWebhookSecretRequest{Id: rest[0], OverlapSeconds: int64(overlap / time.Second)}))
		if err != nil {
			return e.fail("webhooks rotate", err)
		}
		return e.emit(c.json, map[string]any{"secret": res.Msg.GetSecret()}, "New secret (shown once): "+res.Msg.GetSecret())
	case "deliveries":
		res, err := svc.ListWebhookDeliveries(ctx, connect.NewRequest(&pluxv1.ListWebhookDeliveriesRequest{TargetId: id, Limit: int32(limit)})) //nolint:gosec // a small row count
		if err != nil {
			return e.fail("webhooks deliveries", err)
		}
		return e.emitDeliveries(c, res.Msg.GetDeliveries())
	}
	res, err := svc.ListWebhooks(ctx, connect.NewRequest(&pluxv1.ListWebhooksRequest{}))
	if err != nil {
		return e.fail("webhooks list", err)
	}
	views := []map[string]any{}
	var text strings.Builder
	for _, w := range res.Msg.GetWebhooks() {
		views = append(views, map[string]any{"id": w.GetId(), "name": w.GetName(), "url": w.GetUrl(), "eventTypes": nonNil(w.GetEventTypes()), "enabled": w.GetEnabled(), "secretHint": w.GetSecretHint()})
		fmt.Fprintf(&text, "%s\t%s\t%s\t...%s\n", w.GetId(), w.GetName(), w.GetUrl(), w.GetSecretHint())
	}
	return e.emit(c.json, map[string]any{"webhooks": views}, text.String())
}

// fnTriggers manages the schedules and inbound webhooks of functions
// (FN-025).
func (e env) fnTriggers(args []string) int {
	var in triggerInput
	c, rest, code, ok := e.governanceCmd("fn triggers", triggersUsage, args, map[string]subUsage{
		"list":        {"Usage: plux fn triggers list --app id [--json]\n\nLists an app's schedules.", 0},
		"create":      {"Usage: plux fn triggers create --app id --env id --function name --alias name --cron \"*/5 * * * *\" [--tz zone] [--input json] [--json]\n\nSchedules a function alias.", 0},
		"delete":      {"Usage: plux fn triggers delete <id>\n\nRemoves a schedule.", 1},
		"hook-list":   {"Usage: plux fn triggers hook-list --app id [--json]\n\nLists an app's inbound webhooks.", 0},
		"hook-create": {"Usage: plux fn triggers hook-create --app id --env id --function name --alias name [--verifier name] [--secret-env VAR] [--json]\n\nOpens /hooks/<app>/<function>. A generated signing secret is printed once.", 0},
		"hook-delete": {"Usage: plux fn triggers hook-delete <id>\n\nCloses an inbound webhook.", 1},
		"hook-rotate": {"Usage: plux fn triggers hook-rotate [--overlap 24h] [--json] <id>\n\nIssues a new signing secret for a hook.", 1},
	}, in.register)
	if !ok {
		return code
	}
	opt, err := e.authOptions(c)
	if err != nil {
		return e.fail("fn triggers", err)
	}
	svc := pluxv1connect.NewFunctionTriggerServiceClient(e.httpClient, c.server, opt)
	return e.runTriggers(context.Background(), svc, c, args[0], rest, in)
}

// triggerInput gathers the flags of the trigger subcommands.
type triggerInput struct {
	app, env, function, alias, cron, tz, input, verifier, secretEnv string
	overlap                                                         time.Duration
}

func (t *triggerInput) register(set *flag.FlagSet) {
	set.StringVar(&t.app, "app", "", "the app `id`")
	set.StringVar(&t.env, "env", "", "the environment `id`")
	set.StringVar(&t.function, "function", "", "the function `name`")
	set.StringVar(&t.alias, "alias", "", "the alias `name` to run")
	set.StringVar(&t.cron, "cron", "", "five cron fields, or @hourly, @daily, @weekly, @monthly")
	set.StringVar(&t.tz, "tz", "", "an IANA time `zone` (default UTC)")
	set.StringVar(&t.input, "input", "", "the `json` passed to every run")
	set.StringVar(&t.verifier, "verifier", "", "a provider's verifier `name` (default standard-webhooks)")
	set.StringVar(&t.secretEnv, "secret-env", "", "the environment `variable` holding a provider's signing secret")
	set.DurationVar(&t.overlap, "overlap", 0, "how long the old secret stays valid (default 24h)")
}

func (e env) runTriggers(ctx context.Context, svc pluxv1connect.FunctionTriggerServiceClient, c common, sub string, rest []string, in triggerInput) int {
	switch sub {
	case "list":
		res, err := svc.ListTriggers(ctx, connect.NewRequest(&pluxv1.ListTriggersRequest{AppId: in.app}))
		if err != nil {
			return e.fail("fn triggers list", err)
		}
		views := []map[string]any{}
		var text strings.Builder
		for _, t := range res.Msg.GetTriggers() {
			views = append(views, map[string]any{"id": t.GetId(), "function": t.GetFunction(), "alias": t.GetAlias(), "expression": t.GetExpression(), "timezone": t.GetTimezone(), "enabled": t.GetEnabled(), "nextFireAt": stamp(t.GetNextFireAt()), "lastResult": t.GetLastResult()})
			fmt.Fprintf(&text, "%s\t%s@%s\t%s %s\tnext %s\t%s\n", t.GetId(), t.GetFunction(), t.GetAlias(), t.GetExpression(), t.GetTimezone(), stamp(t.GetNextFireAt()), t.GetLastResult())
		}
		return e.emit(c.json, map[string]any{"triggers": views}, text.String())
	case "create":
		res, err := svc.CreateTrigger(ctx, connect.NewRequest(&pluxv1.CreateTriggerRequest{
			AppId: in.app, EnvironmentId: in.env, Function: in.function, Alias: in.alias, Expression: in.cron, Timezone: in.tz, Input: []byte(in.input),
		}))
		if err != nil {
			return e.fail("fn triggers create", err)
		}
		t := res.Msg.GetTrigger()
		return e.emit(c.json, map[string]any{"id": t.GetId(), "nextFireAt": stamp(t.GetNextFireAt())}, fmt.Sprintf("Scheduled %s; next run %s", t.GetId(), stamp(t.GetNextFireAt())))
	case "delete":
		if _, err := svc.DeleteTrigger(ctx, connect.NewRequest(&pluxv1.DeleteTriggerRequest{Id: rest[0]})); err != nil {
			return e.fail("fn triggers delete", err)
		}
		return e.emit(c.json, map[string]any{"deleted": rest[0]}, "Removed schedule "+rest[0])
	}
	return e.runHooks(ctx, svc, c, sub, rest, in)
}

func (e env) runHooks(ctx context.Context, svc pluxv1connect.FunctionTriggerServiceClient, c common, sub string, rest []string, in triggerInput) int {
	switch sub {
	case "hook-create":
		secret, err := secretFrom(in.secretEnv)
		if err != nil {
			return e.fail("fn triggers hook-create", err)
		}
		res, err := svc.CreateHook(ctx, connect.NewRequest(&pluxv1.CreateHookRequest{
			AppId: in.app, EnvironmentId: in.env, Function: in.function, Alias: in.alias, Verifier: in.verifier, Secret: secret,
		}))
		if err != nil {
			return e.fail("fn triggers hook-create", err)
		}
		text := "Opened hook " + res.Msg.GetHook().GetId()
		if secret == "" {
			text += "\nSecret (shown once): " + res.Msg.GetSecret()
		}
		return e.emit(c.json, map[string]any{"id": res.Msg.GetHook().GetId(), "secret": res.Msg.GetSecret()}, text)
	case "hook-delete":
		if _, err := svc.DeleteHook(ctx, connect.NewRequest(&pluxv1.DeleteHookRequest{Id: rest[0]})); err != nil {
			return e.fail("fn triggers hook-delete", err)
		}
		return e.emit(c.json, map[string]any{"deleted": rest[0]}, "Closed hook "+rest[0])
	case "hook-rotate":
		res, err := svc.RotateHookSecret(ctx, connect.NewRequest(&pluxv1.RotateHookSecretRequest{Id: rest[0], OverlapSeconds: int64(in.overlap / time.Second)}))
		if err != nil {
			return e.fail("fn triggers hook-rotate", err)
		}
		return e.emit(c.json, map[string]any{"secret": res.Msg.GetSecret()}, "New secret (shown once): "+res.Msg.GetSecret())
	}
	res, err := svc.ListHooks(ctx, connect.NewRequest(&pluxv1.ListHooksRequest{AppId: in.app}))
	if err != nil {
		return e.fail("fn triggers hook-list", err)
	}
	views := []map[string]any{}
	var text strings.Builder
	for _, h := range res.Msg.GetHooks() {
		views = append(views, map[string]any{"id": h.GetId(), "function": h.GetFunction(), "alias": h.GetAlias(), "verifier": h.GetVerifier(), "enabled": h.GetEnabled()})
		fmt.Fprintf(&text, "%s\t/hooks/%s/%s\t%s@%s\t%s\n", h.GetId(), h.GetAppId(), h.GetFunction(), h.GetFunction(), h.GetAlias(), h.GetVerifier())
	}
	return e.emit(c.json, map[string]any{"hooks": views}, text.String())
}

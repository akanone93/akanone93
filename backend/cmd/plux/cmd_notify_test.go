// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

// fakeNotify records what the CLI asked and answers from memory.
type fakeNotify struct {
	pluxv1connect.UnimplementedNotificationServiceHandler
	pluxv1connect.UnimplementedWebhookServiceHandler
	pluxv1connect.UnimplementedFunctionTriggerServiceHandler
	calls *[]string
}

func (f fakeNotify) record(s string) { *f.calls = append(*f.calls, s) }

func (f fakeNotify) ListInbox(_ context.Context, r *connect.Request[pluxv1.ListInboxRequest]) (*connect.Response[pluxv1.ListInboxResponse], error) {
	if r.Header().Get("Authorization") != "Bearer plux_pat_notify" {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}
	f.record("inbox unread=" + boolText(r.Msg.GetUnreadOnly()))
	return connect.NewResponse(&pluxv1.ListInboxResponse{Unread: 1, Items: []*pluxv1.InboxItem{{Id: "n1", EventType: "release.published", Title: "Released"}}}), nil
}

func boolText(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func (f fakeNotify) MarkRead(_ context.Context, r *connect.Request[pluxv1.MarkReadRequest]) (*connect.Response[pluxv1.MarkReadResponse], error) {
	f.record("read " + strings.Join(r.Msg.GetIds(), ","))
	return connect.NewResponse(&pluxv1.MarkReadResponse{Marked: 1}), nil
}

func (f fakeNotify) CreateNotificationChannel(_ context.Context, r *connect.Request[pluxv1.CreateNotificationChannelRequest]) (*connect.Response[pluxv1.CreateNotificationChannelResponse], error) {
	m := r.Msg
	f.record("channel " + m.GetKind() + " host=" + m.GetSettings()["host"] + " secret=" + m.GetSecret())
	return connect.NewResponse(&pluxv1.CreateNotificationChannelResponse{Channel: &pluxv1.NotificationChannel{Id: "c1", Name: m.GetName(), Kind: m.GetKind(), Hint: "cret"}}), nil
}

func (fakeNotify) ListNotificationChannels(context.Context, *connect.Request[pluxv1.ListNotificationChannelsRequest]) (*connect.Response[pluxv1.ListNotificationChannelsResponse], error) {
	return connect.NewResponse(&pluxv1.ListNotificationChannelsResponse{Channels: []*pluxv1.NotificationChannel{{Id: "c1", Name: "ops", Kind: "slack", Hint: "cret"}}}), nil
}

func (f fakeNotify) CreateWebhook(_ context.Context, r *connect.Request[pluxv1.CreateWebhookRequest]) (*connect.Response[pluxv1.CreateWebhookResponse], error) {
	f.record("webhook " + r.Msg.GetUrl() + " " + strings.Join(r.Msg.GetEventTypes(), "+"))
	return connect.NewResponse(&pluxv1.CreateWebhookResponse{Webhook: &pluxv1.OutboundWebhook{Id: "w1"}, Secret: "whsec_abc"}), nil
}

func (f fakeNotify) RotateWebhookSecret(_ context.Context, r *connect.Request[pluxv1.RotateWebhookSecretRequest]) (*connect.Response[pluxv1.RotateWebhookSecretResponse], error) {
	f.record("rotate " + r.Msg.GetId())
	return connect.NewResponse(&pluxv1.RotateWebhookSecretResponse{Secret: "whsec_new"}), nil
}

func (f fakeNotify) CreateTrigger(_ context.Context, r *connect.Request[pluxv1.CreateTriggerRequest]) (*connect.Response[pluxv1.CreateTriggerResponse], error) {
	m := r.Msg
	f.record("trigger " + m.GetFunction() + "@" + m.GetAlias() + " " + m.GetExpression() + " " + m.GetTimezone())
	return connect.NewResponse(&pluxv1.CreateTriggerResponse{Trigger: &pluxv1.FunctionTrigger{Id: "t1"}}), nil
}

func (f fakeNotify) CreateHook(_ context.Context, r *connect.Request[pluxv1.CreateHookRequest]) (*connect.Response[pluxv1.CreateHookResponse], error) {
	f.record("hook " + r.Msg.GetFunction())
	return connect.NewResponse(&pluxv1.CreateHookResponse{Hook: &pluxv1.FunctionHook{Id: "h1"}, Secret: "whsec_hook"}), nil
}

// Verifies: SRV-062, SRV-063, FN-025.
// plux notifications, webhooks and fn triggers call their services with the
// caller's credential, read a channel credential from the environment and
// never from a flag, and show a generated secret once.
func TestNotifyCommands_SRV_062(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	t.Setenv(tokenEnv, "plux_pat_notify")
	t.Setenv(serverEnv, "")
	t.Setenv(orgEnv, "")
	t.Setenv("PLUX_TEST_SMTP_PASSWORD", "hunter2")
	var calls []string
	fake := fakeNotify{calls: &calls}
	mux := http.NewServeMux()
	mux.Handle(pluxv1connect.NewNotificationServiceHandler(fake))
	mux.Handle(pluxv1connect.NewWebhookServiceHandler(fake))
	mux.Handle(pluxv1connect.NewFunctionTriggerServiceHandler(fake))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	run := func(cmd func(env, []string) int, args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		e := newEnv(&out, &errOut)
		e.creds = credentials{dir: t.TempDir()}
		if len(args) > 0 {
			args = append([]string{args[0], "--server", srv.URL, "--org", "org-1"}, args[1:]...)
		}
		code := cmd(e, args)
		return code, out.String(), errOut.String()
	}

	if code, out, _ := run(env.notifications, "inbox", "--unread"); code != exitOK || !strings.Contains(out, "n1\trelease.published") || !strings.Contains(out, "1 unread") {
		t.Errorf("inbox: %d %q", code, out)
	}
	if code, _, _ := run(env.notifications, "read", "n1"); code != exitOK {
		t.Errorf("read: %d", code)
	}
	if code, _, _ := run(env.notifications, "channel-add", "--name", "mail", "--kind", "smtp", "--setting", "host=smtp.example.com", "--secret-env", "PLUX_TEST_SMTP_PASSWORD"); code != exitOK {
		t.Errorf("channel-add: %d", code)
	}
	if code, _, errOut := run(env.notifications, "channel-add", "--name", "x", "--kind", "smtp", "--secret-env", "PLUX_TEST_UNSET"); code == exitOK || errOut == "" {
		t.Errorf("an empty credential variable must fail: %d %q", code, errOut)
	}
	if code, _, _ := run(env.notifications, "channel-add", "--name", "x", "--kind", "smtp", "--setting", "novalue"); code != exitUsage && code != exitFailed {
		t.Errorf("a malformed setting must fail: %d", code)
	}
	if code, out, _ := run(env.notifications, "channels"); code != exitOK || !strings.Contains(out, "c1\tops\tslack\t...cret") || strings.Contains(out, "hunter2") {
		t.Errorf("channels: %d %q", code, out)
	}
	if code, out, _ := run(env.webhooks, "create", "--name", "ci", "--url", "https://example.com/hook", "--events", "release.published,rollout.paused"); code != exitOK || !strings.Contains(out, "Secret (shown once): whsec_abc") {
		t.Errorf("webhooks create: %d %q", code, out)
	}
	if code, out, _ := run(env.webhooks, "rotate", "w1"); code != exitOK || !strings.Contains(out, "whsec_new") {
		t.Errorf("webhooks rotate: %d %q", code, out)
	}
	if code, _, _ := run(env.fnTriggers, "create", "--app", "a1", "--env", "e1", "--function", "sweep", "--alias", "live", "--cron", "@daily", "--tz", "Europe/Berlin"); code != exitOK {
		t.Errorf("fn triggers create: %d", code)
	}
	if code, out, _ := run(env.fnTriggers, "hook-create", "--app", "a1", "--env", "e1", "--function", "pay", "--alias", "live"); code != exitOK || !strings.Contains(out, "whsec_hook") {
		t.Errorf("hook-create: %d %q", code, out)
	}
	want := []string{
		"inbox unread=yes", "read n1", "channel smtp host=smtp.example.com secret=hunter2",
		"webhook https://example.com/hook release.published+rollout.paused", "rotate w1",
		"trigger sweep@live @daily Europe/Berlin", "hook pay",
	}
	if strings.Join(calls, "|") != strings.Join(want, "|") {
		t.Errorf("calls = %q, want %q", calls, want)
	}
	if code, _, _ := run(env.notifications); code != exitUsage {
		t.Errorf("no subcommand: %d", code)
	}
}

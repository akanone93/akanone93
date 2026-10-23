// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

// FunctionTrigger serves FunctionTriggerService: the schedules and inbound
// webhooks of functions (FN-025, ADR-0081).
type FunctionTrigger struct{ h *Handlers }

var _ pluxv1connect.FunctionTriggerServiceHandler = FunctionTrigger{}

// FunctionTrigger returns the FunctionTriggerService handler.
func (h *Handlers) FunctionTrigger() FunctionTrigger { return FunctionTrigger{h: h} }

func triggerProto(t function.Trigger) *pluxv1.FunctionTrigger {
	return &pluxv1.FunctionTrigger{
		Id: t.ID, AppId: t.AppID, EnvironmentId: t.EnvironmentID, Function: t.Function, Alias: t.Alias,
		Expression: t.Expression, Timezone: t.Timezone, Input: t.Input, Enabled: t.Enabled,
		NextFireAt: ts(t.NextFireAt), LastFireAt: ts(t.LastFireAt), LastResult: t.LastResult,
	}
}

func hookProto(h function.Hook) *pluxv1.FunctionHook {
	return &pluxv1.FunctionHook{
		Id: h.ID, AppId: h.AppID, EnvironmentId: h.EnvironmentID, Function: h.Function, Alias: h.Alias,
		Verifier: h.Verifier, Enabled: h.Enabled, CreatedAt: ts(h.CreatedAt),
	}
}

// CreateTrigger schedules a function alias.
func (s FunctionTrigger) CreateTrigger(ctx context.Context, req *connect.Request[pluxv1.CreateTriggerRequest]) (*connect.Response[pluxv1.CreateTriggerResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.CreateTriggerResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		t, err := s.h.Functions.CreateTrigger(ctx, p, function.TriggerSpec{
			AppID: m.GetAppId(), EnvironmentID: m.GetEnvironmentId(), Function: m.GetFunction(), Alias: m.GetAlias(),
			Expression: m.GetExpression(), Timezone: m.GetTimezone(), Input: m.GetInput(),
		})
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.CreateTriggerResponse{Trigger: triggerProto(t)}, nil
	})
}

// ListTriggers lists an app's schedules.
func (s FunctionTrigger) ListTriggers(ctx context.Context, req *connect.Request[pluxv1.ListTriggersRequest]) (*connect.Response[pluxv1.ListTriggersResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListTriggersResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		triggers, err := s.h.Functions.ListTriggers(ctx, p, req.Msg.GetAppId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListTriggersResponse{}
		for _, t := range triggers {
			out.Triggers = append(out.Triggers, triggerProto(t))
		}
		return out, nil
	})
}

// DeleteTrigger removes a schedule.
func (s FunctionTrigger) DeleteTrigger(ctx context.Context, req *connect.Request[pluxv1.DeleteTriggerRequest]) (*connect.Response[pluxv1.DeleteTriggerResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.DeleteTriggerResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		if err := s.h.Functions.DeleteTrigger(ctx, p, req.Msg.GetId()); err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.DeleteTriggerResponse{}, nil
	})
}

// CreateHook opens an inbound webhook for a function alias.
func (s FunctionTrigger) CreateHook(ctx context.Context, req *connect.Request[pluxv1.CreateHookRequest]) (*connect.Response[pluxv1.CreateHookResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.CreateHookResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		hook, secret, err := s.h.Functions.CreateHook(ctx, p, function.HookSpec{
			AppID: m.GetAppId(), EnvironmentID: m.GetEnvironmentId(), Function: m.GetFunction(), Alias: m.GetAlias(),
			Verifier: m.GetVerifier(), Secret: m.GetSecret(),
		})
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.CreateHookResponse{Hook: hookProto(hook), Secret: secret}, nil
	})
}

// ListHooks lists an app's inbound webhooks.
func (s FunctionTrigger) ListHooks(ctx context.Context, req *connect.Request[pluxv1.ListHooksRequest]) (*connect.Response[pluxv1.ListHooksResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListHooksResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		hooks, err := s.h.Functions.ListHooks(ctx, p, req.Msg.GetAppId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListHooksResponse{}
		for _, h := range hooks {
			out.Hooks = append(out.Hooks, hookProto(h))
		}
		return out, nil
	})
}

// DeleteHook closes an inbound webhook.
func (s FunctionTrigger) DeleteHook(ctx context.Context, req *connect.Request[pluxv1.DeleteHookRequest]) (*connect.Response[pluxv1.DeleteHookResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.DeleteHookResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		if err := s.h.Functions.DeleteHook(ctx, p, req.Msg.GetId()); err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.DeleteHookResponse{}, nil
	})
}

// RotateHookSecret issues a new secret for a hook.
func (s FunctionTrigger) RotateHookSecret(ctx context.Context, req *connect.Request[pluxv1.RotateHookSecretRequest]) (*connect.Response[pluxv1.RotateHookSecretResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.RotateHookSecretResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		secret, err := s.h.Functions.RotateHookSecret(ctx, p, req.Msg.GetId(), overlapOf(req.Msg.GetOverlapSeconds()))
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.RotateHookSecretResponse{Secret: secret}, nil
	})
}

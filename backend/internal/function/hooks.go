// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/notify"
	"github.com/nightCode42/plux3/backend/internal/notify/webhook"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// InboundRequest is what a verifier sees of a request to a hook.
type InboundRequest struct {
	Header http.Header
	Body   []byte
}

// Verifier authenticates an inbound request against the hook's secrets
// (FN-025). It returns the message's unique identifier, which the hook
// remembers to refuse a replay, and fails with PLX-7151 for a bad signature
// or PLX-7152 for a message outside the time it accepts. A payment
// provider's scheme plugs in here (PAY-006). A verifier must compare
// signatures in constant time and must not trust anything before the
// signature checks.
type Verifier interface {
	Verify(r InboundRequest, secrets []string, now time.Time) (messageID string, err error)
}

// standardVerifier is the Standard Webhooks verifier with the default
// five-minute tolerance.
type standardVerifier struct{}

func (standardVerifier) Verify(r InboundRequest, secrets []string, now time.Time) (string, error) {
	m, err := webhook.Verify(secrets, r.Header, r.Body, now, webhook.DefaultTolerance)
	if err != nil {
		return "", err //nolint:wrapcheck // a domain error
	}
	return m.ID, nil
}

// seenRetention is how long a message identifier is remembered: well past
// every verifier's tolerance.
const seenRetention = time.Hour

// HookHandler serves POST /hooks/{app}/{function} (FN-025).
type HookHandler struct {
	svc *Service
}

// HookHandler returns the handler for inbound hooks. Mount it at
// "POST /hooks/{app}/{function}".
func (s *Service) HookHandler() *HookHandler { return &HookHandler{svc: s} }

// hookTarget is what a request resolves to before its body is read.
type hookTarget struct {
	hook    dbgen.FunctionHook
	secrets []string
	limit   int64
}

// ServeHTTP verifies, de-duplicates and runs. The order matters: nothing
// about the payload is trusted before the signature checks, and the
// message ID is remembered only after it does, so an attacker cannot use
// up the IDs of real messages.
func (h *HookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s := h.svc
	ctx := r.Context()
	inv := s.serverInvoker()
	if inv == nil {
		reply(w, plxerr.New(plxerr.FunctionNotFound, "this server does not run functions"))
		return
	}
	appID, name := r.PathValue("app"), r.PathValue("function")
	target, err := s.resolveHook(ctx, appID, name)
	if err != nil {
		reply(w, err)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, target.limit))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			reply(w, plxerr.New(plxerr.FunctionInputTooLarge, "the body is at most function.inputSize = %d bytes", target.limit))
			return
		}
		reply(w, plxerr.New(plxerr.InvalidFormat, "the body could not be read"))
		return
	}
	v := s.verifier(target.hook.Verifier)
	if v == nil {
		reply(w, plxerr.New(plxerr.WebhookSignatureInvalid, "the hook has no verifier"))
		return
	}
	id, err := v.Verify(InboundRequest{Header: r.Header, Body: body}, target.secrets, s.o.Now())
	if err != nil {
		reply(w, err)
		return
	}
	if err := s.markSeen(ctx, target.hook, id); err != nil {
		reply(w, err)
		return
	}
	res, err := inv.InvokeServer(ctx, ServerCall{
		OrganizationID: storage.ID(target.hook.OrganizationID), AppID: appID, EnvironmentID: storage.ID(target.hook.EnvironmentID),
		Function: name, Alias: target.hook.Alias, Input: body, Traceparent: r.Header.Get("traceparent"),
	})
	if err != nil {
		reply(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if res.Business != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(res.Business)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(res.Output)
}

// resolveHook finds the hook, its secrets and the body limit. The lookup
// runs in the 'hook' scope, which sees a hook by app and name before its
// organisation is known; the transaction is then rescoped to that
// organisation for everything else, so the scope reaches nothing more.
func (s *Service) resolveHook(ctx context.Context, appID, name string) (hookTarget, error) {
	notFound := plxerr.New(plxerr.ResourceNotFound, "no such hook")
	app, err := storage.UUID(appID)
	if err != nil || ValidateName(name) != nil {
		return hookTarget{}, notFound
	}
	var t hookTarget
	err = s.inTenant(ctx, storage.Tenant{Scope: storage.ScopeHook}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, err := q.GetFunctionHookByName(ctx, dbgen.GetFunctionHookByNameParams{AppID: app, FunctionName: name})
		if errors.Is(err, pgx.ErrNoRows) {
			return notFound
		}
		if err != nil {
			return failure(err, "hook")
		}
		if !row.Enabled {
			return notFound
		}
		if err := storage.Rescope(ctx, tx, storage.Tenant{OrganizationID: storage.ID(row.OrganizationID)}); err != nil {
			return err //nolint:wrapcheck // Rescope wraps its own failures
		}
		lim, err := s.o.Limits(ctx, tx, storage.ID(row.OrganizationID), appID)
		if err != nil {
			return err
		}
		secrets, err := s.hookSecrets(ctx, q, row)
		if err != nil {
			return err
		}
		t = hookTarget{hook: row, secrets: secrets, limit: lim.Get(limits.FunctionInputSize)}
		return nil
	})
	return t, err
}

func (s *Service) hookSecrets(ctx context.Context, q *dbgen.Queries, row dbgen.FunctionHook) ([]string, error) {
	if s.o.Secrets == nil {
		return nil, plxerr.New(plxerr.WebhookSignatureInvalid, "the hook has no secret")
	}
	secrets, err := s.o.Secrets.ActiveSecrets(ctx, q, notify.OwnerHook, row.ID, s.o.Now())
	if err != nil {
		return nil, err //nolint:wrapcheck // a domain error
	}
	return secrets, nil
}

func (s *Service) verifier(name string) Verifier {
	if name == StandardWebhooks {
		return standardVerifier{}
	}
	return s.o.Verifiers[name]
}

// markSeen remembers a message identifier, or reports a replay as PLX-7152.
func (s *Service) markSeen(ctx context.Context, hook dbgen.FunctionHook, id string) error {
	if id == "" || len(id) > 256 {
		return plxerr.New(plxerr.WebhookSignatureInvalid, "the message has no usable identifier")
	}
	return s.inTenant(ctx, storage.Tenant{OrganizationID: storage.ID(hook.OrganizationID)}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		n, err := q.InsertHookSeen(ctx, dbgen.InsertHookSeenParams{HookID: hook.ID, WebhookID: id, OrganizationID: hook.OrganizationID})
		if err != nil {
			return failure(err, "hook")
		}
		if n == 0 {
			return plxerr.New(plxerr.WebhookReplayed, "this message was already received")
		}
		return nil
	})
}

// PurgeHooks forgets message identifiers past their retention.
func (s *Service) PurgeHooks(ctx context.Context, org string) (int64, error) {
	var n int64
	err := s.inTenant(ctx, storage.Tenant{OrganizationID: org}, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		n, err = dbgen.New(tx).PurgeHookSeen(ctx, storage.Timestamp(s.o.Now().Add(-seenRetention)))
		return failure(err, "hook")
	})
	if err != nil {
		return 0, err
	}
	fires, err := s.purgeFires(ctx, org)
	return n + fires, err
}

func (s *Service) purgeFires(ctx context.Context, org string) (int64, error) {
	var n int64
	err := s.inTenant(ctx, storage.Tenant{OrganizationID: org}, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		n, err = dbgen.New(tx).PurgeTriggerFires(ctx, storage.Timestamp(s.o.Now().Add(-30*24*time.Hour)))
		return failure(err, "trigger")
	})
	return n, err
}

// reply writes an error as the status a webhook sender understands, with
// the PLX code and nothing about the cause.
func reply(w http.ResponseWriter, err error) {
	code, _ := plxerr.CodeOf(err)
	status := http.StatusInternalServerError
	switch code {
	case plxerr.WebhookSignatureInvalid:
		status = http.StatusUnauthorized
	case plxerr.WebhookReplayed:
		status = http.StatusConflict
	case plxerr.ResourceNotFound, plxerr.FunctionNotFound, plxerr.FunctionAliasNotFound:
		status = http.StatusNotFound
	case plxerr.FunctionInputTooLarge:
		status = http.StatusRequestEntityTooLarge
	case plxerr.FunctionRateLimited:
		status = http.StatusTooManyRequests
	case plxerr.AssuranceInsufficient, plxerr.FunctionCapabilityDenied:
		status = http.StatusForbidden
	case plxerr.InvalidFormat:
		status = http.StatusBadRequest
	default:
		if code != 0 {
			status = http.StatusBadGateway
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	body := map[string]any{"code": "PLX-" + strconv.Itoa(int(code))}
	if code == 0 {
		body["code"] = "internal"
	}
	_ = json.NewEncoder(w).Encode(body)
}

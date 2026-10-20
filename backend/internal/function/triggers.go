// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/notify"
	"github.com/nightCode42/plux3/backend/internal/notify/webhook"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// HookSecrets keeps the signing secrets of inbound hooks; *notify.Service
// is one, so a hook's secret is stored and rotated like an outbound
// webhook's (FN-025, SRV-063).
type HookSecrets interface {
	SealSecret(ctx context.Context, q *dbgen.Queries, org pgtype.UUID, owner string, ownerID pgtype.UUID, secret string) (string, error)
	ActiveSecrets(ctx context.Context, q *dbgen.Queries, owner string, ownerID pgtype.UUID, now time.Time) ([]string, error)
	RotateSecret(ctx context.Context, q *dbgen.Queries, org pgtype.UUID, owner string, ownerID pgtype.UUID, overlap time.Duration) (string, error)
}

// triggerBatch is how many due schedules one pass takes.
const triggerBatch = 50

// Trigger is a cron schedule on a function alias (FN-025).
type Trigger struct {
	ID            string
	AppID         string
	EnvironmentID string
	Function      string
	Alias         string
	Expression    string
	Timezone      string
	Input         []byte
	Enabled       bool
	NextFireAt    time.Time
	LastFireAt    time.Time
	LastResult    string
}

// TriggerSpec is a schedule to create.
type TriggerSpec struct {
	AppID         string
	EnvironmentID string
	Function      string
	// Alias is required: a schedule runs what an alias points at, so a
	// deploy moves it deliberately.
	Alias string
	// Expression is five cron fields or @hourly, @daily and the like.
	Expression string
	// Timezone is "UTC" (empty) or an IANA zone name.
	Timezone string
	// Input is passed to the function on every run.
	Input []byte
}

func triggerOf(r dbgen.FunctionTrigger) Trigger {
	t := Trigger{
		ID: storage.ID(r.ID), AppID: storage.ID(r.AppID), EnvironmentID: storage.ID(r.EnvironmentID), Function: r.FunctionName, Alias: r.Alias,
		Expression: r.Expression, Timezone: r.Timezone, Input: r.Input, Enabled: r.Enabled, NextFireAt: storage.Time(r.NextFireAt), LastResult: r.LastResult,
	}
	if r.LastFireAt.Valid {
		t.LastFireAt = storage.Time(r.LastFireAt)
	}
	return t
}

// authorizeSome is the first check of a call that names a trigger or a hook
// by its identifier: the caller must hold function.deploy somewhere, so a
// caller who holds it nowhere learns nothing about which identifiers exist.
// The call then checks the app and the environment of the row it finds.
func authorizeSome(p auth.Principal) error {
	if p.Holds(auth.FunctionDeploy) {
		return nil
	}
	for _, perms := range p.AppPermissions {
		if slices.Contains(perms, auth.FunctionDeploy) {
			return nil
		}
	}
	for _, perms := range p.EnvPermissions {
		if slices.Contains(perms, auth.FunctionDeploy) {
			return nil
		}
	}
	return authorize(p, auth.FunctionDeploy, "")
}

// authorizeEnv checks the permission to change triggers of an environment:
// function.deploy, and function.deploy.production for a production one,
// the stand-in for an approval until the approval engine decides (FN-007).
func authorizeEnv(p auth.Principal, appID string, env dbgen.Environment) error {
	if err := authorize(p, auth.FunctionDeploy, appID); err != nil {
		return err
	}
	if env.Production {
		return authorize(p, auth.FunctionDeployProduction, appID)
	}
	return nil
}

// CreateTrigger schedules a function alias (FN-025). An invalid expression
// or zone is PLX-7150; the alias must exist and point at a server-placed
// version. Running the function needs nothing the caller could not do
// anyway: a trigger carries no credentials (ADR-0081).
func (s *Service) CreateTrigger(ctx context.Context, p auth.Principal, spec TriggerSpec) (Trigger, error) {
	if err := authorize(p, auth.FunctionDeploy, spec.AppID); err != nil {
		return Trigger{}, err
	}
	sched, err := ParseSchedule(spec.Expression, spec.Timezone)
	if err != nil {
		return Trigger{}, err
	}
	if spec.Timezone == "" {
		spec.Timezone = "UTC"
	}
	if spec.Alias == "" {
		return Trigger{}, plxerr.New(plxerr.MissingProperty, "a schedule names the alias it runs")
	}
	id, err := s.newID()
	if err != nil {
		return Trigger{}, err
	}
	next := sched.Next(s.o.Now())
	if next.IsZero() {
		return Trigger{}, plxerr.New(plxerr.CronExpressionInvalid, "the schedule never fires")
	}
	var out Trigger
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		env, err := environmentOf(ctx, q, spec.AppID, spec.EnvironmentID)
		if err != nil {
			return err
		}
		if err := authorizeEnv(p, spec.AppID, env); err != nil {
			return err
		}
		lim, err := s.o.Limits(ctx, tx, p.OrganizationID, spec.AppID)
		if err != nil {
			return err
		}
		if most := lim.Get(limits.FunctionInputSize); int64(len(spec.Input)) > most {
			return plxerr.New(plxerr.FunctionInputTooLarge, "the input is at most function.inputSize = %d bytes", most)
		}
		res, err := ResolveVersionTx(ctx, tx, spec.AppID, spec.EnvironmentID, spec.Function, spec.Alias)
		if err != nil {
			return err
		}
		if res.Placement != PlacementServer {
			return plxerr.New(plxerr.FunctionNotFound, "the function %s does not run on the server", spec.Function)
		}
		row, err := q.InsertFunctionTrigger(ctx, dbgen.InsertFunctionTriggerParams{
			ID: storage.MustUUID(id), OrganizationID: env.OrganizationID, AppID: env.AppID, EnvironmentID: env.ID,
			FunctionName: spec.Function, Alias: spec.Alias, Expression: spec.Expression, Timezone: spec.Timezone,
			Input: orEmpty(spec.Input), NextFireAt: storage.Timestamp(next), CreatedBy: createdBy(p),
		})
		if err != nil {
			return failure(err, "trigger")
		}
		out = triggerOf(row)
		return s.record(ctx, tx, p, audit.FunctionTriggerSet, "function_trigger", id, spec.Function+"@"+spec.Alias+" "+spec.Expression)
	})
	return out, err
}

func orEmpty(b []byte) []byte {
	if b == nil {
		return []byte{}
	}
	return b
}

// ListTriggers lists an app's schedules (function.read).
func (s *Service) ListTriggers(ctx context.Context, p auth.Principal, appID string) ([]Trigger, error) {
	if err := authorize(p, auth.FunctionRead, appID); err != nil {
		return nil, err
	}
	app, err := parseID(appID, "app")
	if err != nil {
		return nil, err
	}
	var out []Trigger
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListFunctionTriggers(ctx, app)
		if err != nil {
			return failure(err, "triggers")
		}
		for _, r := range rows {
			out = append(out, triggerOf(r))
		}
		return nil
	})
	return out, err
}

// DeleteTrigger removes a schedule.
func (s *Service) DeleteTrigger(ctx context.Context, p auth.Principal, id string) error {
	if err := authorizeSome(p); err != nil {
		return err
	}
	u, err := parseID(id, "trigger")
	if err != nil {
		return err
	}
	return s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, err := q.GetFunctionTrigger(ctx, u)
		if err != nil {
			return failure(err, "trigger")
		}
		env, err := environmentOf(ctx, q, storage.ID(row.AppID), storage.ID(row.EnvironmentID))
		if err != nil {
			return err
		}
		if err := authorizeEnv(p, storage.ID(row.AppID), env); err != nil {
			return err
		}
		if _, err := q.DeleteFunctionTrigger(ctx, u); err != nil {
			return failure(err, "trigger")
		}
		return s.record(ctx, tx, p, audit.FunctionTriggerDeleted, "function_trigger", id, row.FunctionName+"@"+row.Alias)
	})
}

// RunTriggers fires the organisation's due schedules once (FN-025). Each
// fire time is claimed by a row keyed on the trigger and the time, so two
// workers, or a restarted one, run a fire time at most once. A schedule
// that fell behind — the worker was down — fires once for the time it
// missed and then resumes from now: it does not replay every missed minute.
// It returns how many functions it ran.
func (s *Service) RunTriggers(ctx context.Context, org string) (int, error) {
	inv := s.serverInvoker()
	if inv == nil {
		return 0, nil
	}
	now := s.o.Now().UTC()
	type fire struct {
		t  dbgen.FunctionTrigger
		at time.Time
	}
	var fires []fire
	err := s.inTenant(ctx, storage.Tenant{OrganizationID: org}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		due, err := q.ListDueFunctionTriggers(ctx, dbgen.ListDueFunctionTriggersParams{NextFireAt: storage.Timestamp(now), Limit: triggerBatch})
		if err != nil {
			return failure(err, "triggers")
		}
		for _, t := range due {
			at := storage.Time(t.NextFireAt)
			sched, perr := ParseSchedule(t.Expression, t.Timezone)
			next := time.Time{}
			if perr == nil {
				next = sched.Next(now)
			}
			if next.IsZero() {
				// A schedule that no longer parses or never fires again
				// (a zone database change) is parked, not spun on.
				next = now.Add(24 * time.Hour)
				if err := q.SetFunctionTriggerResult(ctx, dbgen.SetFunctionTriggerResultParams{ID: t.ID, LastResult: "invalid schedule"}); err != nil {
					return failure(err, "trigger")
				}
			}
			if err := q.AdvanceFunctionTrigger(ctx, dbgen.AdvanceFunctionTriggerParams{ID: t.ID, NextFireAt: storage.Timestamp(next), LastFireAt: storage.Timestamp(at)}); err != nil {
				return failure(err, "trigger")
			}
			if perr != nil {
				continue
			}
			n, err := q.InsertTriggerFire(ctx, dbgen.InsertTriggerFireParams{TriggerID: t.ID, FireAt: storage.Timestamp(at), OrganizationID: t.OrganizationID})
			if err != nil {
				return failure(err, "trigger")
			}
			if n == 1 {
				fires = append(fires, fire{t, at})
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	for _, f := range fires {
		result := s.runFire(ctx, inv, f.t)
		err := s.inTenant(ctx, storage.Tenant{OrganizationID: org}, func(ctx context.Context, tx pgx.Tx) error {
			q := dbgen.New(tx)
			if err := q.FinishTriggerFire(ctx, dbgen.FinishTriggerFireParams{TriggerID: f.t.ID, FireAt: storage.Timestamp(f.at), Result: result}); err != nil {
				return failure(err, "trigger")
			}
			return failure(q.SetFunctionTriggerResult(ctx, dbgen.SetFunctionTriggerResultParams{ID: f.t.ID, LastResult: result}), "trigger")
		})
		if err != nil && s.o.Log != nil {
			s.o.Log.WarnContext(ctx, "function: record the outcome of a schedule", "trigger", storage.ID(f.t.ID), "error", err)
		}
		s.noteOutcome(ctx, f.t, result)
	}
	return len(fires), nil
}

// runFire runs one fire and describes the outcome without a payload.
func (s *Service) runFire(ctx context.Context, inv ServerInvoker, t dbgen.FunctionTrigger) string {
	res, err := inv.InvokeServer(ctx, ServerCall{
		OrganizationID: storage.ID(t.OrganizationID), AppID: storage.ID(t.AppID), EnvironmentID: storage.ID(t.EnvironmentID),
		Function: t.FunctionName, Alias: t.Alias, Input: t.Input,
	})
	switch {
	case err != nil:
		code, _ := plxerr.CodeOf(err)
		return fmt.Sprintf("failed: PLX-%d", code)
	case res.Business != nil:
		return "business_error"
	}
	return "ok"
}

// noteOutcome publishes function.failing when a schedule that was healthy
// starts failing, once per transition so a broken function does not flood
// the channels (SRV-062).
func (s *Service) noteOutcome(ctx context.Context, t dbgen.FunctionTrigger, result string) {
	if s.o.Notify == nil || !strings.HasPrefix(result, "failed") || strings.HasPrefix(t.LastResult, "failed") {
		return
	}
	err := s.o.Notify.Publish(ctx, notify.Event{
		Type: notify.FunctionFailing, OrganizationID: storage.ID(t.OrganizationID), AppID: storage.ID(t.AppID),
		Title: fmt.Sprintf("Scheduled function %s is failing", t.FunctionName),
		Body:  fmt.Sprintf("The schedule %q for %s@%s failed: %s.", t.Expression, t.FunctionName, t.Alias, result),
		Link:  "/apps/" + storage.ID(t.AppID) + "/functions/" + t.FunctionName,
		Attributes: map[string]string{
			"function": t.FunctionName, "alias": t.Alias, "result": result, "trigger": storage.ID(t.ID),
		},
	})
	if err != nil && s.o.Log != nil {
		s.o.Log.WarnContext(ctx, "function: publish function.failing", "error", err)
	}
}

// ---- inbound hooks -------------------------------------------------------

// StandardWebhooks is the name of the built-in verifier.
const StandardWebhooks = "standard-webhooks"

// Hook is an inbound webhook of a function at /hooks/<app>/<function>.
type Hook struct {
	ID            string
	AppID         string
	EnvironmentID string
	Function      string
	Alias         string
	Verifier      string
	Enabled       bool
	CreatedAt     time.Time
}

// HookSpec is a hook to create.
type HookSpec struct {
	AppID         string
	EnvironmentID string
	Function      string
	Alias         string
	// Verifier is StandardWebhooks (empty) or the name of a provider
	// verifier the server registers (PAY-006).
	Verifier string
	// Secret is the provider's signing secret for a provider verifier; a
	// Standard Webhooks hook gets a generated whsec_ secret and ignores it.
	Secret string
}

func hookOf(r dbgen.FunctionHook) Hook {
	return Hook{
		ID: storage.ID(r.ID), AppID: storage.ID(r.AppID), EnvironmentID: storage.ID(r.EnvironmentID), Function: r.FunctionName, Alias: r.Alias,
		Verifier: r.Verifier, Enabled: r.Enabled, CreatedAt: storage.Time(r.CreatedAt),
	}
}

// CreateHook opens /hooks/<app>/<function> for a function alias and returns
// the hook with its secret, which is shown once (FN-025). One app has one
// hook per function.
func (s *Service) CreateHook(ctx context.Context, p auth.Principal, spec HookSpec) (Hook, string, error) {
	if err := authorize(p, auth.FunctionDeploy, spec.AppID); err != nil {
		return Hook{}, "", err
	}
	if s.o.Secrets == nil {
		return Hook{}, "", plxerr.New(plxerr.ChannelMisconfigured, "this server keeps no webhook secrets")
	}
	if spec.Verifier == "" {
		spec.Verifier = StandardWebhooks
	}
	secret := spec.Secret
	if spec.Verifier == StandardWebhooks {
		var err error
		if secret, err = webhook.NewSecret(); err != nil {
			return Hook{}, "", err //nolint:wrapcheck // a domain error
		}
	} else if _, ok := s.o.Verifiers[spec.Verifier]; !ok || secret == "" {
		return Hook{}, "", plxerr.New(plxerr.InvalidEnumValue, "the verifier %q is not registered, or no secret was given for it", spec.Verifier)
	}
	if spec.Alias == "" {
		return Hook{}, "", plxerr.New(plxerr.MissingProperty, "a hook names the alias it runs")
	}
	id, err := s.newID()
	if err != nil {
		return Hook{}, "", err
	}
	var out Hook
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		env, err := environmentOf(ctx, q, spec.AppID, spec.EnvironmentID)
		if err != nil {
			return err
		}
		if err := authorizeEnv(p, spec.AppID, env); err != nil {
			return err
		}
		res, err := ResolveVersionTx(ctx, tx, spec.AppID, spec.EnvironmentID, spec.Function, spec.Alias)
		if err != nil {
			return err
		}
		if res.Placement != PlacementServer {
			return plxerr.New(plxerr.FunctionNotFound, "the function %s does not run on the server", spec.Function)
		}
		row, err := q.InsertFunctionHook(ctx, dbgen.InsertFunctionHookParams{
			ID: storage.MustUUID(id), OrganizationID: env.OrganizationID, AppID: env.AppID, EnvironmentID: env.ID,
			FunctionName: spec.Function, Alias: spec.Alias, Verifier: spec.Verifier, CreatedBy: createdBy(p),
		})
		if err != nil {
			return failure(err, "hook")
		}
		if _, err := s.o.Secrets.SealSecret(ctx, q, env.OrganizationID, notify.OwnerHook, row.ID, secret); err != nil {
			return err //nolint:wrapcheck // a domain error
		}
		out = hookOf(row)
		return s.record(ctx, tx, p, audit.FunctionHookSet, "function_hook", id, spec.Function+"@"+spec.Alias)
	})
	if err != nil {
		return Hook{}, "", err
	}
	return out, secret, nil
}

// ListHooks lists an app's hooks (function.read).
func (s *Service) ListHooks(ctx context.Context, p auth.Principal, appID string) ([]Hook, error) {
	if err := authorize(p, auth.FunctionRead, appID); err != nil {
		return nil, err
	}
	app, err := parseID(appID, "app")
	if err != nil {
		return nil, err
	}
	var out []Hook
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListFunctionHooks(ctx, app)
		if err != nil {
			return failure(err, "hooks")
		}
		for _, r := range rows {
			out = append(out, hookOf(r))
		}
		return nil
	})
	return out, err
}

// hookForChange reads a hook and checks the caller may change it.
func (s *Service) hookForChange(ctx context.Context, q *dbgen.Queries, p auth.Principal, id pgtype.UUID) (dbgen.FunctionHook, error) {
	row, err := q.GetFunctionHook(ctx, id)
	if err != nil {
		return row, failure(err, "hook")
	}
	env, err := environmentOf(ctx, q, storage.ID(row.AppID), storage.ID(row.EnvironmentID))
	if err != nil {
		return row, err
	}
	return row, authorizeEnv(p, storage.ID(row.AppID), env)
}

// DeleteHook closes a hook and forgets its secrets.
func (s *Service) DeleteHook(ctx context.Context, p auth.Principal, id string) error {
	if err := authorizeSome(p); err != nil {
		return err
	}
	u, err := parseID(id, "hook")
	if err != nil {
		return err
	}
	return s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, err := s.hookForChange(ctx, q, p, u)
		if err != nil {
			return err
		}
		if _, err := q.DeleteFunctionHook(ctx, u); err != nil {
			return failure(err, "hook")
		}
		if err := q.DeleteWebhookSecrets(ctx, dbgen.DeleteWebhookSecretsParams{OwnerKind: notify.OwnerHook, OwnerID: u}); err != nil {
			return failure(err, "hook secrets")
		}
		return s.record(ctx, tx, p, audit.FunctionHookDeleted, "function_hook", id, row.FunctionName)
	})
}

// RotateHookSecret gives a hook a new secret; the old one stays valid for
// the overlap (zero is a day, negative ends it at once). The new secret is
// returned once.
func (s *Service) RotateHookSecret(ctx context.Context, p auth.Principal, id string, overlap time.Duration) (string, error) {
	if err := authorizeSome(p); err != nil {
		return "", err
	}
	u, err := parseID(id, "hook")
	if err != nil {
		return "", err
	}
	if s.o.Secrets == nil {
		return "", plxerr.New(plxerr.ChannelMisconfigured, "this server keeps no webhook secrets")
	}
	switch {
	case overlap == 0:
		overlap = notify.DefaultOverlap
	case overlap < 0:
		overlap = 0
	}
	var secret string
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, err := s.hookForChange(ctx, q, p, u)
		if err != nil {
			return err
		}
		if row.Verifier != StandardWebhooks {
			return plxerr.New(plxerr.InvalidEnumValue, "the secret of a %s hook is the provider's; create the hook again with the new one", row.Verifier)
		}
		if secret, err = s.o.Secrets.RotateSecret(ctx, q, row.OrganizationID, notify.OwnerHook, u, overlap); err != nil {
			return err //nolint:wrapcheck // a domain error
		}
		return s.record(ctx, tx, p, audit.FunctionHookSecretRotated, "function_hook", id, "overlap "+overlap.String())
	})
	return secret, err
}

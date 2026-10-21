// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/api"
	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/config"
	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/httpx"
	"github.com/nightCode42/plux3/backend/internal/notify"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/scheduler"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
)

// HooksRoute is where inbound function hooks are served (FN-025).
const HooksRoute = "POST /hooks/{app}/{function}"

// buildNotify assembles the notification and webhook service (SRV-062,
// SRV-063). Its HTTP client and SMTP dialer refuse private destinations
// unless the installation allows insecure development settings.
func buildNotify(cfg *config.Config, db *storage.DB, crypter signing.Crypter, gen interface{ New() (string, error) }, log *audit.Log, ten *tenancy.Service, work WorkDeps) (*notify.Service, error) {
	svc, err := notify.New(notify.Options{
		DB: db, Audit: log, Crypter: crypter, IDs: gen,
		Policy:        httpx.Policy{AllowLoopback: cfg.Notifications.AllowInsecure},
		StudioURL:     cfg.Server.PublicBaseURL,
		AllowInsecure: cfg.Notifications.AllowInsecure,
		Log:           work.Log,
		Limits: func(ctx context.Context, organizationID string) (limits.Set, error) {
			var set limits.Set
			err := db.InTx(ctx, storage.Tenant{OrganizationID: organizationID}, func(ctx context.Context, tx pgx.Tx) error {
				var err error
				set, err = ten.Effective(ctx, tx, organizationID, "", "")
				return err //nolint:wrapcheck // a domain error
			})
			return set, err //nolint:wrapcheck // InTx wraps its own failures
		},
	})
	if err != nil {
		return nil, fmt.Errorf("server: %w", err)
	}
	return svc, nil
}

// attachTriggerInvoker gives the function service the invoker that runs
// schedules and inbound hooks (FN-025). The api role reuses the invoker of
// FunctionService; the worker role builds its own over the same runner
// link, so that a schedule runs with the same limits, declarations and
// assurance as any server invocation and never with more.
func attachTriggerInvoker(functions *function.Service, rt *functionRuntime, d functionDeps) error {
	if functions == nil {
		return nil
	}
	if rt != nil && rt.invoker != nil {
		functions.UseInvoker(rt.invoker)
		return nil
	}
	if d.link == nil || !d.cfg.Has(config.RoleWorker) {
		return nil
	}
	inv, err := buildInvoker(d, d.link)
	if err != nil {
		return err
	}
	functions.UseInvoker(inv)
	return nil
}

// buildScheduler returns the worker role's scheduler: it drains the
// notification and webhook queues and fires function schedules on the
// elected leader (SRV-062, SRV-063, FN-025).
func buildScheduler(svc *Services, log *slog.Logger, db *storage.DB) (*scheduler.Scheduler, error) {
	jobs := []scheduler.Job{
		scheduler.JobFunc{Label: "approval decision signatures", F: func(ctx context.Context, org string) error {
			_, err := svc.Approvals.SignDecisions(ctx, org)
			return err //nolint:wrapcheck // a domain error
		}},
		scheduler.JobFunc{Label: "break-glass review escalation", F: func(ctx context.Context, org string) error {
			_, err := svc.BreakGlass.Escalate(ctx, org)
			return err //nolint:wrapcheck // a domain error
		}},
		scheduler.JobFunc{Label: "compliance report signatures", F: func(ctx context.Context, org string) error {
			_, err := svc.Reports.Sign(ctx, org)
			return err //nolint:wrapcheck // a domain error
		}},
		scheduler.JobFunc{Label: "notification deliveries", F: func(ctx context.Context, org string) error {
			_, err := svc.Notify.DeliverDue(ctx, org)
			return err //nolint:wrapcheck // a domain error
		}},
		scheduler.JobFunc{Label: "notification purge", F: func(ctx context.Context, org string) error {
			_, err := svc.Notify.Purge(ctx, org)
			return err //nolint:wrapcheck // a domain error
		}},
	}
	if svc.Functions != nil {
		jobs = append(jobs,
			scheduler.JobFunc{Label: "function schedules", F: func(ctx context.Context, org string) error {
				_, err := svc.Functions.RunTriggers(ctx, org)
				return err //nolint:wrapcheck // a domain error
			}},
			scheduler.JobFunc{Label: "function hook purge", F: func(ctx context.Context, org string) error {
				_, err := svc.Functions.PurgeHooks(ctx, org)
				return err //nolint:wrapcheck // a domain error
			}})
	}
	if svc.Rollouts != nil {
		jobs = append(jobs, scheduler.JobFunc{Label: "rollouts", F: svc.Rollouts.Job()})
	}
	s, err := scheduler.New(scheduler.Options{
		Locker: db, Organizations: svc.Tenancy.ForEachOrganization, Jobs: jobs, Log: log,
	})
	if err != nil {
		return nil, fmt.Errorf("server: %w", err)
	}
	return s, nil
}

// notifyRegistrations lists the notification, webhook and trigger services
// to mount.
func notifyRegistrations(svc *Services, h *api.Handlers, opts connect.HandlerOption) []func() (string, http.Handler) {
	var out []func() (string, http.Handler)
	if svc.Notify != nil {
		h.Notify = svc.Notify
		out = append(out,
			func() (string, http.Handler) {
				return pluxv1connect.NewNotificationServiceHandler(h.Notification(), opts)
			},
			func() (string, http.Handler) { return pluxv1connect.NewWebhookServiceHandler(h.Webhook(), opts) })
	}
	if svc.Functions != nil {
		out = append(out, func() (string, http.Handler) {
			return pluxv1connect.NewFunctionTriggerServiceHandler(h.FunctionTrigger(), opts)
		})
	}
	return out
}

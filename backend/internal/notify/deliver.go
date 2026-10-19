// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package notify

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/notify/webhook"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// The retry schedule: the first retry after about 30 seconds, doubling to a
// ceiling of 4 hours, with jitter, until the 24-hour window ends (SRV-063).
const (
	backoffBase = 30 * time.Second
	backoffCap  = 4 * time.Hour
	// claimLease is how long a claimed delivery is left alone: a worker
	// that dies mid-attempt has its deliveries retried after it.
	claimLease = 5 * time.Minute
	// claimBatch is how many deliveries one pass takes.
	claimBatch = 50
	// keepDeliveries is how long the log keeps finished deliveries.
	keepDeliveries = 30 * 24 * time.Hour
	// maxLoggedError bounds the error text kept in the delivery log.
	maxLoggedError = 200
)

// Backoff returns the wait after the given number of failed attempts
// (1 for the first failure). jitter is in [0, 1): the wait is between half
// and all of the exponential step, so retries of many deliveries that failed
// together spread out.
func Backoff(failed int, jitter float64) time.Duration {
	if failed < 1 {
		failed = 1
	}
	step := backoffBase
	for i := 1; i < failed && step < backoffCap; i++ {
		step *= 2
	}
	step = min(step, backoffCap)
	jitter = max(0, min(jitter, 0.999999))
	return step/2 + time.Duration(jitter*float64(step/2))
}

func randomJitter() float64 {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0.5
	}
	return float64(binary.BigEndian.Uint64(b[:])>>11) / (1 << 53)
}

// DeliverDue sends the organisation's due deliveries once and records the
// outcome of each: delivered, scheduled for retry, or — after 24 hours —
// given up on with PLX-8401. It returns how many it attempted. Several
// workers may call it at once: a claim is exclusive for the lease.
func (s *Service) DeliverDue(ctx context.Context, org string) (int, error) {
	now := s.now()
	var due []dbgen.NotificationDelivery
	err := s.inTenant(ctx, storage.Tenant{OrganizationID: org}, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		due, err = dbgen.New(tx).ClaimDueDeliveries(ctx, dbgen.ClaimDueDeliveriesParams{
			NextAttemptAt: storage.Timestamp(now), NextAttemptAt_2: storage.Timestamp(now.Add(claimLease)), Limit: claimBatch,
		})
		return failure(err, "deliveries")
	})
	if err != nil {
		return 0, err
	}
	var firstErr error
	for _, d := range due {
		status, sendErr := s.attempt(ctx, org, d)
		if err := s.settle(ctx, org, d, status, sendErr); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return len(due), firstErr
}

// attempt performs one delivery.
func (s *Service) attempt(ctx context.Context, org string, d dbgen.NotificationDelivery) (int, error) {
	var env envelope
	if err := json.Unmarshal(d.Payload, &env); err != nil {
		return 0, errors.New("the stored payload is unreadable")
	}
	if d.Kind == "webhook" {
		return s.attemptWebhook(ctx, org, d)
	}
	var (
		row      dbgen.NotificationChannel
		settings map[string]string
		secret   string
	)
	err := s.inTenant(ctx, storage.Tenant{OrganizationID: org}, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		row, settings, secret, err = s.openChannel(ctx, dbgen.New(tx), d.ChannelID)
		return err
	})
	if err != nil {
		return 0, clean(err)
	}
	return s.send(ctx, Kind(row.Kind), settings, secret, Message{
		Type: EventType(env.Type), ID: env.Data.ID, Title: env.Data.Title, Body: env.Data.Body, Link: s.absolute(env.Data.Link), To: d.Recipient,
	})
}

// attemptWebhook posts a signed message to an endpoint. The webhook-id is
// the delivery's, so it is the same on every retry; the timestamp and the
// signatures are made afresh for each attempt (Standard Webhooks).
func (s *Service) attemptWebhook(ctx context.Context, org string, d dbgen.NotificationDelivery) (int, error) {
	var (
		endpoint dbgen.WebhookEndpoint
		secrets  []string
	)
	err := s.inTenant(ctx, storage.Tenant{OrganizationID: org}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		var err error
		if endpoint, err = q.GetWebhookEndpoint(ctx, d.EndpointID); err != nil {
			return failure(err, "webhook")
		}
		secrets, err = s.ActiveSecrets(ctx, q, OwnerEndpoint, d.EndpointID, s.now())
		return err
	})
	if err != nil {
		return 0, clean(err)
	}
	if len(secrets) == 0 {
		return 0, plxerr.New(plxerr.ChannelMisconfigured, "the webhook has no valid secret")
	}
	headers, err := webhook.Headers(secrets, "msg_"+storage.ID(d.ID), s.now(), d.Payload)
	if err != nil {
		return 0, err //nolint:wrapcheck // a domain error
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.Url, bytes.NewReader(d.Payload))
	if err != nil {
		return 0, clean(err)
	}
	req.Header = headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "plux-server")
	return s.do(req, secrets...)
}

// settle records the outcome of an attempt.
func (s *Service) settle(ctx context.Context, org string, d dbgen.NotificationDelivery, status int, sendErr error) error {
	now := s.now()
	return s.inTenant(ctx, storage.Tenant{OrganizationID: org}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		if sendErr == nil {
			return failure(q.MarkDeliveryDelivered(ctx, dbgen.MarkDeliveryDeliveredParams{ID: d.ID, LastStatus: int32(status), DeliveredAt: storage.Timestamp(now)}), "delivery") //nolint:gosec // an HTTP status
		}
		text := sendErr.Error()
		if len(text) > maxLoggedError {
			text = text[:maxLoggedError]
		}
		failed := int(d.Attempts) + 1
		next := now.Add(Backoff(failed, s.o.Jitter()))
		if next.After(storage.Time(d.ExpiresAt)) {
			if err := q.MarkDeliveryFailed(ctx, dbgen.MarkDeliveryFailedParams{ID: d.ID, LastStatus: int32(status), LastError: text}); err != nil { //nolint:gosec // an HTTP status
				return failure(err, "delivery")
			}
			s.warn(ctx, "a delivery was given up on", "code", "PLX-8401", "delivery", storage.ID(d.ID), "kind", d.Kind, "attempts", failed)
			return s.record(ctx, tx, org, auth.System(org).Actor(), audit.DeliveryFailed, "notification_delivery", storage.ID(d.ID),
				fmt.Sprintf("PLX-%d after %d attempts (%s, %s)", plxerr.DeliveryFailed, failed, d.Kind, d.EventType))
		}
		return failure(q.MarkDeliveryRetry(ctx, dbgen.MarkDeliveryRetryParams{
			ID: d.ID, NextAttemptAt: storage.Timestamp(next), LastStatus: int32(status), LastError: text, //nolint:gosec // an HTTP status
		}), "delivery")
	})
}

// Purge forgets finished deliveries past their retention and secrets whose
// overlap has ended.
func (s *Service) Purge(ctx context.Context, org string) (int64, error) {
	var n int64
	err := s.inTenant(ctx, storage.Tenant{OrganizationID: org}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		a, err := q.PurgeDeliveries(ctx, storage.Timestamp(s.now().Add(-keepDeliveries)))
		if err != nil {
			return failure(err, "deliveries")
		}
		b, err := q.PurgeExpiredWebhookSecrets(ctx, storage.Timestamp(s.now()))
		n = a + b
		return failure(err, "webhook secrets")
	})
	return n, err
}

// Delivery is a row of the delivery log.
type Delivery struct {
	ID          string
	Kind        string
	TargetID    string
	EventType   EventType
	State       string
	Attempts    int32
	NextAttempt time.Time
	LastStatus  int32
	LastError   string
	CreatedAt   time.Time
	DeliveredAt time.Time
}

// DeliveryFilter narrows the delivery log. Kind is "channel" or "webhook"
// and decides the permission needed: notifications.manage or
// webhooks.manage.
type DeliveryFilter struct {
	Kind     string
	TargetID string
	State    string
	Limit    int32
}

// Deliveries reads the delivery log, newest first (SRV-062, SRV-063).
func (s *Service) Deliveries(ctx context.Context, p auth.Principal, f DeliveryFilter) ([]Delivery, error) {
	want := auth.NotificationsManage
	switch f.Kind {
	case "channel":
	case "webhook":
		want = auth.WebhooksManage
	default:
		return nil, plxerr.New(plxerr.InvalidEnumValue, "the delivery log is read for the kind channel or webhook")
	}
	if err := authorize(p, want); err != nil {
		return nil, err
	}
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	arg := dbgen.ListDeliveriesParams{Kind: &f.Kind, PageSize: f.Limit}
	if f.State != "" {
		arg.State = &f.State
	}
	if f.TargetID != "" {
		u, err := parseUUID(f.TargetID, "target")
		if err != nil {
			return nil, err
		}
		if f.Kind == "webhook" {
			arg.EndpointID = u
		} else {
			arg.ChannelID = u
		}
	}
	var out []Delivery
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListDeliveries(ctx, arg)
		if err != nil {
			return failure(err, "deliveries")
		}
		for _, r := range rows {
			target := r.ChannelID
			if r.Kind == "webhook" {
				target = r.EndpointID
			}
			out = append(out, deliveryOf(r, target))
		}
		return nil
	})
	return out, err
}

func deliveryOf(r dbgen.NotificationDelivery, target pgtype.UUID) Delivery {
	d := Delivery{
		ID: storage.ID(r.ID), Kind: r.Kind, TargetID: storage.ID(target), EventType: EventType(r.EventType), State: r.State, Attempts: r.Attempts,
		LastStatus: r.LastStatus, LastError: r.LastError, CreatedAt: storage.Time(r.CreatedAt),
	}
	if r.State == "pending" {
		d.NextAttempt = storage.Time(r.NextAttemptAt)
	}
	if r.DeliveredAt.Valid {
		d.DeliveredAt = storage.Time(r.DeliveredAt)
	}
	return d
}

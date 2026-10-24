// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

var _ Publisher = (*Service)(nil)

// DeliveryWindow is how long a delivery is retried before it is given up on
// (SRV-063).
const DeliveryWindow = 24 * time.Hour

// envelope is the payload of a delivery and the body of a webhook, in the
// shape Standard Webhooks recommends: type, timestamp and data.
type envelope struct {
	Type      string       `json:"type"`
	Timestamp string       `json:"timestamp"`
	Data      envelopeData `json:"data"`
}

type envelopeData struct {
	ID             string            `json:"id"`
	OrganizationID string            `json:"organizationId"`
	AppID          string            `json:"appId,omitempty"`
	Title          string            `json:"title"`
	Body           string            `json:"body,omitempty"`
	Link           string            `json:"link,omitempty"`
	Attributes     map[string]string `json:"attributes,omitempty"`
}

// target is one place an event goes.
type target struct {
	user      pgtype.UUID // an inbox entry
	channel   pgtype.UUID // a channel delivery
	recipient string      // the address for mail
}

// Publish records an event for everyone subscribed to it (SRV-062,
// SRV-063): inbox entries, channel deliveries and webhook deliveries, all in
// one transaction, so an event is either fully queued or not at all. Sending
// is the worker's job. Over notifications.perHour, the excess is dropped and
// audited once an hour.
func (s *Service) Publish(ctx context.Context, e Event) error {
	pe, err := s.prepareEvent(ctx, &e)
	if err != nil {
		return err
	}
	return s.inTenant(ctx, storage.Tenant{OrganizationID: e.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		return s.queueEvent(ctx, tx, e, pe)
	})
}

// preparedEvent is an event with its identifiers parsed, its payload
// encoded and the organisation's hourly limit read.
type preparedEvent struct {
	org, app, id pgtype.UUID
	now          time.Time
	payload      []byte
	params       []byte
	perHour      int64
}

// prepareEvent validates the event, fills in its identifier and time, and
// encodes the payload every delivery of it carries.
func (s *Service) prepareEvent(ctx context.Context, e *Event) (preparedEvent, error) {
	if err := e.validate(); err != nil {
		return preparedEvent{}, err
	}
	org, err := parseUUID(e.OrganizationID, "organisation")
	if err != nil {
		return preparedEvent{}, err
	}
	var app pgtype.UUID
	if e.AppID != "" {
		if app, err = parseUUID(e.AppID, "app"); err != nil {
			return preparedEvent{}, err
		}
	}
	if e.ID == "" {
		if e.ID, err = s.newID(); err != nil {
			return preparedEvent{}, err
		}
	}
	eventID, err := parseUUID(e.ID, "event")
	if err != nil {
		return preparedEvent{}, err
	}
	now := s.now()
	if e.At.IsZero() {
		e.At = now
	}
	lim, err := s.o.Limits(ctx, e.OrganizationID)
	if err != nil {
		return preparedEvent{}, err
	}
	if err := e.validateParams(lim.Get(limits.NotificationsParamsPerEvent), lim.Get(limits.NotificationsParamBytes)); err != nil {
		return preparedEvent{}, err
	}
	params := []byte("{}")
	if len(e.Params) > 0 {
		if params, err = json.Marshal(e.Params); err != nil {
			return preparedEvent{}, fmt.Errorf("notify: %w", err)
		}
	}
	payload, err := json.Marshal(envelope{
		Type: string(e.Type), Timestamp: e.At.UTC().Format(time.RFC3339Nano),
		Data: envelopeData{
			ID: e.ID, OrganizationID: e.OrganizationID, AppID: e.AppID, Title: e.Title, Body: e.Body, Link: e.Link, Attributes: e.Attributes,
		},
	})
	if err != nil {
		return preparedEvent{}, fmt.Errorf("notify: %w", err)
	}
	return preparedEvent{org: org, app: app, id: eventID, now: now, payload: payload, params: params, perHour: lim.Get(limits.NotificationsPerHour)}, nil
}

// queueEvent writes the inbox entries and deliveries of an event, within
// the hourly limit, and audits the first drop of the hour.
func (s *Service) queueEvent(ctx context.Context, tx pgx.Tx, e Event, pe preparedEvent) error {
	q := dbgen.New(tx)
	targets, err := s.targets(ctx, q, e.Type)
	if err != nil {
		return err
	}
	kept, dropped, first, err := s.reserve(ctx, q, pe.org, pe.now, int64(len(targets)), pe.perHour)
	if err != nil {
		return err
	}
	if dropped > 0 && first {
		if err := s.record(ctx, tx, e.OrganizationID, auth.System(e.OrganizationID).Actor(), audit.NotificationsDropped, "organization", e.OrganizationID,
			fmt.Sprintf("notifications.perHour = %d reached; the excess is dropped", pe.perHour)); err != nil {
			return err
		}
	}
	for _, t := range targets[:kept] {
		if err := s.enqueue(ctx, q, pe, e, t); err != nil {
			return err
		}
	}
	return s.enqueueWebhooks(ctx, q, pe.org, pe.id, e.Type, pe.payload, pe.now)
}

// enqueue writes an inbox entry, with its display parameters and group
// key, or a channel delivery.
func (s *Service) enqueue(ctx context.Context, q *dbgen.Queries, pe preparedEvent, e Event, t target) error {
	id, err := s.newID()
	if err != nil {
		return err
	}
	if t.user.Valid {
		return failure(q.InsertNotification(ctx, dbgen.InsertNotificationParams{
			ID: storage.MustUUID(id), OrganizationID: pe.org, UserID: t.user, EventID: pe.id, EventType: string(e.Type), AppID: pe.app,
			Title: e.Title, Body: e.Body, Link: e.Link, CreatedAt: storage.Timestamp(pe.now), Params: pe.params, GroupKey: groupKey(e, id),
		}), "notification")
	}
	return failure(q.InsertDelivery(ctx, dbgen.InsertDeliveryParams{
		ID: storage.MustUUID(id), OrganizationID: pe.org, Kind: "channel", ChannelID: t.channel, EventID: pe.id, EventType: string(e.Type),
		Recipient: t.recipient, Payload: pe.payload, NextAttemptAt: storage.Timestamp(pe.now), ExpiresAt: storage.Timestamp(pe.now.Add(DeliveryWindow)), CreatedAt: storage.Timestamp(pe.now),
	}), "delivery")
}

// enqueueWebhooks queues a signed delivery for each endpoint that follows
// the event.
func (s *Service) enqueueWebhooks(ctx context.Context, q *dbgen.Queries, org, event pgtype.UUID, t EventType, payload []byte, now time.Time) error {
	endpoints, err := q.ListWebhookEndpointsForEvent(ctx, string(t))
	if err != nil {
		return failure(err, "webhooks")
	}
	for _, ep := range endpoints {
		id, err := s.newID()
		if err != nil {
			return err
		}
		if err := q.InsertDelivery(ctx, dbgen.InsertDeliveryParams{
			ID: storage.MustUUID(id), OrganizationID: org, Kind: "webhook", EndpointID: ep.ID, EventID: event, EventType: string(t),
			Payload: payload, NextAttemptAt: storage.Timestamp(now), ExpiresAt: storage.Timestamp(now.Add(DeliveryWindow)), CreatedAt: storage.Timestamp(now),
		}); err != nil {
			return failure(err, "delivery")
		}
	}
	return nil
}

// targets expands the subscriptions to an event type into the inbox entries
// and channel deliveries to make, without duplicates: a person subscribed
// as themselves and through a team gets one entry, and a chat channel
// several subscriptions name receives one message.
func (*Service) targets(ctx context.Context, q *dbgen.Queries, t EventType) ([]target, error) {
	subs, err := q.ListSubscriptionsForEvent(ctx, string(t))
	if err != nil {
		return nil, failure(err, "subscriptions")
	}
	c := targetSet{seen: map[string]bool{}, channels: map[pgtype.UUID]*dbgen.NotificationChannel{}}
	for _, sub := range subs {
		if err := c.addSubscription(ctx, q, sub); err != nil {
			return nil, err
		}
	}
	return c.out, nil
}

// recipient is a person a subscription reaches.
type recipient struct {
	id    pgtype.UUID
	email string
}

// targetSet collects the targets of one event without duplicates, and
// remembers the channels it has read.
type targetSet struct {
	out      []target
	seen     map[string]bool
	channels map[pgtype.UUID]*dbgen.NotificationChannel
}

// add keeps a target the first time its key is seen.
func (c *targetSet) add(key string, t target) {
	if !c.seen[key] {
		c.seen[key] = true
		c.out = append(c.out, t)
	}
}

// addSubscription adds the inbox entries or channel deliveries one
// subscription asks for.
func (c *targetSet) addSubscription(ctx context.Context, q *dbgen.Queries, sub dbgen.NotificationSubscription) error {
	people, ok, err := subscribers(ctx, q, sub)
	if err != nil || !ok {
		return err
	}
	if !sub.ChannelID.Valid {
		for _, r := range people {
			c.add("inbox:"+storage.ID(r.id), target{user: r.id})
		}
		return nil
	}
	ch, err := c.channel(ctx, q, sub.ChannelID)
	if err != nil || ch == nil || !ch.Enabled {
		return err
	}
	if Kind(ch.Kind) == KindSMTP {
		for _, r := range people {
			c.add("mail:"+storage.ID(ch.ID)+":"+r.email, target{channel: ch.ID, recipient: r.email})
		}
		return nil
	}
	c.add("chat:"+storage.ID(ch.ID), target{channel: ch.ID})
	return nil
}

// channel reads a channel once; a channel that no longer exists is nil.
func (c *targetSet) channel(ctx context.Context, q *dbgen.Queries, id pgtype.UUID) (*dbgen.NotificationChannel, error) {
	if ch, ok := c.channels[id]; ok {
		return ch, nil
	}
	var ch *dbgen.NotificationChannel
	row, err := q.GetNotificationChannel(ctx, id)
	if err == nil {
		ch = &row
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, failure(err, "channel")
	}
	c.channels[id] = ch
	return ch, nil
}

// subscribers lists the people a subscription names: the members of a team,
// or one user. ok is false for a user who is disabled or removed, whose
// subscription reaches nobody.
func subscribers(ctx context.Context, q *dbgen.Queries, sub dbgen.NotificationSubscription) (people []recipient, ok bool, err error) {
	if sub.SubjectKind == "team" {
		rows, err := q.ListTeamRecipients(ctx, sub.SubjectID)
		if err != nil {
			return nil, false, failure(err, "team")
		}
		for _, r := range rows {
			people = append(people, recipient{r.ID, r.Email})
		}
		return people, true, nil
	}
	r, err := q.GetUserRecipient(ctx, sub.SubjectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, failure(err, "user")
	}
	return []recipient{{r.ID, r.Email}}, true, nil
}

// reserve counts targets against notifications.perHour and returns how many
// may be kept. first is true when this call is the first to drop in the
// hour, which is when the drop is audited.
func (*Service) reserve(ctx context.Context, q *dbgen.Queries, org pgtype.UUID, now time.Time, want, limit int64) (kept int, dropped int64, first bool, err error) {
	if want == 0 {
		return 0, 0, false, nil
	}
	hour := storage.Timestamp(now.Truncate(time.Hour))
	row, err := q.LockNotificationHour(ctx, dbgen.LockNotificationHourParams{OrganizationID: org, Hour: hour})
	if err != nil {
		return 0, 0, false, failure(err, "notification count")
	}
	room := limit - int64(row.N)
	if room < 0 {
		room = 0
	}
	k := min(want, room)
	dropped = want - k
	if err := q.AddNotificationHour(ctx, dbgen.AddNotificationHourParams{
		OrganizationID: org, Hour: hour, N: int32(k), Dropped: int32(dropped), //nolint:gosec // bounded by the subscriptions of one event
	}); err != nil {
		return 0, 0, false, failure(err, "notification count")
	}
	return int(k), dropped, dropped > 0 && row.Dropped == 0, nil
}

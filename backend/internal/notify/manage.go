// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// Channel is a configured destination. The secret is never returned; Hint
// is the last characters of it, to tell channels apart.
type Channel struct {
	ID        string
	Name      string
	Kind      Kind
	Settings  map[string]string
	Hint      string
	Enabled   bool
	CreatedAt time.Time
}

// ChannelSpec is a channel to create. Secret is the SMTP password, the Slack
// or Teams webhook URL, or the Telegram bot token.
type ChannelSpec struct {
	Name     string
	Kind     Kind
	Settings map[string]string
	Secret   string
}

func channelBinding(id string) []byte { return []byte("notification-channel:" + id) }

func checkName(name string) error {
	if name == "" || len(name) > 64 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return plxerr.New(plxerr.InvalidFormat, "a name is 1 to 64 characters without control characters")
	}
	return nil
}

func channelOf(row dbgen.NotificationChannel) Channel {
	settings := map[string]string{}
	_ = json.Unmarshal(row.Settings, &settings)
	return Channel{
		ID: storage.ID(row.ID), Name: row.Name, Kind: Kind(row.Kind), Settings: settings, Hint: row.Hint,
		Enabled: row.Enabled, CreatedAt: storage.Time(row.CreatedAt),
	}
}

func actorUser(p auth.Principal) pgtype.UUID {
	if p.UserID == "" {
		return pgtype.UUID{}
	}
	return storage.MustUUID(p.UserID)
}

// CreateChannel configures a channel (SRV-062). A misconfiguration is
// PLX-8400. It needs notifications.manage and is audited.
func (s *Service) CreateChannel(ctx context.Context, p auth.Principal, spec ChannelSpec) (Channel, error) {
	if err := authorize(p, auth.NotificationsManage); err != nil {
		return Channel{}, err
	}
	if err := checkName(spec.Name); err != nil {
		return Channel{}, err
	}
	settings, err := s.validateChannel(spec.Kind, spec.Settings, spec.Secret)
	if err != nil {
		return Channel{}, err
	}
	stored, err := json.Marshal(settings)
	if err != nil {
		return Channel{}, fmt.Errorf("notify: %w", err)
	}
	id, err := s.newID()
	if err != nil {
		return Channel{}, err
	}
	var sealed signing.Sealed
	if spec.Secret != "" {
		if sealed, err = signing.Seal(ctx, s.o.Crypter, []byte(spec.Secret), channelBinding(id)); err != nil {
			return Channel{}, fmt.Errorf("notify: seal a channel secret: %w", err)
		}
	}
	var out Channel
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		row, err := dbgen.New(tx).InsertNotificationChannel(ctx, dbgen.InsertNotificationChannelParams{
			ID: storage.MustUUID(id), OrganizationID: storage.MustUUID(p.OrganizationID), Name: spec.Name, Kind: string(spec.Kind),
			Settings: stored, Ciphertext: orEmpty(sealed.Ciphertext), WrappedKey: orEmpty(sealed.WrappedKey), KeyID: sealed.KeyID,
			Hint: sealed.Hint, CreatedBy: actorUser(p),
		})
		if err != nil {
			return failure(err, "channel")
		}
		out = channelOf(row)
		return s.record(ctx, tx, p.OrganizationID, p.Actor(), audit.NotificationChannelSet, "notification_channel", id, string(spec.Kind))
	})
	return out, err
}

func orEmpty(b []byte) []byte {
	if b == nil {
		return []byte{}
	}
	return b
}

// ListChannels lists the channels (notifications.manage).
func (s *Service) ListChannels(ctx context.Context, p auth.Principal) ([]Channel, error) {
	if err := authorize(p, auth.NotificationsManage); err != nil {
		return nil, err
	}
	var out []Channel
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListNotificationChannels(ctx)
		if err != nil {
			return failure(err, "channels")
		}
		for _, r := range rows {
			out = append(out, channelOf(r))
		}
		return nil
	})
	return out, err
}

// DeleteChannel removes a channel and the subscriptions and deliveries that
// name it.
func (s *Service) DeleteChannel(ctx context.Context, p auth.Principal, id string) error {
	if err := authorize(p, auth.NotificationsManage); err != nil {
		return err
	}
	u, err := parseUUID(id, "channel")
	if err != nil {
		return err
	}
	return s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		n, err := dbgen.New(tx).DeleteNotificationChannel(ctx, u)
		if err != nil {
			return failure(err, "channel")
		}
		if n == 0 {
			return plxerr.New(plxerr.ResourceNotFound, "no such channel")
		}
		return s.record(ctx, tx, p.OrganizationID, p.Actor(), audit.NotificationChannelDeleted, "notification_channel", id, "")
	})
}

// openChannel reads a channel and opens its secret.
func (s *Service) openChannel(ctx context.Context, q *dbgen.Queries, id pgtype.UUID) (dbgen.NotificationChannel, map[string]string, string, error) {
	row, err := q.GetNotificationChannel(ctx, id)
	if err != nil {
		return row, nil, "", failure(err, "channel")
	}
	settings := map[string]string{}
	if err := json.Unmarshal(row.Settings, &settings); err != nil {
		return row, nil, "", fmt.Errorf("notify: channel settings: %w", err)
	}
	var secret []byte
	if len(row.Ciphertext) > 0 {
		secret, err = signing.Open(ctx, s.o.Crypter, signing.Sealed{Ciphertext: row.Ciphertext, WrappedKey: row.WrappedKey, KeyID: row.KeyID},
			channelBinding(storage.ID(row.ID)))
		if err != nil {
			return row, nil, "", fmt.Errorf("notify: open a channel secret: %w", err)
		}
	}
	return row, settings, string(secret), nil
}

// TestChannel sends a sample message through a channel and reports the
// failure as PLX-8400. Mail goes to the caller's own address.
func (s *Service) TestChannel(ctx context.Context, p auth.Principal, id string) error {
	if err := authorize(p, auth.NotificationsManage); err != nil {
		return err
	}
	u, err := parseUUID(id, "channel")
	if err != nil {
		return err
	}
	var (
		row      dbgen.NotificationChannel
		settings map[string]string
		secret   string
		to       string
	)
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		if row, settings, secret, err = s.openChannel(ctx, q, u); err != nil {
			return err
		}
		if Kind(row.Kind) == KindSMTP {
			if p.UserID == "" {
				return plxerr.New(plxerr.ChannelMisconfigured, "a test mail goes to a signed-in user's address")
			}
			me, err := q.GetUserRecipient(ctx, storage.MustUUID(p.UserID))
			if err != nil {
				return failure(err, "user")
			}
			to = me.Email
		}
		return nil
	})
	if err != nil {
		return err
	}
	msgID, err := s.newID()
	if err != nil {
		return err
	}
	_, err = s.send(ctx, Kind(row.Kind), settings, secret, Message{
		Type: AlertFired, ID: msgID, Title: "Plux test notification", Body: "This channel is set up correctly.", To: to,
	})
	if err != nil {
		return plxerr.New(plxerr.ChannelMisconfigured, "the test message was not delivered: %v", err)
	}
	return nil
}

// Subscription says who follows an event type, and where it goes.
type Subscription struct {
	ID string
	// SubjectKind is "user" or "team".
	SubjectKind string
	SubjectID   string
	EventType   EventType
	// ChannelID is empty for the in-app inbox.
	ChannelID string
	CreatedAt time.Time
}

func subscriptionOf(r dbgen.NotificationSubscription) Subscription {
	return Subscription{
		ID: storage.ID(r.ID), SubjectKind: r.SubjectKind, SubjectID: storage.ID(r.SubjectID), EventType: EventType(r.EventType),
		ChannelID: storage.ID(r.ChannelID), CreatedAt: storage.Time(r.CreatedAt),
	}
}

// Subscribe follows an event type. A person may subscribe themselves to
// their inbox or to a mail channel; anything else — a team, another person,
// a chat channel — needs notifications.manage. Subscribing twice is not an
// error.
func (s *Service) Subscribe(ctx context.Context, p auth.Principal, sub Subscription) (Subscription, error) {
	if sub.SubjectKind == "" {
		sub.SubjectKind, sub.SubjectID = "user", p.UserID
	}
	target, err := checkSubscription(p, sub)
	if err != nil {
		return Subscription{}, err
	}
	id, err := s.newID()
	if err != nil {
		return Subscription{}, err
	}
	var out Subscription
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		out, err = s.insertSubscription(ctx, tx, p, sub, target, id)
		return err
	})
	return out, err
}

// subscriptionTarget is a subscription's subject and channel, parsed, and
// whether the principal subscribes themselves.
type subscriptionTarget struct {
	subject, channel pgtype.UUID
	self             bool
}

// checkSubscription validates a subscription's kind, event type and
// identifiers, and that a person who subscribes themselves may read the
// organisation.
func checkSubscription(p auth.Principal, sub Subscription) (subscriptionTarget, error) {
	if sub.SubjectKind != "user" && sub.SubjectKind != "team" {
		return subscriptionTarget{}, plxerr.New(plxerr.InvalidEnumValue, "a subscription is for a user or a team")
	}
	if !Known(sub.EventType) {
		return subscriptionTarget{}, plxerr.New(plxerr.InvalidEnumValue, "%q is not an event type", sub.EventType)
	}
	subject, err := parseUUID(sub.SubjectID, "subject")
	if err != nil {
		return subscriptionTarget{}, err
	}
	t := subscriptionTarget{subject: subject, self: sub.SubjectKind == "user" && sub.SubjectID == p.UserID && p.UserID != ""}
	if sub.ChannelID != "" {
		if t.channel, err = parseUUID(sub.ChannelID, "channel"); err != nil {
			return subscriptionTarget{}, err
		}
	}
	if t.self && p.OrganizationID != "" {
		if err := p.Authorize(auth.OrganizationRead); err != nil {
			return subscriptionTarget{}, fmt.Errorf("notify: %w", err)
		}
	}
	return t, nil
}

// insertSubscription authorises the subscription, checks that its subject
// belongs to the organisation and stores it, in the transaction of the
// request.
func (s *Service) insertSubscription(ctx context.Context, tx pgx.Tx, p auth.Principal, sub Subscription, t subscriptionTarget, id string) (Subscription, error) {
	q := dbgen.New(tx)
	if err := authorizeSubscription(ctx, q, p, t); err != nil {
		return Subscription{}, err
	}
	org := storage.MustUUID(p.OrganizationID)
	if err := subjectExists(ctx, q, org, sub.SubjectKind, t.subject); err != nil {
		return Subscription{}, err
	}
	n, err := q.InsertNotificationSubscription(ctx, dbgen.InsertNotificationSubscriptionParams{
		ID: storage.MustUUID(id), OrganizationID: org, SubjectKind: sub.SubjectKind, SubjectID: t.subject,
		EventType: string(sub.EventType), ChannelID: t.channel, CreatedBy: actorUser(p),
	})
	if err != nil {
		return Subscription{}, failure(err, "subscription")
	}
	out := Subscription{ID: id, SubjectKind: sub.SubjectKind, SubjectID: sub.SubjectID, EventType: sub.EventType, ChannelID: sub.ChannelID}
	if n == 0 {
		return out, nil // already subscribed
	}
	return out, s.record(ctx, tx, p.OrganizationID, p.Actor(), audit.NotificationSubscriptionSet, sub.SubjectKind, sub.SubjectID, string(sub.EventType))
}

// authorizeSubscription asks for notifications.manage unless the principal
// subscribes themselves to their inbox or a mail channel.
func authorizeSubscription(ctx context.Context, q *dbgen.Queries, p auth.Principal, t subscriptionTarget) error {
	needManage := !t.self
	if t.channel.Valid {
		ch, err := q.GetNotificationChannel(ctx, t.channel)
		if err != nil {
			return failure(err, "channel")
		}
		if Kind(ch.Kind) != KindSMTP {
			needManage = true
		}
	}
	if needManage {
		return authorize(p, auth.NotificationsManage)
	}
	return nil
}

// subjectExists checks that a team or a member exists in the organisation.
func subjectExists(ctx context.Context, q *dbgen.Queries, org pgtype.UUID, kind string, subject pgtype.UUID) error {
	if kind == "team" {
		if ok, err := q.TeamExists(ctx, subject); err != nil || !ok {
			return orNotFound(err, "team")
		}
		return nil
	}
	if ok, err := q.IsOrganizationMember(ctx, dbgen.IsOrganizationMemberParams{OrganizationID: org, UserID: subject}); err != nil || !ok {
		return orNotFound(err, "member")
	}
	return nil
}

func orNotFound(err error, what string) error {
	if err != nil {
		return failure(err, what)
	}
	return plxerr.New(plxerr.ResourceNotFound, "no such %s in this organisation", what)
}

// Unsubscribe removes a subscription: your own freely, others' with
// notifications.manage.
func (s *Service) Unsubscribe(ctx context.Context, p auth.Principal, id string) error {
	u, err := parseUUID(id, "subscription")
	if err != nil {
		return err
	}
	return s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, err := q.GetNotificationSubscription(ctx, u)
		if err != nil {
			return failure(err, "subscription")
		}
		if row.SubjectKind != "user" || p.UserID == "" || storage.ID(row.SubjectID) != p.UserID {
			if err := authorize(p, auth.NotificationsManage); err != nil {
				return err
			}
		}
		if _, err := q.DeleteNotificationSubscription(ctx, u); err != nil {
			return failure(err, "subscription")
		}
		return s.record(ctx, tx, p.OrganizationID, p.Actor(), audit.NotificationSubscriptionSet, row.SubjectKind, storage.ID(row.SubjectID), "removed "+row.EventType)
	})
}

// ListSubscriptions lists the caller's own subscriptions, or with
// notifications.manage and all set, everyone's.
func (s *Service) ListSubscriptions(ctx context.Context, p auth.Principal, all bool) ([]Subscription, error) {
	arg := dbgen.ListNotificationSubscriptionsParams{}
	if all {
		if err := authorize(p, auth.NotificationsManage); err != nil {
			return nil, err
		}
	} else {
		if p.UserID == "" {
			return nil, plxerr.New(plxerr.AuthenticationRequired, "subscriptions belong to a signed-in user")
		}
		kind, id := "user", storage.MustUUID(p.UserID)
		arg.SubjectKind, arg.SubjectID = &kind, id
	}
	var out []Subscription
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListNotificationSubscriptions(ctx, arg)
		if err != nil {
			return failure(err, "subscriptions")
		}
		for _, r := range rows {
			out = append(out, subscriptionOf(r))
		}
		return nil
	})
	return out, err
}

// InboxItem is a stored notification.
type InboxItem struct {
	ID        string
	EventID   string
	Type      EventType
	AppID     string
	Title     string
	Body      string
	Link      string
	CreatedAt time.Time
	Read      bool
	// Params are the display parameters of the event (SRV-062, STU-012).
	Params map[string]string
	// GroupKey is equal for repeats of one event; GroupCount is how many
	// of the listed notifications the group holds, this one included.
	GroupKey   string
	GroupCount int32
}

// maxInboxPage bounds a page of the inbox.
const maxInboxPage = 200

// Inbox lists the caller's notifications, newest first, one per group:
// repeats of one event show as their newest, with the group's count.
func (s *Service) Inbox(ctx context.Context, p auth.Principal, unreadOnly bool, limit int32) ([]InboxItem, int64, error) {
	if p.UserID == "" {
		return nil, 0, plxerr.New(plxerr.AuthenticationRequired, "an inbox belongs to a signed-in user")
	}
	if limit <= 0 || limit > maxInboxPage {
		limit = 50
	}
	var (
		out    []InboxItem
		unread int64
	)
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		me := storage.MustUUID(p.UserID)
		rows, err := q.ListNotificationGroups(ctx, dbgen.ListNotificationGroupsParams{UserID: me, UnreadOnly: unreadOnly, PageSize: limit})
		if err != nil {
			return failure(err, "inbox")
		}
		for _, g := range rows {
			r := g.Notification
			params := map[string]string{}
			if err := json.Unmarshal(r.Params, &params); err != nil {
				return fmt.Errorf("notify: the parameters of notification %s: %w", storage.ID(r.ID), err)
			}
			key := r.GroupKey
			if key == "" {
				key = storage.ID(r.ID)
			}
			out = append(out, InboxItem{
				ID: storage.ID(r.ID), EventID: storage.ID(r.EventID), Type: EventType(r.EventType), AppID: storage.ID(r.AppID),
				Title: r.Title, Body: r.Body, Link: r.Link, CreatedAt: storage.Time(r.CreatedAt), Read: r.ReadAt.Valid,
				Params: params, GroupKey: key, GroupCount: g.GroupCount,
			})
		}
		unread, err = q.CountUnreadNotifications(ctx, me)
		return failure(err, "inbox")
	})
	return out, unread, err
}

// MarkRead marks notifications read; with no IDs, all of the caller's.
func (s *Service) MarkRead(ctx context.Context, p auth.Principal, ids []string) (int64, error) {
	if p.UserID == "" {
		return 0, plxerr.New(plxerr.AuthenticationRequired, "an inbox belongs to a signed-in user")
	}
	parsed := make([]pgtype.UUID, 0, len(ids))
	for _, id := range ids {
		u, err := parseUUID(id, "notification")
		if err != nil {
			return 0, err
		}
		parsed = append(parsed, u)
	}
	var n, unread int64
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		n, err = dbgen.New(tx).MarkNotificationsRead(ctx, dbgen.MarkNotificationsReadParams{UserID: storage.MustUUID(p.UserID), Ids: parsed})
		if err != nil {
			return failure(err, "inbox")
		}
		unread, err = dbgen.New(tx).CountUnreadNotifications(ctx, storage.MustUUID(p.UserID))
		return failure(err, "inbox")
	})
	if err == nil && s.o.Events != nil {
		s.o.Events.InboxChanged(ctx, p.OrganizationID, p.UserID, unread)
	}
	return n, err
}

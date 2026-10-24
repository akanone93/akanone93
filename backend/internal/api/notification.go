// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"net/http"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/notify"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

// Notification serves NotificationService: the inbox, subscriptions and
// channels (SRV-062, ADR-0081).
type Notification struct{ h *Handlers }

var _ pluxv1connect.NotificationServiceHandler = Notification{}

// Notification returns the NotificationService handler.
func (h *Handlers) Notification() Notification { return Notification{h: h} }

// ListEventTypes returns the event types.
func (s Notification) ListEventTypes(ctx context.Context, req *connect.Request[pluxv1.ListEventTypesRequest]) (*connect.Response[pluxv1.ListEventTypesResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListEventTypesResponse, error) {
		if _, err := s.h.principal(ctx, req.Header(), ""); err != nil {
			return nil, err
		}
		out := &pluxv1.ListEventTypesResponse{}
		for _, t := range notify.Types() {
			out.EventTypes = append(out.EventTypes, &pluxv1.EventTypeInfo{Type: string(t.Type), Description: t.Description, Severity: string(t.Severity)})
		}
		return out, nil
	})
}

// ListInbox lists the caller's notifications.
func (s Notification) ListInbox(ctx context.Context, req *connect.Request[pluxv1.ListInboxRequest]) (*connect.Response[pluxv1.ListInboxResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListInboxResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		items, unread, err := s.h.Notify.Inbox(ctx, p, req.Msg.GetUnreadOnly(), req.Msg.GetLimit())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListInboxResponse{Unread: unread}
		for _, i := range items {
			out.Items = append(out.Items, &pluxv1.InboxItem{
				Id: i.ID, EventType: string(i.Type), AppId: i.AppID, Title: i.Title, Body: i.Body, Link: i.Link,
				CreatedAt: ts(i.CreatedAt), Read: i.Read, Params: i.Params, GroupKey: i.GroupKey, GroupCount: i.GroupCount,
			})
		}
		return out, nil
	})
}

// MarkRead marks notifications read.
func (s Notification) MarkRead(ctx context.Context, req *connect.Request[pluxv1.MarkReadRequest]) (*connect.Response[pluxv1.MarkReadResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.MarkReadResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		n, err := s.h.Notify.MarkRead(ctx, p, req.Msg.GetIds())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.MarkReadResponse{Marked: n}, nil
	})
}

func subscriptionProto(s notify.Subscription) *pluxv1.NotificationSubscription {
	return &pluxv1.NotificationSubscription{
		Id: s.ID, SubjectKind: s.SubjectKind, SubjectId: s.SubjectID, EventType: string(s.EventType),
		ChannelId: s.ChannelID, CreatedAt: ts(s.CreatedAt),
	}
}

// Subscribe follows an event type.
func (s Notification) Subscribe(ctx context.Context, req *connect.Request[pluxv1.SubscribeRequest]) (*connect.Response[pluxv1.SubscribeResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.SubscribeResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		in := req.Msg.GetSubscription()
		sub, err := s.h.Notify.Subscribe(ctx, p, notify.Subscription{
			SubjectKind: in.GetSubjectKind(), SubjectID: in.GetSubjectId(), EventType: notify.EventType(in.GetEventType()), ChannelID: in.GetChannelId(),
		})
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.SubscribeResponse{Subscription: subscriptionProto(sub)}, nil
	})
}

// Unsubscribe removes a subscription.
func (s Notification) Unsubscribe(ctx context.Context, req *connect.Request[pluxv1.UnsubscribeRequest]) (*connect.Response[pluxv1.UnsubscribeResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.UnsubscribeResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		if err := s.h.Notify.Unsubscribe(ctx, p, req.Msg.GetId()); err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.UnsubscribeResponse{}, nil
	})
}

// ListSubscriptions lists subscriptions.
func (s Notification) ListSubscriptions(ctx context.Context, req *connect.Request[pluxv1.ListSubscriptionsRequest]) (*connect.Response[pluxv1.ListSubscriptionsResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListSubscriptionsResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		subs, err := s.h.Notify.ListSubscriptions(ctx, p, req.Msg.GetAll())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListSubscriptionsResponse{}
		for _, sub := range subs {
			out.Subscriptions = append(out.Subscriptions, subscriptionProto(sub))
		}
		return out, nil
	})
}

func notificationChannelProto(c notify.Channel) *pluxv1.NotificationChannel {
	return &pluxv1.NotificationChannel{
		Id: c.ID, Name: c.Name, Kind: string(c.Kind), Settings: c.Settings, Hint: c.Hint, Enabled: c.Enabled, CreatedAt: ts(c.CreatedAt),
	}
}

// CreateNotificationChannel configures a destination.
func (s Notification) CreateNotificationChannel(ctx context.Context, req *connect.Request[pluxv1.CreateNotificationChannelRequest]) (*connect.Response[pluxv1.CreateNotificationChannelResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.CreateNotificationChannelResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		c, err := s.h.Notify.CreateChannel(ctx, p, notify.ChannelSpec{
			Name: m.GetName(), Kind: notify.Kind(m.GetKind()), Settings: m.GetSettings(), Secret: m.GetSecret(),
		})
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.CreateNotificationChannelResponse{Channel: notificationChannelProto(c)}, nil
	})
}

// ListNotificationChannels lists the destinations.
func (s Notification) ListNotificationChannels(ctx context.Context, req *connect.Request[pluxv1.ListNotificationChannelsRequest]) (*connect.Response[pluxv1.ListNotificationChannelsResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListNotificationChannelsResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		channels, err := s.h.Notify.ListChannels(ctx, p)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListNotificationChannelsResponse{}
		for _, c := range channels {
			out.Channels = append(out.Channels, notificationChannelProto(c))
		}
		return out, nil
	})
}

// DeleteNotificationChannel removes a destination.
func (s Notification) DeleteNotificationChannel(ctx context.Context, req *connect.Request[pluxv1.DeleteNotificationChannelRequest]) (*connect.Response[pluxv1.DeleteNotificationChannelResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.DeleteNotificationChannelResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		if err := s.h.Notify.DeleteChannel(ctx, p, req.Msg.GetId()); err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.DeleteNotificationChannelResponse{}, nil
	})
}

// TestNotificationChannel sends a test message.
func (s Notification) TestNotificationChannel(ctx context.Context, req *connect.Request[pluxv1.TestNotificationChannelRequest]) (*connect.Response[pluxv1.TestNotificationChannelResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.TestNotificationChannelResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		if err := s.h.Notify.TestChannel(ctx, p, req.Msg.GetId()); err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.TestNotificationChannelResponse{}, nil
	})
}

// ListDeliveries reads the delivery log of channels.
func (s Notification) ListDeliveries(ctx context.Context, req *connect.Request[pluxv1.ListDeliveriesRequest]) (*connect.Response[pluxv1.ListDeliveriesResponse], error) {
	m := req.Msg
	return read(ctx, func(ctx context.Context) (*pluxv1.ListDeliveriesResponse, error) {
		list, err := deliveries(ctx, s.h, req.Header(), notify.DeliveryFilter{Kind: "channel", TargetID: m.GetTargetId(), State: m.GetState(), Limit: m.GetLimit()})
		if err != nil {
			return nil, err
		}
		return &pluxv1.ListDeliveriesResponse{Deliveries: list}, nil
	})
}

// deliveries reads one kind of delivery log.
func deliveries(ctx context.Context, h *Handlers, header http.Header, filter notify.DeliveryFilter) ([]*pluxv1.Delivery, error) {
	p, err := h.principal(ctx, header, "")
	if err != nil {
		return nil, err
	}
	rows, err := h.Notify.Deliveries(ctx, p, filter)
	if err != nil {
		return nil, err //nolint:wrapcheck // a domain error
	}
	out := make([]*pluxv1.Delivery, 0, len(rows))
	for _, d := range rows {
		out = append(out, &pluxv1.Delivery{
			Id: d.ID, Kind: d.Kind, TargetId: d.TargetID, EventType: string(d.EventType), State: d.State, Attempts: d.Attempts,
			NextAttempt: ts(d.NextAttempt), LastStatus: d.LastStatus, LastError: d.LastError, CreatedAt: ts(d.CreatedAt), DeliveredAt: ts(d.DeliveredAt),
		})
	}
	return out, nil
}

// Webhook serves WebhookService: outbound webhooks (SRV-063, ADR-0081).
type Webhook struct{ h *Handlers }

var _ pluxv1connect.WebhookServiceHandler = Webhook{}

// Webhook returns the WebhookService handler.
func (h *Handlers) Webhook() Webhook { return Webhook{h: h} }

func webhookProto(e notify.Endpoint) *pluxv1.OutboundWebhook {
	types := make([]string, len(e.EventTypes))
	for i, t := range e.EventTypes {
		types[i] = string(t)
	}
	return &pluxv1.OutboundWebhook{
		Id: e.ID, Name: e.Name, Url: e.URL, EventTypes: types, Enabled: e.Enabled, CreatedAt: ts(e.CreatedAt), SecretHint: e.SecretHint,
	}
}

// overlapOf converts the wire's seconds: zero stays zero for the default,
// a negative ends the overlap at once.
func overlapOf(seconds int64) time.Duration { return time.Duration(seconds) * time.Second }

// CreateWebhook registers an endpoint.
func (s Webhook) CreateWebhook(ctx context.Context, req *connect.Request[pluxv1.CreateWebhookRequest]) (*connect.Response[pluxv1.CreateWebhookResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.CreateWebhookResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		types := make([]notify.EventType, len(m.GetEventTypes()))
		for i, t := range m.GetEventTypes() {
			types[i] = notify.EventType(t)
		}
		e, secret, err := s.h.Notify.CreateEndpoint(ctx, p, notify.EndpointSpec{Name: m.GetName(), URL: m.GetUrl(), EventTypes: types})
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.CreateWebhookResponse{Webhook: webhookProto(e), Secret: secret}, nil
	})
}

// ListWebhooks lists the endpoints.
func (s Webhook) ListWebhooks(ctx context.Context, req *connect.Request[pluxv1.ListWebhooksRequest]) (*connect.Response[pluxv1.ListWebhooksResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListWebhooksResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		endpoints, err := s.h.Notify.ListEndpoints(ctx, p)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListWebhooksResponse{}
		for _, e := range endpoints {
			out.Webhooks = append(out.Webhooks, webhookProto(e))
		}
		return out, nil
	})
}

// DeleteWebhook removes an endpoint.
func (s Webhook) DeleteWebhook(ctx context.Context, req *connect.Request[pluxv1.DeleteWebhookRequest]) (*connect.Response[pluxv1.DeleteWebhookResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.DeleteWebhookResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		if err := s.h.Notify.DeleteEndpoint(ctx, p, req.Msg.GetId()); err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.DeleteWebhookResponse{}, nil
	})
}

// RotateWebhookSecret issues a new secret.
func (s Webhook) RotateWebhookSecret(ctx context.Context, req *connect.Request[pluxv1.RotateWebhookSecretRequest]) (*connect.Response[pluxv1.RotateWebhookSecretResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.RotateWebhookSecretResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		secret, err := s.h.Notify.RotateEndpointSecret(ctx, p, req.Msg.GetId(), overlapOf(req.Msg.GetOverlapSeconds()))
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.RotateWebhookSecretResponse{Secret: secret}, nil
	})
}

// ListWebhookDeliveries reads the delivery log of webhooks.
func (s Webhook) ListWebhookDeliveries(ctx context.Context, req *connect.Request[pluxv1.ListWebhookDeliveriesRequest]) (*connect.Response[pluxv1.ListWebhookDeliveriesResponse], error) {
	m := req.Msg
	return read(ctx, func(ctx context.Context) (*pluxv1.ListWebhookDeliveriesResponse, error) {
		list, err := deliveries(ctx, s.h, req.Header(), notify.DeliveryFilter{Kind: "webhook", TargetID: m.GetTargetId(), State: m.GetState(), Limit: m.GetLimit()})
		if err != nil {
			return nil, err
		}
		return &pluxv1.ListWebhookDeliveriesResponse{Deliveries: list}, nil
	})
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package studioevents carries the events of the Studio stream: lock
// changes, document revision changes and the inbox unread count, on the
// shared log layer so that every replica serves every tab (STU-075,
// SRV-062, ADR-0096).
package studioevents

import (
	"context"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/document"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// Publisher announces Studio events. A publisher never fails its caller:
// a lost event is covered by the stream's reset (STU-075).
type Publisher interface {
	// LockChanged announces a lock taken, renewed by a takeover, or released.
	LockChanged(ctx context.Context, org string, l document.Lock, released bool)
	// DocumentChanged announces a new revision of a document.
	DocumentChanged(ctx context.Context, org, appID, pluginID, path string, revision int64, actor audit.Actor)
	// InboxChanged announces a person's new unread count.
	InboxChanged(ctx context.Context, org, user string, unread int64)
}

// orgLog names the log of an organisation's lock and document events.
func orgLog(org string) string { return "studio:org:" + org }

// userLog names the log of one person's events.
func userLog(org, user string) string { return "studio:user:" + org + ":" + user }

// LogPublisher publishes to the shared logs.
type LogPublisher struct{ org, user cache.LogWriter }

var _ Publisher = LogPublisher{}

// NewPublisher returns a publisher on the shared cache, or a no-op one
// when the cache keeps no logs or carries no notices.
func NewPublisher(shared cache.Cache, set limits.Set) Publisher {
	logs, okLogs := shared.(cache.Logs)
	n, okNotifier := shared.(cache.Notifier)
	if !okLogs || !okNotifier {
		return NopPublisher{}
	}
	maxLen := set.Get(limits.StudioEventsStreamBufferEvents)
	ttl := time.Duration(set.Get(limits.StudioUserEventsTTLMinutes)) * time.Minute
	return LogPublisher{
		org:  cache.LogWriter{Logs: logs, Notifier: n, MaxLen: maxLen},
		user: cache.LogWriter{Logs: logs, Notifier: n, MaxLen: maxLen, TTL: ttl},
	}
}

// LockChanged implements Publisher.
func (p LogPublisher) LockChanged(ctx context.Context, org string, l document.Lock, released bool) {
	ev := &pluxv1.LockChanged{AppId: l.AppID, PluginId: l.PluginID, Scope: l.Scope}
	if !released {
		ev.Lock = lockProto(l)
	}
	p.append(ctx, p.org, orgLog(org), &pluxv1.StudioEventsServiceWatchResponse{Event: &pluxv1.StudioEventsServiceWatchResponse_Lock{Lock: ev}})
}

// DocumentChanged implements Publisher.
func (p LogPublisher) DocumentChanged(ctx context.Context, org, appID, pluginID, path string, revision int64, actor audit.Actor) {
	ev := &pluxv1.DocumentChanged{AppId: appID, PluginId: pluginID, Path: path, Revision: revision, Actor: actorProto(actor)}
	p.append(ctx, p.org, orgLog(org), &pluxv1.StudioEventsServiceWatchResponse{Event: &pluxv1.StudioEventsServiceWatchResponse_Document{Document: ev}})
}

// InboxChanged implements Publisher.
func (p LogPublisher) InboxChanged(ctx context.Context, org, user string, unread int64) {
	ev := &pluxv1.InboxChanged{Unread: unread}
	p.append(ctx, p.user, userLog(org, user), &pluxv1.StudioEventsServiceWatchResponse{Event: &pluxv1.StudioEventsServiceWatchResponse_Inbox{Inbox: ev}})
}

// append encodes and appends an event; a failure is dropped on purpose.
func (LogPublisher) append(ctx context.Context, w cache.LogWriter, log string, ev *pluxv1.StudioEventsServiceWatchResponse) {
	payload, err := proto.Marshal(ev)
	if err != nil {
		return
	}
	_, _ = w.Append(context.WithoutCancel(ctx), log, payload)
}

// NopPublisher announces nothing.
type NopPublisher struct{}

var _ Publisher = NopPublisher{}

// LockChanged implements Publisher.
func (NopPublisher) LockChanged(context.Context, string, document.Lock, bool) {}

// DocumentChanged implements Publisher.
func (NopPublisher) DocumentChanged(context.Context, string, string, string, string, int64, audit.Actor) {
}

// InboxChanged implements Publisher.
func (NopPublisher) InboxChanged(context.Context, string, string, int64) {}

func actorProto(a audit.Actor) *pluxv1.Actor {
	return &pluxv1.Actor{Kind: a.Kind, Id: a.ID, Display: a.Display}
}

func lockProto(l document.Lock) *pluxv1.Lock {
	out := &pluxv1.Lock{
		AppId: l.AppID, PluginId: l.PluginID, Scope: l.Scope, Holder: actorProto(l.Holder), Session: l.Session,
		AcquiredAt: timestamppb.New(l.AcquiredAt), ExpiresAt: timestamppb.New(l.ExpiresAt),
	}
	for _, a := range l.RequestedBy {
		out.RequestedBy = append(out.RequestedBy, actorProto(a))
	}
	return out
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package studioevents

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Defaults of the watch loop.
const (
	// DefaultPoll is the fallback poll of each followed log (ADR-0086).
	DefaultPoll = 2 * time.Second
	// DefaultHeartbeat keeps an idle stream alive through proxies.
	DefaultHeartbeat = 25 * time.Second
	readBatch        = 100
)

// Service serves the Studio events stream.
type Service struct {
	logs      cache.Logs
	notifier  cache.Notifier
	poll      time.Duration
	heartbeat time.Duration
}

// Options configures a Service.
type Options struct {
	Logs     cache.Logs
	Notifier cache.Notifier
	// Poll and Heartbeat default to DefaultPoll and DefaultHeartbeat.
	Poll, Heartbeat time.Duration
}

// NewService returns the stream service.
func NewService(o Options) (*Service, error) {
	if o.Logs == nil || o.Notifier == nil {
		return nil, errors.New("studioevents: logs and a notifier are required")
	}
	if o.Poll <= 0 {
		o.Poll = DefaultPoll
	}
	if o.Heartbeat <= 0 {
		o.Heartbeat = DefaultHeartbeat
	}
	return &Service{logs: o.Logs, notifier: o.Notifier, poll: o.Poll, heartbeat: o.Heartbeat}, nil
}

// item is one thing a follower hands to the send loop.
type item struct {
	log     int // 0 organisation log, 1 user log
	id      string
	payload []byte
	gap     bool // the follower fell behind; id is the newest entry
}

// Watch sends the principal's events after cursor ("<org entry>|<user entry>",
// empty starts now) until ctx ends or send fails. Events about apps the
// principal cannot read are dropped; a reader that fell behind gets a reset
// and carries on from the newest entry (STU-075). At every heartbeat refresh
// re-resolves the principal: its error ends the stream and its result filters
// later events (SEC-101, GOV-005).
func (s *Service) Watch(ctx context.Context, p auth.Principal, cursor string, refresh func(context.Context) (auth.Principal, error), send func(*pluxv1.StudioEventsServiceWatchResponse) error) error {
	names := [2]string{orgLog(p.OrganizationID), userLog(p.OrganizationID, p.UserID)}
	cur, err := s.start(ctx, names, cursor)
	if err != nil {
		return err
	}
	out := make(chan item)
	errc := make(chan error, len(names))
	var wg sync.WaitGroup
	defer wg.Wait() // runs after cancel below
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	for i := range names {
		wg.Go(func() { errc <- s.follow(ctx, i, names[i], cur[i], out) })
	}
	ticker := time.NewTicker(s.heartbeat)
	defer ticker.Stop()
	for {
		var msg *pluxv1.StudioEventsServiceWatchResponse
		select {
		case <-ctx.Done():
			return nil
		case err := <-errc:
			if ctx.Err() != nil {
				return nil
			}
			return err
		case <-ticker.C:
			np, err := refresh(ctx)
			if err != nil {
				return err
			}
			p = np
			msg = &pluxv1.StudioEventsServiceWatchResponse{Event: &pluxv1.StudioEventsServiceWatchResponse_Heartbeat{Heartbeat: &pluxv1.StudioHeartbeat{}}}
		case it := <-out:
			cur[it.log] = it.id
			if it.gap {
				msg = &pluxv1.StudioEventsServiceWatchResponse{Event: &pluxv1.StudioEventsServiceWatchResponse_Reset_{Reset_: &pluxv1.StudioReset{}}}
				break
			}
			if msg = decode(p, it.payload); msg == nil {
				continue
			}
		}
		msg.Cursor = cur[0] + "|" + cur[1]
		if err := send(msg); err != nil {
			return err
		}
	}
}

// start resolves the cursor: the one given, or the newest entry of each log.
func (s *Service) start(ctx context.Context, names [2]string, cursor string) ([2]string, error) {
	var cur [2]string
	if cursor != "" {
		org, user, ok := strings.Cut(cursor, "|")
		if !ok {
			return cur, plxerr.New(plxerr.InvalidPageToken, "the cursor is not one this stream issued")
		}
		return [2]string{org, user}, nil
	}
	for i, name := range names {
		last, err := s.logs.LogLast(ctx, name)
		if err != nil {
			return cur, err //nolint:wrapcheck // a cache error is reported as is
		}
		cur[i] = last
	}
	return cur, nil
}

// follow feeds one log into out, turning a gap into an item and carrying on.
func (s *Service) follow(ctx context.Context, idx int, name, after string, out chan<- item) error {
	push := func(it item) error {
		select {
		case out <- it:
			return nil
		case <-ctx.Done():
			return ctx.Err() //nolint:wrapcheck // the caller's own cancellation
		}
	}
	for {
		err := cache.Follow(ctx, s.logs, s.notifier, name, after, s.poll, readBatch, func(e cache.StreamEntry) error {
			return push(item{log: idx, id: e.ID, payload: e.Payload})
		})
		if !errors.Is(err, cache.ErrLogGap) {
			return err //nolint:wrapcheck // reported as is
		}
		last, lerr := s.logs.LogLast(ctx, name)
		if lerr != nil {
			return lerr //nolint:wrapcheck // reported as is
		}
		if err := push(item{log: idx, id: last, gap: true}); err != nil {
			return err
		}
		after = last
	}
}

// decode reads an entry and returns it, or nil when the principal may not
// see it or it is unreadable.
func decode(p auth.Principal, payload []byte) *pluxv1.StudioEventsServiceWatchResponse {
	msg := &pluxv1.StudioEventsServiceWatchResponse{}
	if proto.Unmarshal(payload, msg) != nil {
		return nil
	}
	switch ev := msg.GetEvent().(type) {
	case *pluxv1.StudioEventsServiceWatchResponse_Lock:
		if !mayRead(p, ev.Lock.GetAppId()) {
			return nil
		}
	case *pluxv1.StudioEventsServiceWatchResponse_Document:
		if !mayRead(p, ev.Document.GetAppId()) {
			return nil
		}
	case *pluxv1.StudioEventsServiceWatchResponse_Inbox:
	default:
		return nil
	}
	return msg
}

// mayRead reports whether the principal reads the app, the check a document
// read makes.
func mayRead(p auth.Principal, appID string) bool {
	return p.HoldsOnApp(auth.PluginRead, appID) || p.HoldsOnApp(auth.AppRead, appID)
}

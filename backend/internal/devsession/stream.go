// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package devsession

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/encoding/protodelim"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

const (
	// TraceFormatVersion is the version of the trace files ExportTrace
	// writes (TraceFileHeader).
	TraceFormatVersion = 1
	// mergedLogLines bounds the server log lines merged into one function
	// call event (FN-036).
	mergedLogLines = 50
	// mergeScanRows bounds the log rows read to find them.
	mergeScanRows = 500
	// mergeSlack widens the window of a call's log lines around the
	// device's clock, which may be off from the server's.
	mergeSlack = 5 * time.Minute
)

// Hub returns the relay of the sessions this process serves, which the
// push of patches delivers its bundles through.
func (s *Service) Hub() *Hub { return s.hub }

// CommandSent says where a command went.
type CommandSent struct {
	CommandID string
	// DeviceIDs are the session devices the command was sent to.
	DeviceIDs []string
}

// SendCommand sends a command to a device of a session, or to each of its
// devices when the request names none (DEV-011, DEV-031). It needs
// devsession.create on the session's environment; the device must be
// paired with the session (PLX-9204), and the session within
// devSession.commandsPerMinute (PLX-9206). The audit entry names the
// command's kind, never its values.
func (s *Service) SendCommand(ctx context.Context, p auth.Principal, req *pluxv1.DevSessionServiceSendCommandRequest) (CommandSent, error) {
	if err := authorizeAnywhere(p, auth.DevSessionCreate); err != nil {
		return CommandSent{}, err
	}
	id, err := parseID(req.GetSessionId(), "session")
	if err != nil {
		return CommandSent{}, err
	}
	kind := commandKind(req)
	if kind == "" {
		return CommandSent{}, plxerr.New(plxerr.InvalidFormat, "the request carries no command")
	}
	var out CommandSent
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		out, err = s.command(ctx, tx, p, id, req.GetDeviceId(), kind)
		return err
	})
	if err != nil {
		return CommandSent{}, err
	}
	cmd := proto.CloneOf(req)
	cmd.SessionId, cmd.DeviceId = "", ""
	frame := &pluxv1.DevServerFrame{Payload: &pluxv1.DevServerFrame_Command{Command: &pluxv1.DevCommandFrame{CommandId: out.CommandID, Command: cmd}}}
	for _, device := range out.DeviceIDs {
		if err := s.hub.Deliver(storage.ID(id), device, frame); err != nil {
			return CommandSent{}, err
		}
	}
	return out, nil
}

// command authorises a command inside the transaction, counts it against
// the session's rate and audits it.
func (s *Service) command(ctx context.Context, tx pgx.Tx, p auth.Principal, id pgtype.UUID, deviceID, kind string) (CommandSent, error) {
	session, err := s.authorized(ctx, tx, p, id, auth.DevSessionCreate, true)
	if err != nil {
		return CommandSent{}, err
	}
	set, err := s.o.Limits(ctx, tx, session.OrganizationID, session.AppID)
	if err != nil {
		return CommandSent{}, fmt.Errorf("devsession: read the limits: %w", err)
	}
	now := s.now()
	if !session.Live(now, minutes(set, limits.DevSessionIdleMinutes)) {
		return CommandSent{}, expired()
	}
	q := dbgen.New(tx)
	rows, err := q.ListDevSessionDevices(ctx, id)
	if err != nil {
		return CommandSent{}, fmt.Errorf("devsession: list the devices: %w", err)
	}
	targets := make([]string, 0, len(rows))
	for _, r := range rows {
		if deviceID == "" || storage.ID(r.ID) == deviceID {
			targets = append(targets, storage.ID(r.ID))
		}
	}
	if deviceID != "" && len(targets) == 0 {
		return CommandSent{}, notPaired()
	}
	lim := relayLimitsOf(set)
	if !s.hub.room(session.ID, &lim).allowCommand(now) {
		return CommandSent{}, plxerr.New(plxerr.DevCommandNotAllowed, "the dev session may send %d commands a minute (devSession.commandsPerMinute)", lim.commands)
	}
	commandID, err := s.o.IDs.New()
	if err != nil {
		return CommandSent{}, fmt.Errorf("devsession: %w", err)
	}
	if err := q.TouchDevSession(ctx, dbgen.TouchDevSessionParams{ID: id, At: storage.Timestamp(now)}); err != nil {
		return CommandSent{}, fmt.Errorf("devsession: record the activity: %w", err)
	}
	_, err = s.o.Audit.Append(ctx, tx, audit.Entry{
		OrganizationID: p.OrganizationID, Actor: p.Actor(), Action: audit.DevSessionCommandSent, TargetKind: "dev_session", TargetID: session.ID,
		Detail: fmt.Sprintf("command %s %s to %d device(s)", commandID, kind, len(targets)),
	})
	if err != nil {
		return CommandSent{}, fmt.Errorf("devsession: %w", err)
	}
	return CommandSent{CommandID: commandID, DeviceIDs: targets}, nil
}

// commandKind names the command a request carries, such as
// "open_screen"; empty when it carries none.
func commandKind(req *pluxv1.DevSessionServiceSendCommandRequest) string {
	m := req.ProtoReflect()
	f := m.WhichOneof(m.Descriptor().Oneofs().ByName("command"))
	if f == nil {
		return ""
	}
	return string(f.Name())
}

// Watch streams a session's events to send (DEV-021): first the retained
// events after the sequence after (all of them when it is zero), then each
// event as it is relayed, until the session ends or ctx is done. It needs
// devsession.observe on the session's environment. A watcher that falls
// behind loses its oldest events and is told how many with a drop event;
// the devices are never held up.
func (s *Service) Watch(ctx context.Context, p auth.Principal, sessionID string, deviceIDs []string, after uint64, send func(*pluxv1.DevEvent) error) error {
	session, lim, err := s.observed(ctx, p, sessionID)
	if err != nil {
		return err
	}
	room := s.hub.room(session.ID, &lim)
	if live, err := s.live(ctx, session.OrganizationID, session.ID); err != nil {
		return err
	} else if !live {
		s.hub.End(session.ID, plxerr.DevSessionExpired)
	}
	w, backlog := room.subscribe(after, filterOf(deviceIDs))
	defer room.unsubscribe(w)
	for _, ev := range backlog {
		if err := send(ev); err != nil {
			return err
		}
	}
	return s.follow(ctx, session, room, w, after, send)
}

// follow sends a watcher's events as they are relayed, until the session
// ends or ctx is done, and ends the relay when it finds the session no
// longer live.
func (s *Service) follow(ctx context.Context, session Session, room *room, w *watcher, after uint64, send func(*pluxv1.DevEvent) error) error {
	tick := time.NewTicker(s.pingInterval())
	defer tick.Stop()
	for {
		events, ended := room.take(w)
		for _, ev := range events {
			if ev.GetDrop() == nil && ev.GetSequence() <= after {
				continue
			}
			if err := send(ev); err != nil {
				return err
			}
		}
		if ended {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-w.wake:
		case <-tick.C:
			if live, err := s.live(ctx, session.OrganizationID, session.ID); err == nil && !live {
				s.hub.End(session.ID, plxerr.DevSessionExpired)
			}
		}
	}
}

// Trace is an exported trace file.
type Trace struct {
	// File is the gzip-compressed stream of length-prefixed messages.
	File []byte
	// Events is the number of events in it.
	Events uint64
}

// ExportTrace writes the retained events of a session, of the devices
// named or of all of them, as a trace file: gzip over length-prefixed
// binary messages, a TraceFileHeader first and then the events in
// sequence order, redacted as the device sent them (DEV-021). It needs
// devsession.observe on the session's environment.
func (s *Service) ExportTrace(ctx context.Context, p auth.Principal, sessionID string, deviceIDs []string) (Trace, error) {
	session, _, err := s.observed(ctx, p, sessionID)
	if err != nil {
		return Trace{}, err
	}
	var appKey string
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		app, err := dbgen.New(tx).GetApp(ctx, storage.MustUUID(session.AppID))
		if err != nil {
			return fmt.Errorf("devsession: find the app: %w", err)
		}
		appKey = app.Key
		return nil
	})
	if err != nil {
		return Trace{}, err
	}
	var events []*pluxv1.DevEvent
	if room, ok := s.hub.lookup(session.ID); ok {
		events = room.retainedEvents(filterOf(deviceIDs))
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	header := &pluxv1.TraceFileHeader{
		FormatVersion: TraceFormatVersion, SessionId: session.ID, AppId: session.AppID, AppKey: appKey,
		EnvironmentId: session.EnvironmentID, EnvironmentKey: session.EnvironmentKey,
		ExportedAt: timestamppb.New(s.now()), ServerVersion: s.o.ServerVersion,
	}
	if _, err := protodelim.MarshalTo(zw, header); err != nil {
		return Trace{}, fmt.Errorf("devsession: write the trace: %w", err)
	}
	for _, ev := range events {
		if _, err := protodelim.MarshalTo(zw, ev); err != nil {
			return Trace{}, fmt.Errorf("devsession: write the trace: %w", err)
		}
	}
	if err := zw.Close(); err != nil {
		return Trace{}, fmt.Errorf("devsession: write the trace: %w", err)
	}
	return Trace{File: buf.Bytes(), Events: uint64(len(events))}, nil
}

// observed reads a session a principal may observe, with its relay bounds.
func (s *Service) observed(ctx context.Context, p auth.Principal, sessionID string) (Session, relayLimits, error) {
	if err := authorizeAnywhere(p, auth.DevSessionObserve); err != nil {
		return Session{}, relayLimits{}, err
	}
	id, err := parseID(sessionID, "session")
	if err != nil {
		return Session{}, relayLimits{}, err
	}
	var (
		session Session
		lim     relayLimits
	)
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		session, err = s.authorized(ctx, tx, p, id, auth.DevSessionObserve, false)
		if err != nil {
			return err
		}
		set, err := s.o.Limits(ctx, tx, session.OrganizationID, session.AppID)
		if err != nil {
			return fmt.Errorf("devsession: read the limits: %w", err)
		}
		lim = relayLimitsOf(set)
		return nil
	})
	return session, lim, err
}

// live reports whether a session is live, without recording traffic.
func (s *Service) live(ctx context.Context, organizationID, sessionID string) (bool, error) {
	var live bool
	err := s.inTenant(ctx, storage.Tenant{OrganizationID: organizationID}, func(ctx context.Context, tx pgx.Tx) error {
		row, err := dbgen.New(tx).GetDevSession(ctx, storage.MustUUID(sessionID))
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("devsession: find the session: %w", err)
		}
		session := sessionOf(row)
		set, err := s.o.Limits(ctx, tx, session.OrganizationID, session.AppID)
		if err != nil {
			return fmt.Errorf("devsession: read the limits: %w", err)
		}
		live = session.Live(s.now(), minutes(set, limits.DevSessionIdleMinutes))
		return nil
	})
	return live, err
}

// filterOf turns a list of session devices into a filter; an empty list
// allows all.
func filterOf(deviceIDs []string) map[string]bool {
	if len(deviceIDs) == 0 {
		return nil
	}
	out := make(map[string]bool, len(deviceIDs))
	for _, id := range deviceIDs {
		out[id] = true
	}
	return out
}

// logLevel maps a stored function log level to the stream's.
func logLevel(level string) pluxv1.DevLogLevel {
	switch level {
	case "debug":
		return pluxv1.DevLogLevel_DEV_LOG_LEVEL_DEBUG
	case "info":
		return pluxv1.DevLogLevel_DEV_LOG_LEVEL_INFO
	case "warn":
		return pluxv1.DevLogLevel_DEV_LOG_LEVEL_WARNING
	case "error":
		return pluxv1.DevLogLevel_DEV_LOG_LEVEL_ERROR
	}
	return pluxv1.DevLogLevel_DEV_LOG_LEVEL_UNSPECIFIED
}

// mergeFunctionLogs adds to a server function's call event the log lines
// its run wrote, joined by the event's trace ID (D-7, FN-036). Only lines
// of the session's own organisation, app and environment are read, at most
// mergedLogLines of them, and no more than keep the event within maxBytes.
// The relay carries the event without them when they cannot be read.
func (s *Service) mergeFunctionLogs(ctx context.Context, b DeviceBinding, ev *pluxv1.DevEvent, maxBytes int64) {
	call := ev.GetFunctionCall()
	if call == nil || ev.GetTraceId() == "" || call.GetPlacement() != "server" || call.GetFunction() == "" {
		return
	}
	app, env := storage.MustUUID(b.AppID), storage.MustUUID(b.EnvironmentID)
	var version int32
	if v := call.GetVersion(); v <= math.MaxInt32 {
		version = int32(v)
	}
	start := time.UnixMicro(call.GetStartUnixMicros())
	end := start.Add(time.Duration(call.GetDurationMicros()) * time.Microsecond)
	var rows []dbgen.FunctionLog
	err := s.inTenant(ctx, storage.Tenant{OrganizationID: b.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		rows, err = dbgen.New(tx).ListFunctionLogs(ctx, dbgen.ListFunctionLogsParams{
			AppID: app, EnvironmentID: env, Function: call.GetFunction(), Version: version,
			Since: storage.Timestamp(start.Add(-mergeSlack)), BeforeTime: storage.Timestamp(end.Add(mergeSlack)),
			BeforeID: math.MaxInt64, PageSize: mergeScanRows,
		})
		return err //nolint:wrapcheck // the relay carries the event without the lines
	})
	if err != nil {
		return
	}
	slices.Reverse(rows)
	merged := 0
	for _, r := range rows {
		if merged == mergedLogLines {
			break
		}
		if r.TraceID != ev.GetTraceId() || r.AppID != app || r.EnvironmentID != env {
			continue
		}
		call.Logs = append(call.Logs, &pluxv1.DevLogLine{
			TimeUnixMicros: r.LoggedAt.Time.UnixMicro(), Level: logLevel(r.Level), Message: r.Message,
		})
		if int64(proto.Size(ev)) > maxBytes {
			call.Logs = call.Logs[:len(call.Logs)-1]
			break
		}
		merged++
	}
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// DefaultRetention is how long function logs and invocation records are kept
// unless the operator sets another (FN-032); the server passes the telemetry
// retention.
const DefaultRetention = 30 * 24 * time.Hour

// Results an invocation is counted under besides the lower-case reason of a
// system error.
const (
	ResultOK            = "ok"
	ResultBusinessError = "business_error"
)

// Log levels as stored and reported.
const (
	levelDebug = "debug"
	levelInfo  = "info"
	levelWarn  = "warn"
	levelError = "error"
)

// maxLogMessageBytes bounds one stored line. The runner already bounds the
// lines of an invocation by function.logBytes (LIM-001); this keeps a single
// line within a row of sane size should that bound be raised.
const maxLogMessageBytes = 8 * 1024

// Observation is a finished run of a server function, or the replay of a
// stored result, as the invoker reports it (FN-032).
type Observation struct {
	OrganizationID, AppID, EnvironmentID string
	Function                             string
	Version                              int32
	// Result is ResultOK, ResultBusinessError or the lower-case reason of
	// the system error that ended the call.
	Result string
	// Duration is the time the runner took; zero for a replay.
	Duration        time.Duration
	MemoryPeakBytes uint64
	// Replayed is true when the answer was the stored one (FN-021).
	Replayed bool
}

// InvocationObserver stores finished invocations for the per-function
// metrics (FN-032). It never fails an invocation.
type InvocationObserver interface {
	Observe(ctx context.Context, o Observation)
}

// InvokeMetrics counts what the api role saw end to end (FN-032). The labels
// are the result only: never an app, user, function or input.
type InvokeMetrics interface {
	// Invoked records one call: its result, its time from the request to the
	// answer, and whether the answer was a stored one.
	Invoked(result string, d time.Duration, replayed bool)
}

var (
	_ FunctionLogSink    = (*Service)(nil)
	_ InvocationObserver = (*Service)(nil)
)

// Record stores the lines a finished invocation logged (FN-032). A failure
// is logged without the lines and never fails the invocation.
func (s *Service) Record(ctx context.Context, meta LogMeta, logs []LogLine) {
	if len(logs) == 0 {
		return
	}
	if err := s.storeLogs(ctx, meta, logs); err != nil {
		s.warn(ctx, "storing function logs failed", err)
	}
}

func (s *Service) storeLogs(ctx context.Context, meta LogMeta, logs []LogLine) error {
	org, app, env, err := scopeIDs(meta.OrganizationID, meta.AppID, meta.EnvironmentID)
	if err != nil {
		return err
	}
	invocation, err := s.newID()
	if err != nil {
		return err
	}
	inv, err := storage.UUID(invocation)
	if err != nil {
		return fmt.Errorf("function: %w", err)
	}
	p := dbgen.InsertFunctionLogsParams{
		OrganizationID: org, AppID: app, EnvironmentID: env, Function: meta.Function, Version: meta.Version,
		InvocationID: inv, TraceID: traceIDOf(meta.Traceparent),
	}
	for _, l := range logs {
		p.Levels = append(p.Levels, levelName(l.Level))
		p.Messages = append(p.Messages, clipMessage(l.Message))
		p.Times = append(p.Times, storage.Timestamp(l.Time))
	}
	return s.inTenant(ctx, storage.Tenant{OrganizationID: meta.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		if err := dbgen.New(tx).InsertFunctionLogs(ctx, p); err != nil {
			return fmt.Errorf("function: insert logs: %w", err)
		}
		return nil
	})
}

// Observe stores a finished invocation for the metrics (FN-032).
func (s *Service) Observe(ctx context.Context, o Observation) {
	org, app, env, err := scopeIDs(o.OrganizationID, o.AppID, o.EnvironmentID)
	if err == nil {
		err = s.inTenant(ctx, storage.Tenant{OrganizationID: o.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
			return dbgen.New(tx).InsertFunctionInvocation(ctx, dbgen.InsertFunctionInvocationParams{
				OrganizationID: org, AppID: app, EnvironmentID: env, Function: o.Function, Version: o.Version, Result: o.Result,
				DurationMicros: o.Duration.Microseconds(), MemoryPeakBytes: int64(min(o.MemoryPeakBytes, math.MaxInt64)), //nolint:gosec // G115: clamped
				Replayed: o.Replayed, FinishedAt: storage.Timestamp(s.o.Now()),
			})
		})
	}
	if err != nil {
		s.warn(ctx, "storing a function invocation failed", err)
	}
}

// warn logs a recording failure; the message carries no function output.
func (s *Service) warn(ctx context.Context, msg string, err error) {
	if s.o.Log != nil {
		s.o.Log.WarnContext(ctx, msg, slog.String("error", err.Error()))
	}
}

// scopeIDs parses the organisation, app and environment of an invocation.
func scopeIDs(org, app, env string) (o, a, e pgtype.UUID, err error) {
	if o, err = storage.UUID(org); err != nil {
		return o, a, e, fmt.Errorf("function: invocation organisation: %w", err)
	}
	if a, err = storage.UUID(app); err != nil {
		return o, a, e, fmt.Errorf("function: invocation app: %w", err)
	}
	if e, err = storage.UUID(env); err != nil {
		return o, a, e, fmt.Errorf("function: invocation environment: %w", err)
	}
	return o, a, e, nil
}

// levelName maps the runner's level to its name; an unknown level is an error
// line rather than a lost one.
func levelName(l uint32) string {
	switch l {
	case 0:
		return levelDebug
	case 1:
		return levelInfo
	case 2:
		return levelWarn
	default:
		return levelError
	}
}

// clipMessage bounds one line, cutting on a character boundary, and removes
// what PostgreSQL text cannot hold.
func clipMessage(m string) string {
	m = strings.ReplaceAll(m, "\x00", "")
	if len(m) > maxLogMessageBytes {
		return strings.ToValidUTF8(m[:maxLogMessageBytes], "") + "…"
	}
	return strings.ToValidUTF8(m, "\uFFFD")
}

// traceIDOf returns the trace id of a W3C traceparent, or "" when it is not
// one.
func traceIDOf(traceparent string) string {
	parts := strings.Split(traceparent, "-")
	if len(parts) != 4 || len(parts[1]) != 32 || strings.Trim(parts[1], "0123456789abcdef") != "" {
		return ""
	}
	return parts[1]
}

// LogLineRecord is a stored log line (FN-032).
type LogLineRecord struct {
	Time    time.Time
	Version int32
	Level   string
	Message string
	TraceID string
}

// LogQuery selects the logs of a function.
type LogQuery struct {
	AppID, EnvironmentID, Name string
	// Version limits the lines to one version when non-zero.
	Version int32
	// Since limits the lines to those written at or after it; zero means the
	// retention.
	Since time.Time
}

// ListLogs returns the retained log lines of a function, newest first, after
// the cursor (FN-032).
func (s *Service) ListLogs(ctx context.Context, p auth.Principal, q LogQuery, after storage.Cursor, size int32) ([]LogLineRecord, storage.Cursor, error) {
	if err := authorize(p, auth.FunctionRead, q.AppID); err != nil {
		return nil, storage.Cursor{}, err
	}
	if q.Version < 0 {
		return nil, storage.Cursor{}, plxerr.New(plxerr.InvalidFormat, "the version must not be negative")
	}
	since := q.Since
	if oldest := s.o.Now().Add(-s.o.Retention); since.Before(oldest) {
		since = oldest
	}
	beforeTime, beforeID := s.o.Now().Add(time.Hour), int64(math.MaxInt64)
	if !after.Time.IsZero() {
		beforeTime, beforeID = after.Time, after.Sequence
	}
	var out []LogLineRecord
	var last storage.Cursor
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		dq := dbgen.New(tx)
		env, err := environmentOf(ctx, dq, q.AppID, q.EnvironmentID)
		if err != nil {
			return err
		}
		rows, err := dq.ListFunctionLogs(ctx, dbgen.ListFunctionLogsParams{
			AppID: env.AppID, EnvironmentID: env.ID, Function: q.Name, Version: q.Version, Since: storage.Timestamp(since),
			BeforeTime: storage.Timestamp(beforeTime), BeforeID: beforeID, PageSize: size,
		})
		if err != nil {
			return fmt.Errorf("function: list logs: %w", err)
		}
		for _, r := range rows {
			out = append(out, LogLineRecord{Time: storage.Time(r.LoggedAt), Version: r.Version, Level: r.Level, Message: r.Message, TraceID: r.TraceID})
			last = storage.Cursor{Time: storage.Time(r.LoggedAt), Sequence: r.ID}
		}
		return nil
	})
	return out, last, err
}

// Metrics are the invocations of one function over a window (FN-032).
type Metrics struct {
	// Invocations counts runs and replays.
	Invocations uint64
	// ErrorsByKind counts the system errors by the reason that ended them.
	ErrorsByKind map[string]uint64
	// LatencyP50, LatencyP95 and LatencyP99 are the runner's time of the
	// runs, not of the replays.
	LatencyP50, LatencyP95, LatencyP99 time.Duration
	MemoryPeakBytes                    uint64
	// CacheHits counts the replays of stored results (FN-021).
	CacheHits uint64
}

// Metrics aggregates the invocations of a function over the window before
// now; a window beyond the retention is cut to it.
func (s *Service) Metrics(ctx context.Context, p auth.Principal, appID, environmentID, name string, window time.Duration) (Metrics, error) {
	if err := authorize(p, auth.FunctionRead, appID); err != nil {
		return Metrics{}, err
	}
	if window <= 0 {
		window = time.Hour
	}
	window = min(window, s.o.Retention)
	out := Metrics{ErrorsByKind: map[string]uint64{}}
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		dq := dbgen.New(tx)
		env, err := environmentOf(ctx, dq, appID, environmentID)
		if err != nil {
			return err
		}
		since := storage.Timestamp(s.o.Now().Add(-window))
		counts, err := dq.CountFunctionInvocations(ctx, dbgen.CountFunctionInvocationsParams{AppID: env.AppID, EnvironmentID: env.ID, Function: name, Since: since})
		if err != nil {
			return fmt.Errorf("function: count invocations: %w", err)
		}
		for _, c := range counts {
			n := uint64(c.Invocations) //nolint:gosec // G115: a count is not negative
			out.Invocations += n
			if c.Replayed {
				out.CacheHits += n
			}
			if c.Result != ResultOK && c.Result != ResultBusinessError {
				out.ErrorsByKind[c.Result] += n
			}
		}
		lat, err := dq.FunctionLatency(ctx, dbgen.FunctionLatencyParams{AppID: env.AppID, EnvironmentID: env.ID, Function: name, Since: since})
		if err != nil {
			return fmt.Errorf("function: latency: %w", err)
		}
		out.LatencyP50, out.LatencyP95, out.LatencyP99 = time.Duration(lat.P50)*time.Microsecond, time.Duration(lat.P95)*time.Microsecond, time.Duration(lat.P99)*time.Microsecond
		out.MemoryPeakBytes = uint64(lat.MemoryPeak) //nolint:gosec // G115: a maximum of non-negative values
		return nil
	})
	return out, err
}

// PurgeObservations deletes an organisation's logs and invocation records
// past the retention and returns how many rows it deleted.
func (s *Service) PurgeObservations(ctx context.Context, org string) (int64, error) {
	var n int64
	cutoff := storage.Timestamp(s.o.Now().Add(-s.o.Retention))
	err := s.inTenant(ctx, storage.Tenant{OrganizationID: org}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		logs, err := q.PurgeFunctionLogs(ctx, cutoff)
		if err != nil {
			return fmt.Errorf("function: purge logs: %w", err)
		}
		runs, err := q.PurgeFunctionInvocations(ctx, cutoff)
		if err != nil {
			return fmt.Errorf("function: purge invocations: %w", err)
		}
		n = logs + runs
		return nil
	})
	return n, err
}

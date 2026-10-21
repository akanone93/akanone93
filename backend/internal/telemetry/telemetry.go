// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package telemetry ingests the runtime's batched events (Appendix G.2).
// It validates them, enforces the per-device and per-batch quotas
// (telemetry.eventsPerDevicePerHour, telemetry.batchBytes) and the
// consent categories, and hands them to analytics storage: through a
// Valkey stream written in bulk by worker consumers where one is
// configured, directly otherwise (ANL-010, ADR-0080). Storage, rollups
// and retention are in package analytics.
//
// Events never carry payloads: fields are flat, scalar and small, and a
// field whose name marks it as sensitive is refused rather than stored
// (SCH-012, ANL-003).
package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/analytics"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// DefaultRetention is how long raw events are kept unless the operator sets
// telemetry.retentionDays or an organisation sets its own: 13 months
// (SEC-160, maintainer decision Q17).
const DefaultRetention = 396 * 24 * time.Hour

// Bounds of one event's fields.
const (
	maxFieldsBytes = 4096
	maxFields      = 32
	maxText        = 256
)

// The consent categories of events (SEC-161, Appendix G.2).
const (
	CategoryNecessary   = "necessary"
	CategoryAnalytics   = "analytics"
	CategoryExperiments = "experiments"
)

// categories is the event catalogue of Appendix G.2, with the consent
// category of each event. Operational events carry no personal data and
// need no consent; the others are sent only with it.
var categories = map[string]string{
	"session_start": CategoryNecessary, "session_end": CategoryNecessary, "sync_result": CategoryNecessary,
	"error": CategoryNecessary, "fatal_error": CategoryNecessary, "control_applied": CategoryNecessary,
	"rasp_detection": CategoryNecessary, "attestation_result": CategoryNecessary, "pin_failure": CategoryNecessary,
	"screen_view": CategoryAnalytics, "render_perf": CategoryAnalytics, "action_run": CategoryAnalytics,
	"api_call": CategoryAnalytics, "function_call": CategoryAnalytics, "custom": CategoryAnalytics,
	"l10n_missing":        CategoryAnalytics,
	"experiment_exposure": CategoryExperiments, "flag_evaluated": CategoryExperiments,
}

// Category returns the consent category of a catalogued event name, and
// whether the name is catalogued.
func Category(name string) (string, bool) {
	c, ok := categories[name]
	return c, ok
}

// quotaKeyPrefix is the cache key prefix of the per-device hourly quota.
const quotaKeyPrefix = "telemetry:quota:"

// fieldName is the form of a field name.
var fieldName = regexp.MustCompile(`^[a-z][A-Za-z0-9_]{0,63}$`)

// sensitiveWords mark a field name that must never be stored.
var sensitiveWords = []string{"password", "passcode", "secret", "token", "pin", "otp", "cvv", "card", "iban", "ssn", "email", "phone"}

// ErrorRecorder groups the error events of a stored batch (ANL-040); package
// analytics implements it.
type ErrorRecorder interface {
	RecordErrors(ctx context.Context, b analytics.Batch) error
}

// IDs generates identifiers.
type IDs interface {
	New() (string, error)
}

// Options configures a Service.
type Options struct {
	DB     *storage.DB
	IDs    IDs
	Limits limits.Set
	Now    func() time.Time
	// Retention is the oldest an event may be when it arrives; zero means
	// DefaultRetention. What is kept is analytics' to decide.
	Retention time.Duration
	// Store keeps the events; the default is the PostgreSQL store.
	Store analytics.Store
	// Errors, when set, groups the error events of every stored batch
	// (ANL-040); without it errors stay in the events and rollups only.
	Errors ErrorRecorder
	// Queue, when set, takes accepted batches for the worker role's
	// consumers to write in bulk. Without it, or when it fails, the batch
	// is written directly.
	Queue Queue
	// Counter counts events per device and hour for the quota; without it
	// no quota is enforced (a single-node development setup always has the
	// in-memory cache, which counts).
	Counter cache.Counter
	// Changes and Propagation, when both are set, measure how long a
	// control change takes to reach devices (NFR-024).
	Changes     ControlChanges
	Propagation Propagation
}

// Queue carries an accepted batch from the api role to a consumer.
type Queue interface {
	Enqueue(ctx context.Context, payload []byte) error
}

// Service ingests and lists events.
type Service struct {
	o   Options
	now func() time.Time
}

// NewService returns the service.
func NewService(o Options) (*Service, error) {
	if o.DB == nil || o.IDs == nil {
		return nil, errors.New("telemetry: a database and identifiers are required")
	}
	if o.Limits.IsZero() {
		o.Limits = limits.Defaults()
	}
	if o.Retention <= 0 {
		o.Retention = DefaultRetention
	}
	now := o.Now
	if now == nil {
		now = time.Now
	}
	if o.Store == nil {
		o.Store = analytics.NewPostgres(o.DB, now)
	}
	return &Service{o: o, now: now}, nil
}

// Event is one runtime event.
type Event struct {
	ID, Name, DeviceID, PluginKey, Route string
	Time                                 time.Time
	ReleaseSequence                      int64
	// Fields is a flat JSON object of scalar values.
	Fields []byte
}

// Consent is what the host app's user agreed to, as the runtime reports it
// (SEC-161). Operational events need none.
type Consent struct {
	Analytics, Experiments bool
}

// Source is the device that sent a batch.
type Source struct {
	OrganizationID, AppID, EnvironmentID, DeviceID string
	// Bytes is the size of the request after decompression; zero means it
	// is estimated from the events (telemetry.batchBytes).
	Bytes int
	// Consent is what the runtime declared; nil when it declared nothing,
	// as older runtimes do, and the events are then taken as the device
	// gated them.
	Consent *Consent
}

// Ingest validates a batch and hands the valid events to storage. It
// reports each refused event as a diagnostic whose path is its index. The
// device is always the sender; an event naming another device is refused.
//
// A batch larger than telemetry.batchBytes is refused whole, and a device
// that sends more than telemetry.eventsPerDevicePerHour events in an hour
// has the surplus dropped and reported, both as PLX-8300. An event whose
// consent category the runtime did not declare consent for is refused too:
// the runtime filters first, and the server does not trust it to have done
// so (SEC-161, ANL-003).
func (s *Service) Ingest(ctx context.Context, src Source, events []Event) (int, plxerr.Diagnostics, error) {
	if max := s.o.Limits.Get(limits.TelemetryEventsPerRequest); int64(len(events)) > max {
		return 0, nil, plxerr.New(plxerr.LimitExceeded, "%d events, more than %s = %d", len(events), limits.TelemetryEventsPerRequest, max)
	}
	if err := s.checkBytes(src, events); err != nil {
		return 0, nil, err
	}
	var diags plxerr.Diagnostics
	valid := make([]queued, 0, len(events))
	var indexes []int
	for i, e := range events {
		fields, category, err := s.check(src, e)
		if err != nil {
			diags = append(diags, plxerr.NewDiagnostic(plxerr.InvalidFormat, plxerr.Location{Path: fmt.Sprintf("/events/%d", i)}, "%v", err))
			continue
		}
		valid = append(valid, queued{Name: e.Name, Time: e.Time, ReleaseSequence: e.ReleaseSequence, PluginKey: e.PluginKey, Route: e.Route, Category: category, Fields: fields})
		indexes = append(indexes, i)
	}
	allowed, err := s.admit(ctx, src.DeviceID, len(valid))
	if err != nil {
		return 0, nil, err
	}
	for _, i := range indexes[allowed:] {
		diags = append(diags, plxerr.NewDiagnostic(plxerr.IngestionOverQuota, plxerr.Location{Path: fmt.Sprintf("/events/%d", i)},
			"the device sent more than %s = %d events in an hour; this one is dropped", limits.TelemetryEventsPerDevicePerHour,
			s.o.Limits.Get(limits.TelemetryEventsPerDevicePerHour)))
	}
	valid = valid[:allowed]
	for i := range valid {
		id, err := s.o.IDs.New()
		if err != nil {
			return 0, nil, fmt.Errorf("telemetry: %w", err)
		}
		valid[i].ID = id
	}
	if err := s.deliver(ctx, message{Source: src, Events: valid}); err != nil {
		return 0, nil, err
	}
	return len(valid), diags, nil
}

// checkBytes refuses a batch over telemetry.batchBytes. Without the size
// of the request it adds up what the events carry.
func (s *Service) checkBytes(src Source, events []Event) error {
	size := src.Bytes
	if size == 0 {
		for _, e := range events {
			size += len(e.Name) + len(e.PluginKey) + len(e.Route) + len(e.Fields) + len(e.DeviceID)
		}
	}
	if max := s.o.Limits.Get(limits.TelemetryBatchBytes); int64(size) > max {
		return plxerr.New(plxerr.IngestionOverQuota, "the batch is %d bytes, more than %s = %d", size, limits.TelemetryBatchBytes, max)
	}
	return nil
}

// admit counts n events against the device's hourly quota and returns how
// many of them fit. The whole batch is counted even when part of it is
// dropped, so a device that keeps sending is not let in again by the retry
// of what was refused.
func (s *Service) admit(ctx context.Context, deviceID string, n int) (int, error) {
	if s.o.Counter == nil || n == 0 {
		return n, nil
	}
	hour := s.now().Unix() / 3600
	total, err := s.o.Counter.IncrementBy(ctx, quotaKeyPrefix+deviceID+":"+strconv.FormatInt(hour, 10), int64(n), 2*time.Hour)
	if err != nil {
		return 0, fmt.Errorf("telemetry: count the device's events: %w", err)
	}
	over := total - s.o.Limits.Get(limits.TelemetryEventsPerDevicePerHour)
	switch {
	case over <= 0:
		return n, nil
	case over >= int64(n):
		return 0, nil
	}
	return n - int(over), nil
}

// queued is an accepted event on its way to storage; its identifier is
// assigned on acceptance, so delivering it twice stores it once.
type queued struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	Time            time.Time       `json:"time"`
	ReleaseSequence int64           `json:"seq,omitempty"`
	PluginKey       string          `json:"plugin,omitempty"`
	Route           string          `json:"route,omitempty"`
	Category        string          `json:"category"`
	Fields          json.RawMessage `json:"fields"`
}

// message is what the queue carries: one device's accepted events.
type message struct {
	Source Source   `json:"source"`
	Events []queued `json:"events"`
}

// deliver queues a message for the consumers, or writes it at once when
// there is no queue or the queue fails: losing telemetry because Valkey is
// down would be worse than writing it from the api role.
func (s *Service) deliver(ctx context.Context, m message) error {
	if len(m.Events) == 0 {
		return nil
	}
	if s.o.Queue != nil {
		payload, err := json.Marshal(m)
		if err != nil {
			return fmt.Errorf("telemetry: %w", err)
		}
		if err := s.o.Queue.Enqueue(ctx, payload); err == nil {
			return nil
		}
	}
	return s.write(ctx, m)
}

// write stores a message: the device's facts first, then the events.
func (s *Service) write(ctx context.Context, m message) error {
	src := m.Source
	var newest *session
	for _, q := range m.Events {
		newest = newest.newer(Event{Name: q.Name, Time: q.Time}, q.Fields)
	}
	dims, err := s.recordDevice(ctx, src, newest)
	if err != nil {
		return err
	}
	b := analytics.Batch{
		OrganizationID: src.OrganizationID, AppID: src.AppID, EnvironmentID: src.EnvironmentID, DeviceID: src.DeviceID,
		DeviceClass: dims.class, Locale: dims.locale, Events: make([]analytics.Event, 0, len(m.Events)),
	}
	for _, q := range m.Events {
		b.Events = append(b.Events, analytics.Event{
			ID: q.ID, Name: q.Name, Time: q.Time, ReleaseSequence: q.ReleaseSequence, PluginKey: q.PluginKey,
			Route: q.Route, Category: q.Category, Fields: q.Fields,
		})
	}
	if err := s.o.Store.WriteEvents(ctx, b); err != nil {
		return fmt.Errorf("telemetry: %w", err)
	}
	if s.o.Errors != nil {
		if err := s.o.Errors.RecordErrors(ctx, b); err != nil {
			return fmt.Errorf("telemetry: %w", err)
		}
	}
	s.measurePropagation(ctx, m)
	return nil
}

// dimensions are the device facts every one of its events is rolled up by.
type dimensions struct{ class, locale string }

// recordDevice updates the device to what its newest session_start says it
// runs (REL-080) and returns the dimensions to store its events with: the
// session's, or the device's last known.
func (s *Service) recordDevice(ctx context.Context, src Source, newest *session) (dimensions, error) {
	var dims dimensions
	err := s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: src.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		dev := storage.MustUUID(src.DeviceID)
		if newest != nil {
			dims = newest.dims
			if err := newest.record(ctx, q, src.DeviceID); err != nil {
				return err
			}
			return q.SetDeviceDimensions(ctx, dbgen.SetDeviceDimensionsParams{ID: dev, DeviceClass: dims.class, Locale: dims.locale}) //nolint:wrapcheck // wrapped below
		}
		d, err := q.GetDeviceDimensions(ctx, dev)
		if err != nil {
			return err //nolint:wrapcheck // wrapped below
		}
		dims = dimensions{class: d.DeviceClass, locale: d.Locale}
		return nil
	})
	if err != nil {
		return dimensions{}, fmt.Errorf("telemetry: %w", err)
	}
	return dims, nil
}

// session is the newest valid session_start of a batch.
type session struct {
	time     time.Time
	versions deviceVersions
	dims     dimensions
}

// newer returns the newer of s and e, when e is a valid session_start.
func (s *session) newer(e Event, fields []byte) *session {
	v, ok := sessionVersions(e, fields)
	if !ok || s != nil && e.Time.Before(s.time) {
		return s
	}
	return &session{time: e.Time, versions: v, dims: sessionDimensions(fields)}
}

// record updates the device to what its newest session_start says it
// runs, which may be newer than what it registered with (REL-080).
func (s *session) record(ctx context.Context, q *dbgen.Queries, deviceID string) error {
	if s == nil {
		return nil
	}
	if err := q.UpdateDeviceVersions(ctx, dbgen.UpdateDeviceVersionsParams{
		ID: storage.MustUUID(deviceID), RuntimeVersion: s.versions.runtime, HostBuild: s.versions.host, OsVersion: s.versions.os,
	}); err != nil {
		return fmt.Errorf("telemetry: %w", err)
	}
	return nil
}

// deviceVersions is what a session_start says the device runs.
type deviceVersions struct{ runtime, host, os string }

// dimension is the form of a device class or a locale: letters, digits,
// underscore and hyphen.
var dimension = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// sessionDimensions reads the device class and locale of a session_start;
// a value of another form is dropped rather than stored.
func sessionDimensions(fields []byte) dimensions {
	var f struct {
		Class  string `json:"device_class"`
		Locale string `json:"locale"`
	}
	var d dimensions
	if json.Unmarshal(fields, &f) != nil {
		return d
	}
	if dimension.MatchString(f.Class) {
		d.class = f.Class
	}
	if dimension.MatchString(f.Locale) {
		d.locale = f.Locale
	}
	return d
}

// version is the form of a runtime version: MAJOR.MINOR.PATCH, with an
// optional pre-release or build suffix.
var version = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.+-]{1,64})?$`)

// sessionVersions reads the versions of a valid session_start event.
func sessionVersions(e Event, fields []byte) (deviceVersions, bool) {
	if e.Name != "session_start" {
		return deviceVersions{}, false
	}
	var f struct {
		Runtime string `json:"runtime_version"`
		Host    string `json:"host_build"`
		OS      string `json:"os_version"`
	}
	// The same bound as at registration (device.Register).
	if json.Unmarshal(fields, &f) != nil || !version.MatchString(f.Runtime) || len(f.Host) > 64 || len(f.OS) > 64 {
		return deviceVersions{}, false
	}
	return deviceVersions{runtime: f.Runtime, host: f.Host, os: f.OS}, true
}

// check validates one event and returns its fields and consent category to
// store.
func (s *Service) check(src Source, e Event) ([]byte, string, error) {
	category, catalogued := categories[e.Name]
	switch {
	case !catalogued:
		return nil, "", fmt.Errorf("%q is not a catalogued event", e.Name)
	case e.DeviceID != "" && e.DeviceID != src.DeviceID:
		return nil, "", errors.New("an event names another device")
	case e.Time.IsZero() || e.Time.After(s.now().Add(time.Hour)) || e.Time.Before(s.now().Add(-s.o.Retention)):
		return nil, "", errors.New("the event time is missing or out of range")
	case e.ReleaseSequence < 0 || len(e.PluginKey) > 64 || len(e.Route) > maxText:
		return nil, "", errors.New("a field is out of range")
	case !src.Consent.allows(category):
		return nil, "", fmt.Errorf("%s events need the user's consent to %s, which the runtime did not declare", e.Name, category)
	}
	fields, err := checkFields(e.Fields)
	if err != nil {
		return nil, "", err
	}
	if !src.Consent.allows(CategoryAnalytics) {
		fields = withoutLocale(fields)
	}
	return fields, category, nil
}

// allows reports whether the declared consent covers a category; a nil
// consent is no declaration, and everything the device sent is taken as
// gated on the device.
func (c *Consent) allows(category string) bool {
	switch {
	case c == nil, category == CategoryNecessary:
		return true
	case category == CategoryAnalytics:
		return c.Analytics
	}
	return c.Experiments
}

// withoutLocale drops the locale of an event: it is sent only with
// analytics consent (ADR-0034).
func withoutLocale(fields []byte) []byte {
	var m map[string]json.RawMessage
	if json.Unmarshal(fields, &m) != nil {
		return fields
	}
	if _, ok := m["locale"]; !ok {
		return fields
	}
	delete(m, "locale")
	out, err := json.Marshal(m)
	if err != nil {
		return fields
	}
	return out
}

// checkFields accepts a flat object of at most maxFields scalar values
// with well-formed, non-sensitive names.
func checkFields(raw []byte) ([]byte, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return []byte("{}"), nil
	}
	if len(raw) > maxFieldsBytes {
		return nil, fmt.Errorf("the fields exceed %d bytes", maxFieldsBytes)
	}
	var fields map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&fields); err != nil || fields == nil {
		return nil, errors.New("the fields are not a JSON object")
	}
	if len(fields) > maxFields {
		return nil, fmt.Errorf("more than %d fields", maxFields)
	}
	for k, v := range fields {
		if !fieldName.MatchString(k) {
			return nil, fmt.Errorf("field name %q is not allowed", k)
		}
		if Sensitive(k) {
			return nil, fmt.Errorf("field %q looks sensitive and is refused", k)
		}
		switch x := v.(type) {
		case string:
			if len(x) > maxText {
				return nil, fmt.Errorf("field %q is longer than %d bytes", k, maxText)
			}
		case json.Number, bool, nil:
		default:
			return nil, fmt.Errorf("field %q is not a scalar", k)
		}
	}
	out, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("telemetry: %w", err)
	}
	return out, nil
}

// Sensitive reports whether a field name marks sensitive data.
func Sensitive(name string) bool {
	lower := strings.ToLower(name)
	for _, w := range sensitiveWords {
		if strings.Contains(lower, w) {
			return true
		}
	}
	return false
}

// List lists an environment's events, newest first, optionally by name
// and since a time.
func (s *Service) List(ctx context.Context, p auth.Principal, appID, envID, name string, since time.Time, before storage.Cursor, size int32) ([]Event, error) {
	if err := p.AuthorizeApp(auth.AppRead, appID); err != nil {
		return nil, fmt.Errorf("telemetry: %w", err)
	}
	app, err1 := storage.UUID(appID)
	env, err2 := storage.UUID(envID)
	if err1 != nil || err2 != nil {
		return nil, plxerr.New(plxerr.InvalidFormat, "the app or environment identifier is not valid")
	}
	t := before.Time
	if t.IsZero() {
		t = s.now().Add(2 * time.Hour)
	}
	if since.IsZero() {
		since = s.now().Add(-s.o.Retention)
	}
	id := before.AfterID()
	if before.ID == "" {
		for i := range id.Bytes {
			id.Bytes[i] = 0xff
		}
	}
	var out []Event
	err := s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID, UserID: p.UserID}, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListTelemetryEvents(ctx, dbgen.ListTelemetryEventsParams{
			AppID: app, EnvironmentID: env, Name: name, Since: storage.Timestamp(since),
			BeforeTime: storage.Timestamp(t), BeforeID: id, PageSize: size,
		})
		if err != nil {
			return fmt.Errorf("telemetry: %w", err)
		}
		for _, r := range rows {
			out = append(out, Event{
				ID: storage.ID(r.ID), Name: r.Name, DeviceID: storage.ID(r.DeviceID), PluginKey: r.PluginKey, Route: r.Route,
				Time: storage.Time(r.Time), ReleaseSequence: r.ReleaseSequence, Fields: r.Fields,
			})
		}
		return nil
	})
	return out, err //nolint:wrapcheck // wrapped inside
}

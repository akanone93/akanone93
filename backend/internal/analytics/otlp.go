// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package analytics

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/httpx"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// ExportOptions configures the OTLP export. Rollups are aggregates, which is
// what metrics are for; raw events would be logs and carry more than a
// customer's collector needs.
type ExportOptions struct {
	// Client sends the requests; the default is an SSRF-safe client.
	Client *http.Client
	// AllowHTTP accepts an http:// destination. The default requires
	// https, so rollups are not sent in the clear.
	AllowHTTP bool
	// Lag is how old a bucket must be before it is sent, so that late
	// events have been folded in; the default is ten minutes. An event
	// that arrives after its bucket was sent is not sent again.
	Lag time.Duration
	// Chunk is the most data points one request carries; the default is
	// 1000. A bucket is never split across requests.
	Chunk int
}

// withDefaults fills the unset options. It fails when the default client
// cannot be built.
func (o ExportOptions) withDefaults() (ExportOptions, error) {
	if o.Client == nil {
		client, err := httpx.NewClient(httpx.Options{Destination: httpx.KindAnalyticsOTLP, Timeout: 30 * time.Second})
		if err != nil {
			return ExportOptions{}, fmt.Errorf("analytics: %w", err)
		}
		o.Client = client
	}
	if o.Lag <= 0 {
		o.Lag = 10 * time.Minute
	}
	if o.Chunk <= 0 {
		o.Chunk = 1000
	}
	return o, nil
}

// Export is an organisation's OTLP destination.
type Export struct {
	Endpoint      string
	Enabled       bool
	ExportedUntil time.Time
	LastError     string
	LastAttemptAt time.Time
	ChangedBy     string
	ChangedAt     time.Time
}

func exportOf(r dbgen.AnalyticsExport) Export {
	return Export{
		Endpoint: r.Endpoint, Enabled: r.Enabled, ExportedUntil: storage.Time(r.ExportedUntil), LastError: r.LastError,
		LastAttemptAt: storage.Time(r.LastAttemptAt), ChangedBy: r.ChangedBy, ChangedAt: storage.Time(r.ChangedAt),
	}
}

// metricsPath is where OTLP/HTTP receives metrics.
const metricsPath = "/v1/metrics"

// normalizeEndpoint checks a destination and returns it with the metrics
// path. Credentials, a query and a fragment are refused: the address is
// stored and shown, so it must not carry a secret.
func (o ExportOptions) normalizeEndpoint(raw string) (string, error) {
	u, err := url.Parse(raw)
	switch {
	case err != nil || u.Host == "":
		return "", plxerr.New(plxerr.InvalidFormat, "the OTLP endpoint is not a URL")
	case u.Scheme != "https" && (!o.AllowHTTP || u.Scheme != "http"):
		return "", plxerr.New(plxerr.InvalidFormat, "the OTLP endpoint must use https")
	case u.User != nil || u.RawQuery != "" || u.Fragment != "":
		return "", plxerr.New(plxerr.InvalidFormat, "the OTLP endpoint must not carry credentials, a query or a fragment")
	}
	if err := httpx.CheckURL(u.Scheme, u.Hostname()); err != nil {
		return "", plxerr.Wrap(plxerr.InvalidFormat, err, "the OTLP endpoint is not allowed")
	}
	if !strings.HasSuffix(u.Path, metricsPath) {
		u.Path = strings.TrimSuffix(u.Path, "/") + metricsPath
	}
	return u.String(), nil
}

// GetExport returns the organisation's destination, or ResourceNotFound.
func (s *Service) GetExport(ctx context.Context, p auth.Principal) (Export, error) {
	if err := p.Authorize(auth.AnalyticsRead); err != nil {
		return Export{}, fmt.Errorf("analytics: %w", err)
	}
	var out Export
	err := s.inOrg(ctx, p.OrganizationID, p.UserID, func(ctx context.Context, tx pgx.Tx) error {
		r, err := dbgen.New(tx).GetAnalyticsExport(ctx, storage.MustUUID(p.OrganizationID))
		if errors.Is(err, pgx.ErrNoRows) {
			return plxerr.New(plxerr.ResourceNotFound, "no OTLP export is configured")
		}
		out = exportOf(r)
		return err //nolint:wrapcheck // wrapped below
	})
	return out, err
}

// SetExport sets the OTLP destination. It sends an empty metrics request
// first, so a destination that does not answer is refused with PLX-8302
// rather than saved. The export starts with the buckets after now: history
// is not replayed.
func (s *Service) SetExport(ctx context.Context, p auth.Principal, endpoint string, enabled bool) (Export, error) {
	if err := p.Authorize(auth.OrganizationManage); err != nil {
		return Export{}, fmt.Errorf("analytics: %w", err)
	}
	target, err := s.o.Export.normalizeEndpoint(endpoint)
	if err != nil {
		return Export{}, err
	}
	if err := s.send(ctx, target, otlpRequest{ResourceMetrics: []resourceMetrics{}}); err != nil {
		return Export{}, err
	}
	var out Export
	err = s.inOrg(ctx, p.OrganizationID, p.UserID, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		org := storage.MustUUID(p.OrganizationID)
		if err := q.UpsertAnalyticsExport(ctx, dbgen.UpsertAnalyticsExportParams{
			OrganizationID: org, Endpoint: target, Enabled: enabled, ExportedUntil: storage.Timestamp(s.now()), ChangedBy: p.Actor().ID,
		}); err != nil {
			return fmt.Errorf("analytics: save the export: %w", err)
		}
		r, err := q.GetAnalyticsExport(ctx, org)
		if err != nil {
			return fmt.Errorf("analytics: read the export: %w", err)
		}
		out = exportOf(r)
		_, err = s.o.Audit.Append(ctx, tx, audit.Entry{
			Action: audit.AnalyticsExportSet, TargetKind: "analytics_export", TargetID: p.OrganizationID,
			Detail: fmt.Sprintf("%s, enabled=%t", target, enabled), OrganizationID: p.OrganizationID, Actor: p.Actor(),
		})
		return err //nolint:wrapcheck // wrapped by the audit log
	})
	return out, err
}

// DeleteExport removes the destination.
func (s *Service) DeleteExport(ctx context.Context, p auth.Principal) error {
	if err := p.Authorize(auth.OrganizationManage); err != nil {
		return fmt.Errorf("analytics: %w", err)
	}
	return s.inOrg(ctx, p.OrganizationID, p.UserID, func(ctx context.Context, tx pgx.Tx) error {
		n, err := dbgen.New(tx).DeleteAnalyticsExport(ctx, storage.MustUUID(p.OrganizationID))
		if err != nil {
			return fmt.Errorf("analytics: delete the export: %w", err)
		}
		if n == 0 {
			return plxerr.New(plxerr.ResourceNotFound, "no OTLP export is configured")
		}
		_, err = s.o.Audit.Append(ctx, tx, audit.Entry{
			Action: audit.AnalyticsExportDeleted, TargetKind: "analytics_export", TargetID: p.OrganizationID,
			OrganizationID: p.OrganizationID, Actor: p.Actor(),
		})
		return err //nolint:wrapcheck // wrapped by the audit log
	})
}

// ExportAll sends every organisation's new rollups to its destination; one
// organisation's failure does not stop the others.
func (s *Service) ExportAll(ctx context.Context) error {
	var errs []error
	err := s.o.Organizations(ctx, func(org string) error {
		if err := s.exportOrg(ctx, org); err != nil {
			errs = append(errs, err)
		}
		return nil
	})
	return errors.Join(append(errs, err)...)
}

// exportOrg sends the buckets between the last export and now minus the
// lag, bucket by bucket group, and records how far it got.
func (s *Service) exportOrg(ctx context.Context, org string) error {
	var cfg dbgen.AnalyticsExport
	err := s.inOrg(ctx, org, "", func(ctx context.Context, tx pgx.Tx) error {
		var err error
		cfg, err = dbgen.New(tx).GetAnalyticsExport(ctx, storage.MustUUID(org))
		return err //nolint:wrapcheck // handled below
	})
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !cfg.Enabled {
		return nil
	}
	if err != nil {
		return fmt.Errorf("analytics: read the export: %w", err)
	}
	now := s.now()
	from := storage.Time(cfg.ExportedUntil).Truncate(FiveMinutes.Width())
	to := now.Add(-s.o.Export.Lag).Truncate(FiveMinutes.Width())
	if !to.After(from) {
		return nil
	}
	points, err := s.o.Store.Window(ctx, org, FiveMinutes, from, to)
	if err != nil {
		return err //nolint:wrapcheck // wrapped by the store
	}
	done, sendErr := from, error(nil)
	for _, chunk := range chunks(points, s.o.Export.Chunk) {
		if sendErr = s.send(ctx, cfg.Endpoint, buildRequest(org, chunk)); sendErr != nil {
			break
		}
		done = chunk[len(chunk)-1].Bucket.Add(FiveMinutes.Width())
	}
	if sendErr == nil {
		done = to
	}
	msg := ""
	if sendErr != nil {
		msg = sendErr.Error()
	}
	err = s.inOrg(ctx, org, "", func(ctx context.Context, tx pgx.Tx) error {
		return dbgen.New(tx).RecordExportAttempt(ctx, dbgen.RecordExportAttemptParams{ //nolint:wrapcheck // wrapped below
			OrganizationID: storage.MustUUID(org), ExportedUntil: storage.Timestamp(done), LastError: msg, LastAttemptAt: storage.Timestamp(now),
		})
	})
	if err != nil {
		return fmt.Errorf("analytics: record the export: %w", err)
	}
	return sendErr
}

// chunks splits sorted points into groups of about size, never inside a
// bucket.
func chunks(points []Point, size int) [][]Point {
	var out [][]Point
	start := 0
	for i := 1; i <= len(points); i++ {
		if i == len(points) || points[i].Bucket != points[i-1].Bucket && i-start >= size {
			out = append(out, points[start:i])
			start = i
		}
	}
	return out
}

// send posts one request. Any failure to deliver is PLX-8302.
func (s *Service) send(ctx context.Context, endpoint string, req otlpRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("analytics: encode the export: %w", err)
	}
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return plxerr.Wrap(plxerr.ExportDestinationUnreachable, err, "the OTLP endpoint is not a URL")
	}
	if err := httpx.CheckURL(hr.URL.Scheme, hr.URL.Hostname()); err != nil {
		return plxerr.Wrap(plxerr.ExportDestinationUnreachable, err, "the OTLP endpoint is not allowed")
	}
	hr.Header.Set("Content-Type", "application/json")
	resp, err := s.o.Export.Client.Do(hr)
	if err != nil {
		return plxerr.Wrap(plxerr.ExportDestinationUnreachable, err, "the OTLP endpoint %s did not answer", hr.URL.Host)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return plxerr.New(plxerr.ExportDestinationUnreachable, "the OTLP endpoint %s answered %d", hr.URL.Host, resp.StatusCode)
	}
	return nil
}

// The OTLP/HTTP JSON encoding. 64-bit integers are strings, as the
// protobuf JSON mapping says.
type (
	otlpRequest struct {
		ResourceMetrics []resourceMetrics `json:"resourceMetrics"`
	}
	resourceMetrics struct {
		Resource     resource       `json:"resource"`
		ScopeMetrics []scopeMetrics `json:"scopeMetrics"`
	}
	resource struct {
		Attributes []keyValue `json:"attributes"`
	}
	scopeMetrics struct {
		Scope   scope    `json:"scope"`
		Metrics []metric `json:"metrics"`
	}
	scope struct {
		Name string `json:"name"`
	}
	metric struct {
		Name      string     `json:"name"`
		Unit      string     `json:"unit,omitempty"`
		Sum       *sum       `json:"sum,omitempty"`
		Histogram *histogram `json:"histogram,omitempty"`
	}
	sum struct {
		DataPoints             []numberPoint `json:"dataPoints"`
		AggregationTemporality int           `json:"aggregationTemporality"`
		IsMonotonic            bool          `json:"isMonotonic"`
	}
	histogram struct {
		DataPoints             []histogramPoint `json:"dataPoints"`
		AggregationTemporality int              `json:"aggregationTemporality"`
	}
	numberPoint struct {
		Attributes []keyValue `json:"attributes"`
		Start      string     `json:"startTimeUnixNano"`
		Time       string     `json:"timeUnixNano"`
		AsInt      string     `json:"asInt"`
	}
	histogramPoint struct {
		Attributes     []keyValue `json:"attributes"`
		Start          string     `json:"startTimeUnixNano"`
		Time           string     `json:"timeUnixNano"`
		Count          string     `json:"count"`
		Sum            float64    `json:"sum"`
		BucketCounts   []string   `json:"bucketCounts"`
		ExplicitBounds []float64  `json:"explicitBounds"`
		Min            float64    `json:"min"`
		Max            float64    `json:"max"`
	}
	keyValue struct {
		Key   string   `json:"key"`
		Value anyValue `json:"value"`
	}
	anyValue struct {
		String *string `json:"stringValue,omitempty"`
		Int    *string `json:"intValue,omitempty"`
	}
)

// deltaTemporality is AGGREGATION_TEMPORALITY_DELTA.
const deltaTemporality = 1

func strAttr(k, v string) keyValue { return keyValue{Key: k, Value: anyValue{String: &v}} }
func intAttr(k string, v int64) keyValue {
	s := strconv.FormatInt(v, 10)
	return keyValue{Key: k, Value: anyValue{Int: &s}}
}

// buildRequest encodes points as one resource of one organisation.
func buildRequest(org string, points []Point) otlpRequest {
	events := metric{Name: "plux.events", Unit: "{event}", Sum: &sum{AggregationTemporality: deltaTemporality, IsMonotonic: true}}
	durations := metric{Name: "plux.event.duration", Unit: "ms", Histogram: &histogram{AggregationTemporality: deltaTemporality}}
	for _, pt := range points {
		start := strconv.FormatInt(pt.Bucket.UnixNano(), 10)
		end := strconv.FormatInt(pt.Bucket.Add(pt.Granularity.Width()).UnixNano(), 10)
		attrs := []keyValue{
			strAttr("plux.app.id", pt.AppID), strAttr("plux.environment.id", pt.Group.EnvironmentID),
			strAttr("plux.event.name", pt.EventName), intAttr("plux.release.sequence", pt.Group.Release),
			strAttr("plux.plugin.key", pt.Group.Plugin), strAttr("plux.page", pt.Group.Page),
			strAttr("plux.device.class", pt.Group.DeviceClass), strAttr("plux.locale", pt.Group.Locale),
		}
		events.Sum.DataPoints = append(events.Sum.DataPoints, numberPoint{
			Attributes: attrs, Start: start, Time: end, AsInt: strconv.FormatInt(pt.Events, 10),
		})
		if pt.ValueCount == 0 {
			continue
		}
		counts := make([]string, len(pt.Histogram))
		for i, c := range pt.Histogram {
			counts[i] = strconv.FormatInt(c, 10)
		}
		durations.Histogram.DataPoints = append(durations.Histogram.DataPoints, histogramPoint{
			Attributes: attrs, Start: start, Time: end, Count: strconv.FormatInt(pt.ValueCount, 10), Sum: pt.Sum,
			BucketCounts: counts, ExplicitBounds: Bounds[:], Min: pt.Min, Max: pt.Max,
		})
	}
	ms := []metric{events}
	if len(durations.Histogram.DataPoints) > 0 {
		ms = append(ms, durations)
	}
	return otlpRequest{ResourceMetrics: []resourceMetrics{{
		Resource:     resource{Attributes: []keyValue{strAttr("service.name", "plux-server"), strAttr("plux.organization.id", org)}},
		ScopeMetrics: []scopeMetrics{{Scope: scope{Name: "plux.analytics"}, Metrics: ms}},
	}}}
}

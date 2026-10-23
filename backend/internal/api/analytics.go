// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"encoding/base64"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/analytics"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// AnalyticsHandler serves AnalyticsService (ANL-010, ANL-011, ADR-0080).
type AnalyticsHandler struct{ h *Handlers }

var _ pluxv1connect.AnalyticsServiceHandler = AnalyticsHandler{}

// AnalyticsService returns the AnalyticsService handler.
func (h *Handlers) AnalyticsService() AnalyticsHandler { return AnalyticsHandler{h: h} }

// QueryRollups reads rollups.
func (s AnalyticsHandler) QueryRollups(ctx context.Context, req *connect.Request[pluxv1.QueryRollupsRequest]) (*connect.Response[pluxv1.QueryRollupsResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.QueryRollupsResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		q, err := rollupQuery(req.Msg)
		if err != nil {
			return nil, err
		}
		if key := req.Msg.GetEnvironment(); key != "" {
			env, err := s.h.Tenancy.GetEnvironmentByKey(ctx, p, req.Msg.GetAppId(), key)
			if err != nil {
				return nil, err //nolint:wrapcheck // a domain error
			}
			q.EnvironmentID = env.ID
		}
		series, err := s.h.Analytics.Query(ctx, p, q)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return rollupResponse(series), nil
	})
}

// rollupQuery turns a request into a store query, resuming after the page
// token when there is one.
func rollupQuery(m *pluxv1.QueryRollupsRequest) (analytics.Query, error) {
	q := analytics.Query{
		AppID: m.GetAppId(), Metric: m.GetMetric(), Granularity: analytics.Granularity(m.GetGranularity()),
		From: m.GetFrom().AsTime(), To: m.GetTo().AsTime(), Limit: int(m.GetLimit()),
	}
	if m.GetFrom() == nil || m.GetTo() == nil {
		q.From, q.To = time.Time{}, time.Time{}
	}
	if tok := m.GetPageToken(); tok != "" {
		next, err := decodeRollupToken(tok, q.From, q.To)
		if err != nil {
			return analytics.Query{}, err
		}
		q.From = next
	}
	for _, d := range m.GetGroupBy() {
		q.GroupBy = append(q.GroupBy, analytics.Dimension(d))
	}
	return q, nil
}

// rollupResponse turns a series into the response, with the token of the next
// page when the series was cut short.
func rollupResponse(series analytics.Series) *pluxv1.QueryRollupsResponse {
	out := &pluxv1.QueryRollupsResponse{}
	if series.Truncated {
		out.NextPageToken = encodeRollupToken(series.NextFrom)
	}
	for _, pt := range series.Points {
		out.Points = append(out.Points, &pluxv1.RollupPoint{
			Bucket: ts(pt.Bucket),
			Group: &pluxv1.RollupGroup{
				EnvironmentId: pt.Group.EnvironmentID, ReleaseSequence: pt.Group.Release, PluginKey: pt.Group.Plugin,
				Page: pt.Group.Page, DeviceClass: pt.Group.DeviceClass, Locale: pt.Group.Locale,
			},
			Events: pt.Events, ValueCount: pt.ValueCount, Sum: pt.Sum, Min: pt.Min, Max: pt.Max, P50: pt.P50, P90: pt.P90, P99: pt.P99,
		})
	}
	return out
}

// GetRetention returns the effective retention and the waiting changes.
func (s AnalyticsHandler) GetRetention(ctx context.Context, req *connect.Request[pluxv1.GetRetentionRequest]) (*connect.Response[pluxv1.GetRetentionResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.GetRetentionResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		settings, err := s.h.Analytics.GetRetention(ctx, p)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		changes, err := s.h.Analytics.ListChanges(ctx, p, true, 0)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.GetRetentionResponse{}
		for _, st := range settings {
			out.Settings = append(out.Settings, &pluxv1.RetentionSetting{
				DataClass: st.Class, Days: daysProto(st.Days), IsDefault: st.Default, Enforced: st.Enforced,
				ChangedBy: st.ChangedBy, ChangedAt: ts(st.ChangedAt),
			})
		}
		for _, c := range changes {
			out.Scheduled = append(out.Scheduled, retentionChangeProto(c))
		}
		return out, nil
	})
}

// SetRetention schedules or makes a retention change.
func (s AnalyticsHandler) SetRetention(ctx context.Context, req *connect.Request[pluxv1.SetRetentionRequest]) (*connect.Response[pluxv1.SetRetentionResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.SetRetentionResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		change, diags, err := s.h.Analytics.SetRetention(ctx, p, m.GetDataClass(), int(m.GetDays()), m.GetImmediate())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.SetRetentionResponse{Change: retentionChangeProto(change), Diagnostics: diagnosticProtos(diags)}, nil
	})
}

// ListRetentionChanges lists changes.
func (s AnalyticsHandler) ListRetentionChanges(ctx context.Context, req *connect.Request[pluxv1.ListRetentionChangesRequest]) (*connect.Response[pluxv1.ListRetentionChangesResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListRetentionChangesResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		changes, err := s.h.Analytics.ListChanges(ctx, p, req.Msg.GetOnlyScheduled(), int(req.Msg.GetLimit()))
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListRetentionChangesResponse{}
		for _, c := range changes {
			out.Changes = append(out.Changes, retentionChangeProto(c))
		}
		return out, nil
	})
}

// CancelRetentionChange withdraws a scheduled change.
func (s AnalyticsHandler) CancelRetentionChange(ctx context.Context, req *connect.Request[pluxv1.CancelRetentionChangeRequest]) (*connect.Response[pluxv1.CancelRetentionChangeResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.CancelRetentionChangeResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		c, err := s.h.Analytics.CancelChange(ctx, p, req.Msg.GetId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.CancelRetentionChangeResponse{Change: retentionChangeProto(c)}, nil
	})
}

// GetExport returns the OTLP destination.
func (s AnalyticsHandler) GetExport(ctx context.Context, req *connect.Request[pluxv1.GetExportRequest]) (*connect.Response[pluxv1.GetExportResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.GetExportResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		e, err := s.h.Analytics.GetExport(ctx, p)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.GetExportResponse{Export: exportProto(e)}, nil
	})
}

// SetExport sets the OTLP destination.
func (s AnalyticsHandler) SetExport(ctx context.Context, req *connect.Request[pluxv1.SetExportRequest]) (*connect.Response[pluxv1.SetExportResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.SetExportResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		e, err := s.h.Analytics.SetExport(ctx, p, req.Msg.GetEndpoint(), req.Msg.GetEnabled())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.SetExportResponse{Export: exportProto(e)}, nil
	})
}

// GetFileExport returns the settings of the Parquet and webhook exports.
func (s AnalyticsHandler) GetFileExport(ctx context.Context, req *connect.Request[pluxv1.GetFileExportRequest]) (*connect.Response[pluxv1.GetFileExportResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.GetFileExportResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		e, err := s.h.Analytics.GetFileExport(ctx, p)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.GetFileExportResponse{Export: fileExportProto(e)}, nil
	})
}

// SetFileExport sets the Parquet and webhook exports.
func (s AnalyticsHandler) SetFileExport(ctx context.Context, req *connect.Request[pluxv1.SetFileExportRequest]) (*connect.Response[pluxv1.SetFileExportResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.SetFileExportResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		e, err := s.h.Analytics.SetFileExport(ctx, p, fileExportSettings(req.Msg.GetSettings()), req.Msg.GetWebhookSecret())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.SetFileExportResponse{Export: fileExportProto(e)}, nil
	})
}

// fileExportSettings reads the settings of a request.
func fileExportSettings(m *pluxv1.FileExportSettings) analytics.ExportSettings {
	var st analytics.ExportSettings
	st.Parquet.Enabled, st.Parquet.Events, st.Parquet.Rollups = m.GetParquetEnabled(), m.GetParquetEvents(), m.GetParquetRollups()
	st.Parquet.Granularity = analytics.Granularity(m.GetParquetGranularity())
	st.Webhook.Enabled, st.Webhook.URL = m.GetWebhookEnabled(), m.GetWebhookUrl()
	return st
}

func fileExportProto(e analytics.FileExport) *pluxv1.FileExport {
	return &pluxv1.FileExport{
		Settings: &pluxv1.FileExportSettings{
			ParquetEnabled: e.Settings.Parquet.Enabled, ParquetEvents: e.Settings.Parquet.Events,
			ParquetRollups: e.Settings.Parquet.Rollups, ParquetGranularity: string(e.Settings.Parquet.Granularity),
			WebhookEnabled: e.Settings.Webhook.Enabled, WebhookUrl: e.Settings.Webhook.URL,
		},
		SecretHint: e.SecretHint, ExportedUntil: ts(e.ExportedUntil), LastError: e.LastError, LastAttemptAt: ts(e.LastAttemptAt),
	}
}

// DeleteExport removes the OTLP destination.
func (s AnalyticsHandler) DeleteExport(ctx context.Context, req *connect.Request[pluxv1.DeleteExportRequest]) (*connect.Response[pluxv1.DeleteExportResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.DeleteExportResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		if err := s.h.Analytics.DeleteExport(ctx, p); err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.DeleteExportResponse{}, nil
	})
}

// daysProto is 0 for indefinitely.
func daysProto(d *int) int32 {
	if d == nil {
		return 0
	}
	return int32(*d) //nolint:gosec // bounded by analytics.maxRetentionDays
}

func retentionChangeProto(c analytics.Change) *pluxv1.RetentionChange {
	return &pluxv1.RetentionChange{
		Id: c.ID, DataClass: c.Class, Days: daysProto(c.Days), State: c.State, RequestedBy: c.RequestedBy,
		RequestedAt: ts(c.RequestedAt), ApplyAt: ts(c.ApplyAt), FinishedAt: ts(c.FinishedAt),
	}
}

func exportProto(e analytics.Export) *pluxv1.Export {
	return &pluxv1.Export{
		Endpoint: e.Endpoint, Enabled: e.Enabled, ExportedUntil: ts(e.ExportedUntil), LastError: e.LastError,
		LastAttemptAt: ts(e.LastAttemptAt),
	}
}

// encodeRollupToken makes the page token that continues a query at a bucket.
func encodeRollupToken(next time.Time) string {
	return base64.RawURLEncoding.EncodeToString([]byte(next.UTC().Format(time.RFC3339Nano)))
}

// decodeRollupToken reads a page token and checks that it lies inside the
// range asked for, so a token from another query is refused, not followed.
func decodeRollupToken(tok string, from, to time.Time) (time.Time, error) {
	raw, err := base64.RawURLEncoding.DecodeString(tok)
	if err != nil {
		return time.Time{}, plxerr.New(plxerr.InvalidFormat, "the page token is not valid")
	}
	next, err := time.Parse(time.RFC3339Nano, string(raw))
	if err != nil || next.Before(from) || !next.Before(to) {
		return time.Time{}, plxerr.New(plxerr.InvalidFormat, "the page token is not valid for this query")
	}
	return next, nil
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package analytics

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"

	ext "github.com/nightCode42/plux3/backend/ext/analytics"
	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/httpx"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
	"github.com/nightCode42/plux3/backend/internal/storage/objects"
)

// The settings and the contracts of the enterprise exports are public, so
// the enterprise edition can implement them.
type (
	// ExportSettings is an organisation's configuration of the Parquet and
	// webhook exports.
	ExportSettings = ext.ExportSettings
	// Exporter runs the enterprise exports.
	Exporter = ext.Exporter
)

// exportPrefix is where the Parquet files of the export are kept in the
// object store.
const exportPrefix = "exports/"

// fileExportRunBudget bounds one worker round: a backlog is delivered over
// several rounds, each window complete or not at all.
const fileExportRunBudget = 4 * time.Minute

// FileExport is an organisation's settings of the Parquet and webhook
// exports (ANL-011) and how far the export has come.
type FileExport struct {
	Settings ExportSettings
	// SecretHint is the end of the webhook's signing secret, which is never
	// returned; empty when none is stored.
	SecretHint    string
	ExportedUntil time.Time
	LastError     string
	LastAttemptAt time.Time
	ChangedBy     string
	ChangedAt     time.Time
}

// defaultFileExport is what an organisation that configured nothing has.
func defaultFileExport() FileExport {
	return FileExport{Settings: ExportSettings{Parquet: ext.ParquetSettings{Events: true, Rollups: true, Granularity: Hour}}}
}

func fileExportOf(r dbgen.AnalyticsFileExport) FileExport {
	return FileExport{
		Settings: ExportSettings{
			Parquet: ext.ParquetSettings{
				Enabled: r.ParquetEnabled, Events: r.ParquetEvents, Rollups: r.ParquetRollups, Granularity: Granularity(r.ParquetGranularity),
			},
			Webhook: ext.WebhookSettings{Enabled: r.WebhookEnabled, URL: r.WebhookUrl},
		},
		SecretHint: r.SecretHint, ExportedUntil: storage.Time(r.ExportedUntil), LastError: r.LastError,
		LastAttemptAt: storage.Time(r.LastAttemptAt), ChangedBy: r.ChangedBy, ChangedAt: storage.Time(r.ChangedAt),
	}
}

// secretBinding ties a sealed signing secret to its organisation, so a
// ciphertext copied to another row fails to open (SEC-092).
func secretBinding(org string) []byte { return []byte("analytics-file-export:" + org) }

// GetFileExport returns the organisation's settings of the Parquet and
// webhook exports; an organisation that set none has them all off.
func (s *Service) GetFileExport(ctx context.Context, p auth.Principal) (FileExport, error) {
	if err := p.Authorize(auth.AnalyticsRead); err != nil {
		return FileExport{}, fmt.Errorf("analytics: %w", err)
	}
	out := defaultFileExport()
	err := s.inOrg(ctx, p.OrganizationID, p.UserID, func(ctx context.Context, tx pgx.Tx) error {
		r, err := dbgen.New(tx).GetAnalyticsFileExport(ctx, storage.MustUUID(p.OrganizationID))
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("analytics: read the file export: %w", err)
		}
		out = fileExportOf(r)
		return nil
	})
	return out, err
}

// SetFileExport sets the Parquet and webhook exports (ANL-011). The webhook
// signing secret is "whsec_" and base64; empty keeps the stored one. The
// export starts with the first hour after now: history is not replayed. It
// is refused with PLX-8600 where the licence does not allow an export the
// settings turn on.
func (s *Service) SetFileExport(ctx context.Context, p auth.Principal, settings ExportSettings, secret string) (FileExport, error) {
	if err := p.Authorize(auth.OrganizationManage); err != nil {
		return FileExport{}, fmt.Errorf("analytics: %w", err)
	}
	if s.o.Exporter == nil {
		return FileExport{}, plxerr.New(plxerr.EnterpriseFeatureNotLicensed, "the Parquet and webhook exports are part of the enterprise edition")
	}
	settings, err := normalizeSettings(settings)
	if err != nil {
		return FileExport{}, err
	}
	if err := s.o.Exporter.Validate(ctx, settings, secret); err != nil {
		return FileExport{}, fmt.Errorf("analytics: %w", err)
	}
	var out FileExport
	err = s.inOrg(ctx, p.OrganizationID, p.UserID, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		params, err := s.upsertParams(ctx, q, p, settings, secret)
		if err != nil {
			return err
		}
		if err := q.UpsertAnalyticsFileExport(ctx, params); err != nil {
			return fmt.Errorf("analytics: save the file export: %w", err)
		}
		r, err := q.GetAnalyticsFileExport(ctx, params.OrganizationID)
		if err != nil {
			return fmt.Errorf("analytics: read the file export: %w", err)
		}
		out = fileExportOf(r)
		_, err = s.o.Audit.Append(ctx, tx, audit.Entry{
			Action: audit.AnalyticsFileExportSet, TargetKind: "analytics_file_export", TargetID: p.OrganizationID,
			Detail:         fmt.Sprintf("parquet=%t webhook=%t", settings.Parquet.Enabled, settings.Webhook.Enabled),
			OrganizationID: p.OrganizationID, Actor: p.Actor(),
		})
		return err //nolint:wrapcheck // wrapped by the audit log
	})
	return out, err
}

// upsertParams builds the row to store: the settings, and the signing
// secret sealed, or the one already stored when none is given.
func (s *Service) upsertParams(ctx context.Context, q *dbgen.Queries, p auth.Principal, st ExportSettings, secret string) (dbgen.UpsertAnalyticsFileExportParams, error) {
	org := storage.MustUUID(p.OrganizationID)
	params := dbgen.UpsertAnalyticsFileExportParams{
		OrganizationID: org, ParquetEnabled: st.Parquet.Enabled, ParquetEvents: st.Parquet.Events, ParquetRollups: st.Parquet.Rollups,
		ParquetGranularity: string(st.Parquet.Granularity), WebhookEnabled: st.Webhook.Enabled, WebhookUrl: st.Webhook.URL,
		ExportedUntil: storage.Timestamp(s.now().Truncate(time.Hour).Add(time.Hour)), ChangedBy: p.Actor().ID,
		SecretCiphertext: []byte{}, SecretWrappedKey: []byte{},
	}
	switch old, err := q.GetAnalyticsFileExport(ctx, org); {
	case err == nil:
		params.SecretCiphertext, params.SecretWrappedKey, params.SecretKeyID, params.SecretHint =
			old.SecretCiphertext, old.SecretWrappedKey, old.SecretKeyID, old.SecretHint
	case !errors.Is(err, pgx.ErrNoRows):
		return params, fmt.Errorf("analytics: read the file export: %w", err)
	}
	if secret != "" {
		sealed, err := signing.Seal(ctx, s.o.Crypter, []byte(secret), secretBinding(p.OrganizationID))
		if err != nil {
			return params, fmt.Errorf("analytics: seal the webhook secret: %w", err)
		}
		params.SecretCiphertext, params.SecretWrappedKey, params.SecretKeyID, params.SecretHint =
			sealed.Ciphertext, sealed.WrappedKey, sealed.KeyID, sealed.Hint
	}
	if st.Webhook.Enabled && len(params.SecretCiphertext) == 0 {
		return params, plxerr.New(plxerr.InvalidFormat, "a webhook export needs a signing secret")
	}
	return params, nil
}

// normalizeSettings fills the defaults of settings and refuses the ones that
// cannot work. The webhook address is stored and shown, so it carries no
// credentials, query or fragment, and must be an address the egress policy
// allows.
func normalizeSettings(st ExportSettings) (ExportSettings, error) {
	if st.Parquet.Granularity == "" {
		st.Parquet.Granularity = Hour
	}
	if !st.Parquet.Granularity.Valid() {
		return st, plxerr.New(plxerr.InvalidEnumValue, "%q is not a granularity; use 5m, 1h or 1d", st.Parquet.Granularity)
	}
	if st.Parquet.Enabled && !st.Parquet.Events && !st.Parquet.Rollups {
		return st, plxerr.New(plxerr.InvalidFormat, "the Parquet export writes events, rollups or both")
	}
	if !st.Webhook.Enabled && st.Webhook.URL == "" {
		return st, nil
	}
	u, err := url.Parse(st.Webhook.URL)
	switch {
	case err != nil || u.Host == "":
		return st, plxerr.New(plxerr.InvalidFormat, "the webhook address is not a URL")
	case u.Scheme != "https":
		return st, plxerr.New(plxerr.InvalidFormat, "the webhook address must use https")
	case u.User != nil || u.RawQuery != "" || u.Fragment != "":
		return st, plxerr.New(plxerr.InvalidFormat, "the webhook address must not carry credentials, a query or a fragment")
	}
	if err := httpx.CheckURL(u.Scheme, u.Hostname()); err != nil {
		return st, plxerr.Wrap(plxerr.InvalidFormat, err, "the webhook address is not allowed")
	}
	st.Webhook.URL = u.String()
	return st, nil
}

// objectsAdapter lets an exporter put its files into the object store: a
// file goes under "exports/" and a body is read whole, because the store
// takes bytes. The exporter bounds a file by rows, which come from the
// limits registry.
type objectsAdapter struct {
	store objects.Store
}

var _ ext.ObjectStore = objectsAdapter{}

// Put stores the body under the exporter's key below exportPrefix.
func (a objectsAdapter) Put(ctx context.Context, key string, body io.Reader, contentType string) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return fmt.Errorf("analytics: read the export file: %w", err)
	}
	if _, err := a.store.Put(ctx, exportPrefix+key, data, contentType); err != nil {
		return fmt.Errorf("analytics: store the export file: %w", err)
	}
	return nil
}

// ExportFiles delivers the closed hours since each organisation's last
// delivery to its Parquet and webhook destinations; one organisation's
// failure does not stop the others (ANL-011). The licence is read from ctx
// (edition.WithProvider), which a worker job does not carry by itself.
func (s *Service) ExportFiles(ctx context.Context) error {
	if s.o.Exporter == nil {
		return nil
	}
	stop := s.now().Add(fileExportRunBudget)
	var errs []error
	err := s.o.Organizations(ctx, func(org string) error {
		if err := s.exportFilesOrg(ctx, org, stop); err != nil {
			errs = append(errs, err)
		}
		return nil
	})
	return errors.Join(append(errs, err)...)
}

// exportFilesOrg delivers one organisation's closed hours, oldest first,
// until it has caught up, a window fails or the budget is spent, and records
// how far it got.
func (s *Service) exportFilesOrg(ctx context.Context, org string, stop time.Time) error {
	cfg, secret, err := s.fileExportOf(ctx, org)
	if err != nil || cfg == nil {
		return err
	}
	from := storage.Time(cfg.ExportedUntil).Truncate(time.Hour)
	to := s.now().Add(-s.o.Export.Lag).Truncate(time.Hour)
	done, exportErr := from, error(nil)
	for done.Before(to) && s.now().Before(stop) {
		next := done.Add(time.Hour)
		if exportErr = s.exportWindow(ctx, org, done, next, fileExportOf(*cfg).Settings, secret); exportErr != nil {
			break
		}
		done = next
	}
	if !done.After(from) && exportErr == nil {
		return nil
	}
	msg := ""
	if exportErr != nil {
		msg = exportErr.Error()
	}
	err = s.inOrg(ctx, org, "", func(ctx context.Context, tx pgx.Tx) error {
		return dbgen.New(tx).RecordFileExportAttempt(ctx, dbgen.RecordFileExportAttemptParams{ //nolint:wrapcheck // wrapped below
			OrganizationID: storage.MustUUID(org), ExportedUntil: storage.Timestamp(done), LastError: msg, LastAttemptAt: storage.Timestamp(s.now()),
		})
	})
	if err != nil {
		return fmt.Errorf("analytics: record the file export: %w", err)
	}
	return exportErr
}

// fileExportOf reads an organisation's enabled export and opens its signing
// secret; it returns no row when the organisation exports nothing.
func (s *Service) fileExportOf(ctx context.Context, org string) (*dbgen.AnalyticsFileExport, string, error) {
	var row dbgen.AnalyticsFileExport
	err := s.inOrg(ctx, org, "", func(ctx context.Context, tx pgx.Tx) error {
		var err error
		row, err = dbgen.New(tx).GetAnalyticsFileExport(ctx, storage.MustUUID(org))
		return err //nolint:wrapcheck // handled below
	})
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !(row.ParquetEnabled || row.WebhookEnabled) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", fmt.Errorf("analytics: read the file export: %w", err)
	}
	if len(row.SecretCiphertext) == 0 {
		return &row, "", nil
	}
	plain, err := signing.Open(ctx, s.o.Crypter, signing.Sealed{
		Ciphertext: row.SecretCiphertext, WrappedKey: row.SecretWrappedKey, KeyID: row.SecretKeyID,
	}, secretBinding(org))
	if err != nil {
		return nil, "", fmt.Errorf("analytics: open the webhook secret: %w", err)
	}
	return &row, string(plain), nil
}

// exportWindow hands one closed window to the exporter.
func (s *Service) exportWindow(ctx context.Context, org string, from, to time.Time, settings ExportSettings, secret string) error {
	req := ext.ExportRequest{
		OrganizationID: org, From: from, To: to, Settings: settings, Secret: secret, Rollups: s.o.Store, HTTP: s.o.WebhookClient,
		RowsPerFile: int(s.o.Limits.Get(limits.TelemetryExportFileRows)), BatchSize: int(s.o.Limits.Get(limits.TelemetryExportBatchEvents)),
		Now: s.now,
	}
	if reader, ok := s.o.Store.(ext.EventReader); ok {
		req.Events = reader
	}
	if s.o.Objects != nil {
		req.Objects = objectsAdapter{store: s.o.Objects}
	}
	if err := s.o.Exporter.Export(ctx, req); err != nil {
		return fmt.Errorf("analytics: export %s to %s: %w", from.Format(time.RFC3339), to.Format(time.RFC3339), err)
	}
	return nil
}

// defaultWebhookClient is the SSRF-safe client of the webhook export.
func defaultWebhookClient() (*http.Client, error) {
	client, err := httpx.NewClient(httpx.Options{Destination: httpx.KindAnalyticsWebhook, Timeout: 30 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("analytics: %w", err)
	}
	return client, nil
}

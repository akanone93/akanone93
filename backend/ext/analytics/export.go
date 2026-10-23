// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package analytics

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

// StoredEvent is a raw event with the dimensions of its device, as the
// exports carry it. Its JSON form is the body of the webhook export.
type StoredEvent struct {
	ID              string          `json:"id"`
	OrganizationID  string          `json:"organization_id"`
	AppID           string          `json:"app_id"`
	EnvironmentID   string          `json:"environment_id"`
	DeviceID        string          `json:"device_id"`
	Name            string          `json:"name"`
	Time            time.Time       `json:"time"`
	ReleaseSequence int64           `json:"release_sequence"`
	PluginKey       string          `json:"plugin_key"`
	Route           string          `json:"route"`
	Category        string          `json:"category"`
	DeviceClass     string          `json:"device_class"`
	Locale          string          `json:"locale"`
	Fields          json.RawMessage `json:"fields"`
}

// EventReader reads the raw events of an organisation. A Store that keeps
// raw events implements it next to Store; the exports read events through
// it, so they work with whichever store the installation runs.
type EventReader interface {
	// ReadEvents returns up to limit events of org in [from, to), ordered by
	// time and identifier, that follow the opaque cursor after; the empty
	// cursor starts at the beginning. Each event is returned once, however
	// often it was written. next continues after the last event returned; a
	// page shorter than limit is the last.
	ReadEvents(ctx context.Context, org string, from, to time.Time, after string, limit int) (events []StoredEvent, next string, err error)
}

// RollupReader reads rollups; every Store is one.
type RollupReader interface {
	Window(ctx context.Context, org string, g Granularity, from, to time.Time) ([]Point, error)
}

// ObjectStore is where an exporter puts its files. Put stores the body
// under key. A file is written once for its window, so an export repeated
// after a failure writes the same bytes under the same key again, which the
// store may keep as it is.
type ObjectStore interface {
	Put(ctx context.Context, key string, body io.Reader, contentType string) error
}

// ParquetSettings is what an organisation exports as Parquet files.
type ParquetSettings struct {
	Enabled bool
	// Events and Rollups choose the kinds of file.
	Events, Rollups bool
	// Granularity is the width of the exported rollups.
	Granularity Granularity
}

// WebhookSettings is where an organisation's events are posted.
type WebhookSettings struct {
	Enabled bool
	// URL is the https address of the receiver. The signing secret is not
	// a setting: it travels beside the settings and is never returned.
	URL string
}

// ExportSettings is an organisation's configuration of the enterprise
// exports (ANL-011). The OTLP export of the core is configured separately.
type ExportSettings struct {
	Parquet ParquetSettings
	Webhook WebhookSettings
}

// Enabled reports whether any export is on.
func (s ExportSettings) Enabled() bool { return s.Parquet.Enabled || s.Webhook.Enabled }

// ExportRequest asks for the export of one closed window of one
// organisation. The core hands an exporter every connection and every size
// it may use: it opens none of its own and reads no limit of its own.
type ExportRequest struct {
	OrganizationID string
	// From and To bound the window, [From, To), on UTC hour boundaries.
	From, To time.Time
	Settings ExportSettings
	// Secret is the webhook's Standard Webhooks signing secret ("whsec_"
	// and base64); empty when no webhook is enabled.
	Secret  string
	Events  EventReader
	Rollups RollupReader
	Objects ObjectStore
	// HTTP is the core's SSRF-safe, egress-registered client.
	HTTP *http.Client
	// RowsPerFile bounds the rows of a Parquet file and BatchSize the events
	// of a webhook message; both come from the limits registry.
	RowsPerFile, BatchSize int
	Now                    func() time.Time
}

// Exporter runs the enterprise exports. The enterprise binary sets one in
// the server's options; without one the exports are refused with PLX-8600
// (GOV-030).
type Exporter interface {
	// Validate says why settings, and the signing secret that goes with
	// them (empty when it is unchanged), are not acceptable. It opens no
	// connection and refuses settings that enable an export the licence does
	// not allow.
	Validate(ctx context.Context, s ExportSettings, secret string) error
	// Export writes the window to every destination the settings enable and
	// returns nil only when all of them accepted it. The core exports the
	// window again from its start after a failure, so an exporter must make
	// the repetition harmless: the same files under the same keys, the same
	// message identifiers.
	Export(ctx context.Context, r ExportRequest) error
}

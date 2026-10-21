// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

const filesUsage = "Usage: plux analytics files show|set [flags]\n"

// fileExportFlags are the flags of `plux analytics files set`.
type fileExportFlags struct {
	parquetEnabled, parquetEvents, parquetRollups bool
	parquetGranularity                            string
	webhookEnabled                                bool
	webhookURL                                    string
	secretFile                                    string
}

// settings returns the export settings the flags describe.
func (f fileExportFlags) settings() *pluxv1.FileExportSettings {
	return &pluxv1.FileExportSettings{
		ParquetEnabled: f.parquetEnabled, ParquetEvents: f.parquetEvents, ParquetRollups: f.parquetRollups,
		ParquetGranularity: f.parquetGranularity, WebhookEnabled: f.webhookEnabled, WebhookUrl: f.webhookURL,
	}
}

// analyticsFiles shows and sets the Parquet and webhook file exports (ANL-011).
func (e env) analyticsFiles(args []string) int {
	var f fileExportFlags
	c, _, code, ok := e.governanceCmd("analytics files", filesUsage, args, map[string]subUsage{
		"show": {"Usage: plux analytics files show [--json]\n\nShows the Parquet and webhook exports, the end of the last hour delivered and the last error.", 0},
		"set": {"Usage: plux analytics files set [--parquet-enabled] [--parquet-events] [--parquet-rollups] [--parquet-granularity 5m|1h|1d]" +
			" [--webhook-enabled --webhook-url url] [--webhook-secret-file path|-] [--json]\n\n" +
			"Sets every export setting: a flag that is not given is off or empty. The webhook secret is read from the file, or from standard input with -, " +
			"so it stays out of the process list; without a file the stored secret is kept.", 0},
	}, func(set *flag.FlagSet) {
		set.BoolVar(&f.parquetEnabled, "parquet-enabled", false, "write Parquet files")
		set.BoolVar(&f.parquetEvents, "parquet-events", false, "include the raw events in the Parquet files")
		set.BoolVar(&f.parquetRollups, "parquet-rollups", false, "include the rollups in the Parquet files")
		set.StringVar(&f.parquetGranularity, "parquet-granularity", "", "the width of the exported rollups: `granularity` 5m, 1h or 1d")
		set.BoolVar(&f.webhookEnabled, "webhook-enabled", false, "deliver the exports to the webhook")
		set.StringVar(&f.webhookURL, "webhook-url", "", "the webhook's https `url`")
		set.StringVar(&f.secretFile, "webhook-secret-file", "", "the `path` of the webhook signing secret, or - for standard input")
	})
	if !ok {
		return code
	}
	client, err := e.analyticsClient(c)
	if err != nil {
		return e.fail("analytics files", err)
	}
	ctx := context.Background()
	switch args[0] {
	case "show":
		res, err := client.GetFileExport(ctx, connect.NewRequest(&pluxv1.GetFileExportRequest{}))
		if err != nil {
			return e.fail("analytics files show", err)
		}
		return e.emit(c.json, fileExportView(res.Msg.GetExport()), fileExportLine(res.Msg.GetExport()))
	default: // set
		return e.setFileExport(ctx, client, c, f)
	}
}

// setFileExport sends the settings and the webhook secret, then prints the
// export as the server stores it.
func (e env) setFileExport(ctx context.Context, client pluxv1connect.AnalyticsServiceClient, c common, f fileExportFlags) int {
	secret, err := e.webhookSecret(f.secretFile)
	if err != nil {
		return e.fail("analytics files set", err)
	}
	res, err := client.SetFileExport(ctx, connect.NewRequest(&pluxv1.SetFileExportRequest{Settings: f.settings(), WebhookSecret: secret}))
	if err != nil {
		return e.fail("analytics files set", err)
	}
	return e.emit(c.json, fileExportView(res.Msg.GetExport()), fileExportLine(res.Msg.GetExport()))
}

// webhookSecret reads the signing secret from path, or from standard input
// when path is "-". An empty path returns no secret, which keeps the stored one.
func (e env) webhookSecret(path string) (string, error) {
	var raw []byte
	var err error
	switch path {
	case "":
		return "", nil
	case "-":
		raw, err = io.ReadAll(e.stdin)
	default:
		raw, err = os.ReadFile(path) //nolint:gosec // the developer's own file, named on the command line
	}
	if err != nil {
		return "", fmt.Errorf("read the webhook secret: %w", err)
	}
	secret := strings.TrimSpace(string(raw))
	if secret == "" {
		return "", usageError("the webhook secret is empty")
	}
	return secret, nil
}

// fileExportView is the JSON form of a file export. The secret itself is never
// part of it, only the hint the server returns.
func fileExportView(x *pluxv1.FileExport) map[string]any {
	s := x.GetSettings()
	return map[string]any{
		"parquetEnabled": s.GetParquetEnabled(), "parquetEvents": s.GetParquetEvents(), "parquetRollups": s.GetParquetRollups(),
		"parquetGranularity": s.GetParquetGranularity(), "webhookEnabled": s.GetWebhookEnabled(), "webhookUrl": s.GetWebhookUrl(),
		"secretHint": x.GetSecretHint(), "exportedUntil": x.GetExportedUntil().AsTime().Format(time.RFC3339),
		"lastAttemptAt": x.GetLastAttemptAt().AsTime().Format(time.RFC3339), "lastError": x.GetLastError(),
	}
}

// fileExportLine is the text form of a file export: one line per export, then
// the progress and the last error.
func fileExportLine(x *pluxv1.FileExport) string {
	s := x.GetSettings()
	var text strings.Builder
	_, _ = fmt.Fprintf(&text, "parquet\t%s\t%s\n", enabledText(s.GetParquetEnabled()), s.GetParquetGranularity())
	secret := "no secret stored"
	if hint := x.GetSecretHint(); hint != "" {
		secret = "secret ending " + hint
	}
	_, _ = fmt.Fprintf(&text, "webhook\t%s\t%s\t%s\n", enabledText(s.GetWebhookEnabled()), s.GetWebhookUrl(), secret)
	_, _ = fmt.Fprintf(&text, "exported until %s", x.GetExportedUntil().AsTime().Format(time.RFC3339))
	if x.GetLastError() != "" {
		text.WriteString("\tlast error: " + x.GetLastError())
	}
	return text.String()
}

// enabledText names an export's state for text output.
func enabledText(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

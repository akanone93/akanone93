// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Package observability checks the alert rules, recording rules and Grafana
// dashboards under deploy/observability (OBS-004, OBS-005, ADR-0085) without
// a YAML library: every alert has a severity, a summary and a runbook_url
// whose file exists with the sections an operator needs; every metric an
// expression reads is registered by the server (found in its sources) or is
// listed with the exporter that provides it; dashboards are valid JSON with
// unique identifiers; the copies the Helm chart ships are identical to the
// source of truth. The rules file keeps one key per line and expressions in
// block scalars, which is all the reader understands; a rule file that leaves
// that layout is refused rather than half-read.
package observability

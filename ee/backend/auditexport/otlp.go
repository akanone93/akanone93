// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package auditexport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/nightCode42/plux3/backend/ext/export"
)

const (
	// otlpScopeName names the instrumentation scope of every log record.
	otlpScopeName = "plux.audit"
	// otlpSeverityNumber is OTLP's INFO severity (9).
	otlpSeverityNumber = 9
	// otlpSeverityText is the text of otlpSeverityNumber.
	otlpSeverityText = "INFO"
)

// otlpSink posts batches of records as OTLP/HTTP JSON log requests (SEC-143).
// Each record is one log record whose body is the record's JSON.
type otlpSink struct {
	client   *http.Client
	endpoint string
	hostname string
	version  string
	now      func() time.Time
}

// newOTLPSink returns the OTLP sink of a destination, posting through the
// client the core built with its egress guard.
func newOTLPSink(c export.Config) (*otlpSink, error) {
	if c.HTTP == nil || c.Now == nil {
		return nil, errors.New("auditexport: an OTLP sink needs an HTTP client and a clock")
	}
	return &otlpSink{client: c.HTTP, endpoint: c.Destination.Address, hostname: c.Hostname, version: c.Version, now: c.Now}, nil
}

// Name identifies the sink kind.
func (*otlpSink) Name() string { return KindOTLP }

// Deliver posts the batch as one export request. It refuses the write
// without a licence that lists the SIEM feature, and returns an error unless
// the collector answers with a success status.
func (s *otlpSink) Deliver(ctx context.Context, records []export.Record) error {
	if err := requireLicence(ctx); err != nil {
		return err
	}
	if len(records) == 0 {
		return nil
	}
	body, err := s.request(records)
	if err != nil {
		return err
	}
	return postJSON(ctx, s.client, s.endpoint, nil, body)
}

// request renders the batch as an OTLP ExportLogsServiceRequest.
func (s *otlpSink) request(records []export.Record) ([]byte, error) {
	observed := strconv.FormatInt(s.now().UnixNano(), 10)
	logs := make([]otlpLogRecord, 0, len(records))
	for _, r := range records {
		text, err := recordJSON(r)
		if err != nil {
			return nil, err
		}
		logs = append(logs, otlpLogRecord{
			TimeUnixNano:         strconv.FormatInt(r.At.UnixNano(), 10),
			ObservedTimeUnixNano: observed,
			SeverityNumber:       otlpSeverityNumber,
			SeverityText:         otlpSeverityText,
			Body:                 otlpString(string(text)),
			Attributes:           recordAttributes(r),
		})
	}
	req := otlpExportRequest{ResourceLogs: []otlpResourceLogs{{
		Resource: otlpResource{Attributes: []otlpKeyValue{
			{Key: "service.name", Value: otlpString(cefProduct)},
			{Key: "service.version", Value: otlpString(s.version)},
			{Key: "host.name", Value: otlpString(s.hostname)},
		}},
		ScopeLogs: []otlpScopeLogs{{
			Scope:      otlpScope{Name: otlpScopeName},
			LogRecords: logs,
		}},
	}}}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("auditexport: encode the OTLP request: %w", err)
	}
	return body, nil
}

// recordAttributes are the attributes a collector can index without parsing
// the body.
func recordAttributes(r export.Record) []otlpKeyValue {
	return []otlpKeyValue{
		{Key: "plux.organization", Value: otlpString(r.OrganizationID)},
		{Key: "plux.sequence", Value: otlpString(strconv.FormatInt(r.Sequence, 10))},
		{Key: "plux.action", Value: otlpString(r.Action)},
		{Key: "plux.actor", Value: otlpString(pair(r.ActorKind, r.ActorID))},
		{Key: "plux.target", Value: otlpString(pair(r.TargetKind, r.TargetID))},
		{Key: "plux.entry_hash", Value: otlpString(r.EntryHash)},
		{Key: "plux.previous_hash", Value: otlpString(r.PreviousHash)},
	}
}

// otlpExportRequest is the JSON form of an OTLP ExportLogsServiceRequest.
type otlpExportRequest struct {
	ResourceLogs []otlpResourceLogs `json:"resourceLogs"`
}

// otlpResourceLogs groups the log records of one resource.
type otlpResourceLogs struct {
	Resource  otlpResource    `json:"resource"`
	ScopeLogs []otlpScopeLogs `json:"scopeLogs"`
}

// otlpResource describes the entity that produced the logs.
type otlpResource struct {
	Attributes []otlpKeyValue `json:"attributes"`
}

// otlpScopeLogs groups the log records of one instrumentation scope.
type otlpScopeLogs struct {
	Scope      otlpScope       `json:"scope"`
	LogRecords []otlpLogRecord `json:"logRecords"`
}

// otlpScope names an instrumentation scope.
type otlpScope struct {
	Name string `json:"name"`
}

// otlpLogRecord is one log record. The OTLP JSON mapping writes 64-bit
// integers as decimal strings, which is why the times are strings.
type otlpLogRecord struct {
	TimeUnixNano         string         `json:"timeUnixNano"`
	ObservedTimeUnixNano string         `json:"observedTimeUnixNano"`
	SeverityNumber       int            `json:"severityNumber"`
	SeverityText         string         `json:"severityText"`
	Body                 otlpValue      `json:"body"`
	Attributes           []otlpKeyValue `json:"attributes"`
}

// otlpKeyValue is one attribute.
type otlpKeyValue struct {
	Key   string    `json:"key"`
	Value otlpValue `json:"value"`
}

// otlpValue is an OTLP AnyValue holding a string.
type otlpValue struct {
	StringValue string `json:"stringValue"`
}

// otlpString wraps a string as an AnyValue.
func otlpString(s string) otlpValue { return otlpValue{StringValue: s} }

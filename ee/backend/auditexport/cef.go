// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package auditexport

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/nightCode42/plux3/backend/ext/export"
)

const (
	// cefVendor and cefProduct are the first two header fields of every
	// event (ArcSight CEF:0).
	cefVendor  = "Plux"
	cefProduct = "Plux Server"
	// cefSeverity is the header severity of every event, on the CEF scale
	// of 0 to 10: audit entries are records of what happened, not alerts.
	cefSeverity = 3
	// cefAppName is the APP-NAME of the syslog message that carries an event.
	cefAppName = "plux-audit"
	// connectTimeout bounds a TLS handshake and each write of a stream.
	connectTimeout = 30 * time.Second
)

// cefSink delivers records as CEF:0 events, one per RFC 5424 message, over
// syslog with TLS and the octet-counting framing of RFC 5425 (SEC-143).
type cefSink struct {
	stream   *export.Stream
	hostname string
	version  string
}

// newCEFSink returns the CEF sink of a destination. Its stream dials through
// the core's guarded dialer and verifies the receiver's certificate.
func newCEFSink(c export.Config) (*cefSink, error) {
	stream, err := export.NewStream(c.Destination.Address, c.Dial, c.TLS, connectTimeout)
	if err != nil {
		return nil, fmt.Errorf("auditexport: open the CEF stream: %w", err)
	}
	return &cefSink{stream: stream, hostname: c.Hostname, version: c.Version}, nil
}

// Name identifies the sink kind.
func (*cefSink) Name() string { return KindCEF }

// Deliver sends the batch on one connection and returns once the receiver
// has read it and closed the connection. It refuses the write without a
// licence that lists the SIEM feature.
func (s *cefSink) Deliver(ctx context.Context, records []export.Record) error {
	if err := requireLicence(ctx); err != nil {
		return err
	}
	var batch bytes.Buffer
	for _, r := range records {
		message, err := export.SyslogMessage{
			Facility: export.FacilityLogAudit,
			Severity: export.SeverityInformational,
			Time:     r.At,
			Hostname: s.hostname,
			AppName:  cefAppName,
			Text:     []byte(cefLine(r, s.version)),
		}.Marshal()
		if err != nil {
			return fmt.Errorf("auditexport: format the CEF event %d: %w", r.Sequence, err)
		}
		batch.Write(export.Frame(message))
	}
	if err := s.stream.Send(ctx, batch.Bytes()); err != nil {
		return fmt.Errorf("auditexport: send the CEF batch: %w", err)
	}
	return nil
}

// cefLine renders a record as a CEF:0 event. The header names the vendor,
// product, version, action and severity; the extension carries the record's
// fields with fixed keys, in a fixed order, so the same record always gives
// the same line.
func cefLine(r export.Record, version string) string {
	header := []string{
		"CEF:0",
		cefHeaderField(cefVendor),
		cefHeaderField(cefProduct),
		cefHeaderField(version),
		cefHeaderField(r.Action),
		cefHeaderField(r.Action),
		strconv.Itoa(cefSeverity),
	}
	return strings.Join(header, "|") + "|" + cefExtension(r)
}

// cefHeaderField escapes a header field: a backslash and a pipe are
// preceded by a backslash, and a line break becomes a space, because a
// header field may not hold one.
func cefHeaderField(s string) string {
	return strings.NewReplacer(`\`, `\\`, `|`, `\|`, "\r", " ", "\n", " ").Replace(s)
}

// cefExtensionValue escapes an extension value: a backslash and an equals
// sign are preceded by a backslash, and a line break is written as \r or \n.
func cefExtensionValue(s string) string {
	return strings.NewReplacer(`\`, `\\`, `=`, `\=`, "\r", `\r`, "\n", `\n`).Replace(s)
}

// cefExtension renders the extension of a record as key=value pairs
// separated by spaces. The keys are the ones ArcSight defines for the
// fields they name; cs1 to cs4 carry the organisation, the entry's hash, its
// previous hash and the resource it concerns, labelled as the labels say.
func cefExtension(r export.Record) string {
	fields := [][2]string{
		{"rt", strconv.FormatInt(r.At.UnixMilli(), 10)},
		{"externalId", r.ID},
		{"suid", r.ActorID},
		{"suser", r.ActorDisplay},
		{"src", r.SourceIP},
		{"requestClientApplication", r.UserAgent},
		{"cn1Label", "sequence"},
		{"cn1", strconv.FormatInt(r.Sequence, 10)},
		{"cs1Label", "organization"},
		{"cs1", r.OrganizationID},
		{"cs2Label", "entryHash"},
		{"cs2", r.EntryHash},
		{"cs3Label", "previousHash"},
		{"cs3", r.PreviousHash},
		{"cs4Label", "resource"},
		{"cs4", pair(r.TargetKind, r.TargetID)},
		{"msg", r.Detail},
	}
	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		parts = append(parts, f[0]+"="+cefExtensionValue(f[1]))
	}
	return strings.Join(parts, " ")
}

// pair renders a kind and an identifier as "kind:id"; empty when both are.
func pair(kind, id string) string {
	if kind == "" && id == "" {
		return ""
	}
	return kind + ":" + id
}

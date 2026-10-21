// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auditexport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/nightCode42/plux3/backend/ext/export"
)

const (
	// syslogAppName is the APP-NAME of every message.
	syslogAppName = "plux-audit"
	// structuredDataID is the SD-ID of the element that carries an entry's
	// identity. RFC 5424 section 7.2 asks for "name@enterprise-number"; Plux
	// has no number of its own, so the number IANA reserves for
	// documentation (RFC 5612) stands in until it registers one.
	structuredDataID = "plux@32473"
)

// syslogSink delivers records as RFC 5424 messages over TLS with the
// octet-counting framing of RFC 5425 (SEC-143). The message is compact JSON
// of the whole record; the structured data repeats the fields a SIEM routes
// on.
type syslogSink struct {
	stream   *export.Stream
	hostname string
}

// newSyslogSink returns the sink over a stream.
func newSyslogSink(stream *export.Stream, hostname string) *syslogSink {
	return &syslogSink{stream: stream, hostname: hostname}
}

// Name identifies the sink.
func (*syslogSink) Name() string { return KindSyslog }

// Deliver sends the batch on a connection of its own and returns once the
// receiver has read it and closed the connection.
func (s *syslogSink) Deliver(ctx context.Context, records []export.Record) error {
	var batch bytes.Buffer
	for _, r := range records {
		message, err := SyslogMessage(r, s.hostname)
		if err != nil {
			return err
		}
		batch.Write(export.Frame(message))
	}
	if err := s.stream.Send(ctx, batch.Bytes()); err != nil {
		return fmt.Errorf("auditexport: send: %w", err)
	}
	return nil
}

// SyslogMessage renders a record as an RFC 5424 message: facility 13 (log
// audit), severity 6 (informational), the entry's action as MSGID, a
// structured-data element with organisation, actor, action, resource,
// sequence and the entry's hash and previous hash, and the whole record as
// compact JSON.
func SyslogMessage(r export.Record, hostname string) ([]byte, error) {
	text, err := recordJSON(r)
	if err != nil {
		return nil, err
	}
	message, err := export.SyslogMessage{
		Facility: export.FacilityLogAudit,
		Severity: export.SeverityInformational,
		Time:     r.At,
		Hostname: hostname,
		AppName:  syslogAppName,
		MsgID:    r.Action,
		Data: []export.SDElement{{ID: structuredDataID, Params: []export.SDParam{
			{Name: "organization", Value: r.OrganizationID},
			{Name: "actor", Value: pair(r.ActorKind, r.ActorID)},
			{Name: "action", Value: r.Action},
			{Name: "resource", Value: pair(r.TargetKind, r.TargetID)},
			{Name: "sequence", Value: strconv.FormatInt(r.Sequence, 10)},
			{Name: "entryHash", Value: r.EntryHash},
			{Name: "previousHash", Value: r.PreviousHash},
		}}},
		Text: text,
	}.Marshal()
	if err != nil {
		return nil, fmt.Errorf("auditexport: format the record %d: %w", r.Sequence, err)
	}
	return message, nil
}

// pair renders a kind and an identifier as "kind:id"; empty when both are.
func pair(kind, id string) string {
	if kind == "" && id == "" {
		return ""
	}
	return kind + ":" + id
}

// wireRecord is the JSON form of a record. It names every field the entry's
// hash covers, so a SIEM can recompute the hash, and the hash itself.
type wireRecord struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization"`
	Sequence       int64     `json:"sequence"`
	Time           time.Time `json:"time"`
	Actor          wireActor `json:"actor"`
	Action         string    `json:"action"`
	Target         wireActor `json:"target"`
	SourceIP       string    `json:"sourceIp,omitempty"`
	UserAgent      string    `json:"userAgent,omitempty"`
	RequestID      string    `json:"requestId,omitempty"`
	BeforeHash     string    `json:"beforeHash,omitempty"`
	AfterHash      string    `json:"afterHash,omitempty"`
	Detail         string    `json:"detail,omitempty"`
	PreviousHash   string    `json:"previousHash"`
	EntryHash      string    `json:"entryHash"`
}

// wireActor is an actor or a target: a kind, an identifier and, for an
// actor, a display name.
type wireActor struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Display string `json:"display,omitempty"`
}

// recordJSON renders a record as compact JSON.
func recordJSON(r export.Record) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	err := enc.Encode(wireRecord{
		ID: r.ID, OrganizationID: r.OrganizationID, Sequence: r.Sequence, Time: r.At.UTC(),
		Actor:  wireActor{Kind: r.ActorKind, ID: r.ActorID, Display: r.ActorDisplay},
		Action: r.Action, Target: wireActor{Kind: r.TargetKind, ID: r.TargetID},
		SourceIP: r.SourceIP, UserAgent: r.UserAgent, RequestID: r.RequestID,
		BeforeHash: r.BeforeHash, AfterHash: r.AfterHash, Detail: r.Detail,
		PreviousHash: r.PreviousHash, EntryHash: r.EntryHash,
	})
	if err != nil {
		return nil, fmt.Errorf("auditexport: encode the record %d: %w", r.Sequence, err)
	}
	return bytes.TrimRight(b.Bytes(), "\n"), nil
}

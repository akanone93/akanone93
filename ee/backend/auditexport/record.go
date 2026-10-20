// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package auditexport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nightCode42/plux3/backend/ext/export"
)

// wireRecord is the JSON form of a record in the OTLP body and the webhook
// data. It names every field the entry's hash covers, so a receiver can
// recompute the hash, and the hash itself. It has the same shape as the
// core's syslog body.
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

// recordJSON renders a record as compact JSON without a trailing newline.
// HTML characters are not escaped, so the text reads as the record holds it.
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

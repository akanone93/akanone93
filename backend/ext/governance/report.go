// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package governance

import (
	"context"
	"time"
)

// ReportVersion is the version of the report statement. A change to the
// statement's fields adds a version; Envelope.Open refuses one it does not
// know rather than guess at it.
const ReportVersion = 1

// Report is a compliance report: every production change an organisation
// made in a period, with who made it, who approved it and the evidence
// that proves both (GOV-026).
type Report struct {
	// Version is ReportVersion.
	Version        int    `json:"version"`
	OrganizationID string `json:"organizationId"`
	// From and To bound the period: a change belongs to it from From,
	// inclusive, until To, exclusive. Both are UTC.
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
	// GeneratedAt is when the report was requested, and GeneratedBy the
	// identifier of who requested it.
	GeneratedAt time.Time `json:"generatedAt"`
	GeneratedBy string    `json:"generatedBy"`
	// Changes are the production changes of the period, oldest first.
	Changes []Change `json:"changes"`
}

// Change is one production change.
type Change struct {
	ID string `json:"id"`
	// AppID is empty for a change that belongs to the organisation.
	AppID       string `json:"appId,omitempty"`
	Environment string `json:"environment"`
	// Resource and Action say what kind of artifact changed and how, in
	// the words of the approval engine; Subject names it stably.
	Resource string `json:"resource"`
	Action   string `json:"action"`
	Subject  string `json:"subject"`
	// ContentHash is the SHA-256, in hex, of the content the subject
	// changed to: the hash every approval of it is bound to.
	ContentHash string `json:"contentHash"`
	Summary     string `json:"summary,omitempty"`
	// Author is the identifier of the user who wrote the content, when
	// that is not who made the change.
	Author string `json:"author,omitempty"`
	// ChangedBy is who made the change, and ChangedAt when.
	ChangedBy string    `json:"changedBy"`
	ChangedAt time.Time `json:"changedAt"`
	// Approvals are the decisions of the request that authorised the
	// change; empty for a change no policy asked approval for.
	Approvals []Approval `json:"approvals,omitempty"`
	// BreakGlass is set when the change was made through break-glass.
	BreakGlass *BreakGlass `json:"breakGlass,omitempty"`
	// Evidence links the change to the records that prove it.
	Evidence []Evidence `json:"evidence,omitempty"`
}

// Approval is one decision on the request that authorised a change.
type Approval struct {
	DeciderID string    `json:"deciderId"`
	Decider   string    `json:"decider"`
	Decision  string    `json:"decision"`
	Comment   string    `json:"comment,omitempty"`
	DecidedAt time.Time `json:"decidedAt"`
	// AuditSequence is the entry of the organisation's audit chain that
	// records the decision.
	AuditSequence int64 `json:"auditSequence"`
	// KeyID and Signature are the audit key's signature over the decision
	// record, in base64; empty until the record is signed.
	KeyID     string `json:"keyId,omitempty"`
	Signature string `json:"signature,omitempty"`
}

// BreakGlass is the emergency a change was made in (GOV-024).
type BreakGlass struct {
	Justification string    `json:"justification"`
	ReviewDueAt   time.Time `json:"reviewDueAt"`
	// ReviewStatus is "open" or "reviewed"; ReviewedBy and ReviewedAt are
	// set once it is reviewed.
	ReviewStatus string    `json:"reviewStatus"`
	ReviewedBy   string    `json:"reviewedBy,omitempty"`
	ReviewedAt   time.Time `json:"reviewedAt,omitzero"`
}

// Evidence points at a record that proves part of a change.
type Evidence struct {
	// Kind is "approvalRequest", "decisionRecord" or "breakGlass".
	Kind string `json:"kind"`
	// Ref identifies the record; Link is where Studio shows it, as a path.
	Ref  string `json:"ref"`
	Link string `json:"link,omitempty"`
}

// ReportRenderer writes a signed report as a document for people (GOV-026).
//
// The core calls it under the licence's FeatureComplianceReports, after the
// report is signed: the document carries the envelope's signature so that
// it can be tied to the JSON an auditor verifies.
type ReportRenderer interface {
	// PDF renders the report, with a page of signatures for envelope.
	PDF(ctx context.Context, r Report, e Envelope) ([]byte, error)
}

// Extensions are the enterprise implementations a server is built with. A
// nil field selects the community default.
type Extensions struct {
	// Freezes evaluates change-freeze windows (GOV-025).
	Freezes FreezeCalendar
	// Reports renders compliance reports as PDF (GOV-026); nil leaves the
	// feature unavailable.
	Reports ReportRenderer
}

// OrDefault returns e with the community default in every nil field that
// has one.
func (e Extensions) OrDefault() Extensions {
	if e.Freezes == nil {
		e.Freezes = NoFreezes{}
	}
	return e
}

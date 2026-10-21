// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package notify tells people and systems what happened (SRV-062,
// SRV-063, ADR-0081).
//
// Other modules publish typed events to a Publisher. The service turns an
// event into rows, in the publisher's transaction-sized unit of work: an
// inbox entry for each subscribed user, a delivery for each subscribed
// channel (in-app inbox, SMTP, Slack, Microsoft Teams, Telegram) and a
// signed delivery for each outbound webhook endpoint. A worker then sends
// the deliveries, retrying with exponential backoff and jitter for 24 hours
// before it gives up with PLX-8401. Every outbound call goes through the
// SSRF-safe client (SEC-105), and no secret reaches a log, an error or the
// delivery log (SEC-092).
package notify

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// EventType names something that happened. The set is closed: a module that
// needs another adds it here, with its description, and the tests list it.
type EventType string

// The event types of Phase 9.
const (
	// ApprovalRequested: a change waits for an approver (GOV-020).
	ApprovalRequested EventType = "approval.requested"
	// ReleasePublished: a release was published or promoted (REL-001).
	ReleasePublished EventType = "release.published"
	// RolloutPaused: a rollout stopped, by hand or by a health gate (ROL-001).
	RolloutPaused EventType = "rollout.paused"
	// LockRequested: someone asked the holder of an editing lock for it (SRV-041).
	LockRequested EventType = "lock.requested"
	// FunctionFailing: a function keeps failing (FN-032).
	FunctionFailing EventType = "function.failing"
	// AlertFired: an alert rule on a metric fired (ANL-030).
	AlertFired EventType = "alert.fired"
	// BreakGlassOverdue: the post-hoc review of a break-glass action is
	// overdue (GOV-024, PLX-8006).
	BreakGlassOverdue EventType = "breakglass.review_overdue"
	// BreakGlassUsed: someone used break-glass (GOV-024).
	BreakGlassUsed EventType = "breakglass.used"
	// LicenceExpiring: the licence nears its end (GOV-040).
	LicenceExpiring EventType = "licence.expiring"
)

// Severity orders how much attention an event needs.
type Severity string

// The severities.
const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// TypeInfo describes an event type for the API and the documentation.
type TypeInfo struct {
	Type        EventType
	Description string
	Severity    Severity
}

// types is the registry, sorted by type.
var types = []TypeInfo{
	{AlertFired, "An alert rule on a metric fired", SeverityWarning},
	{ApprovalRequested, "A change is waiting for an approver", SeverityInfo},
	{BreakGlassOverdue, "The review of a break-glass action is overdue", SeverityCritical},
	{BreakGlassUsed, "Break-glass was used to bypass an approval", SeverityCritical},
	{FunctionFailing, "A function keeps failing", SeverityWarning},
	{LicenceExpiring, "The licence is about to expire", SeverityWarning},
	{LockRequested, "Someone asked for an editing lock you hold", SeverityInfo},
	{ReleasePublished, "A release was published", SeverityInfo},
	{RolloutPaused, "A rollout was paused", SeverityWarning},
}

// Types returns the registry, in order.
func Types() []TypeInfo { return slices.Clone(types) }

// Known reports whether t is a registered event type.
func Known(t EventType) bool {
	_, found := slices.BinarySearchFunc(types, t, func(i TypeInfo, t EventType) int { return strings.Compare(string(i.Type), string(t)) })
	return found
}

// Bounds on what an event carries; chat services and mail clients truncate
// beyond these anyway.
const (
	maxTitle      = 200
	maxBody       = 2000
	maxLink       = 2000
	maxAttributes = 32
	maxAttribute  = 512
)

// Event is something that happened in an organisation.
type Event struct {
	// ID identifies the event; empty has the service generate one.
	ID   string
	Type EventType
	// OrganizationID is where it happened.
	OrganizationID string
	// AppID narrows it to an app, when it concerns one.
	AppID string
	// Title is one line; Body is a few sentences. Neither may hold a
	// secret or personal data beyond what the recipient may see: they
	// travel to third-party services.
	Title string
	Body  string
	// Link is where to act: an absolute https URL or a path in Studio.
	Link string
	// Attributes are structured details for webhook receivers, such as
	// {"release": "42"}.
	Attributes map[string]string
	// At is when it happened; zero is now.
	At time.Time
}

// Publisher is what other modules publish to; *Service is one.
//
// Publish does not fail a caller's operation for a notification problem:
// modules log the error and go on, because a missed notification must not
// undo a release.
type Publisher interface {
	Publish(ctx context.Context, e Event) error
}

// validate checks an event before it is stored.
func (e Event) validate() error {
	if !Known(e.Type) {
		return plxerr.New(plxerr.InvalidEnumValue, "%q is not an event type", e.Type)
	}
	if e.OrganizationID == "" {
		return plxerr.New(plxerr.MissingProperty, "an event names its organisation")
	}
	if e.Title == "" || len(e.Title) > maxTitle || len(e.Body) > maxBody || len(e.Link) > maxLink {
		return plxerr.New(plxerr.LimitExceeded, "an event has a title of up to %d bytes, a body of up to %d and a link of up to %d", maxTitle, maxBody, maxLink)
	}
	if e.Link != "" && !strings.HasPrefix(e.Link, "https://") && !strings.HasPrefix(e.Link, "/") {
		return plxerr.New(plxerr.InvalidFormat, "an event link is an https URL or a path")
	}
	if len(e.Attributes) > maxAttributes {
		return plxerr.New(plxerr.LimitExceeded, "an event has at most %d attributes", maxAttributes)
	}
	for k, v := range e.Attributes {
		if k == "" || len(k) > 64 || len(v) > maxAttribute {
			return plxerr.New(plxerr.LimitExceeded, "an attribute name is 1 to 64 bytes and its value at most %d", maxAttribute)
		}
	}
	return nil
}

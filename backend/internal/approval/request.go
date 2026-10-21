// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package approval

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/schema/jcs"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// The statuses of a request. StatusExpired is never stored: it is what an
// open or approved request becomes when its time is up.
const (
	StatusOpen       = "open"
	StatusApproved   = "approved"
	StatusRejected   = "rejected"
	StatusExpired    = "expired"
	StatusSuperseded = "superseded"
	StatusUsed       = "used"
)

// DecisionRecord is a decision as stored, with the signature once the
// worker has made it.
type DecisionRecord struct {
	ID        string
	DeciderID string
	Decider   string
	Roles     []string
	Teams     []string
	// Decision is "approve" or "reject".
	Decision    string
	Comment     string
	ContentHash string
	DecidedAt   time.Time
	// AuditSequence is the entry of the audit chain that records it.
	AuditSequence int64
	// Payload is the canonical record the signature covers.
	Payload []byte
	// KeyID and Signature are empty until the record is signed.
	KeyID     string
	Signature []byte
}

// Request asks for the approval of one artifact.
type Request struct {
	ID              string
	OrganizationID  string
	AppID           string
	EnvironmentID   string
	Environment     string
	Production      bool
	Resource        schema.PolicyResource
	Action          string
	Subject         string
	ContentHash     string
	Summary         string
	Risk            []schema.PolicyRisk
	Payload         json.RawMessage
	PolicyKeys      []string
	ApprovalsNeeded int
	AuthorID        string
	RequestedByID   string
	RequestedBy     string
	Status          string
	CreatedAt       time.Time
	ExpiresAt       time.Time
	Decisions       []DecisionRecord
}

// Artifact rebuilds the artifact the request is about, as the matcher sees
// it.
func (r Request) Artifact() Artifact {
	return Artifact{
		Resource: r.Resource, OrganizationID: r.OrganizationID, AppID: r.AppID, Action: r.Action,
		Environment: Environment{ID: r.EnvironmentID, Key: r.Environment, Production: r.Production},
		Subject:     r.Subject, Hash: r.ContentHash, Summary: r.Summary, Risk: r.Risk, Payload: r.Payload, Author: r.AuthorID,
	}
}

// requestOf converts a stored request; decisions are attached separately.
func (s *Service) requestOf(row dbgen.ApprovalRequest) (Request, error) {
	risk := make([]schema.PolicyRisk, len(row.Risk))
	for i, r := range row.Risk {
		risk[i] = schema.PolicyRisk(r)
	}
	out := Request{
		ID: storage.ID(row.ID), OrganizationID: storage.ID(row.OrganizationID), AppID: appOf(row.AppID),
		EnvironmentID: storage.ID(row.EnvironmentID), Environment: row.EnvironmentKey, Production: row.Production,
		Resource: schema.PolicyResource(row.Resource), Action: row.Action, Subject: row.Subject, ContentHash: row.ContentHash,
		Summary: row.Summary, Risk: risk, Payload: row.Payload, PolicyKeys: row.PolicyKeys, ApprovalsNeeded: int(row.ApprovalsRequired),
		AuthorID: row.AuthorID, RequestedByID: row.RequestedByID, RequestedBy: row.RequestedBy, Status: row.Status,
		CreatedAt: storage.Time(row.CreatedAt), ExpiresAt: storage.Time(row.ExpiresAt),
	}
	if out.Payload != nil {
		var err error
		if out.Payload, err = jcs.Canonicalize(out.Payload, maxDepth); err != nil {
			return Request{}, fmt.Errorf("approval: read the content of request %s: %w", out.ID, err)
		}
	}
	if (out.Status == StatusOpen || out.Status == StatusApproved) && !s.now().Before(out.ExpiresAt) {
		out.Status = StatusExpired
	}
	return out, nil
}

// decisionOf converts a stored decision.
func decisionOf(row dbgen.ApprovalDecision) DecisionRecord {
	d := DecisionRecord{
		ID: storage.ID(row.ID), DeciderID: row.DeciderID, Decider: row.Decider, Roles: row.Roles, Teams: row.Teams,
		Decision: row.Decision, Comment: row.Comment, ContentHash: row.ContentHash, DecidedAt: storage.Time(row.DecidedAt),
		AuditSequence: row.AuditSequence, Payload: row.Payload, Signature: row.Signature,
	}
	if row.KeyID != nil {
		d.KeyID = *row.KeyID
	}
	return d
}

// facts reduces recorded decisions to what the evaluation reads.
func facts(decisions []DecisionRecord) []Decision {
	out := make([]Decision, len(decisions))
	for i, d := range decisions {
		out[i] = Decision{DeciderID: d.DeciderID, Roles: d.Roles, Teams: d.Teams, Approve: d.Decision == "approve", At: d.DecidedAt}
	}
	return out
}

// withDecisions attaches a request's decisions.
func (s *Service) withDecisions(ctx context.Context, q *dbgen.Queries, row dbgen.ApprovalRequest) (Request, error) {
	rows, err := q.ListApprovalDecisions(ctx, row.ID)
	if err != nil {
		return Request{}, fmt.Errorf("approval: list the decisions: %w", err)
	}
	r, err := s.requestOf(row)
	if err != nil {
		return Request{}, err
	}
	r.Decisions = make([]DecisionRecord, len(rows))
	for i, d := range rows {
		r.Decisions[i] = decisionOf(d)
	}
	return r, nil
}

// ListApprovals lists the requests of an app, newest first, optionally only
// those with one status (the derived "expired" included).
func (s *Service) ListApprovals(ctx context.Context, p auth.Principal, appID, status string, after storage.Cursor, limit int32) ([]Request, error) {
	if err := authorizeScope(p, auth.AppRead, appID); err != nil {
		return nil, fmt.Errorf("approval: %w", err)
	}
	app, err := scopeUUID(appID)
	if err != nil {
		return nil, err
	}
	stored := status
	if status == StatusExpired {
		stored = ""
	}
	var out []Request
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		rows, err := q.ListApprovalRequests(ctx, dbgen.ListApprovalRequestsParams{
			AppID: app, Status: stored, BeforeTime: beforeTime(after), BeforeID: beforeID(after), PageSize: limit,
		})
		if err != nil {
			return fmt.Errorf("approval: list the requests: %w", err)
		}
		for _, row := range rows {
			r, err := s.withDecisions(ctx, q, row)
			if err != nil {
				return err
			}
			if status == StatusExpired && r.Status != StatusExpired {
				continue
			}
			out = append(out, r)
		}
		return nil
	})
	return out, err
}

// GetApproval returns one request with its decisions.
func (s *Service) GetApproval(ctx context.Context, p auth.Principal, appID, id string) (Request, error) {
	if err := authorizeScope(p, auth.AppRead, appID); err != nil {
		return Request{}, fmt.Errorf("approval: %w", err)
	}
	var out Request
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = s.loadRequest(ctx, dbgen.New(tx), appID, id)
		return err
	})
	return out, err
}

// loadRequest reads a request of an app with its decisions; a request of
// another app is not found.
func (s *Service) loadRequest(ctx context.Context, q *dbgen.Queries, appID, id string) (Request, error) {
	rid, err := storage.UUID(id)
	if err != nil {
		return Request{}, plxerr.New(plxerr.InvalidFormat, "the approval identifier is not valid")
	}
	row, err := q.GetApprovalRequest(ctx, rid)
	if err != nil || appOf(row.AppID) != appID {
		return Request{}, plxerr.New(plxerr.ResourceNotFound, "no such approval request")
	}
	return s.withDecisions(ctx, q, row)
}

// beforeTime is the time a newest-first page continues before: plus
// infinity at the start.
func beforeTime(c storage.Cursor) pgtype.Timestamptz {
	if c.Time.IsZero() {
		return pgtype.Timestamptz{InfinityModifier: pgtype.Infinity, Valid: true}
	}
	return storage.Timestamp(c.Time)
}

// beforeID is the identifier a newest-first page continues before: the
// largest UUID at the start.
func beforeID(c storage.Cursor) pgtype.UUID {
	if id, err := storage.UUID(c.ID); err == nil && c.ID != "" {
		return id
	}
	largest := pgtype.UUID{Valid: true}
	for i := range largest.Bytes {
		largest.Bytes[i] = 0xff
	}
	return largest
}

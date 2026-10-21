// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package compliance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/ext/governance"
	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// The formats a report is fetched in, and their media types.
const (
	FormatJSON = "json"
	FormatPDF  = "pdf"

	// ContentTypeJSON is the media type of a DSSE envelope.
	ContentTypeJSON = "application/vnd.dsse.envelope.v1+json"
	// ContentTypePDF is the media type of the PDF.
	ContentTypePDF = "application/pdf"
)

// IDs generates identifiers.
type IDs interface{ New() (string, error) }

// LimitsFunc returns the limits in force for an organisation, read in the
// transaction.
type LimitsFunc func(ctx context.Context, tx pgx.Tx, organizationID string) (limits.Set, error)

// Options configures a Service.
type Options struct {
	DB     *storage.DB
	Audit  *audit.Log
	IDs    IDs
	Limits LimitsFunc
	// Signer signs statements with SignerKey, the audit key. Only the worker
	// role holds one (L-3, ADR-0006); nil leaves reports for the worker to
	// sign.
	Signer    signing.Signer
	SignerKey string
	// Renderer writes the PDF (GOV-026); nil leaves PDF unavailable.
	Renderer governance.ReportRenderer
	// Log receives the failures that must not fail a request; nil uses the
	// default logger.
	Log *slog.Logger
	// Now is the clock; nil uses time.Now.
	Now func() time.Time
}

// Service produces compliance reports.
type Service struct {
	o   Options
	now func() time.Time
}

// New returns the service.
func New(o Options) (*Service, error) {
	if o.DB == nil || o.Audit == nil || o.IDs == nil || o.Limits == nil {
		return nil, errors.New("compliance: a database, an audit log, identifiers and limits are required")
	}
	if o.Signer != nil && o.SignerKey == "" {
		return nil, errors.New("compliance: a signer needs the reference of the key it signs with")
	}
	if o.Log == nil {
		o.Log = slog.Default()
	}
	now := o.Now
	if now == nil {
		now = time.Now
	}
	return &Service{o: o, now: now}, nil
}

// Info describes a report without its content.
type Info struct {
	ID            string
	From, To      time.Time
	Changes       int
	RequestedByID string
	RequestedBy   string
	RequestedAt   time.Time
	// Signed is whether the worker has signed the report; KeyID and SignedAt
	// are set once it has.
	Signed   bool
	KeyID    string
	SignedAt time.Time
}

// Create gathers the production changes of the period from, inclusive, until
// to, exclusive, into a report and stores its statement. The principal needs
// compliance.report and the licence's FeatureComplianceReports for writing.
// A period with more changes than governance.reportChanges is refused. The
// report is signed by the worker, or at once when this process holds the
// audit key.
func (s *Service) Create(ctx context.Context, p auth.Principal, from, to time.Time) (Info, error) {
	if err := p.Authorize(auth.ComplianceReport); err != nil {
		return Info{}, fmt.Errorf("compliance: %w", err)
	}
	if err := edition.Require(ctx, edition.FeatureComplianceReports); err != nil {
		return Info{}, fmt.Errorf("compliance: %w", err)
	}
	if from.IsZero() || to.IsZero() || !from.Before(to) {
		return Info{}, plxerr.New(plxerr.InvalidFormat, "a report covers a period that starts before it ends")
	}
	var info Info
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		info, err = s.create(ctx, tx, p, stored(from), stored(to))
		return err
	})
	if err != nil {
		return Info{}, err
	}
	if s.o.Signer == nil {
		return info, nil
	}
	if _, err := s.Sign(ctx, p.OrganizationID); err != nil {
		s.o.Log.WarnContext(ctx, "compliance: the report was stored but not yet signed", slog.Any("error", err))
		return info, nil
	}
	return s.info(ctx, p, info.ID)
}

// stored returns an instant in UTC at the resolution PostgreSQL stores, so that
// the period a report states, the period it is stored with and the period the
// caller is told are the same.
func stored(t time.Time) time.Time { return t.UTC().Truncate(time.Microsecond) }

// create builds, stores and audits a report inside the transaction.
func (s *Service) create(ctx context.Context, tx pgx.Tx, p auth.Principal, from, to time.Time) (Info, error) {
	report, err := s.build(ctx, tx, p, from, to)
	if err != nil {
		return Info{}, err
	}
	statement, err := json.Marshal(report)
	if err != nil {
		return Info{}, fmt.Errorf("compliance: encode the report: %w", err)
	}
	id, err := s.o.IDs.New()
	if err != nil {
		return Info{}, fmt.Errorf("compliance: %w", err)
	}
	actor := p.Actor()
	err = dbgen.New(tx).InsertComplianceReport(ctx, dbgen.InsertComplianceReportParams{
		ID: storage.MustUUID(id), OrganizationID: storage.MustUUID(p.OrganizationID), PeriodFrom: storage.Timestamp(from), PeriodTo: storage.Timestamp(to),
		Changes: int32(len(report.Changes)), RequestedByID: actor.ID, RequestedBy: actor.Display, RequestedAt: storage.Timestamp(report.GeneratedAt), //nolint:gosec // bounded by governance.reportChanges
		Statement: statement,
	})
	if err != nil {
		return Info{}, fmt.Errorf("compliance: store the report: %w", err)
	}
	sum := sha256.Sum256(statement)
	_, err = s.o.Audit.Append(ctx, tx, audit.Entry{
		OrganizationID: p.OrganizationID, Actor: actor, Action: audit.ComplianceReportCreated, TargetKind: "compliance_report", TargetID: id,
		AfterHash: hex.EncodeToString(sum[:]),
		Detail:    fmt.Sprintf("%d changes from %s to %s", len(report.Changes), from.Format(time.RFC3339), to.Format(time.RFC3339)),
	})
	if err != nil {
		return Info{}, fmt.Errorf("compliance: %w", err)
	}
	return Info{ID: id, From: from, To: to, Changes: len(report.Changes), RequestedByID: actor.ID, RequestedBy: actor.Display, RequestedAt: report.GeneratedAt}, nil
}

// Get returns a report's description and, once it is signed, its content in
// the format asked for: the DSSE envelope as JSON, or the PDF. The principal
// needs compliance.report and the licence's FeatureComplianceReports for
// reading. A report that is not yet signed has no content.
func (s *Service) Get(ctx context.Context, p auth.Principal, id, format string) (Info, []byte, string, error) {
	if err := p.Authorize(auth.ComplianceReport); err != nil {
		return Info{}, nil, "", fmt.Errorf("compliance: %w", err)
	}
	if err := edition.RequireRead(ctx, edition.FeatureComplianceReports); err != nil {
		return Info{}, nil, "", fmt.Errorf("compliance: %w", err)
	}
	if format != FormatJSON && format != FormatPDF {
		return Info{}, nil, "", plxerr.New(plxerr.InvalidEnumValue, "a report format is %q or %q", FormatJSON, FormatPDF)
	}
	row, err := s.row(ctx, p, id)
	if err != nil {
		return Info{}, nil, "", err
	}
	info := infoOf(row)
	if !info.Signed {
		return info, nil, "", nil
	}
	env := governance.NewEnvelope(row.Statement, *row.KeyID, row.Signature)
	if format == FormatJSON {
		content, err := json.Marshal(env)
		if err != nil {
			return Info{}, nil, "", fmt.Errorf("compliance: encode the envelope: %w", err)
		}
		return info, content, ContentTypeJSON, nil
	}
	content, err := s.pdf(ctx, row.Statement, env)
	return info, content, ContentTypePDF, err
}

// pdf renders a signed report's statement.
func (s *Service) pdf(ctx context.Context, statement []byte, env governance.Envelope) ([]byte, error) {
	if s.o.Renderer == nil {
		return nil, plxerr.New(plxerr.PreconditionFailed, "PDF reports are not available on this server")
	}
	var report governance.Report
	if err := json.Unmarshal(statement, &report); err != nil {
		return nil, fmt.Errorf("compliance: read the stored statement: %w", err)
	}
	out, err := s.o.Renderer.PDF(ctx, report, env)
	if err != nil {
		return nil, fmt.Errorf("compliance: render the report: %w", err)
	}
	return out, nil
}

// List returns the organisation's reports, newest first, after the cursor.
// The principal needs compliance.report and the licence's
// FeatureComplianceReports for reading.
func (s *Service) List(ctx context.Context, p auth.Principal, after storage.Cursor, limit int32) ([]Info, error) {
	if err := p.Authorize(auth.ComplianceReport); err != nil {
		return nil, fmt.Errorf("compliance: %w", err)
	}
	if err := edition.RequireRead(ctx, edition.FeatureComplianceReports); err != nil {
		return nil, fmt.Errorf("compliance: %w", err)
	}
	var out []Info
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListComplianceReports(ctx, dbgen.ListComplianceReportsParams{BeforeTime: after.BeforeTime(), BeforeID: after.BeforeID(), PageSize: limit})
		if err != nil {
			return fmt.Errorf("compliance: list the reports: %w", err)
		}
		for _, r := range rows {
			out = append(out, Info{
				ID: storage.ID(r.ID), From: r.PeriodFrom.Time, To: r.PeriodTo.Time, Changes: int(r.Changes), RequestedByID: r.RequestedByID,
				RequestedBy: r.RequestedBy, RequestedAt: r.RequestedAt.Time, Signed: r.KeyID != nil, KeyID: deref(r.KeyID), SignedAt: r.SignedAt.Time,
			})
		}
		return nil
	})
	return out, err
}

// row reads one report.
func (s *Service) row(ctx context.Context, p auth.Principal, id string) (dbgen.ComplianceReport, error) {
	uid, err := storage.UUID(id)
	if err != nil {
		return dbgen.ComplianceReport{}, plxerr.New(plxerr.InvalidFormat, "the report identifier is not valid")
	}
	var row dbgen.ComplianceReport
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		row, err = dbgen.New(tx).GetComplianceReport(ctx, uid)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return dbgen.ComplianceReport{}, plxerr.New(plxerr.ResourceNotFound, "no such compliance report")
	}
	if err != nil {
		return dbgen.ComplianceReport{}, fmt.Errorf("compliance: read the report: %w", err)
	}
	return row, nil
}

// info reads a report's description.
func (s *Service) info(ctx context.Context, p auth.Principal, id string) (Info, error) {
	row, err := s.row(ctx, p, id)
	if err != nil {
		return Info{}, err
	}
	return infoOf(row), nil
}

// infoOf describes a stored report.
func infoOf(r dbgen.ComplianceReport) Info {
	return Info{
		ID: storage.ID(r.ID), From: r.PeriodFrom.Time, To: r.PeriodTo.Time, Changes: int(r.Changes), RequestedByID: r.RequestedByID,
		RequestedBy: r.RequestedBy, RequestedAt: r.RequestedAt.Time, Signed: r.KeyID != nil, KeyID: deref(r.KeyID), SignedAt: r.SignedAt.Time,
	}
}

// deref returns the string a pointer holds, or "".
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// inOrg runs f in a transaction of the principal's organisation.
func (s *Service) inOrg(ctx context.Context, p auth.Principal, f func(context.Context, pgx.Tx) error) error {
	return s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID, UserID: p.UserID}, f) //nolint:wrapcheck // InTx wraps its own failures
}

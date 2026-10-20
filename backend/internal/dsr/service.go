// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package dsr

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// IDs generates identifiers.
type IDs interface{ New() (string, error) }

// LimitsFunc returns the limits in force for an organisation, read in the
// caller's transaction.
type LimitsFunc func(ctx context.Context, tx pgx.Tx, organizationID string) (limits.Set, error)

// Options configures a Service.
type Options struct {
	DB    *storage.DB
	Audit *audit.Log
	IDs   IDs
	// Limits reads an organisation's limits (dsr.exportBytes).
	Limits LimitsFunc
	// Now is the clock; nil uses time.Now.
	Now func() time.Time
}

// Service exports and erases the data of data subjects (SEC-162).
type Service struct {
	o   Options
	now func() time.Time
}

// NewService returns the service.
func NewService(o Options) (*Service, error) {
	if o.DB == nil || o.Audit == nil || o.IDs == nil || o.Limits == nil {
		return nil, errors.New("dsr: a database, an audit log, identifiers and limits are required")
	}
	now := o.Now
	if now == nil {
		now = time.Now
	}
	return &Service{o: o, now: now}, nil
}

// StoreCount is the number of rows a store held for a subject.
type StoreCount struct {
	Store string
	Rows  int64
}

// Request is a completed export or erasure. The subject appears only as
// its hash.
type Request struct {
	ID string
	// Action is "export" or "erase".
	Action string
	Kind   Kind
	// SubjectHash is the subject's hash in lower-case hex.
	SubjectHash  string
	Actor        string
	RowCount     int64
	ArchiveBytes int64
	RequestedAt  time.Time
	// Tombstones are the stores an erasure emptied.
	Tombstones []StoreCount
}

// Archive is the result of an export.
type Archive struct {
	// RequestID is the recorded request.
	RequestID string
	// JSON is the archive document.
	JSON []byte
	// Rows is the number of rows in it.
	Rows int64
}

// Erasure is the result of an erasure.
type Erasure struct {
	// RequestID is the recorded request; empty when there was nothing left
	// to erase.
	RequestID string
	// Erased lists the stores that lost rows in this call.
	Erased []StoreCount
	// AlreadyErased is true when an earlier erasure had removed everything.
	AlreadyErased bool
}

// Erase deletes everything held about the subject, one transaction per
// store in a fixed order, and leaves a tombstone per store that lost rows.
// A subject with nothing left but earlier tombstones is reported as already
// erased; a subject that never had data is PLX-8501. A failure part-way
// leaves the stores already erased erased: repeating the call finishes the
// rest.
func (s *Service) Erase(ctx context.Context, p auth.Principal, subject Subject) (Erasure, error) {
	if err := p.Authorize(auth.DSRManage); err != nil {
		return Erasure{}, fmt.Errorf("dsr: %w", err)
	}
	sub, err := resolve(p.OrganizationID, subject)
	if err != nil {
		return Erasure{}, err
	}
	requestID, err := s.o.IDs.New()
	if err != nil {
		return Erasure{}, fmt.Errorf("dsr: %w", err)
	}
	var out Erasure
	for _, x := range executors(sub.kind) {
		n, err := s.eraseStore(ctx, p, sub, requestID, x)
		if err != nil {
			return Erasure{}, err
		}
		if n > 0 {
			out.Erased = append(out.Erased, StoreCount{Store: x.store, Rows: n})
		}
	}
	if len(out.Erased) > 0 {
		out.RequestID = requestID
		return out, s.recordErasure(ctx, p, sub, requestID, out.Erased)
	}
	return out, s.reportNothingLeft(ctx, p, sub, &out)
}

// eraseStore deletes one store's rows of the subject in a transaction of
// their own and writes the tombstone in it, so a row is never gone without
// its tombstone.
func (s *Service) eraseStore(ctx context.Context, p auth.Principal, sub resolved, requestID string, x executor) (int64, error) {
	var n int64
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		var err error
		if n, err = x.erase(ctx, q, sub); err != nil {
			return fmt.Errorf("dsr: erase %s: %w", x.store, err)
		}
		if n == 0 {
			return nil
		}
		return q.InsertDsrTombstone(ctx, dbgen.InsertDsrTombstoneParams{
			OrganizationID: sub.org, RequestID: storage.MustUUID(requestID), SubjectKind: string(sub.kind),
			SubjectHash: sub.hash, Store: x.store, RowCount: n, ErasedAt: storage.Timestamp(s.now()),
		})
	})
	return n, err
}

// recordErasure writes the request and its audit entry.
func (s *Service) recordErasure(ctx context.Context, p auth.Principal, sub resolved, requestID string, erased []StoreCount) error {
	var rows int64
	for _, e := range erased {
		rows += e.Rows
	}
	return s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		return s.record(ctx, tx, p, sub, requestID, "erase", audit.DataSubjectErased, rows, 0)
	})
}

// reportNothingLeft settles an erasure that removed nothing: the subject is
// already erased when earlier tombstones exist, and unknown otherwise.
func (s *Service) reportNothingLeft(ctx context.Context, p auth.Principal, sub resolved, out *Erasure) error {
	return s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		n, err := dbgen.New(tx).CountDsrTombstones(ctx, dbgen.CountDsrTombstonesParams{OrganizationID: sub.org, SubjectHash: sub.hash})
		if err != nil {
			return fmt.Errorf("dsr: %w", err)
		}
		if n == 0 {
			return plxerr.New(plxerr.SubjectNotFound, "no data is held about this %s", sub.kind)
		}
		out.AlreadyErased = true
		return nil
	})
}

// Map returns the personal-data map to an administrator who may manage data
// subject requests.
func (*Service) Map(p auth.Principal) ([]Store, error) {
	if err := p.Authorize(auth.DSRManage); err != nil {
		return nil, fmt.Errorf("dsr: %w", err)
	}
	return personalDataMap(), nil
}

// Get returns one completed request of the caller's organisation.
func (s *Service) Get(ctx context.Context, p auth.Principal, id string) (Request, error) {
	if err := p.Authorize(auth.DSRManage); err != nil {
		return Request{}, fmt.Errorf("dsr: %w", err)
	}
	uid, err := storage.UUID(id)
	if err != nil {
		return Request{}, plxerr.New(plxerr.InvalidFormat, "the request identifier is not valid")
	}
	var out Request
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, err := q.GetDsrRequest(ctx, uid)
		if errors.Is(err, pgx.ErrNoRows) {
			return plxerr.New(plxerr.ResourceNotFound, "no such request")
		}
		if err != nil {
			return fmt.Errorf("dsr: %w", err)
		}
		out, err = s.withTombstones(ctx, q, row)
		return err
	})
	return out, err
}

// List returns the organisation's completed requests, newest first, before
// a cursor.
func (s *Service) List(ctx context.Context, p auth.Principal, before storage.Cursor, size int32) ([]Request, error) {
	if err := p.Authorize(auth.DSRManage); err != nil {
		return nil, fmt.Errorf("dsr: %w", err)
	}
	t := before.Time
	if t.IsZero() {
		t = s.now().Add(time.Hour)
	}
	var out []Request
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		rows, err := q.ListDsrRequests(ctx, dbgen.ListDsrRequestsParams{
			OrganizationID: storage.MustUUID(p.OrganizationID), BeforeTime: storage.Timestamp(t), BeforeID: maxUUID(before), PageSize: size,
		})
		if err != nil {
			return fmt.Errorf("dsr: list: %w", err)
		}
		for _, row := range rows {
			r, err := s.withTombstones(ctx, q, row)
			if err != nil {
				return err
			}
			out = append(out, r)
		}
		return nil
	})
	return out, err
}

// withTombstones converts a request row and loads the stores it emptied.
func (*Service) withTombstones(ctx context.Context, q *dbgen.Queries, row dbgen.DsrRequest) (Request, error) {
	r := Request{
		ID: storage.ID(row.ID), Action: row.Action, Kind: Kind(row.SubjectKind), SubjectHash: hex.EncodeToString(row.SubjectHash),
		Actor: row.Actor, RowCount: row.RowCount, ArchiveBytes: row.ArchiveBytes, RequestedAt: storage.Time(row.RequestedAt),
	}
	if row.Action != "erase" {
		return r, nil
	}
	ts, err := q.ListDsrRequestTombstones(ctx, row.ID)
	if err != nil {
		return Request{}, fmt.Errorf("dsr: tombstones: %w", err)
	}
	for _, t := range ts {
		r.Tombstones = append(r.Tombstones, StoreCount{Store: t.Store, Rows: t.RowCount})
	}
	return r, nil
}

// record writes a request and its audit entry in the caller's transaction.
// The entry names the subject by its hash.
func (s *Service) record(ctx context.Context, tx pgx.Tx, p auth.Principal, sub resolved, requestID, action string, event audit.Action, rows, archiveBytes int64) error {
	actor := p.Actor()
	err := dbgen.New(tx).InsertDsrRequest(ctx, dbgen.InsertDsrRequestParams{
		ID: storage.MustUUID(requestID), OrganizationID: sub.org, Action: action, SubjectKind: string(sub.kind),
		SubjectHash: sub.hash, Actor: actor.Kind + ":" + actor.ID, RowCount: rows, ArchiveBytes: archiveBytes,
		RequestedAt: storage.Timestamp(s.now()),
	})
	if err != nil {
		return fmt.Errorf("dsr: %w", err)
	}
	if _, err := s.o.Audit.Append(ctx, tx, audit.Entry{
		OrganizationID: p.OrganizationID, Actor: actor, Action: event,
		TargetKind: "data_subject_" + string(sub.kind), TargetID: hex.EncodeToString(sub.hash),
		Detail: fmt.Sprintf("request %s, %d rows", requestID, rows),
	}); err != nil {
		return fmt.Errorf("dsr: %w", err)
	}
	return nil
}

// inOrg runs f in a transaction bound to the principal's organisation.
func (s *Service) inOrg(ctx context.Context, p auth.Principal, f func(context.Context, pgx.Tx) error) error {
	return s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID, UserID: p.UserID}, f) //nolint:wrapcheck // InTx wraps its own failures
}

// maxUUID is the cursor's identifier, or the largest one for a first page
// ordered descending.
func maxUUID(c storage.Cursor) pgtype.UUID {
	if c.ID != "" {
		return storage.MustUUID(c.ID)
	}
	u := pgtype.UUID{Valid: true}
	for i := range u.Bytes {
		u.Bytes[i] = 0xff
	}
	return u
}

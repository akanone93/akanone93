// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auditexport

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/nightCode42/plux3/backend/ext/export"
	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/buildinfo"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// IDs generates identifiers.
type IDs interface{ New() (string, error) }

// LimitsFunc returns the limits in force for an organisation, read in the
// caller's transaction.
type LimitsFunc func(ctx context.Context, tx pgx.Tx, organizationID string) (limits.Set, error)

// Options configures a Service and an Exporter.
type Options struct {
	DB      *storage.DB
	Audit   *audit.Log
	IDs     IDs
	Crypter signing.Crypter
	// Limits reads an organisation's limits (audit.exportBatchEntries,
	// audit.exportDestinationsPerOrg).
	Limits LimitsFunc
	// Provider builds the sinks of the enterprise kinds; nil is the
	// community edition, which offers syslog only.
	Provider export.Provider
	// Hostname identifies this server in the messages it sends; empty uses
	// the machine's name.
	Hostname string
	// Now is the clock; nil uses time.Now.
	Now func() time.Time
}

// check reports a missing dependency.
func (o Options) check() error {
	if o.DB == nil || o.Audit == nil || o.IDs == nil || o.Crypter == nil || o.Limits == nil {
		return errors.New("auditexport: a database, an audit log, identifiers, envelope encryption and limits are required")
	}
	return nil
}

// opener builds the options' sink opener.
func (o Options) opener() opener {
	host := o.Hostname
	if host == "" {
		host, _ = os.Hostname()
	}
	now := o.Now
	if now == nil {
		now = time.Now
	}
	return opener{crypter: o.Crypter, provider: o.Provider, hostname: host, version: buildinfo.Get().Version, now: now}
}

// Service administers the export destinations of an organisation
// (SEC-143). Delivery is the Exporter's.
type Service struct {
	o    Options
	open opener
	now  func() time.Time
}

// NewService returns the service.
func NewService(o Options) (*Service, error) {
	if err := o.check(); err != nil {
		return nil, err
	}
	now := o.Now
	if now == nil {
		now = time.Now
	}
	return &Service{o: o, open: o.opener(), now: now}, nil
}

// Create registers a destination and audits it. The chain is delivered from
// the first entry unless the destination already has some. The syslog kind
// is the core's; the others are PLX-8600 without the SIEM licence.
func (s *Service) Create(ctx context.Context, p auth.Principal, n NewDestination) (Destination, error) {
	if err := p.Authorize(auth.AuditExport); err != nil {
		return Destination{}, fmt.Errorf("auditexport: %w", err)
	}
	if err := checkName(n.Name); err != nil {
		return Destination{}, err
	}
	if n.StartAfterSequence < 0 {
		return Destination{}, plxerr.New(plxerr.InvalidFormat, "the sequence to start after cannot be negative")
	}
	if err := s.open.validate(ctx, n.Kind, n.Address, n.AllowPrivate, n.CAPEM, n.Credentials); err != nil {
		return Destination{}, err
	}
	id, err := s.o.IDs.New()
	if err != nil {
		return Destination{}, fmt.Errorf("auditexport: %w", err)
	}
	sealed, err := s.seal(ctx, id, n.Credentials)
	if err != nil {
		return Destination{}, err
	}
	var out Destination
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		if err := s.checkQuota(ctx, tx, q, p.OrganizationID); err != nil {
			return err
		}
		cursorHash, err := startHash(ctx, q, p.OrganizationID, n.StartAfterSequence)
		if err != nil {
			return err
		}
		row, err := q.InsertAuditExportDestination(ctx, dbgen.InsertAuditExportDestinationParams{
			ID: storage.MustUUID(id), OrganizationID: storage.MustUUID(p.OrganizationID), Name: n.Name, Kind: n.Kind,
			Address: n.Address, Enabled: true, AllowPrivate: n.AllowPrivate, CaPem: n.CAPEM,
			CredentialsCiphertext: sealed.Ciphertext, CredentialsWrappedKey: sealed.WrappedKey,
			CredentialsKeyID: sealed.KeyID, CredentialsHint: n.Credentials.hint(),
			CursorSequence: n.StartAfterSequence, CursorHash: cursorHash,
		})
		if err != nil {
			return failure(err, "destination")
		}
		out = fromRow(row)
		return s.record(ctx, tx, p, audit.ExportDestinationCreated, out)
	})
	return out, err
}

// Get returns a destination and the newest sequence of its organisation's
// chain, so the lag is the difference to the cursor.
func (s *Service) Get(ctx context.Context, p auth.Principal, id string) (Destination, int64, error) {
	if err := p.Authorize(auth.AuditExport); err != nil {
		return Destination{}, 0, fmt.Errorf("auditexport: %w", err)
	}
	uid, err := storage.UUID(id)
	if err != nil {
		return Destination{}, 0, plxerr.New(plxerr.InvalidFormat, "the destination identifier is not valid")
	}
	var (
		out  Destination
		head int64
	)
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, err := q.GetAuditExportDestination(ctx, uid)
		if err != nil {
			return failure(err, "destination")
		}
		out = fromRow(row)
		head, err = chainHead(ctx, q, p.OrganizationID)
		return err
	})
	return out, head, err
}

// List lists the organisation's destinations by name, after a name.
func (s *Service) List(ctx context.Context, p auth.Principal, afterName string, size int32) ([]Destination, error) {
	if err := p.Authorize(auth.AuditExport); err != nil {
		return nil, fmt.Errorf("auditexport: %w", err)
	}
	var out []Destination
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListAuditExportDestinations(ctx, dbgen.ListAuditExportDestinationsParams{
			OrganizationID: storage.MustUUID(p.OrganizationID), AfterName: afterName, PageSize: size,
		})
		if err != nil {
			return fmt.Errorf("auditexport: list: %w", err)
		}
		for _, row := range rows {
			out = append(out, fromRow(row))
		}
		return nil
	})
	return out, err
}

// Update replaces a destination's address and settings and audits it. The
// cursor stays, the failure count clears and the next run tries at once.
func (s *Service) Update(ctx context.Context, p auth.Principal, c Change) (Destination, error) {
	if err := p.Authorize(auth.AuditExport); err != nil {
		return Destination{}, fmt.Errorf("auditexport: %w", err)
	}
	uid, err := storage.UUID(c.ID)
	if err != nil {
		return Destination{}, plxerr.New(plxerr.InvalidFormat, "the destination identifier is not valid")
	}
	var out Destination
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		current, err := q.GetAuditExportDestination(ctx, uid)
		if err != nil {
			return failure(err, "destination")
		}
		params, err := s.updateParams(ctx, current, c)
		if err != nil {
			return err
		}
		row, err := q.UpdateAuditExportDestination(ctx, params)
		if err != nil {
			return failure(err, "destination")
		}
		out = fromRow(row)
		return s.record(ctx, tx, p, audit.ExportDestinationUpdated, out)
	})
	return out, err
}

// updateParams validates a change against the stored destination and
// builds the statement's parameters.
func (s *Service) updateParams(ctx context.Context, current dbgen.AuditExportDestination, c Change) (dbgen.UpdateAuditExportDestinationParams, error) {
	params := dbgen.UpdateAuditExportDestinationParams{
		ID: current.ID, Address: c.Address, Enabled: c.Enabled, AllowPrivate: c.AllowPrivate, CaPem: c.CAPEM,
		CredentialsCiphertext: current.CredentialsCiphertext, CredentialsWrappedKey: current.CredentialsWrappedKey,
		CredentialsKeyID: current.CredentialsKeyID, CredentialsHint: current.CredentialsHint,
	}
	creds := c.Credentials
	switch {
	case c.ClearCredentials && !creds.IsZero():
		return params, plxerr.New(plxerr.InvalidFormat, "credentials cannot be both replaced and cleared")
	case c.ClearCredentials:
		params.CredentialsCiphertext, params.CredentialsWrappedKey, params.CredentialsKeyID, params.CredentialsHint = []byte{}, []byte{}, "", ""
	case !creds.IsZero():
		sealed, err := s.seal(ctx, storage.ID(current.ID), creds)
		if err != nil {
			return params, err
		}
		params.CredentialsCiphertext, params.CredentialsWrappedKey, params.CredentialsKeyID = sealed.Ciphertext, sealed.WrappedKey, sealed.KeyID
		params.CredentialsHint = creds.hint()
	default:
		kept, err := s.open.credentialsOf(ctx, current)
		if err != nil {
			return params, err
		}
		creds = kept
	}
	if err := s.open.validate(ctx, current.Kind, c.Address, c.AllowPrivate, c.CAPEM, creds); err != nil {
		return params, err
	}
	return params, nil
}

// Delete removes a destination and audits it.
func (s *Service) Delete(ctx context.Context, p auth.Principal, id string) error {
	if err := p.Authorize(auth.AuditExport); err != nil {
		return fmt.Errorf("auditexport: %w", err)
	}
	uid, err := storage.UUID(id)
	if err != nil {
		return plxerr.New(plxerr.InvalidFormat, "the destination identifier is not valid")
	}
	return s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, err := q.GetAuditExportDestination(ctx, uid)
		if err != nil {
			return failure(err, "destination")
		}
		if _, err := q.DeleteAuditExportDestination(ctx, uid); err != nil {
			return failure(err, "destination")
		}
		return s.record(ctx, tx, p, audit.ExportDestinationDeleted, fromRow(row))
	})
}

// seal encrypts credentials for storage; the zero value seals to nothing.
func (s *Service) seal(ctx context.Context, destinationID string, c Credentials) (signing.Sealed, error) {
	if c.IsZero() {
		return signing.Sealed{Ciphertext: []byte{}, WrappedKey: []byte{}}, nil
	}
	plain, err := marshalCredentials(c)
	if err != nil {
		return signing.Sealed{}, err
	}
	sealed, err := signing.Seal(ctx, s.o.Crypter, plain, bindingOf(destinationID))
	if err != nil {
		return signing.Sealed{}, fmt.Errorf("auditexport: seal the credentials: %w", err)
	}
	return sealed, nil
}

// checkQuota refuses a destination beyond the organisation's limit.
func (s *Service) checkQuota(ctx context.Context, tx pgx.Tx, q *dbgen.Queries, organizationID string) error {
	set, err := s.o.Limits(ctx, tx, organizationID)
	if err != nil {
		return fmt.Errorf("auditexport: read the limits: %w", err)
	}
	have, err := q.CountAuditExportDestinations(ctx, storage.MustUUID(organizationID))
	if err != nil {
		return fmt.Errorf("auditexport: count the destinations: %w", err)
	}
	if allowed := set.Get(limits.AuditExportDestinationsPerOrg); have >= allowed {
		return plxerr.New(plxerr.LimitExceeded, "the organisation has %d export destinations; audit.exportDestinationsPerOrg allows %d", have, allowed)
	}
	return nil
}

// startHash returns the hash of the entry a destination starts after; empty
// when it starts at the beginning.
func startHash(ctx context.Context, q *dbgen.Queries, organizationID string, sequence int64) (string, error) {
	if sequence == 0 {
		return "", nil
	}
	row, err := q.GetAuditEntry(ctx, dbgen.GetAuditEntryParams{OrganizationID: storage.MustUUID(organizationID), Sequence: sequence})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", plxerr.New(plxerr.InvalidFormat, "the organisation's audit chain has no entry %d to start after", sequence)
	}
	if err != nil {
		return "", fmt.Errorf("auditexport: read the entry to start after: %w", err)
	}
	return row.EntryHash, nil
}

// chainHead returns the newest sequence of an organisation's chain; zero
// for an empty one.
func chainHead(ctx context.Context, q *dbgen.Queries, organizationID string) (int64, error) {
	last, err := q.LastAuditEntry(ctx, storage.MustUUID(organizationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("auditexport: read the chain head: %w", err)
	}
	return last.Sequence, nil
}

// inOrg runs f in a transaction bound to the principal's organisation.
func (s *Service) inOrg(ctx context.Context, p auth.Principal, f func(context.Context, pgx.Tx) error) error {
	return s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID, UserID: p.UserID}, f) //nolint:wrapcheck // InTx wraps its own failures
}

// record audits a change. The detail names the kind and the host, never the
// address's path or query, which can carry a token, nor a credential
// (SEC-092).
func (s *Service) record(ctx context.Context, tx pgx.Tx, p auth.Principal, action audit.Action, d Destination) error {
	host, _ := hostOf(d.Address)
	detail := fmt.Sprintf("name %s, kind %s, host %s, enabled %t, private addresses allowed %t", d.Name, d.Kind, host, d.Enabled, d.AllowPrivate)
	if _, err := s.o.Audit.Append(ctx, tx, audit.Entry{
		OrganizationID: p.OrganizationID, Actor: p.Actor(), Action: action,
		TargetKind: "audit_export_destination", TargetID: d.ID, Detail: detail,
	}); err != nil {
		return fmt.Errorf("auditexport: %w", err)
	}
	return nil
}

// failure maps a storage error to a domain one.
func failure(err error, what string) error {
	var pg *pgconn.PgError
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return plxerr.New(plxerr.ResourceNotFound, "no such %s", what)
	case errors.As(err, &pg) && pg.Code == "23505":
		return plxerr.New(plxerr.ResourceExists, "a %s with this name already exists", what)
	}
	return fmt.Errorf("auditexport: %s: %w", what, err)
}

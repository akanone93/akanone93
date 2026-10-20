// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package reconstruct

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/release"
	"github.com/nightCode42/plux3/backend/internal/schema/jcs"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
	"github.com/nightCode42/plux3/backend/internal/storage/objects"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
)

// manifestDepth bounds the reconstruction manifest's JSON nesting.
const manifestDepth = 12

// SignJob asks the worker to sign a pending reconstruction release.
type SignJob struct {
	OrganizationID   string `json:"organizationId"`
	ReconstructionID string `json:"reconstructionId"`
}

// Kind names the job.
func (SignJob) Kind() string { return "reconstruction.sign" }

// Enqueuer adds a signing job in the caller's transaction.
type Enqueuer interface {
	Enqueue(ctx context.Context, tx pgx.Tx, job SignJob) error
}

// IDs generates identifiers.
type IDs interface{ New() (string, error) }

// Signer signs documents with an environment's manifest key; the worker
// role's *release.Service is one.
type Signer interface {
	SignDocument(ctx context.Context, q *dbgen.Queries, env dbgen.Environment, canonical []byte) ([]release.ManifestSignature, error)
}

// Locator says where a stored bundle is downloaded from; *release.Service
// is one.
type Locator interface {
	ObjectURL(ctx context.Context, kind objects.Kind, sum []byte) (string, error)
}

// LimitsFunc returns the limits in force for an app, read in the caller's
// transaction.
type LimitsFunc func(ctx context.Context, tx pgx.Tx, organizationID, appID string) (limits.Set, error)

// Options configures a Service.
type Options struct {
	DB      *storage.DB
	Audit   *audit.Log
	IDs     IDs
	Objects objects.Store
	// Limits reads reconstruction.retentionDays.
	Limits LimitsFunc
	// Jobs enqueues signing; the API role sets it.
	Jobs Enqueuer
	// Signer signs releases; only the worker role has one (SRV-052).
	Signer Signer
	// Locator gives bundle download URLs.
	Locator Locator
	// PublicBaseURL is where this server is reached, for the URI that opens
	// a release in the viewer.
	PublicBaseURL string
	// Now is the clock; nil uses time.Now.
	Now func() time.Time
}

// Service reconstructs what a device ran and showed (SEC-142).
type Service struct {
	o   Options
	now func() time.Time
}

// NewService returns the service.
func NewService(o Options) (*Service, error) {
	if o.DB == nil || o.Audit == nil || o.IDs == nil || o.Objects == nil || o.Limits == nil || o.Locator == nil {
		return nil, errors.New("reconstruct: a database, an audit log, identifiers, an object store, limits and a bundle locator are required")
	}
	now := o.Now
	if now == nil {
		now = time.Now
	}
	return &Service{o: o, now: now}, nil
}

// Reconstruction is a reconstruction release without its content.
type Reconstruction struct {
	ID, AppID, EnvironmentID, DeviceID string
	// UserRef is the pseudonymous user the request named, empty when it
	// named a device.
	UserRef         string
	Moment          time.Time
	ReleaseSequence int64
	// Signed is false until the worker has signed the release.
	Signed    bool
	CreatedAt time.Time
	SignedAt  time.Time
	CreatedBy audit.Actor
	// OpenURI opens the release in the viewer host.
	OpenURI string
}

// BundleLink is a bundle of a reconstruction release with its location.
type BundleLink struct {
	Bundle
	URL string
}

// Opened is a reconstruction release as the viewer reads it.
type Opened struct {
	Reconstruction
	// Manifest is the canonical JSON the signatures cover; nil while the
	// release is pending.
	Manifest   []byte
	Signatures []release.ManifestSignature
	Bundles    []BundleLink
}

// Reconstruct resolves the moment and records a reconstruction release for
// the worker to sign. It needs reconstruction.view on the app and is
// audited (SEC-142).
func (s *Service) Reconstruct(ctx context.Context, p auth.Principal, appID string, sub Subject, at time.Time) (Reconstruction, error) {
	if err := p.AuthorizeApp(auth.ReconstructionView, appID); err != nil {
		return Reconstruction{}, fmt.Errorf("reconstruct: %w", err)
	}
	app, err := storage.UUID(appID)
	if err != nil {
		return Reconstruction{}, plxerr.New(plxerr.InvalidFormat, "the app identifier is not valid")
	}
	var out Reconstruction
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		row, err := s.create(ctx, tx, p, app, sub, at)
		if err != nil {
			return err
		}
		out = s.fromRow(row)
		_, err = s.o.Audit.Append(ctx, tx, audit.Entry{
			OrganizationID: p.OrganizationID, Actor: p.Actor(), Action: audit.ReconstructionCreated,
			TargetKind: "reconstruction", TargetID: out.ID,
			Detail: fmt.Sprintf("app %s, %s subject, moment %s, release %d", appID, row.SubjectKind, at.UTC().Format(time.RFC3339), row.ReleaseSequence),
		})
		if err != nil {
			return fmt.Errorf("reconstruct: %w", err)
		}
		return nil
	})
	return out, err
}

// create resolves the moment and stores and enqueues the release.
func (s *Service) create(ctx context.Context, tx pgx.Tx, p auth.Principal, app pgtype.UUID, sub Subject, at time.Time) (dbgen.Reconstruction, error) {
	q := dbgen.New(tx)
	r, err := s.resolver(ctx, tx, q, p.OrganizationID, storage.ID(app))
	if err != nil {
		return dbgen.Reconstruction{}, err
	}
	res, err := r.resolve(ctx, app, sub, at)
	if err != nil {
		return dbgen.Reconstruction{}, err
	}
	ch, err := s.auditChannel(ctx, q, res.env)
	if err != nil {
		return dbgen.Reconstruction{}, err
	}
	id, err := s.o.IDs.New()
	if err != nil {
		return dbgen.Reconstruction{}, fmt.Errorf("reconstruct: %w", err)
	}
	res.manifest.ID, res.manifest.IssuedAt = id, r.now.UTC().Format(time.RFC3339)
	raw, err := json.Marshal(res.manifest)
	if err != nil {
		return dbgen.Reconstruction{}, fmt.Errorf("reconstruct: %w", err)
	}
	actor := p.Actor()
	row, err := q.InsertReconstruction(ctx, dbgen.InsertReconstructionParams{
		ID: storage.MustUUID(id), OrganizationID: res.env.OrganizationID, AppID: app, EnvironmentID: res.env.ID, ChannelID: ch.ID,
		DeviceID: res.device.ID, SubjectKind: sub.kind(), Moment: storage.Timestamp(at), ReleaseSequence: res.manifest.Release.Sequence,
		Manifest: raw, CreatedByKind: actor.Kind, CreatedByID: actor.ID, CreatedBy: actor.Display,
	})
	if err != nil {
		return dbgen.Reconstruction{}, fmt.Errorf("reconstruct: %w", err)
	}
	if s.o.Jobs == nil {
		return dbgen.Reconstruction{}, errors.New("reconstruct: this role cannot enqueue signing")
	}
	if err := s.o.Jobs.Enqueue(ctx, tx, SignJob{OrganizationID: p.OrganizationID, ReconstructionID: id}); err != nil {
		return dbgen.Reconstruction{}, fmt.Errorf("reconstruct: %w", err)
	}
	return row, nil
}

// resolver builds a resolver for an app with the retention in force.
func (s *Service) resolver(ctx context.Context, tx pgx.Tx, q *dbgen.Queries, org, appID string) (resolver, error) {
	set, err := s.o.Limits(ctx, tx, org, appID)
	if err != nil {
		return resolver{}, fmt.Errorf("reconstruct: read the limits: %w", err)
	}
	days := set.Get(limits.ReconstructionRetentionDays)
	now := s.now()
	return resolver{q: q, objects: s.o.Objects, now: now, cutoff: now.AddDate(0, 0, -int(days)), days: days}, nil
}

// auditChannel returns the environment's audit channel, creating it on
// first use. A channel of that key that a release points at is refused: it
// would serve a manifest.
func (s *Service) auditChannel(ctx context.Context, q *dbgen.Queries, env dbgen.Environment) (dbgen.Channel, error) {
	ch, err := q.GetChannel(ctx, dbgen.GetChannelParams{EnvironmentID: env.ID, Key: tenancy.AuditChannelKey})
	if errors.Is(err, pgx.ErrNoRows) {
		id, idErr := s.o.IDs.New()
		if idErr != nil {
			return dbgen.Channel{}, fmt.Errorf("reconstruct: %w", idErr)
		}
		ch, err = q.CreateChannel(ctx, dbgen.CreateChannelParams{
			ID: storage.MustUUID(id), OrganizationID: env.OrganizationID, EnvironmentID: env.ID, Key: tenancy.AuditChannelKey,
		})
	}
	if err != nil {
		return dbgen.Channel{}, fmt.Errorf("reconstruct: audit channel: %w", err)
	}
	if ch.ReleaseSequence != 0 {
		return dbgen.Channel{}, plxerr.New(plxerr.ResourceExists, "the %s channel of environment %s points at a release; it is reserved for reconstructions", tenancy.AuditChannelKey, env.Key)
	}
	return ch, nil
}

// Get opens a reconstruction release: its signed manifest, signatures and
// bundle locations. It needs reconstruction.view on the app and is audited
// every time (SEC-142).
func (s *Service) Get(ctx context.Context, p auth.Principal, id string) (Opened, error) {
	if !holdsOnSomeApp(p) {
		return Opened{}, fmt.Errorf("reconstruct: %w", p.Authorize(auth.ReconstructionView))
	}
	rid, err := storage.UUID(id)
	if err != nil {
		return Opened{}, plxerr.New(plxerr.InvalidFormat, "the reconstruction identifier is not valid")
	}
	var row dbgen.Reconstruction
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		if row, err = dbgen.New(tx).GetReconstruction(ctx, rid); errors.Is(err, pgx.ErrNoRows) {
			return plxerr.New(plxerr.ResourceNotFound, "no such reconstruction")
		} else if err != nil {
			return fmt.Errorf("reconstruct: %w", err)
		}
		if err := p.AuthorizeApp(auth.ReconstructionView, storage.ID(row.AppID)); err != nil {
			return fmt.Errorf("reconstruct: %w", err)
		}
		_, err = s.o.Audit.Append(ctx, tx, audit.Entry{
			OrganizationID: p.OrganizationID, Actor: p.Actor(), Action: audit.ReconstructionViewed,
			TargetKind: "reconstruction", TargetID: id, Detail: "app " + storage.ID(row.AppID),
		})
		if err != nil {
			return fmt.Errorf("reconstruct: %w", err)
		}
		return nil
	})
	if err != nil {
		return Opened{}, err
	}
	return s.open(ctx, row)
}

// holdsOnSomeApp reports whether the principal holds reconstruction.view
// across the organisation or on any app, so that a caller without it is
// refused before the lookup says whether a release exists.
func holdsOnSomeApp(p auth.Principal) bool {
	if p.Holds(auth.ReconstructionView) {
		return true
	}
	for _, perms := range p.AppPermissions {
		if slices.Contains(perms, auth.ReconstructionView) {
			return true
		}
	}
	return false
}

// open adds the signed content and bundle locations to a stored release.
func (s *Service) open(ctx context.Context, row dbgen.Reconstruction) (Opened, error) {
	out := Opened{Reconstruction: s.fromRow(row)}
	if row.Signed == nil {
		return out, nil
	}
	out.Manifest = row.Signed
	if err := json.Unmarshal(row.Signatures, &out.Signatures); err != nil {
		return Opened{}, fmt.Errorf("reconstruct: stored signatures: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(row.Manifest, &m); err != nil {
		return Opened{}, fmt.Errorf("reconstruct: stored manifest: %w", err)
	}
	for _, b := range m.Bundles {
		sum, ok := parseHash(b.Hash)
		if !ok {
			return Opened{}, fmt.Errorf("reconstruct: stored manifest: bad bundle hash %q", b.Hash)
		}
		url, err := s.o.Locator.ObjectURL(ctx, objects.KindBundle, sum)
		if err != nil {
			return Opened{}, fmt.Errorf("reconstruct: %w", err)
		}
		out.Bundles = append(out.Bundles, BundleLink{Bundle: b, URL: url})
	}
	return out, nil
}

// List lists an app's reconstruction releases, newest first.
func (s *Service) List(ctx context.Context, p auth.Principal, appID string, before storage.Cursor, size int32) ([]Reconstruction, error) {
	if err := p.AuthorizeApp(auth.ReconstructionView, appID); err != nil {
		return nil, fmt.Errorf("reconstruct: %w", err)
	}
	app, err := storage.UUID(appID)
	if err != nil {
		return nil, plxerr.New(plxerr.InvalidFormat, "the app identifier is not valid")
	}
	t := before.Time
	if t.IsZero() {
		t = s.now().Add(time.Hour)
	}
	var out []Reconstruction
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListReconstructions(ctx, dbgen.ListReconstructionsParams{
			AppID: app, BeforeTime: storage.Timestamp(t), BeforeID: maxUUID(before), PageSize: size,
		})
		if err != nil {
			return fmt.Errorf("reconstruct: %w", err)
		}
		for _, r := range rows {
			out = append(out, s.fromRow(r))
		}
		return nil
	})
	return out, err
}

// Sign signs a pending reconstruction release with the environment's
// manifest key and stores the signatures (SRV-052). Signing a signed release
// does nothing, so a retried job is harmless.
func (s *Service) Sign(ctx context.Context, job SignJob) error {
	if s.o.Signer == nil {
		return errors.New("reconstruct: this role cannot sign")
	}
	id, err := storage.UUID(job.ReconstructionID)
	if err != nil {
		return fmt.Errorf("reconstruct: %w", err)
	}
	err = s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: job.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, err := q.GetReconstructionForUpdate(ctx, id)
		if err != nil {
			return fmt.Errorf("reconstruct: %w", err)
		}
		if row.Signed != nil {
			return nil
		}
		env, err := q.GetEnvironment(ctx, row.EnvironmentID)
		if err != nil {
			return fmt.Errorf("reconstruct: %w", err)
		}
		canonical, err := jcs.Canonicalize(row.Manifest, manifestDepth)
		if err != nil {
			return fmt.Errorf("reconstruct: %w", err)
		}
		sigs, err := s.o.Signer.SignDocument(ctx, q, env, canonical)
		if err != nil {
			return fmt.Errorf("reconstruct: %w", err)
		}
		raw, err := json.Marshal(sigs)
		if err != nil {
			return fmt.Errorf("reconstruct: %w", err)
		}
		_, err = q.StoreReconstructionSignature(ctx, dbgen.StoreReconstructionSignatureParams{
			ID: id, Signed: canonical, Signatures: raw, SignedAt: storage.Timestamp(s.now()),
		})
		if err != nil {
			return fmt.Errorf("reconstruct: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("reconstruct: sign: %w", err)
	}
	return nil
}

// Purge deletes an organisation's device history and reconstruction releases
// past reconstruction.retentionDays, keeping each device's state at the start
// of the window. It returns how many rows it deleted.
func (s *Service) Purge(ctx context.Context, organizationID string) (int64, error) {
	var n int64
	err := s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: organizationID}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		r, err := s.resolver(ctx, tx, q, organizationID, "")
		if err != nil {
			return err
		}
		org, before := storage.MustUUID(organizationID), storage.Timestamp(r.cutoff)
		history, err := q.PurgeDeviceInstalls(ctx, dbgen.PurgeDeviceInstallsParams{OrganizationID: org, Before: before})
		if err != nil {
			return fmt.Errorf("reconstruct: %w", err)
		}
		releases, err := q.PurgeReconstructions(ctx, dbgen.PurgeReconstructionsParams{OrganizationID: org, Before: before})
		if err != nil {
			return fmt.Errorf("reconstruct: %w", err)
		}
		n = history + releases
		return nil
	})
	return n, err
}

// inOrg runs f in a transaction of the principal's organisation.
func (s *Service) inOrg(ctx context.Context, p auth.Principal, f func(context.Context, pgx.Tx) error) error {
	return s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID, UserID: p.UserID}, f) //nolint:wrapcheck // InTx wraps its own failures
}

// fromRow converts a stored release.
func (s *Service) fromRow(r dbgen.Reconstruction) Reconstruction {
	out := Reconstruction{
		ID: storage.ID(r.ID), AppID: storage.ID(r.AppID), EnvironmentID: storage.ID(r.EnvironmentID), DeviceID: storage.ID(r.DeviceID),
		Moment: storage.Time(r.Moment), ReleaseSequence: r.ReleaseSequence, Signed: r.Signed != nil,
		CreatedAt: storage.Time(r.CreatedAt), SignedAt: storage.Time(r.SignedAt),
		CreatedBy: audit.Actor{Kind: r.CreatedByKind, ID: r.CreatedByID, Display: r.CreatedBy},
		OpenURI:   openURI(s.o.PublicBaseURL, storage.ID(r.ID)),
	}
	if r.SubjectKind == KindUser {
		var m struct {
			Subject SubjectRef `json:"subject"`
		}
		if json.Unmarshal(r.Manifest, &m) == nil {
			out.UserRef = m.Subject.UserRef
		}
	}
	return out
}

// maxUUID is the cursor's identifier, or the largest one for a first page
// ordered descending.
func maxUUID(c storage.Cursor) pgtype.UUID {
	if c.ID == "" {
		u := pgtype.UUID{Valid: true}
		for i := range u.Bytes {
			u.Bytes[i] = 0xff
		}
		return u
	}
	return storage.MustUUID(c.ID)
}

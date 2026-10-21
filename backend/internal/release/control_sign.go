// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/controldoc"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
	"github.com/nightCode42/plux3/backend/internal/updatemeta"
)

// ControlJob asks the worker to sign a channel's newest control document
// (SYN-060). Only the worker holds the control key (SRV-052).
type ControlJob struct {
	OrganizationID string `json:"organizationId"`
	ChannelID      string `json:"channelId"`
}

// Kind names the job.
func (ControlJob) Kind() string { return "control.sign" }

// ControlNotice tells a ControlNotifier which channel has a new signed
// control document.
type ControlNotice struct {
	OrganizationID, AppID, EnvironmentID, ChannelID string
	// Version is the signed version.
	Version int64
}

// ControlNotifier is told, in the transaction that stores a signature,
// that devices of a channel should look at their control document: the
// silent push of Q22. A nil notifier does nothing.
type ControlNotifier interface {
	NotifyControl(ctx context.Context, tx pgx.Tx, n ControlNotice) error
}

// systemActor is who the sweep acts as when it writes a document nobody
// asked for.
var systemActor = audit.Actor{Kind: "system", ID: "control-sweep", Display: "Plux"}

// controlRefreshDivisor makes the sweep sign a fresh document when no more
// than this fraction of the lifetime remains; the sweep runs hourly, so a
// lifetime of at least two hours is always renewed in time.
const controlRefreshDivisor = 2

// MinControlExpiry is the shortest lifetime the sweep can keep ahead of
// expiry; control.expiry below it is refused when the service starts.
const MinControlExpiry = 2 * time.Hour

// enqueueControl asks the worker to sign the channel's newest document.
func (s *Service) enqueueControl(ctx context.Context, tx pgx.Tx, ch dbgen.Channel) error {
	if s.o.Jobs == nil {
		return nil
	}
	if err := s.o.Jobs.Enqueue(ctx, tx, ControlJob{OrganizationID: storage.ID(ch.OrganizationID), ChannelID: storage.ID(ch.ID)}); err != nil {
		return fmt.Errorf("release: %w", err)
	}
	return nil
}

// controlLifetime is how long a control document is valid after signing.
func (s *Service) controlLifetime() time.Duration {
	return time.Duration(s.o.Limits.Get(limits.ControlExpiry)) * time.Millisecond
}

// SignControl signs the newest control document of a channel if it is not
// signed yet. It is idempotent: a job that finds the document signed does
// nothing, so a retry or a duplicate is harmless.
func (s *Service) SignControl(ctx context.Context, job ControlJob) error {
	if s.o.Signer == nil {
		return errors.New("release: this role cannot sign")
	}
	id, err := parseID(job.ChannelID, "channel")
	if err != nil {
		return err
	}
	err = s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: job.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		ch, err := q.GetChannelByIDForUpdate(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // the channel is gone, and its documents with it
		}
		if err != nil {
			return failure(err, "channel")
		}
		env, err := q.GetEnvironment(ctx, ch.EnvironmentID)
		if err != nil {
			return failure(err, "environment")
		}
		_, err = s.signNewest(ctx, tx, env, ch)
		return err
	})
	if err != nil {
		return fmt.Errorf("release: sign the control document: %w", err)
	}
	return nil
}

// signNewest signs the channel's newest document when it is unsigned and
// the environment's root delegates the control role. It returns whether it
// signed. The caller holds the channel's lock.
func (s *Service) signNewest(ctx context.Context, tx pgx.Tx, env dbgen.Environment, ch dbgen.Channel) (bool, error) {
	q := dbgen.New(tx)
	row, err := q.GetNewestControlDocument(ctx, ch.ID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && row.Signed != nil) {
		return false, nil
	}
	if err != nil {
		return false, failure(err, "control document")
	}
	root, enrolled, err := currentRoot(ctx, q, env)
	if err != nil || !enrolled {
		return false, err
	}
	if _, delegated := root.Roles.Named(updatemeta.RoleControl); !delegated {
		return false, nil // the root does not yet delegate the role; the sweep retries
	}
	refs, err := s.checkRoot(ctx, q, env, root, s.now())
	if err != nil {
		return false, err
	}
	if err := s.signControlRow(ctx, tx, env, ch, row, refs.control); err != nil {
		return false, err
	}
	return true, nil
}

// signControlRow encodes, signs and stores one row, prunes old history and
// tells the notifier.
func (s *Service) signControlRow(ctx context.Context, tx pgx.Tx, env dbgen.Environment, ch dbgen.Channel, row dbgen.ControlDocument, keyRef string) error {
	q := dbgen.New(tx)
	content, err := contentOf(row)
	if err != nil {
		return err
	}
	at := s.now().UTC().Truncate(time.Second)
	doc := controldoc.Document{
		Version: uint64(row.Version), AppID: storage.ID(env.AppID), Environment: env.Key, Channel: ch.Key, //nolint:gosec // versions are positive
		IssuedAt: at, ExpiresAt: at.Add(s.controlLifetime()),
		KillSwitches: content.Switches, MandatoryUpdate: content.Mandatory, ForcedRollback: content.Rollback,
		Flags: content.Flags, Experiments: content.Experiments, Holdout: content.Holdout,
	}
	encoded, err := controldoc.Encode(doc)
	if err != nil {
		return fmt.Errorf("release: %w", err)
	}
	file, _, err := updatemeta.Sign(ctx, s.o.Signer, keyRef, updatemeta.NewControl(doc, encoded))
	if err != nil {
		return fmt.Errorf("release: %w", err)
	}
	sum := sha256.Sum256(file)
	if _, err := q.StoreControlSignature(ctx, dbgen.StoreControlSignatureParams{
		ChannelID: ch.ID, Version: row.Version, Signed: file, SignedSha256: sum[:],
		IssuedAt: storage.Timestamp(doc.IssuedAt), ExpiresAt: storage.Timestamp(doc.ExpiresAt),
	}); err != nil {
		return failure(err, "control document")
	}
	if _, err := q.PruneControlDocuments(ctx, dbgen.PruneControlDocumentsParams{ChannelID: ch.ID, Keep: controlHistory}); err != nil {
		return failure(err, "control document")
	}
	if s.o.ControlNotifier == nil {
		return nil
	}
	err = s.o.ControlNotifier.NotifyControl(ctx, tx, ControlNotice{
		OrganizationID: storage.ID(ch.OrganizationID), AppID: storage.ID(env.AppID), EnvironmentID: storage.ID(env.ID),
		ChannelID: storage.ID(ch.ID), Version: row.Version,
	})
	if err != nil {
		return fmt.Errorf("release.signControlRow: notify: %w", err)
	}
	return nil
}

// RefreshControl keeps the control documents of an organisation's
// enrolled environments current: it writes a first, empty document for a
// channel that has none, signs a document that still waits for its
// signature, for example because the root delegated the role only later,
// and signs a fresh version of one that has half its lifetime or less to
// run, so that a device always finds one that has not expired. It returns
// how many documents it signed.
func (s *Service) RefreshControl(ctx context.Context, org string) (int, error) {
	if s.o.Signer == nil {
		return 0, nil
	}
	var envs []dbgen.Environment
	err := s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: org}, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		envs, err = dbgen.New(tx).ListEnvironmentsWithRoot(ctx)
		return err //nolint:wrapcheck // one statement
	})
	if err != nil {
		return 0, fmt.Errorf("release: %w", err)
	}
	signed := 0
	var errs []error
	for _, env := range envs {
		n, err := s.refreshControlOf(ctx, org, env.ID)
		signed += n
		errs = append(errs, err)
	}
	return signed, errors.Join(errs...)
}

// refreshControlOf refreshes the channels of one environment, each in a
// transaction of its own.
func (s *Service) refreshControlOf(ctx context.Context, org string, envID pgtype.UUID) (int, error) {
	var channels []dbgen.Channel
	err := s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: org}, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		channels, err = dbgen.New(tx).ListChannelsOfEnvironment(ctx, envID)
		return err //nolint:wrapcheck // one statement
	})
	if err != nil {
		return 0, fmt.Errorf("release: %w", err)
	}
	signed := 0
	var errs []error
	for _, c := range channels {
		ok, err := s.refreshChannelControl(ctx, org, c.ID)
		if ok {
			signed++
		}
		errs = append(errs, err)
	}
	return signed, errors.Join(errs...)
}

// refreshChannelControl brings one channel's control document up to date.
func (s *Service) refreshChannelControl(ctx context.Context, org string, id pgtype.UUID) (bool, error) {
	signed := false
	err := s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: org}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		ch, err := q.GetChannelByIDForUpdate(ctx, id)
		if err != nil {
			return failure(err, "channel")
		}
		env, err := q.GetEnvironment(ctx, ch.EnvironmentID)
		if err != nil {
			return failure(err, "environment")
		}
		if !s.delegatesControl(ctx, q, env) {
			return nil
		}
		if err := s.ensureFreshDocument(ctx, q, ch); err != nil {
			return err
		}
		signed, err = s.signNewest(ctx, tx, env, ch)
		return err
	})
	if err != nil {
		return signed, fmt.Errorf("release: refresh a control document: %w", err)
	}
	return signed, nil
}

// delegatesControl reports whether the environment has a root that
// delegates the control role.
func (*Service) delegatesControl(ctx context.Context, q *dbgen.Queries, env dbgen.Environment) bool {
	root, enrolled, err := currentRoot(ctx, q, env)
	if err != nil || !enrolled {
		return false
	}
	_, delegated := root.Roles.Named(updatemeta.RoleControl)
	return delegated
}

// ensureFreshDocument writes an unsigned version when the channel has no
// document, or when the signed newest one is near its expiry: the same
// content under the next version.
func (s *Service) ensureFreshDocument(ctx context.Context, q *dbgen.Queries, ch dbgen.Channel) error {
	row, err := q.GetNewestControlDocument(ctx, ch.ID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return s.insertFirstControl(ctx, q, ch)
	case err != nil:
		return failure(err, "control document")
	case row.Signed == nil:
		return nil
	case row.ExpiresAt.Time.Sub(s.now()) > s.controlLifetime()/controlRefreshDivisor:
		return nil
	}
	content, err := contentOf(row)
	if err != nil {
		return err
	}
	return s.insertControl(ctx, q, ch, row.Version+1, content, systemActor)
}

// insertFirstControl writes the first version of a channel's document. It
// starts from the flags, experiments and holdout the environment's other
// channels carry, so that a channel made later is not without them.
func (s *Service) insertFirstControl(ctx context.Context, q *dbgen.Queries, ch dbgen.Channel) error {
	var content controlContent
	sibling, err := q.GetEnvironmentExperimentation(ctx, ch.EnvironmentID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return failure(err, "control document")
	default:
		if content.Experimentation, err = experimentationOf(sibling.Flags, sibling.Experiments, sibling.Holdout); err != nil {
			return err
		}
	}
	return s.insertControl(ctx, q, ch, 1, content, systemActor)
}

// ControlRequest is a device's request for its channel's control document.
type ControlRequest struct {
	OrganizationID, EnvironmentID, Channel string
	// IfNoneMatch is the ETag of the document the device holds.
	IfNoneMatch string
}

// ServedControl is the answer to a ControlRequest.
type ServedControl struct {
	// NotModified is set when the device already holds the newest signed
	// document; Signed is then empty.
	NotModified bool
	ETag        string
	Version     int64
	ExpiresAt   time.Time
	// Signed is the signed file a device verifies (SEC-052).
	Signed []byte
}

// GetControlDocument serves the newest signed control document of a
// device's channel. The document is public to its channel's devices and
// verified by each against its trusted root, so nothing here depends on
// who asks beyond the DPoP check at the edge.
func (s *Service) GetControlDocument(ctx context.Context, req ControlRequest) (ServedControl, error) {
	envID, err := parseID(req.EnvironmentID, "environment")
	if err != nil {
		return ServedControl{}, err
	}
	var out ServedControl
	err = s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: req.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		ch, err := q.GetChannel(ctx, dbgen.GetChannelParams{EnvironmentID: envID, Key: channelOrDefault(req.Channel)})
		if err != nil {
			return failure(err, "channel")
		}
		row, err := q.GetNewestSignedControlDocument(ctx, ch.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			return plxerr.New(plxerr.ResourceNotFound, "no control document is signed for this channel yet")
		}
		if err != nil {
			return failure(err, "control document")
		}
		out = ServedControl{ETag: `"` + hex.EncodeToString(row.SignedSha256) + `"`, Version: row.Version, ExpiresAt: storage.Time(row.ExpiresAt)}
		if etagMatches(req.IfNoneMatch, out.ETag) {
			out.NotModified = true
			return nil
		}
		out.Signed = row.Signed
		return nil
	})
	return out, err //nolint:wrapcheck // InTx wraps its own failures
}

// etagMatches implements If-None-Match for one strong ETag: a list of
// entity tags, any of which may be weak, or "*".
func etagMatches(header, etag string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimPrefix(strings.TrimSpace(candidate), "W/")
		if candidate == "*" || candidate == etag {
			return true
		}
	}
	return false
}

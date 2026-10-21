// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// Targeting is what a device reports about itself so that a rollout can
// decide which release it receives (REL-051). The host build and the channel
// are the server's own knowledge and are not part of it.
type Targeting struct {
	OS, OSVersion, RuntimeVersion, DeviceClass, Locale, Region string
	// UserHash is the hashed pseudonymous user identifier, empty if the host
	// app set no user.
	UserHash string
	// UserAttributes maps an attribute of the user context to the hash of its
	// value; plain values never reach the server.
	UserAttributes map[string]string
}

// RolloutSelector decides, when a device fetches its manifest, whether it is
// inside a rollout of its channel (REL-050, REL-051, ADR-0078). *rollout.Service
// is one, behind a small adapter.
type RolloutSelector interface {
	// SelectRollout returns the identifier of the rollout whose candidate
	// release the device is to receive, or "" when it gets the channel's own.
	SelectRollout(ctx context.Context, r ManifestRequest) (string, error)
}

// UseRollouts sets the selector consulted for every manifest request. It is
// called once while the server is assembled, before it serves, because the
// rollout service is itself built on this one.
func (s *Service) UseRollouts(sel RolloutSelector) { s.rollouts = sel }

// manifestFor returns the manifest a device receives: the candidate's of the
// rollout the device is inside, if any, else the channel's own. A device whose
// host build cannot run the candidate, and a rollout whose manifest is not yet
// signed, fall back to the channel's, which is always safe (REL-080).
func (s *Service) manifestFor(ctx context.Context, r ManifestRequest) (cachedManifest, error) {
	r.Channel = channelOrDefault(r.Channel)
	c, ok, err := s.rolloutCandidate(ctx, r)
	if err != nil {
		return cachedManifest{}, err
	}
	if ok {
		return c, nil
	}
	return s.latestManifest(ctx, r.OrganizationID, r.EnvironmentID, r.Channel, r.HostBuild)
}

// rolloutCandidate returns the manifest of the rollout the device is
// inside, and whether the device may take it: there is a signed manifest and
// the device's host build can run it.
func (s *Service) rolloutCandidate(ctx context.Context, r ManifestRequest) (cachedManifest, bool, error) {
	if s.rollouts == nil {
		return cachedManifest{}, false, nil
	}
	id, err := s.rollouts.SelectRollout(ctx, r)
	if err != nil {
		return cachedManifest{}, false, fmt.Errorf("release: select a rollout: %w", err)
	}
	if id == "" {
		return cachedManifest{}, false, nil
	}
	c, found, err := s.rolloutManifest(ctx, r, id)
	if err != nil {
		return cachedManifest{}, false, err
	}
	return c, found && !c.incompatible[r.HostBuild], nil
}

// rolloutManifest returns the newest manifest signed for a rollout, and
// whether there is one.
func (s *Service) rolloutManifest(ctx context.Context, r ManifestRequest, rolloutID string) (cachedManifest, bool, error) {
	id, err := storage.UUID(rolloutID)
	if err != nil {
		return cachedManifest{}, false, fmt.Errorf("release: %w", err)
	}
	c, err := s.cachedManifest(ctx, r.OrganizationID, "rollout/"+rolloutID, func(ctx context.Context, q *dbgen.Queries) (manifestRead, error) {
		row, err := q.LatestRolloutManifest(ctx, id)
		if err != nil {
			return manifestRead{}, err //nolint:wrapcheck // wrapped by the caller
		}
		return s.readRolloutManifest(ctx, q, row)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return cachedManifest{}, false, nil
	}
	return c, err == nil, err
}

// readRolloutManifest completes a rollout's manifest row with the metadata
// versions that vouch for it and the host builds that cannot run it.
func (*Service) readRolloutManifest(ctx context.Context, q *dbgen.Queries, row dbgen.Manifest) (manifestRead, error) {
	ch, err := q.GetChannelByID(ctx, row.ChannelID)
	if err != nil {
		return manifestRead{}, failure(err, "channel")
	}
	env, err := q.GetEnvironment(ctx, ch.EnvironmentID)
	if err != nil {
		return manifestRead{}, failure(err, "environment")
	}
	ref, err := metadataRef(ctx, q, env.ID)
	if err != nil {
		return manifestRead{}, err
	}
	incompatible, err := buildsThatCannotRun(ctx, q, env.AppID, row.ReleaseSequence)
	if err != nil {
		return manifestRead{}, err
	}
	return manifestRead{row: row, ref: ref, incompatible: incompatible}, nil
}

// buildsThatCannotRun returns the host builds of an app whose native catalogue
// cannot run a release.
func buildsThatCannotRun(ctx context.Context, q *dbgen.Queries, app pgtype.UUID, sequence int64) (map[string]bool, error) {
	rel, err := q.GetRelease(ctx, dbgen.GetReleaseParams{AppID: app, Sequence: sequence})
	if err != nil {
		return nil, failure(err, "release")
	}
	uses, err := nativeUses(rel.NativeUses)
	if err != nil {
		return nil, err
	}
	builds, err := catalogues(ctx, q, app)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, b := range builds {
		if !compiler.Compatible(uses, b.catalogue) {
			out[b.build] = true
		}
	}
	return out, nil
}

// signRollouts signs, beside a channel's own manifest, the manifest of the
// candidate of every rollout of the channel that serves one, at the same
// targets version.
func (s *Service) signRollouts(ctx context.Context, q *dbgen.Queries, channel dbgen.Channel, env dbgen.Environment, when signAt) error {
	rollouts, err := q.ListChannelServingRollouts(ctx, channel.ID)
	if err != nil {
		return failure(err, "rollout")
	}
	for _, r := range rollouts {
		rel, err := q.GetRelease(ctx, dbgen.GetReleaseParams{AppID: env.AppID, Sequence: r.CandidateSequence})
		if err != nil {
			return failure(err, "release")
		}
		if _, err := s.signRelease(ctx, q, channel, env, rel, when, "", pgtype.UUID{}, r.ID); err != nil {
			return err
		}
	}
	return nil
}

// RolloutChannel is the channel a rollout is to run on, as it stands.
type RolloutChannel struct {
	ChannelID      string
	EnvironmentID  string
	EnvironmentKey string
	// Current is the sequence of the release the channel points at.
	Current int64
}

// PrepareRollout checks that the principal may roll a release out on a
// channel and that the release may go there: it passes the gates of a
// promotion, and the channel has a release it is newer than, which is what
// devices outside the rollout keep (REL-050).
func (s *Service) PrepareRollout(ctx context.Context, p auth.Principal, appID, envID, channelKey string, candidate int64) (RolloutChannel, error) {
	if err := authorizeSomeEnvironment(p, auth.RolloutManage, appID); err != nil {
		return RolloutChannel{}, err
	}
	var out RolloutChannel
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		_, env, err := s.environment(ctx, q, appID, envID)
		if err != nil {
			return err
		}
		if err := authorizeEnvironment(p, auth.RolloutManage, appID, env.Key); err != nil {
			return err
		}
		row, vs, err := s.releaseRow(ctx, q, appID, candidate)
		if err != nil {
			return err
		}
		if err := requirePromoted(ctx, q, env, row.ID); err != nil {
			return err
		}
		if err := s.gatePromotion(ctx, appID, env, row, vs); err != nil {
			return err
		}
		ch, err := q.GetChannel(ctx, dbgen.GetChannelParams{EnvironmentID: env.ID, Key: channelOrDefault(channelKey)})
		if err != nil {
			return failure(err, "channel")
		}
		if ch.ReleaseSequence == 0 || candidate <= ch.ReleaseSequence {
			return plxerr.New(plxerr.OutOfRange, "release %d is not newer than release %d, which the channel serves; a rollout replaces a release with a newer one", candidate, ch.ReleaseSequence)
		}
		out = RolloutChannel{ChannelID: storage.ID(ch.ID), EnvironmentID: storage.ID(env.ID), EnvironmentKey: env.Key, Current: ch.ReleaseSequence}
		return nil
	})
	return out, err
}

// EnqueueChannelManifest asks the worker to sign the manifests of one channel,
// in the transaction that changed what they say.
func (s *Service) EnqueueChannelManifest(ctx context.Context, tx pgx.Tx, organizationID, channelID string) error {
	if s.o.Jobs == nil {
		return nil
	}
	if _, err := storage.UUID(channelID); err != nil {
		return fmt.Errorf("release: %w", err)
	}
	if err := s.o.Jobs.Enqueue(ctx, tx, ManifestJob{OrganizationID: organizationID, ChannelID: channelID}); err != nil {
		return fmt.Errorf("release: %w", err)
	}
	return nil
}

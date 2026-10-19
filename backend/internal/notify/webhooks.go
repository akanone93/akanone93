// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package notify

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/notify/webhook"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// Secret owners.
const (
	// OwnerEndpoint owns the secrets of an outbound webhook endpoint.
	OwnerEndpoint = "endpoint"
	// OwnerHook owns the secrets of an inbound function hook (FN-025).
	OwnerHook = "hook"
)

// DefaultOverlap is how long the previous secret stays valid after a
// rotation, unless the caller says otherwise.
const DefaultOverlap = 24 * time.Hour

// MaxOverlap bounds the overlap.
const MaxOverlap = 7 * 24 * time.Hour

// Endpoint is an outbound webhook (SRV-063).
type Endpoint struct {
	ID         string
	Name       string
	URL        string
	EventTypes []EventType
	Enabled    bool
	CreatedAt  time.Time
	// SecretHint is the end of the newest secret.
	SecretHint string
}

// EndpointSpec is a webhook to create.
type EndpointSpec struct {
	Name string
	URL  string
	// EventTypes narrows the events sent; empty is all.
	EventTypes []EventType
}

func secretBinding(owner, id string) []byte { return []byte("webhook-secret:" + owner + ":" + id) }

// SealSecret stores a new signing secret for an endpoint or a hook, in the
// caller's transaction. Used by this package and by function hooks.
func (s *Service) SealSecret(ctx context.Context, q *dbgen.Queries, org pgtype.UUID, owner string, ownerID pgtype.UUID, secret string) (hint string, err error) {
	sealed, err := signing.Seal(ctx, s.o.Crypter, []byte(secret), secretBinding(owner, storage.ID(ownerID)))
	if err != nil {
		return "", fmt.Errorf("notify: seal a webhook secret: %w", err)
	}
	id, err := s.newID()
	if err != nil {
		return "", err
	}
	if err := q.InsertWebhookSecret(ctx, dbgen.InsertWebhookSecretParams{
		ID: storage.MustUUID(id), OrganizationID: org, OwnerKind: owner, OwnerID: ownerID,
		Ciphertext: sealed.Ciphertext, WrappedKey: sealed.WrappedKey, KeyID: sealed.KeyID, Hint: sealed.Hint,
	}); err != nil {
		return "", failure(err, "webhook secret")
	}
	return sealed.Hint, nil
}

// ActiveSecrets opens the secrets of an owner that are valid at now: the
// newest, and any older one still inside its overlap. Newest first.
func (s *Service) ActiveSecrets(ctx context.Context, q *dbgen.Queries, owner string, ownerID pgtype.UUID, now time.Time) ([]string, error) {
	rows, err := q.ListActiveWebhookSecrets(ctx, dbgen.ListActiveWebhookSecretsParams{OwnerKind: owner, OwnerID: ownerID, ExpiresAt: storage.Timestamp(now)})
	if err != nil {
		return nil, failure(err, "webhook secrets")
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		v, err := signing.Open(ctx, s.o.Crypter, signing.Sealed{Ciphertext: r.Ciphertext, WrappedKey: r.WrappedKey, KeyID: r.KeyID},
			secretBinding(owner, storage.ID(ownerID)))
		if err != nil {
			return nil, fmt.Errorf("notify: open a webhook secret: %w", err)
		}
		out = append(out, string(v))
	}
	return out, nil
}

// RotateSecret adds a new secret to an owner and ends the validity of the
// older ones after the overlap, so a receiver can switch without missing a
// message (SRV-063). It returns the new secret, the only time it is shown.
func (s *Service) RotateSecret(ctx context.Context, q *dbgen.Queries, org pgtype.UUID, owner string, ownerID pgtype.UUID, overlap time.Duration) (string, error) {
	if overlap < 0 || overlap > MaxOverlap {
		return "", plxerr.New(plxerr.InvalidFormat, "the overlap is from 0 to %s", MaxOverlap)
	}
	secret, err := webhook.NewSecret()
	if err != nil {
		return "", err //nolint:wrapcheck // a domain error
	}
	if err := q.ExpireWebhookSecrets(ctx, dbgen.ExpireWebhookSecretsParams{OwnerKind: owner, OwnerID: ownerID, ExpiresAt: storage.Timestamp(s.now().Add(overlap))}); err != nil {
		return "", failure(err, "webhook secrets")
	}
	if _, err := s.SealSecret(ctx, q, org, owner, ownerID, secret); err != nil {
		return "", err
	}
	return secret, nil
}

func endpointOf(r dbgen.WebhookEndpoint, hint string) Endpoint {
	types := make([]EventType, len(r.EventTypes))
	for i, t := range r.EventTypes {
		types[i] = EventType(t)
	}
	return Endpoint{ID: storage.ID(r.ID), Name: r.Name, URL: r.Url, EventTypes: types, Enabled: r.Enabled, CreatedAt: storage.Time(r.CreatedAt), SecretHint: hint}
}

// CreateEndpoint configures an outbound webhook and returns it with its
// secret, which is shown once (SRV-063). It needs webhooks.manage; the
// number of endpoints is bounded by webhooks.perOrg.
func (s *Service) CreateEndpoint(ctx context.Context, p auth.Principal, spec EndpointSpec) (Endpoint, string, error) {
	if err := authorize(p, auth.WebhooksManage); err != nil {
		return Endpoint{}, "", err
	}
	if err := checkName(spec.Name); err != nil {
		return Endpoint{}, "", err
	}
	if err := s.checkWebhookURL(spec.URL); err != nil {
		return Endpoint{}, "", plxerr.New(plxerr.ChannelMisconfigured, "the webhook URL: %v", err)
	}
	types := make([]string, 0, len(spec.EventTypes))
	for _, t := range spec.EventTypes {
		if !Known(t) {
			return Endpoint{}, "", plxerr.New(plxerr.InvalidEnumValue, "%q is not an event type", t)
		}
		types = append(types, string(t))
	}
	lim, err := s.o.Limits(ctx, p.OrganizationID)
	if err != nil {
		return Endpoint{}, "", err
	}
	id, err := s.newID()
	if err != nil {
		return Endpoint{}, "", err
	}
	secret, err := webhook.NewSecret()
	if err != nil {
		return Endpoint{}, "", err //nolint:wrapcheck // a domain error
	}
	var out Endpoint
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		n, err := q.CountWebhookEndpoints(ctx)
		if err != nil {
			return failure(err, "webhooks")
		}
		if most := lim.Get(limits.WebhooksPerOrg); n >= most {
			return plxerr.New(plxerr.LimitExceeded, "an organisation has at most webhooks.perOrg = %d webhooks", most)
		}
		org := storage.MustUUID(p.OrganizationID)
		row, err := q.InsertWebhookEndpoint(ctx, dbgen.InsertWebhookEndpointParams{
			ID: storage.MustUUID(id), OrganizationID: org, Name: spec.Name, Url: spec.URL, EventTypes: types, CreatedBy: actorUser(p),
		})
		if err != nil {
			return failure(err, "webhook")
		}
		hint, err := s.SealSecret(ctx, q, org, OwnerEndpoint, row.ID, secret)
		if err != nil {
			return err
		}
		out = endpointOf(row, hint)
		return s.record(ctx, tx, p.OrganizationID, p.Actor(), audit.WebhookSet, "webhook", id, "created")
	})
	return out, secret, err
}

// ListEndpoints lists the webhooks (webhooks.manage).
func (s *Service) ListEndpoints(ctx context.Context, p auth.Principal) ([]Endpoint, error) {
	if err := authorize(p, auth.WebhooksManage); err != nil {
		return nil, err
	}
	var out []Endpoint
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		rows, err := q.ListWebhookEndpoints(ctx)
		if err != nil {
			return failure(err, "webhooks")
		}
		for _, r := range rows {
			secrets, err := q.ListActiveWebhookSecrets(ctx, dbgen.ListActiveWebhookSecretsParams{
				OwnerKind: OwnerEndpoint, OwnerID: r.ID, ExpiresAt: storage.Timestamp(s.now()),
			})
			if err != nil {
				return failure(err, "webhook secrets")
			}
			hint := ""
			if len(secrets) > 0 {
				hint = secrets[0].Hint
			}
			out = append(out, endpointOf(r, hint))
		}
		return nil
	})
	return out, err
}

// DeleteEndpoint removes a webhook with its secrets and deliveries.
func (s *Service) DeleteEndpoint(ctx context.Context, p auth.Principal, id string) error {
	if err := authorize(p, auth.WebhooksManage); err != nil {
		return err
	}
	u, err := parseUUID(id, "webhook")
	if err != nil {
		return err
	}
	return s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		n, err := q.DeleteWebhookEndpoint(ctx, u)
		if err != nil {
			return failure(err, "webhook")
		}
		if n == 0 {
			return plxerr.New(plxerr.ResourceNotFound, "no such webhook")
		}
		if err := q.DeleteWebhookSecrets(ctx, dbgen.DeleteWebhookSecretsParams{OwnerKind: OwnerEndpoint, OwnerID: u}); err != nil {
			return failure(err, "webhook secrets")
		}
		return s.record(ctx, tx, p.OrganizationID, p.Actor(), audit.WebhookDeleted, "webhook", id, "")
	})
}

// RotateEndpointSecret gives a webhook a new secret; the old one stays valid
// for the overlap (zero is DefaultOverlap, negative ends it at once). The new secret is returned once.
func (s *Service) RotateEndpointSecret(ctx context.Context, p auth.Principal, id string, overlap time.Duration) (string, error) {
	if err := authorize(p, auth.WebhooksManage); err != nil {
		return "", err
	}
	u, err := parseUUID(id, "webhook")
	if err != nil {
		return "", err
	}
	switch {
	case overlap == 0:
		overlap = DefaultOverlap
	case overlap < 0:
		overlap = 0 // end the old secret now, as after a leak
	}
	var secret string
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		if _, err := q.GetWebhookEndpoint(ctx, u); err != nil {
			return failure(err, "webhook")
		}
		if secret, err = s.RotateSecret(ctx, q, storage.MustUUID(p.OrganizationID), OwnerEndpoint, u, overlap); err != nil {
			return err
		}
		return s.record(ctx, tx, p.OrganizationID, p.Actor(), audit.WebhookSecretRotated, "webhook", id, "overlap "+overlap.String())
	})
	return secret, err
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package gateway

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// ConfigOptions configures Configs.
type ConfigOptions struct {
	DB    *storage.DB
	Audit *audit.Log
	// Crypter seals sensitive values (SEC-106).
	Crypter signing.Crypter
	// Cache holds the epochs the gateways check (DAT-092).
	Cache cache.Cache
	// Limits returns the limits in force for an app (LIM-001).
	Limits func(ctx context.Context, tx pgx.Tx, organizationID, appID string) (limits.Set, error)
}

// Configs stores the server-held configuration values of environments
// (DAT-090). Administrators change them through the API; the gateway reads
// them through a ConfigResolver, which uses Configs as its source.
type Configs struct{ o ConfigOptions }

var _ ConfigSource = (*Configs)(nil)

// NewConfigs returns the configuration store.
func NewConfigs(o ConfigOptions) (*Configs, error) {
	if o.DB == nil || o.Audit == nil || o.Crypter == nil || o.Cache == nil || o.Limits == nil {
		return nil, errors.New("gateway: configuration needs a database, an audit log, a crypter, a cache store and limits")
	}
	return &Configs{o: o}, nil
}

// sealBinding ties a sealed value to its environment and key.
func sealBinding(environmentID, key string) []byte {
	return []byte("config-value:" + environmentID + ":" + key)
}

// Set creates or changes a value. expected, when set, is the version the
// caller read (zero for a key that does not exist) (DAT-090). The new
// epoch reaches the cache store before the change commits, so no gateway
// uses a value older than a change that has been acknowledged (DAT-092).
func (c *Configs) Set(ctx context.Context, p auth.Principal, v ConfigValue, expected *int64) (ConfigValue, error) {
	if err := p.AuthorizeApp(auth.AppManage, v.AppID); err != nil {
		return ConfigValue{}, fmt.Errorf("gateway: %w", err)
	}
	var out ConfigValue
	var published *epochWrite
	err := c.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		env, err := environmentOf(ctx, q, v.AppID, v.EnvironmentID)
		if err != nil {
			return err
		}
		lim, err := c.o.Limits(ctx, tx, storage.ID(env.OrganizationID), v.AppID)
		if err != nil {
			return fmt.Errorf("gateway: %w", err)
		}
		if err := v.Validate(lim.Get(limits.ConfigValueBytes)); err != nil {
			return err
		}
		before, err := lockEnvironment(ctx, q, env)
		if err != nil {
			return err
		}
		old, err := q.GetConfigValue(ctx, dbgen.GetConfigValueParams{EnvironmentID: env.ID, Key: v.Key})
		exists := err == nil
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("gateway: %w", err)
		}
		if err := checkExpected(expected, old.Version, exists); err != nil {
			return err
		}
		if !exists {
			n, err := q.CountConfigValues(ctx, env.ID)
			if err != nil {
				return fmt.Errorf("gateway: %w", err)
			}
			if n >= lim.Get(limits.ConfigValuesPerEnvironment) {
				return plxerr.New(plxerr.LimitExceeded, "an environment holds at most config.valuesPerEnvironment = %d configuration values", lim.Get(limits.ConfigValuesPerEnvironment))
			}
		}
		params, err := c.upsertParams(ctx, env, p, v)
		if err != nil {
			return err
		}
		row, err := q.UpsertConfigValue(ctx, params)
		if err != nil {
			return failure(err, "configuration value")
		}
		live := v.Tier == TierLive || (exists && old.Tier == string(TierLive))
		strict := v.Strict || (exists && old.Strict)
		if published, err = c.publish(ctx, q, env, before, live, strict); err != nil {
			return err
		}
		out = configValueOf(row)
		return c.record(ctx, tx, p, audit.ConfigValueSet, out)
	})
	if err != nil {
		c.restore(ctx, published)
		return ConfigValue{}, err
	}
	return out, nil
}

// checkExpected applies the caller's expected version.
func checkExpected(expected *int64, current int64, exists bool) error {
	if expected == nil {
		return nil
	}
	if !exists {
		current = 0
	}
	if *expected != current {
		return plxerr.New(plxerr.PreconditionFailed, "the configuration value is at version %d, not %d; read it again", current, *expected)
	}
	return nil
}

// upsertParams builds the row of a value, sealing it when sensitive.
func (c *Configs) upsertParams(ctx context.Context, env dbgen.Environment, p auth.Principal, v ConfigValue) (dbgen.UpsertConfigValueParams, error) {
	params := dbgen.UpsertConfigValueParams{
		EnvironmentID: env.ID, Key: v.Key, OrganizationID: env.OrganizationID, AppID: env.AppID,
		Tier: string(v.Tier), ValueType: string(v.Type), Sensitive: v.Sensitive, Strict: v.Strict, Value: v.Value,
	}
	if p.UserID != "" {
		params.UpdatedBy = storage.MustUUID(p.UserID)
	}
	if !v.Sensitive {
		return params, nil
	}
	sealed, err := signing.Seal(ctx, c.o.Crypter, []byte(v.Value), sealBinding(storage.ID(env.ID), v.Key))
	if err != nil {
		return params, fmt.Errorf("gateway: seal a configuration value: %w", err)
	}
	keyID := sealed.KeyID
	params.Value, params.Ciphertext, params.WrappedKey, params.KeyID = "", sealed.Ciphertext, sealed.WrappedKey, &keyID
	return params, nil
}

// Delete removes a value.
func (c *Configs) Delete(ctx context.Context, p auth.Principal, appID, environmentID, key string) error {
	if err := p.AuthorizeApp(auth.AppManage, appID); err != nil {
		return fmt.Errorf("gateway: %w", err)
	}
	var published *epochWrite
	err := c.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		env, err := environmentOf(ctx, q, appID, environmentID)
		if err != nil {
			return err
		}
		before, err := lockEnvironment(ctx, q, env)
		if err != nil {
			return err
		}
		gone, err := q.DeleteConfigValue(ctx, dbgen.DeleteConfigValueParams{EnvironmentID: env.ID, Key: key})
		if err != nil {
			return failure(err, "configuration value")
		}
		if published, err = c.publish(ctx, q, env, before, gone.Tier == string(TierLive), gone.Strict); err != nil {
			return err
		}
		return c.record(ctx, tx, p, audit.ConfigValueDeleted, configValueOf(gone))
	})
	if err != nil {
		c.restore(ctx, published)
	}
	return err
}

// Get returns one value, without its content when it is sensitive.
func (c *Configs) Get(ctx context.Context, p auth.Principal, appID, environmentID, key string) (ConfigValue, error) {
	if err := p.AuthorizeApp(auth.AppRead, appID); err != nil {
		return ConfigValue{}, fmt.Errorf("gateway: %w", err)
	}
	var out ConfigValue
	err := c.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		env, err := environmentOf(ctx, q, appID, environmentID)
		if err != nil {
			return err
		}
		row, err := q.GetConfigValue(ctx, dbgen.GetConfigValueParams{EnvironmentID: env.ID, Key: key})
		if err != nil {
			return failure(err, "configuration value")
		}
		out = configValueOf(row)
		return nil
	})
	return out, err
}

// List lists an environment's values by key, after the key a previous page
// ended with, and returns the environment's epoch.
func (c *Configs) List(ctx context.Context, p auth.Principal, appID, environmentID, afterKey string, size int32) ([]ConfigValue, int64, error) {
	if err := p.AuthorizeApp(auth.AppRead, appID); err != nil {
		return nil, 0, fmt.Errorf("gateway: %w", err)
	}
	var out []ConfigValue
	var epoch int64
	err := c.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		env, err := environmentOf(ctx, q, appID, environmentID)
		if err != nil {
			return err
		}
		if row, err := q.GetConfigEnvironment(ctx, env.ID); err == nil {
			epoch = row.Epoch
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("gateway: %w", err)
		}
		rows, err := q.ListConfigValues(ctx, dbgen.ListConfigValuesParams{EnvironmentID: env.ID, AfterKey: afterKey, PageSize: size})
		if err != nil {
			return fmt.Errorf("gateway: %w", err)
		}
		for _, row := range rows {
			out = append(out, configValueOf(row))
		}
		return nil
	})
	return out, epoch, err
}

// Epoch returns an environment's current epoch (ConfigSource).
func (c *Configs) Epoch(ctx context.Context, organizationID, environmentID string) (int64, error) {
	id, err := storage.UUID(environmentID)
	if err != nil {
		return 0, plxerr.New(plxerr.InvalidFormat, "the environment identifier is not valid")
	}
	var epoch int64
	err = c.o.DB.InTx(ctx, storage.Tenant{OrganizationID: organizationID}, func(ctx context.Context, tx pgx.Tx) error {
		row, err := dbgen.New(tx).GetConfigEnvironment(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("gateway: %w", err)
		}
		epoch = row.Epoch
		return nil
	})
	return epoch, err //nolint:wrapcheck // InTx wraps its own failures
}

// Load returns an environment's configuration with sensitive values
// opened (ConfigSource). It reads the epochs first, so the values are at
// least as new as the epoch says.
func (c *Configs) Load(ctx context.Context, organizationID, environmentID string) (StoredConfig, error) {
	id, err := storage.UUID(environmentID)
	if err != nil {
		return StoredConfig{}, plxerr.New(plxerr.InvalidFormat, "the environment identifier is not valid")
	}
	var out StoredConfig
	err = c.o.DB.InTx(ctx, storage.Tenant{OrganizationID: organizationID}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, err := q.GetConfigEnvironment(ctx, id)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("gateway: %w", err)
		}
		out.Epoch, out.LiveEpoch, out.StrictEpoch = row.Epoch, row.LiveEpoch, row.StrictEpoch
		rows, err := q.ListEnvironmentConfigValues(ctx, id)
		if err != nil {
			return fmt.Errorf("gateway: %w", err)
		}
		for _, r := range rows {
			text := r.Value
			if r.Sensitive {
				opened, err := signing.Open(ctx, c.o.Crypter, signing.Sealed{Ciphertext: r.Ciphertext, WrappedKey: r.WrappedKey, KeyID: deref(r.KeyID)}, sealBinding(environmentID, r.Key))
				if err != nil {
					return fmt.Errorf("gateway: open a configuration value: %w", err)
				}
				text = string(opened)
			}
			out.Values = append(out.Values, StoredValue{
				Key: r.Key, Tier: ConfigTier(r.Tier), Type: ConfigType(r.ValueType), Sensitive: r.Sensitive, Strict: r.Strict, Text: text, Version: r.Version,
			})
		}
		return nil
	})
	return out, err //nolint:wrapcheck // InTx wraps its own failures
}

// epochWrite remembers the epoch a change published, to take it back when
// the change does not commit.
type epochWrite struct {
	key      string
	previous int64
}

// lockEnvironment makes sure the environment has its epoch row and locks
// it, which serialises the changes of one environment. It returns the
// epoch before the change.
func lockEnvironment(ctx context.Context, q *dbgen.Queries, env dbgen.Environment) (dbgen.ConfigEnvironment, error) {
	if err := q.EnsureConfigEnvironment(ctx, dbgen.EnsureConfigEnvironmentParams{EnvironmentID: env.ID, OrganizationID: env.OrganizationID, AppID: env.AppID}); err != nil {
		return dbgen.ConfigEnvironment{}, fmt.Errorf("gateway: %w", err)
	}
	row, err := q.LockConfigEnvironment(ctx, env.ID)
	if err != nil {
		return dbgen.ConfigEnvironment{}, fmt.Errorf("gateway: %w", err)
	}
	return row, nil
}

// publish raises the environment's epochs and writes the new epoch to the
// cache store, still inside the change's transaction: a gateway that reads
// the key from then on reloads, and reads the old state until the change
// commits and the new one after (DAT-092). When the cache store cannot be
// written the change fails, because a gateway would go on trusting an
// epoch that is no longer current.
func (c *Configs) publish(ctx context.Context, q *dbgen.Queries, env dbgen.Environment, before dbgen.ConfigEnvironment, live, strict bool) (*epochWrite, error) {
	row, err := q.BumpConfigEpoch(ctx, dbgen.BumpConfigEpochParams{
		EnvironmentID: env.ID, OrganizationID: env.OrganizationID, AppID: env.AppID, Live: live || strict, Strict: strict,
	})
	if err != nil {
		return nil, fmt.Errorf("gateway: %w", err)
	}
	w := &epochWrite{key: EpochKey(storage.ID(env.AppID), storage.ID(env.ID)), previous: before.Epoch}
	if err := c.o.Cache.Set(ctx, w.key, []byte(strconv.FormatInt(row.Epoch, 10)), epochTTL); err != nil {
		return nil, plxerr.Wrap(plxerr.ConfigStoreUnavailable, err, "the change cannot be announced to the gateways")
	}
	return w, nil
}

// restore takes a published epoch back after a change that did not
// commit. It is best effort: if it fails, gateways reload on every use
// until the next change, which is slower but never stale.
func (c *Configs) restore(ctx context.Context, w *epochWrite) {
	if w == nil {
		return
	}
	_ = c.o.Cache.Set(context.WithoutCancel(ctx), w.key, []byte(strconv.FormatInt(w.previous, 10)), epochTTL)
}

// inOrg runs f in a transaction bound to the principal's organisation.
func (c *Configs) inOrg(ctx context.Context, p auth.Principal, f func(context.Context, pgx.Tx) error) error {
	return c.o.DB.InTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID, UserID: p.UserID}, f) //nolint:wrapcheck // InTx wraps its own failures
}

// record audits a change: the key, the tier and the version, never the
// value (SEC-092).
func (c *Configs) record(ctx context.Context, tx pgx.Tx, p auth.Principal, action audit.Action, v ConfigValue) error {
	_, err := c.o.Audit.Append(ctx, tx, audit.Entry{
		OrganizationID: p.OrganizationID, Actor: p.Actor(), Action: action,
		TargetKind: "config_value", TargetID: v.EnvironmentID + "/" + v.Key,
		Detail: fmt.Sprintf("app %s, key %s, tier %s, sensitive %t, strict %t, version %d", v.AppID, v.Key, v.Tier, v.Sensitive, v.Strict, v.Version),
	})
	if err != nil {
		return fmt.Errorf("gateway: %w", err)
	}
	return nil
}

// configValueOf converts a row; a sensitive value carries no content.
func configValueOf(row dbgen.ConfigValue) ConfigValue {
	return ConfigValue{
		AppID: storage.ID(row.AppID), EnvironmentID: storage.ID(row.EnvironmentID), Key: row.Key,
		Tier: ConfigTier(row.Tier), Type: ConfigType(row.ValueType), Sensitive: row.Sensitive, Strict: row.Strict,
		Value: row.Value, Version: row.Version, UpdatedAt: storage.Time(row.UpdatedAt),
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

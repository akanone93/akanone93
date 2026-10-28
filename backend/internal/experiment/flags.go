// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package experiment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/controldoc"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// namePattern is the form of a PXL name: a flag, experiment or layer key.
var namePattern = regexp.MustCompile(`^[a-z][a-zA-Z0-9]*$`)

// maxName bounds a flag, experiment or layer key, as the schema does.
const maxName = 64

// FlagRef names a flag of an environment.
type FlagRef struct {
	AppID, EnvironmentID, Key string
}

// SegmentValue is the value a flag has for the devices of a segment.
type SegmentValue struct {
	// Segment is the segment's key.
	Segment string
	// Value is JSON of the flag's type.
	Value json.RawMessage
}

// FlagSpec is the content of a flag that its administrator gives: all of it
// replaces what the flag had.
type FlagSpec struct {
	// Type is bool, number, string or json. An update may leave it empty,
	// but may not change it (PLX-8203).
	Type        controldoc.FlagType
	Description string
	// Schema is the JSON Schema of a json flag's values; an update may leave
	// it empty to keep the current one.
	Schema json.RawMessage
	// Default is the value for devices in none of the segments.
	Default json.RawMessage
	// Segments are the per-segment values in precedence order: the first
	// segment a device belongs to wins.
	Segments []SegmentValue
}

// Flag is a feature flag as stored.
type Flag struct {
	FlagRef
	ID          string
	Type        controldoc.FlagType
	Description string
	// Schema is the JSON Schema of a json flag; empty for other types.
	Schema json.RawMessage
	// Default is canonical JSON.
	Default json.RawMessage
	// Segments carry the rule each segment had when the value was written.
	Segments  []controldoc.FlagSegmentValue
	CreatedBy audit.Actor
	UpdatedBy audit.Actor
	CreatedAt time.Time
	UpdatedAt time.Time
}

// flagOf converts a stored flag.
func flagOf(row dbgen.FeatureFlag) (Flag, error) {
	f := Flag{
		FlagRef: FlagRef{AppID: storage.ID(row.AppID), EnvironmentID: storage.ID(row.EnvironmentID), Key: row.Key},
		ID:      storage.ID(row.ID), Type: controldoc.FlagType(row.Type), Description: row.Description,
		Schema: row.JsonSchema, Default: row.DefaultValue,
		CreatedBy: actorOf(row.CreatedByKind, row.CreatedByID, row.CreatedBy), UpdatedBy: actorOf(row.UpdatedByKind, row.UpdatedByID, row.UpdatedBy),
		CreatedAt: storage.Time(row.CreatedAt), UpdatedAt: storage.Time(row.UpdatedAt),
	}
	if err := json.Unmarshal(row.Segments, &f.Segments); err != nil {
		return Flag{}, fmt.Errorf("experiment: the stored segments of flag %q: %w", row.Key, err)
	}
	return f, nil
}

// resolvedSegments resolves the segments a spec names to their rules, before
// any transaction is open: the document service has its own.
func (s *Service) resolvedSegments(ctx context.Context, p auth.Principal, appID string, in []SegmentValue) ([]controldoc.Rule, error) {
	rules := make([]controldoc.Rule, len(in))
	seen := map[string]bool{}
	for i, v := range in {
		if seen[v.Segment] {
			return nil, plxerr.New(plxerr.DuplicateKey, "segment %q appears twice; a flag has one value per segment", v.Segment)
		}
		seen[v.Segment] = true
		rule, err := s.segments.Rule(ctx, p, appID, v.Segment)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		rules[i] = rule
	}
	return rules, nil
}

// checkedFlag is a flag spec after every check, ready to store.
type checkedFlag struct {
	spec     FlagSpec
	def      string
	segments []controldoc.FlagSegmentValue
}

// checkSpec checks a flag spec against its type and schema and writes the
// values in canonical form.
func checkSpec(spec FlagSpec, rules []controldoc.Rule, lim limits.Set) (checkedFlag, error) {
	checker, err := newValueChecker(spec.Type, spec.Schema, lim)
	if err != nil {
		return checkedFlag{}, err
	}
	if max := lim.Get(limits.FlagsSegmentValues); int64(len(spec.Segments)) > max {
		return checkedFlag{}, plxerr.New(plxerr.LimitExceeded, "a flag has at most %d per-segment values (flags.segmentValues), not %d", max, len(spec.Segments))
	}
	out := checkedFlag{spec: spec}
	if out.def, err = checker.canonical(spec.Default, "the default"); err != nil {
		return checkedFlag{}, err
	}
	for i, v := range spec.Segments {
		text, err := checker.canonical(v.Value, fmt.Sprintf("the value for segment %q", v.Segment))
		if err != nil {
			return checkedFlag{}, err
		}
		out.segments = append(out.segments, controldoc.FlagSegmentValue{Segment: v.Segment, Rule: rules[i], Value: text})
	}
	return out, nil
}

// checkKey checks the key of a flag or an experiment.
func checkKey(key, what string) error {
	if len(key) > maxName || !namePattern.MatchString(key) {
		return plxerr.New(plxerr.InvalidFormat, "%q is not a %s key: a lowerCamelCase name of at most %d characters", key, what, maxName)
	}
	return nil
}

// CreateFlag makes a flag in an environment and publishes its value to the
// environment's control documents (ABT-006). A key may be used in one
// environment at most once.
func (s *Service) CreateFlag(ctx context.Context, p auth.Principal, ref FlagRef, spec FlagSpec) (Flag, error) {
	if err := authorizeFlags(p, ref.AppID, nil); err != nil {
		return Flag{}, err
	}
	if err := checkKey(ref.Key, "flag"); err != nil {
		return Flag{}, err
	}
	rules, err := s.resolvedSegments(ctx, p, ref.AppID, spec.Segments)
	if err != nil {
		return Flag{}, err
	}
	var out Flag
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		env, err := environment(ctx, q, ref.AppID, ref.EnvironmentID)
		if err != nil {
			return err
		}
		if err := authorizeFlags(p, ref.AppID, &env); err != nil {
			return err
		}
		lim, err := s.limitsOf(ctx, tx, p, ref.AppID)
		if err != nil {
			return err
		}
		checked, err := checkSpec(spec, rules, lim)
		if err != nil {
			return err
		}
		if err := s.checkFlagCount(ctx, q, env, ref.Key, lim); err != nil {
			return err
		}
		row, err := s.insertFlag(ctx, q, p, env, ref.Key, checked)
		if err != nil {
			return err
		}
		if out, err = flagOf(row); err != nil {
			return err
		}
		return s.afterFlagChange(ctx, tx, p, env, audit.FlagCreated, out.ID, "flag "+ref.Key+" created")
	})
	return out, err
}

// checkFlagCount refuses a flag that would take an app past flags.perApp. A
// key the app already has in another environment is the same flag and adds
// nothing to the count.
func (*Service) checkFlagCount(ctx context.Context, q *dbgen.Queries, env dbgen.Environment, key string, lim limits.Set) error {
	known, err := q.FeatureFlagKeyExistsInApp(ctx, dbgen.FeatureFlagKeyExistsInAppParams{AppID: env.AppID, Key: key})
	if err != nil {
		return failure(err, "flag")
	}
	if known {
		return nil
	}
	count, err := q.CountFeatureFlagKeys(ctx, env.AppID)
	if err != nil {
		return failure(err, "flag")
	}
	if max := lim.Get(limits.FlagsPerApp); count >= max {
		return plxerr.New(plxerr.LimitExceeded, "an app may define at most %d flags (flags.perApp); delete one first", max)
	}
	return nil
}

// insertFlag stores a new flag.
func (s *Service) insertFlag(ctx context.Context, q *dbgen.Queries, p auth.Principal, env dbgen.Environment, key string, c checkedFlag) (dbgen.FeatureFlag, error) {
	id, err := s.o.IDs.New()
	if err != nil {
		return dbgen.FeatureFlag{}, fmt.Errorf("experiment: %w", err)
	}
	segments, err := json.Marshal(nonNilSegments(c.segments))
	if err != nil {
		return dbgen.FeatureFlag{}, fmt.Errorf("experiment: %w", err)
	}
	by := p.Actor()
	row, err := q.InsertFeatureFlag(ctx, dbgen.InsertFeatureFlagParams{
		ID: storage.MustUUID(id), OrganizationID: env.OrganizationID, AppID: env.AppID, EnvironmentID: env.ID, Key: key,
		Type: string(c.spec.Type), Description: c.spec.Description, JsonSchema: schemaColumn(c.spec.Schema),
		DefaultValue: []byte(c.def), Segments: segments,
		CreatedByKind: by.Kind, CreatedByID: by.ID, CreatedBy: by.Display,
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return dbgen.FeatureFlag{}, plxerr.New(plxerr.DuplicateKey, "the environment already has a flag %q", key)
	}
	if err != nil {
		return dbgen.FeatureFlag{}, failure(err, "flag")
	}
	return row, nil
}

// nonNilSegments is an empty list for nil, so that it is stored as [].
func nonNilSegments(in []controldoc.FlagSegmentValue) []controldoc.FlagSegmentValue {
	if in == nil {
		return []controldoc.FlagSegmentValue{}
	}
	return in
}

// schemaColumn is the stored form of a flag's schema: NULL for none.
func schemaColumn(raw json.RawMessage) []byte {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return raw
}

// afterFlagChange publishes the environment's flags and experiments to its
// control documents and audits the change, in the change's transaction.
func (s *Service) afterFlagChange(ctx context.Context, tx pgx.Tx, p auth.Principal, env dbgen.Environment, action audit.Action, id, detail string) error {
	if err := s.syncControl(ctx, tx, p, env, detail); err != nil {
		return err
	}
	return s.record(ctx, tx, p, audit.Entry{Action: action, TargetKind: "flag", TargetID: id, Detail: detail})
}

// UpdateFlag replaces a flag's description, schema, default and segment
// values. Its type cannot change (PLX-8203).
func (s *Service) UpdateFlag(ctx context.Context, p auth.Principal, ref FlagRef, spec FlagSpec) (Flag, error) {
	if err := authorizeFlags(p, ref.AppID, nil); err != nil {
		return Flag{}, err
	}
	rules, err := s.resolvedSegments(ctx, p, ref.AppID, spec.Segments)
	if err != nil {
		return Flag{}, err
	}
	var out Flag
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		env, err := environment(ctx, q, ref.AppID, ref.EnvironmentID)
		if err != nil {
			return err
		}
		if err := authorizeFlags(p, ref.AppID, &env); err != nil {
			return err
		}
		current, err := q.GetFeatureFlagForUpdate(ctx, dbgen.GetFeatureFlagForUpdateParams{EnvironmentID: env.ID, Key: ref.Key})
		if err != nil {
			return failure(err, "flag")
		}
		spec, err = keepUnchanged(spec, current)
		if err != nil {
			return err
		}
		lim, err := s.limitsOf(ctx, tx, p, ref.AppID)
		if err != nil {
			return err
		}
		checked, err := checkSpec(spec, rules, lim)
		if err != nil {
			return err
		}
		row, err := s.updateFlag(ctx, q, p, env, ref.Key, checked)
		if err != nil {
			return err
		}
		if out, err = flagOf(row); err != nil {
			return err
		}
		return s.afterFlagChange(ctx, tx, p, env, audit.FlagUpdated, out.ID, "flag "+ref.Key+" updated")
	})
	return out, err
}

// keepUnchanged refuses a change of a flag's type (PLX-8203) and fills in
// what an update leaves out: the type and the schema.
func keepUnchanged(spec FlagSpec, current dbgen.FeatureFlag) (FlagSpec, error) {
	have := controldoc.FlagType(current.Type)
	if spec.Type != "" && spec.Type != have {
		return spec, plxerr.New(plxerr.FlagTypeChangeRefused,
			"flag %q is %s and cannot become %s: devices may hold values of the old type; delete the flag and create it again", current.Key, have, spec.Type)
	}
	spec.Type = have
	if len(spec.Schema) == 0 {
		spec.Schema = current.JsonSchema
	}
	return spec, nil
}

func (*Service) updateFlag(ctx context.Context, q *dbgen.Queries, p auth.Principal, env dbgen.Environment, key string, c checkedFlag) (dbgen.FeatureFlag, error) {
	segments, err := json.Marshal(nonNilSegments(c.segments))
	if err != nil {
		return dbgen.FeatureFlag{}, fmt.Errorf("experiment: %w", err)
	}
	by := p.Actor()
	row, err := q.UpdateFeatureFlag(ctx, dbgen.UpdateFeatureFlagParams{
		EnvironmentID: env.ID, Key: key, Description: c.spec.Description, JsonSchema: schemaColumn(c.spec.Schema),
		DefaultValue: []byte(c.def), Segments: segments, UpdatedByKind: by.Kind, UpdatedByID: by.ID, UpdatedBy: by.Display,
	})
	if err != nil {
		return dbgen.FeatureFlag{}, failure(err, "flag")
	}
	return row, nil
}

// DeleteFlag removes a flag from an environment and from its control
// documents. Devices fall back to the default compiled into the app.
func (s *Service) DeleteFlag(ctx context.Context, p auth.Principal, ref FlagRef) error {
	if err := authorizeFlags(p, ref.AppID, nil); err != nil {
		return err
	}
	return s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		env, err := environment(ctx, q, ref.AppID, ref.EnvironmentID)
		if err != nil {
			return err
		}
		if err := authorizeFlags(p, ref.AppID, &env); err != nil {
			return err
		}
		row, err := q.GetFeatureFlagForUpdate(ctx, dbgen.GetFeatureFlagForUpdateParams{EnvironmentID: env.ID, Key: ref.Key})
		if err != nil {
			return failure(err, "flag")
		}
		if _, err := q.DeleteFeatureFlag(ctx, dbgen.DeleteFeatureFlagParams{EnvironmentID: env.ID, Key: ref.Key}); err != nil {
			return failure(err, "flag")
		}
		return s.afterFlagChange(ctx, tx, p, env, audit.FlagDeleted, storage.ID(row.ID), "flag "+ref.Key+" deleted")
	})
}

// GetFlag returns one flag.
func (s *Service) GetFlag(ctx context.Context, p auth.Principal, ref FlagRef) (Flag, error) {
	if err := authorizeRead(p, ref.AppID); err != nil {
		return Flag{}, err
	}
	var out Flag
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		env, err := environment(ctx, q, ref.AppID, ref.EnvironmentID)
		if err != nil {
			return err
		}
		row, err := q.GetFeatureFlag(ctx, dbgen.GetFeatureFlagParams{EnvironmentID: env.ID, Key: ref.Key})
		if err != nil {
			return failure(err, "flag")
		}
		out, err = flagOf(row)
		return err
	})
	return out, err
}

// ListFlags lists the flags of an environment by key.
func (s *Service) ListFlags(ctx context.Context, p auth.Principal, appID, environmentID string) ([]Flag, error) {
	if err := authorizeRead(p, appID); err != nil {
		return nil, err
	}
	var out []Flag
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		env, err := environment(ctx, q, appID, environmentID)
		if err != nil {
			return err
		}
		rows, err := q.ListFeatureFlags(ctx, env.ID)
		if err != nil {
			return failure(err, "flag")
		}
		for _, row := range rows {
			f, err := flagOf(row)
			if err != nil {
				return err
			}
			out = append(out, f)
		}
		return nil
	})
	return out, err
}

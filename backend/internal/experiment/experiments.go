// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package experiment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// State is where an experiment is in its life; it only moves forward.
type State string

// The states of an experiment.
const (
	// StateDraft: registered, not allocated to any device.
	StateDraft State = "draft"
	// StateRunning: its allocation is in the environment's control documents.
	StateRunning State = "running"
	// StateStopped: taken out of the control documents; results stay readable.
	StateStopped State = "stopped"
	// StateConcluded: a winner was promoted into the project (ABT-005).
	StateConcluded State = "concluded"
)

// Experiment is an experiment as stored.
type Experiment struct {
	ID            string
	AppID         string
	EnvironmentID string
	Key           string
	State         State
	// Definition is the experiment document it was made from, and Document
	// the same as stored JSON.
	Definition schema.ExperimentDocument
	Document   json.RawMessage
	// Salt seeds the assignment of subjects to variants.
	Salt string
	// LayerShareBP is the share of its layer, in basis points, that the
	// experiment asked for when it started; LayerStart and LayerEnd are the
	// buckets it holds while it runs.
	LayerShareBP         uint32
	LayerStart, LayerEnd uint32
	StartedAt, StoppedAt time.Time
	// Winner and PromoteJobID are set once a winner was promoted.
	Winner       string
	PromoteJobID string
	CreatedBy    audit.Actor
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// experimentOf converts a stored experiment.
func experimentOf(row dbgen.Experiment) (Experiment, error) {
	def, err := parseDefinition(row.Definition)
	if err != nil {
		return Experiment{}, err
	}
	e := Experiment{
		ID: storage.ID(row.ID), AppID: storage.ID(row.AppID), EnvironmentID: storage.ID(row.EnvironmentID), Key: row.Key,
		State: State(row.State), Definition: def, Document: row.Definition, Salt: row.Salt, LayerShareBP: uint32(row.LayerShareBp), //nolint:gosec // checked by the table
		StartedAt: storage.Time(row.StartedAt), StoppedAt: storage.Time(row.StoppedAt), Winner: row.Winner,
		CreatedBy: actorOf(row.CreatedByKind, row.CreatedByID, row.CreatedBy), CreatedAt: storage.Time(row.CreatedAt), UpdatedAt: storage.Time(row.UpdatedAt),
	}
	if row.LayerStart != nil && row.LayerEnd != nil {
		e.LayerStart, e.LayerEnd = uint32(*row.LayerStart), uint32(*row.LayerEnd) //nolint:gosec // checked by the table
	}
	if row.PromoteJobID.Valid {
		e.PromoteJobID = storage.ID(row.PromoteJobID)
	}
	return e, nil
}

// CreateExperimentRequest is an experiment document to register.
type CreateExperimentRequest struct {
	AppID, EnvironmentID string
	// Document is the experiment document (experiments/<key>.experiment.json).
	Document []byte
}

// CreateExperiment registers a draft experiment from its document (ABT-001).
// The document is checked against its JSON Schema and then for one control,
// weights that sum to 100 and a minimum sample size. Nothing reaches a device
// until it is started.
func (s *Service) CreateExperiment(ctx context.Context, p auth.Principal, r CreateExperimentRequest) (Experiment, error) {
	if err := authorizeExperiments(p, r.AppID); err != nil {
		return Experiment{}, err
	}
	src, diags := s.loader.ParseDocument("experiments/new.experiment.json", r.Document, schema.KindExperiment)
	if diags.HasErrors() {
		return Experiment{}, refusal(diags)
	}
	var def schema.ExperimentDocument
	if err := json.Unmarshal(src.Canonical, &def); err != nil {
		return Experiment{}, fmt.Errorf("experiment: %w", err)
	}
	if err := checkDefinition(def); err != nil {
		return Experiment{}, err
	}
	var out Experiment
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		env, err := environment(ctx, q, r.AppID, r.EnvironmentID)
		if err != nil {
			return err
		}
		lim, err := s.limitsOf(ctx, tx, p, r.AppID)
		if err != nil {
			return err
		}
		if size, max := len(src.Canonical), lim.Get(limits.DocumentFileSize); int64(size) > max {
			return plxerr.New(plxerr.RequestTooLarge, "the experiment document is %d bytes; at most %d (document.fileSize)", size, max)
		}
		row, err := s.insertExperiment(ctx, q, p, env, def, src.Canonical)
		if err != nil {
			return err
		}
		if out, err = experimentOf(row); err != nil {
			return err
		}
		return s.record(ctx, tx, p, audit.Entry{Action: audit.ExperimentCreated, TargetKind: "experiment", TargetID: out.ID, Detail: "experiment " + def.Key})
	})
	return out, err
}

// insertExperiment stores a draft with the salt its document names, or one
// drawn from the identifier source.
func (s *Service) insertExperiment(ctx context.Context, q *dbgen.Queries, p auth.Principal, env dbgen.Environment, def schema.ExperimentDocument, canonical []byte) (dbgen.Experiment, error) {
	id, err := s.o.IDs.New()
	if err != nil {
		return dbgen.Experiment{}, fmt.Errorf("experiment: %w", err)
	}
	salt := def.Salt
	if salt == "" {
		if salt, err = s.o.IDs.New(); err != nil {
			return dbgen.Experiment{}, fmt.Errorf("experiment: %w", err)
		}
	}
	by := p.Actor()
	row, err := q.InsertExperiment(ctx, dbgen.InsertExperimentParams{
		ID: storage.MustUUID(id), OrganizationID: env.OrganizationID, AppID: env.AppID, EnvironmentID: env.ID, Key: def.Key,
		Definition: canonical, Salt: salt, Layer: def.Layer,
		CreatedByKind: by.Kind, CreatedByID: by.ID, CreatedBy: by.Display,
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return dbgen.Experiment{}, plxerr.New(plxerr.DuplicateKey, "the environment already has an experiment %q", def.Key)
	}
	if err != nil {
		return dbgen.Experiment{}, failure(err, "experiment")
	}
	return row, nil
}

// loadExperiment reads an experiment by identifier and checks that the
// principal may read its app.
func loadExperiment(ctx context.Context, q *dbgen.Queries, p auth.Principal, id string, forUpdate bool) (dbgen.Experiment, error) {
	uid, err := parseID(id, "experiment")
	if err != nil {
		return dbgen.Experiment{}, err
	}
	get := q.GetExperiment
	if forUpdate {
		get = q.GetExperimentForUpdate
	}
	row, err := get(ctx, uid)
	if err != nil {
		return dbgen.Experiment{}, failure(err, "experiment")
	}
	if err := authorizeRead(p, storage.ID(row.AppID)); err != nil {
		return dbgen.Experiment{}, err
	}
	return row, nil
}

// GetExperiment returns one experiment.
func (s *Service) GetExperiment(ctx context.Context, p auth.Principal, id string) (Experiment, error) {
	var out Experiment
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		row, err := loadExperiment(ctx, dbgen.New(tx), p, id, false)
		if err != nil {
			return err
		}
		out, err = experimentOf(row)
		return err
	})
	return out, err
}

// ListExperiments lists an app's experiments, optionally of one environment
// and one state, ordered by environment and key.
func (s *Service) ListExperiments(ctx context.Context, p auth.Principal, appID, environmentID string, state State) ([]Experiment, error) {
	if err := authorizeRead(p, appID); err != nil {
		return nil, err
	}
	app, err := parseID(appID, "app")
	if err != nil {
		return nil, err
	}
	var env pgtype.UUID
	if environmentID != "" {
		if env, err = parseID(environmentID, "environment"); err != nil {
			return nil, err
		}
	}
	var out []Experiment
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListExperiments(ctx, dbgen.ListExperimentsParams{AppID: app, EnvironmentID: env, State: string(state)})
		if err != nil {
			return failure(err, "experiment")
		}
		for _, row := range rows {
			e, err := experimentOf(row)
			if err != nil {
				return err
			}
			out = append(out, e)
		}
		return nil
	})
	return out, err
}

// DeleteExperiment discards a draft experiment. One that has run is kept for
// its results.
func (s *Service) DeleteExperiment(ctx context.Context, p auth.Principal, id string) error {
	if err := authorizeSomeApp(p, auth.ExperimentsManage); err != nil {
		return err
	}
	return s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, err := loadExperiment(ctx, q, p, id, true)
		if err != nil {
			return err
		}
		if err := authorizeExperiments(p, storage.ID(row.AppID)); err != nil {
			return err
		}
		n, err := q.DeleteDraftExperiment(ctx, row.ID)
		if err != nil {
			return failure(err, "experiment")
		}
		if n == 0 {
			return plxerr.New(plxerr.PreconditionFailed, "experiment %q is %s; only a draft is deleted", row.Key, row.State)
		}
		return s.record(ctx, tx, p, audit.Entry{Action: audit.ExperimentDeleted, TargetKind: "experiment", TargetID: id, Detail: "experiment " + row.Key})
	})
}

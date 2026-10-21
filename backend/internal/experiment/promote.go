// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package experiment

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"slices"
	"testing/fstest"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/document"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/release"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// PromoteRequest names the winner of an experiment.
type PromoteRequest struct {
	// ExperimentID is the experiment; Variant the winning variant.
	ExperimentID, Variant string
	// EnvironmentID is the environment the release is made for.
	EnvironmentID string
	// Label and Notes describe the release.
	Label, Notes string
}

// Promotion is the outcome of a promotion.
type Promotion struct {
	Experiment Experiment
	// Job is the publish job that makes the release.
	Job release.PublishJob
}

// promotionSession is the editing session the promotion holds the draft's
// lock under while it replaces the draft.
const promotionSession = "experiment-promotion"

// Promote makes a variant the app (ABT-005): the compiler's PromoteWinner pass
// replaces every Variant node of the experiment by the winner's subtree and
// inlines its bindings, the draft takes the result, and a publish job makes
// the release through the normal publish path with every gate it has. The
// experiment is concluded, which takes its allocation out of the control
// documents.
//
// The experiment must be running or stopped. Once the draft has taken the
// promoted project the experiment is concluded even if the publish is
// refused, because the draft no longer holds the experiment: the error then
// says so, and the project is published again with the usual command.
func (s *Service) Promote(ctx context.Context, p auth.Principal, r PromoteRequest) (Promotion, error) {
	if s.o.Documents == nil || s.o.Publisher == nil {
		return Promotion{}, errors.New("experiment: no document or release service to promote with")
	}
	if err := authorizeSomeApp(p, auth.ExperimentsManage); err != nil {
		return Promotion{}, err
	}
	exp, err := s.GetExperiment(ctx, p, r.ExperimentID)
	if err != nil {
		return Promotion{}, err
	}
	if err := authorizeExperiments(p, exp.AppID); err != nil {
		return Promotion{}, err
	}
	if exp.State != StateRunning && exp.State != StateStopped {
		return Promotion{}, plxerr.New(plxerr.PreconditionFailed, "experiment %q is %s; only a running or stopped experiment is promoted", exp.Key, exp.State)
	}
	promoted, err := s.promotedProject(ctx, p, exp, r.Variant)
	if err != nil {
		return Promotion{}, err
	}
	if _, err := s.o.Documents.Import(ctx, p, exp.AppID, "", promotionSession, promoted); err != nil {
		return Promotion{}, fmt.Errorf("experiment: replace the draft with the promoted project: %w", err)
	}
	if err := s.o.Documents.ReleaseLock(ctx, p, exp.AppID, "", promotionSession); err != nil {
		return Promotion{}, fmt.Errorf("experiment: release the draft: %w", err)
	}
	concluded, err := s.conclude(ctx, p, r.ExperimentID, r.Variant)
	if err != nil {
		return Promotion{}, err
	}
	job, err := s.o.Publisher.Publish(ctx, p, release.PublishRequest{
		AppID: exp.AppID, EnvironmentID: r.EnvironmentID, Label: r.Label, Notes: r.Notes,
	})
	if err != nil {
		return Promotion{Experiment: concluded}, fmt.Errorf("experiment: the draft holds the promoted project but publishing it was refused; publish it again: %w", err)
	}
	concluded, err = s.recordPromoteJob(ctx, p, concluded, job)
	return Promotion{Experiment: concluded, Job: job}, err
}

// promotedProject exports the draft, runs the compiler's pass for the winner
// and returns the files of the result in the Git layout.
func (s *Service) promotedProject(ctx context.Context, p auth.Principal, exp Experiment, variant string) ([]document.File, error) {
	files, err := s.o.Documents.Export(ctx, p, exp.AppID, "")
	if err != nil {
		return nil, fmt.Errorf("experiment: read the draft: %w", err)
	}
	project := fstest.MapFS{}
	for _, f := range files {
		project[f.Path] = &fstest.MapFile{Data: f.Content, Mode: 0o644}
	}
	out, diags := compiler.PromoteWinner(project, exp.Key, variant)
	if diags.HasErrors() {
		return nil, refusal(diags)
	}
	return filesOf(out)
}

// filesOf lists the files of a file system in path order.
func filesOf(fsys fs.FS) ([]document.File, error) {
	found := map[string][]byte{}
	err := fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(fsys, name)
		found[name] = data
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("experiment: read the promoted project: %w", err)
	}
	out := make([]document.File, 0, len(found))
	for _, name := range slices.Sorted(maps.Keys(found)) {
		out = append(out, document.File{Path: name, Content: found[name]})
	}
	return out, nil
}

// conclude records the winner and takes the experiment out of the control
// documents.
func (s *Service) conclude(ctx context.Context, p auth.Principal, id, winner string) (Experiment, error) {
	var out Experiment
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, env, err := lockExperiment(ctx, q, p, id)
		if err != nil {
			return err
		}
		done, err := q.ConcludeExperiment(ctx, dbgen.ConcludeExperimentParams{ID: row.ID, Winner: winner, UpdatedAt: storage.Timestamp(s.now().UTC())})
		if err != nil {
			return plxerr.Wrap(plxerr.PreconditionFailed, err, "experiment %q is %s; only a running or stopped experiment is promoted", row.Key, row.State)
		}
		if out, err = experimentOf(done); err != nil {
			return err
		}
		return s.afterExperimentChange(ctx, tx, p, env, audit.ExperimentPromoted, out, fmt.Sprintf("experiment %s concluded with %s", out.Key, winner))
	})
	return out, err
}

// recordPromoteJob remembers the publish job of a promotion.
func (s *Service) recordPromoteJob(ctx context.Context, p auth.Principal, e Experiment, job release.PublishJob) (Experiment, error) {
	jobID, err := parseID(job.ID, "publish job")
	if err != nil {
		return e, err
	}
	uid, err := parseID(e.ID, "experiment")
	if err != nil {
		return e, err
	}
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		row, err := dbgen.New(tx).SetExperimentPromoteJob(ctx, dbgen.SetExperimentPromoteJobParams{ID: uid, PromoteJobID: jobID})
		if err != nil {
			return failure(err, "experiment")
		}
		e, err = experimentOf(row)
		return err
	})
	return e, err
}

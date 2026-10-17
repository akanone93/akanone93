// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package golden holds the server side of golden (visual regression) tests
// (TST-003, ADR-0070): runs the CLI uploads, the approved baselines they
// are compared with, and the question the publish gate asks. The images are
// in object storage under the plugin, by SHA-256; the rows are under
// row-level security.
package golden

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
	"github.com/nightCode42/plux3/backend/internal/storage/objects"
)

// IDs generates identifiers for the rows this package writes.
type IDs interface{ New() (string, error) }

// Options are what the service needs.
type Options struct {
	DB      *storage.DB
	Audit   *audit.Log
	Objects objects.Store
	IDs     IDs
	// Limits are the installation's limits (LIM-001); zero means the
	// defaults.
	Limits limits.Set
}

// Service stores golden runs and baselines.
type Service struct{ o Options }

// New returns a service.
func New(o Options) *Service {
	if o.Limits.IsZero() {
		o.Limits = limits.Defaults()
	}
	return &Service{o: o}
}

// Case is one rendered case of a run.
type Case struct {
	ID        string
	Frame     string
	Locale    string
	Theme     string
	TextScale float64
	// SHA256 is the hash of the PNG.
	SHA256 []byte
	// Image is the PNG; empty when the case matches its baseline.
	Image []byte
}

// Run is a run to record.
type Run struct {
	AppID     string
	PluginKey string
	// Bundle and App are the hashes of the bundle under test and of the app
	// bundle it was rendered with.
	Bundle, App []byte
	Canonical   bool
	Renderer    string
	Cases       []Case
}

// Difference is a case that differs from the approved baseline.
type Difference struct {
	Case
	// Baseline is the approved image hash, nil when there is none.
	Baseline []byte
}

// Status is what the publish gate sees for the exact bundles.
type Status struct {
	Found       bool
	RunID       string
	Cases       int
	Differences int
}

const sha256Len = sha256.Size

func authorize(p auth.Principal, want auth.Permission, appID string) error {
	if err := p.AuthorizeApp(want, appID); err != nil {
		return fmt.Errorf("golden: %w", err)
	}
	return nil
}

func (s *Service) inOrg(ctx context.Context, p auth.Principal, f func(context.Context, pgx.Tx) error) error {
	return s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID, UserID: p.UserID}, f) //nolint:wrapcheck // InTx wraps its own failures
}

func failure(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return plxerr.New(plxerr.ResourceNotFound, "no such %s", what)
	}
	return fmt.Errorf("golden: %s: %w", what, err)
}

// scope validates the app and plugin key of a call.
func scope(appID, pluginKey string) (pgtype.UUID, error) {
	app, err := storage.UUID(appID)
	if err != nil {
		return pgtype.UUID{}, plxerr.New(plxerr.InvalidFormat, "the app identifier is not valid")
	}
	if len(pluginKey) > 64 {
		return pgtype.UUID{}, plxerr.New(plxerr.InvalidFormat, "the plugin key is not valid")
	}
	return app, nil
}

// objectKey is where an image lives: by the digest of the organisation,
// the app, the plugin and the image's hash, so an image belongs to one
// plugin and storing it twice is a no-op.
func objectKey(org, app, pluginKey string, sum []byte) (string, error) {
	h := sha256.New()
	for _, part := range []string{org, app, pluginKey} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	h.Write(sum)
	key, err := objects.Key(objects.KindGolden, hex.EncodeToString(h.Sum(nil)))
	if err != nil {
		return "", fmt.Errorf("golden: %w", err)
	}
	return key, nil
}

// Baselines returns the approved image hash of each case of a plugin.
func (s *Service) Baselines(ctx context.Context, p auth.Principal, appID, pluginKey string) (map[string][]byte, error) {
	if err := authorize(p, auth.PluginRead, appID); err != nil {
		return nil, err
	}
	app, err := scope(appID, pluginKey)
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListGoldenBaselines(ctx, dbgen.ListGoldenBaselinesParams{AppID: app, PluginKey: pluginKey})
		if err != nil {
			return failure(err, "golden baselines")
		}
		for _, r := range rows {
			out[r.CaseID] = r.ImageSha256
		}
		return nil
	})
	return out, err
}

// validate checks a run against the limits and for consistency.
func (s *Service) validate(r Run) error {
	if len(r.Bundle) != sha256Len || len(r.App) != sha256Len {
		return plxerr.New(plxerr.InvalidFormat, "the bundle hashes must be SHA-256 values")
	}
	if len(r.Cases) == 0 {
		return plxerr.New(plxerr.InvalidFormat, "a golden run needs at least one case")
	}
	if max := s.o.Limits.Get(limits.GoldenCasesPerRun); int64(len(r.Cases)) > max {
		return plxerr.New(plxerr.LimitExceeded, "the run has %d cases; golden.casesPerRun allows %d", len(r.Cases), max)
	}
	maxImage := s.o.Limits.Get(limits.GoldenImageBytes)
	seen := map[string]bool{}
	for _, c := range r.Cases {
		if c.ID == "" || len(c.ID) > 512 || seen[c.ID] {
			return plxerr.New(plxerr.InvalidFormat, "case IDs must be unique and 1 to 512 bytes; %q is not", clip(c.ID))
		}
		seen[c.ID] = true
		if len(c.SHA256) != sha256Len {
			return plxerr.New(plxerr.InvalidFormat, "case %q needs the SHA-256 of its image", c.ID)
		}
		if int64(len(c.Image)) > maxImage {
			return plxerr.New(plxerr.LimitExceeded, "the image of case %q has %d bytes; golden.imageBytes allows %d", c.ID, len(c.Image), maxImage)
		}
		if len(c.Image) > 0 {
			if sum := sha256.Sum256(c.Image); !bytes.Equal(sum[:], c.SHA256) {
				return plxerr.New(plxerr.InvalidFormat, "the image of case %q does not match its SHA-256", c.ID)
			}
		}
	}
	return nil
}

func clip(s string) string {
	if len(s) > 40 {
		return s[:40] + "..."
	}
	return s
}

// Upload records a run and returns its ID and the number of cases that
// differ from the baselines. A case that differs carries its image.
func (s *Service) Upload(ctx context.Context, p auth.Principal, r Run) (string, int, error) {
	if err := authorize(p, auth.PluginEdit, r.AppID); err != nil {
		return "", 0, err
	}
	app, err := scope(r.AppID, r.PluginKey)
	if err != nil {
		return "", 0, err
	}
	if err := s.validate(r); err != nil {
		return "", 0, err
	}
	id, err := s.o.IDs.New()
	if err != nil {
		return "", 0, fmt.Errorf("golden: %w", err)
	}
	runID, err := storage.UUID(id)
	if err != nil {
		return "", 0, fmt.Errorf("golden: %w", err)
	}
	org, err := storage.UUID(p.OrganizationID)
	if err != nil {
		return "", 0, fmt.Errorf("golden: %w", err)
	}
	differences := 0
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		if _, err := q.GetApp(ctx, app); err != nil {
			return failure(err, "app")
		}
		rows, err := q.ListGoldenBaselines(ctx, dbgen.ListGoldenBaselinesParams{AppID: app, PluginKey: r.PluginKey})
		if err != nil {
			return failure(err, "golden baselines")
		}
		baseline := make(map[string][]byte, len(rows))
		for _, b := range rows {
			baseline[b.CaseID] = b.ImageSha256
		}
		if err := q.InsertGoldenRun(ctx, dbgen.InsertGoldenRunParams{
			ID: runID, OrganizationID: org, AppID: app, PluginKey: r.PluginKey, BundleSha256: r.Bundle, AppSha256: r.App,
			Canonical: r.Canonical, Renderer: clip(r.Renderer), CreatedBy: p.Actor().ID,
		}); err != nil {
			return failure(err, "golden run")
		}
		for _, c := range slices.SortedFunc(slices.Values(r.Cases), func(a, b Case) int { return compareStrings(a.ID, b.ID) }) {
			if !bytes.Equal(baseline[c.ID], c.SHA256) {
				differences++
				if len(c.Image) == 0 {
					return plxerr.New(plxerr.InvalidFormat, "case %q differs from its baseline and needs its image", c.ID)
				}
				key, err := objectKey(p.OrganizationID, r.AppID, r.PluginKey, c.SHA256)
				if err != nil {
					return err
				}
				if _, err := s.o.Objects.Put(ctx, key, c.Image, "image/png"); err != nil {
					return fmt.Errorf("golden: store image: %w", err)
				}
			}
			if err := q.InsertGoldenCase(ctx, dbgen.InsertGoldenCaseParams{
				RunID: runID, CaseID: c.ID, OrganizationID: org, Frame: clip(c.Frame), Locale: clip(c.Locale), Theme: clip(c.Theme),
				TextScale: c.TextScale, ImageSha256: c.SHA256, ImageBytes: int64(len(c.Image)),
			}); err != nil {
				return failure(err, "golden case")
			}
		}
		return nil
	})
	if err != nil {
		return "", 0, err
	}
	return id, differences, nil
}

func compareStrings(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// Differences lists the cases of a run that differ from the baselines now.
func (s *Service) Differences(ctx context.Context, p auth.Principal, appID, runID string) ([]Difference, error) {
	if err := authorize(p, auth.PluginRead, appID); err != nil {
		return nil, err
	}
	app, run, err := ids(appID, runID)
	if err != nil {
		return nil, err
	}
	var out []Difference
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		if _, err := q.GetGoldenRun(ctx, dbgen.GetGoldenRunParams{ID: run, AppID: app}); err != nil {
			return failure(err, "golden run")
		}
		rows, err := q.ListGoldenDifferences(ctx, run)
		if err != nil {
			return failure(err, "golden differences")
		}
		for _, r := range rows {
			out = append(out, Difference{
				Case:     Case{ID: r.CaseID, Frame: r.Frame, Locale: r.Locale, Theme: r.Theme, TextScale: r.TextScale, SHA256: r.ImageSha256},
				Baseline: r.BaselineSha256,
			})
		}
		return nil
	})
	return out, err
}

func ids(appID, runID string) (pgtype.UUID, pgtype.UUID, error) {
	app, err := storage.UUID(appID)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, plxerr.New(plxerr.InvalidFormat, "the app identifier is not valid")
	}
	run, err := storage.UUID(runID)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, plxerr.New(plxerr.InvalidFormat, "the run identifier is not valid")
	}
	return app, run, nil
}

// Approve promotes the images of the differences of a run to baselines:
// those named by caseIDs, or all when none are named. It needs
// goldens.approve, which holds a second factor (SEC-100), and is audited.
func (s *Service) Approve(ctx context.Context, p auth.Principal, appID, runID string, caseIDs []string) ([]string, error) {
	if err := authorize(p, auth.GoldensApprove, appID); err != nil {
		return nil, err
	}
	app, run, err := ids(appID, runID)
	if err != nil {
		return nil, err
	}
	org, err := storage.UUID(p.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("golden: %w", err)
	}
	var approved []string
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		r, err := q.GetGoldenRun(ctx, dbgen.GetGoldenRunParams{ID: run, AppID: app})
		if err != nil {
			return failure(err, "golden run")
		}
		diffs, err := q.ListGoldenDifferences(ctx, run)
		if err != nil {
			return failure(err, "golden differences")
		}
		wanted := map[string]bool{}
		for _, id := range caseIDs {
			wanted[id] = true
		}
		used, err := q.GoldenBaselineBytes(ctx, dbgen.GoldenBaselineBytesParams{AppID: app, PluginKey: r.PluginKey})
		if err != nil {
			return failure(err, "golden baseline size")
		}
		maxTotal := s.o.Limits.Get(limits.GoldenBaselineBytesPerPlugin)
		for _, d := range diffs {
			if len(caseIDs) > 0 && !wanted[d.CaseID] {
				continue
			}
			delete(wanted, d.CaseID)
			if d.ImageBytes == 0 {
				return plxerr.New(plxerr.InvalidFormat, "case %q has no stored image to approve", d.CaseID)
			}
			key, err := objectKey(p.OrganizationID, appID, r.PluginKey, d.ImageSha256)
			if err != nil {
				return err
			}
			if _, err := s.o.Objects.Stat(ctx, key); err != nil {
				return fmt.Errorf("golden: image of case %q: %w", d.CaseID, err)
			}
			prev, err := baselineBytes(ctx, q, app, r.PluginKey, d.CaseID)
			if err != nil {
				return err
			}
			used += d.ImageBytes - prev
			if used > maxTotal {
				return plxerr.New(plxerr.LimitExceeded, "the baselines of this plugin would take %d bytes; golden.baselineBytesPerPlugin allows %d", used, maxTotal)
			}
			if err := q.UpsertGoldenBaseline(ctx, dbgen.UpsertGoldenBaselineParams{
				AppID: app, PluginKey: r.PluginKey, CaseID: d.CaseID, OrganizationID: org,
				ImageSha256: d.ImageSha256, ImageBytes: d.ImageBytes, ApprovedBy: p.Actor().ID,
			}); err != nil {
				return failure(err, "golden baseline")
			}
			approved = append(approved, d.CaseID)
		}
		if len(wanted) > 0 {
			return plxerr.New(plxerr.ResourceNotFound, "the run has no unapproved difference for %d of the named cases", len(wanted))
		}
		if len(approved) == 0 {
			return nil
		}
		e := audit.Entry{
			Action: audit.GoldensApproved, TargetKind: "golden_run", TargetID: runID,
			Detail: fmt.Sprintf("plugin %q: %d case(s)", r.PluginKey, len(approved)), OrganizationID: p.OrganizationID, Actor: p.Actor(),
		}
		if _, err := s.o.Audit.Append(ctx, tx, e); err != nil {
			return fmt.Errorf("golden: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return approved, nil
}

// baselineBytes is the size of the approved image of a case, 0 when there is
// none.
func baselineBytes(ctx context.Context, q *dbgen.Queries, app pgtype.UUID, pluginKey, caseID string) (int64, error) {
	size, err := q.GoldenBaselineSize(ctx, dbgen.GoldenBaselineSizeParams{AppID: app, PluginKey: pluginKey, CaseID: caseID})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, failure(err, "golden baseline")
	}
	return size, nil
}

// StatusOf is the state of the run for the exact bundles, as the publish
// gate sees it.
func (s *Service) StatusOf(ctx context.Context, p auth.Principal, appID, pluginKey string, bundle, appBundle []byte) (Status, error) {
	if err := authorize(p, auth.PluginRead, appID); err != nil {
		return Status{}, err
	}
	app, err := scope(appID, pluginKey)
	if err != nil {
		return Status{}, err
	}
	var st Status
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		st, err = Lookup(ctx, tx, app, pluginKey, bundle, appBundle)
		return err
	})
	return st, err
}

// Lookup reads the state of the newest canonical run for the exact bundles
// inside a tenant transaction: what StatusOf answers and the publish gate
// asks (TST-003). A run that is not canonical never counts.
func Lookup(ctx context.Context, tx pgx.Tx, app pgtype.UUID, pluginKey string, bundle, appBundle []byte) (Status, error) {
	q := dbgen.New(tx)
	run, err := q.LatestCanonicalGoldenRun(ctx, dbgen.LatestCanonicalGoldenRunParams{
		AppID: app, PluginKey: pluginKey, BundleSha256: bundle, AppSha256: appBundle,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Status{}, nil
	}
	if err != nil {
		return Status{}, failure(err, "golden run")
	}
	total, err := q.CountGoldenCases(ctx, run.ID)
	if err != nil {
		return Status{}, failure(err, "golden cases")
	}
	diffs, err := q.ListGoldenDifferences(ctx, run.ID)
	if err != nil {
		return Status{}, failure(err, "golden differences")
	}
	return Status{Found: true, RunID: storage.ID(run.ID), Cases: int(total), Differences: len(diffs)}, nil
}

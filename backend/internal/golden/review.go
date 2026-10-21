// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package golden

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// ReviewImage is a case that differs from its baseline, with both images as
// PNG: what an approver of a release looks at (GOV-023).
type ReviewImage struct {
	PluginKey string
	CaseID    string
	// Baseline is the approved image, nil when the case has no baseline yet.
	Baseline []byte
	// Actual is the image the run produced.
	Actual []byte
}

// ReviewImages returns the cases of the newest canonical run for the exact
// bundles that differ from the baselines the server holds, with both images,
// inside a tenant transaction. It returns nothing when no canonical run
// exists, which the publish gate reports separately (TST-003, TST-005).
func (s *Service) ReviewImages(ctx context.Context, tx pgx.Tx, organizationID, appID, pluginKey string, bundle, appBundle []byte) ([]ReviewImage, error) {
	app, err := scope(appID, pluginKey)
	if err != nil {
		return nil, err
	}
	q := dbgen.New(tx)
	run, err := q.LatestCanonicalGoldenRun(ctx, dbgen.LatestCanonicalGoldenRunParams{
		AppID: app, PluginKey: pluginKey, BundleSha256: bundle, AppSha256: appBundle,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, failure(err, "golden run")
	}
	rows, err := q.ListGoldenDifferences(ctx, run.ID)
	if err != nil {
		return nil, failure(err, "golden differences")
	}
	out := make([]ReviewImage, 0, len(rows))
	for _, r := range rows {
		img := ReviewImage{PluginKey: pluginKey, CaseID: r.CaseID}
		if img.Actual, err = s.image(ctx, organizationID, appID, pluginKey, r.ImageSha256); err != nil {
			return nil, err
		}
		if len(r.BaselineSha256) > 0 {
			if img.Baseline, err = s.image(ctx, organizationID, appID, pluginKey, r.BaselineSha256); err != nil {
				return nil, err
			}
		}
		out = append(out, img)
	}
	return out, nil
}

// image reads a stored image by its hash.
func (s *Service) image(ctx context.Context, organizationID, appID, pluginKey string, sum []byte) ([]byte, error) {
	key, err := objectKey(organizationID, appID, pluginKey, sum)
	if err != nil {
		return nil, err
	}
	data, _, err := s.o.Objects.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("golden: read the image %x: %w", sum[:8], err)
	}
	return data, nil
}

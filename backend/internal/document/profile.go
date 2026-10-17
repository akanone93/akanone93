// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package document

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/schema/jcs"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// AppProfile reads the securityProfile of an app's app.json (SEC-180): in
// the snapshot when snapshotID is not empty, else in the app's current
// draft. Without a document or a profile it is the standard profile. It
// runs in the caller's transaction.
func (s *Service) AppProfile(ctx context.Context, tx pgx.Tx, appID, snapshotID string) (schema.SecurityProfile, error) {
	q := dbgen.New(tx)
	var sum []byte
	var org pgtype.UUID
	if snapshotID != "" {
		sid, err := parseID(snapshotID, "snapshot")
		if err != nil {
			return "", err
		}
		snap, err := q.GetSnapshot(ctx, sid)
		if err != nil {
			return "", failure(err, "snapshot")
		}
		st, err := s.state(ctx, q, snap)
		if err != nil {
			return "", err
		}
		e, ok := st[appDocument]
		if !ok {
			return schema.SecurityProfileStandard, nil
		}
		sum, org = e.sum, snap.OrganizationID
	} else {
		app, err := parseID(appID, "app")
		if err != nil {
			return "", err
		}
		d, err := q.GetDraft(ctx, dbgen.GetDraftParams{AppID: app, PluginID: pgtype.UUID{}})
		if errors.Is(err, pgx.ErrNoRows) {
			return schema.SecurityProfileStandard, nil
		}
		if err != nil {
			return "", fmt.Errorf("document: read the app draft: %w", err)
		}
		row, err := q.GetDocument(ctx, dbgen.GetDocumentParams{DraftID: d.ID, Path: appDocument})
		if errors.Is(err, pgx.ErrNoRows) {
			return schema.SecurityProfileStandard, nil
		}
		if err != nil {
			return "", fmt.Errorf("document: read %s: %w", appDocument, err)
		}
		sum, org = row.Sha256, d.OrganizationID
	}
	content, err := s.blob(ctx, q, org, sum)
	if err != nil {
		return "", err
	}
	tree, err := jcs.Parse(content, int(s.o.Limits.Get(limits.DocumentJSONDepth)))
	if err != nil {
		return "", fmt.Errorf("document: read %s: %w", appDocument, err)
	}
	obj, _ := tree.(map[string]any)
	if profile, ok := obj["securityProfile"].(string); ok && schema.SecurityProfile(profile).Valid() {
		return schema.SecurityProfile(profile), nil
	}
	return schema.SecurityProfileStandard, nil
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package analytics

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/analytics/symbolicate"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
	"github.com/nightCode42/plux3/backend/internal/storage/objects"
)

// symbolOutcome is what trying to symbolicate a group came to.
type symbolOutcome struct {
	// Tried says the attempt is final and is recorded, so it is not repeated:
	// either a location was found, or the release cannot give one.
	Tried bool
	Where symbolicate.Location
}

// symbolArtifacts finds the hashes of the plugin bundle and source map of
// the group's release, in the caller's transaction. ok is false when the
// group has nothing to resolve or the release has no such plugin version.
func symbolArtifacts(ctx context.Context, q *dbgen.Queries, g dbgen.ErrorGroup) (art dbgen.ReleasePluginArtifactsRow, ok bool, err error) {
	if g.NodePath == "" || g.SymbolicatedAt.Valid {
		return art, false, nil
	}
	art, err = q.ReleasePluginArtifacts(ctx, dbgen.ReleasePluginArtifactsParams{AppID: g.AppID, Sequence: g.ReleaseSequence, PluginKey: g.PluginKey})
	if errors.Is(err, pgx.ErrNoRows) {
		return art, false, nil
	}
	if err != nil {
		return art, false, fmt.Errorf("analytics: find the release's source map: %w", err)
	}
	return art, len(art.SourceMapSha256) > 0, nil
}

// symbolicateGroup resolves a group's node path from the stored bundle and
// source map, outside any transaction. A node that is not in the map is a
// final answer; a failure to read storage or an object that does not match
// its hash is an error, and nothing is recorded.
func (s *Service) symbolicateGroup(ctx context.Context, g dbgen.ErrorGroup, art dbgen.ReleasePluginArtifactsRow) (symbolOutcome, error) {
	data, err := s.verifiedObject(ctx, art.BundleSha256)
	if err != nil {
		return symbolOutcome{}, err
	}
	sourceMap, err := s.verifiedObject(ctx, art.SourceMapSha256)
	if err != nil {
		return symbolOutcome{}, err
	}
	where, err := symbolicate.Resolve(data, sourceMap, g.NodePath)
	switch {
	case errors.Is(err, symbolicate.ErrNotFound):
		return symbolOutcome{Tried: true}, nil
	case err != nil:
		return symbolOutcome{}, fmt.Errorf("analytics: %w", err)
	}
	return symbolOutcome{Tried: true, Where: where}, nil
}

// verifiedObject reads a bundle or source map and checks it against the hash
// that names it, so a damaged object is reported, never parsed.
func (s *Service) verifiedObject(ctx context.Context, sum []byte) ([]byte, error) {
	key, err := objects.Key(objects.KindBundle, hex.EncodeToString(sum))
	if err != nil {
		return nil, fmt.Errorf("analytics: %w", err)
	}
	data, _, err := s.o.Objects.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("analytics: read %s: %w", key, err)
	}
	if got := sha256.Sum256(data); !bytes.Equal(got[:], sum) {
		return nil, fmt.Errorf("analytics: %s does not match its hash", key)
	}
	return data, nil
}

// recordSymbol stores the outcome of symbolicating a group.
func (s *Service) recordSymbol(ctx context.Context, q *dbgen.Queries, g *dbgen.ErrorGroup, o symbolOutcome) error {
	if !o.Tried {
		return nil
	}
	now := s.now()
	if err := q.SetErrorGroupSymbol(ctx, dbgen.SetErrorGroupSymbolParams{
		ID: g.ID, JsonPath: o.Where.Pointer, SourceFile: o.Where.File, SymbolicatedAt: storage.Timestamp(now),
	}); err != nil {
		return fmt.Errorf("analytics: record the document path: %w", err)
	}
	g.JsonPath, g.SourceFile, g.SymbolicatedAt = o.Where.Pointer, o.Where.File, storage.Timestamp(now)
	return nil
}

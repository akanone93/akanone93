// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
	"github.com/nightCode42/plux3/backend/internal/storage/objects"
)

// archiveMediaType is how a source archive is stored.
const archiveMediaType = "application/gzip"

// UploadSource stores a source archive of the function name in an
// environment, read from r. The archive is hashed as it arrives and is
// bounded by function.sourceArchiveSize; one byte more is refused with
// PLX-7203 without reading the rest (FN-016). The archive is kept under
// its SHA-256, which the build's provenance records (FN-005).
func (s *Service) UploadSource(ctx context.Context, p auth.Principal, appID, environmentID, name string, r io.Reader) (Source, error) {
	if err := authorize(p, auth.FunctionDeploy, appID); err != nil {
		return Source{}, err
	}
	if err := ValidateName(name); err != nil {
		return Source{}, err
	}
	var most int64
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		env, err := environmentOf(ctx, dbgen.New(tx), appID, environmentID)
		if err != nil {
			return err
		}
		lim, err := s.o.Limits(ctx, tx, storage.ID(env.OrganizationID), appID)
		if err != nil {
			return fmt.Errorf("function: %w", err)
		}
		most = lim.Get(limits.FunctionSourceArchiveSize)
		return nil
	})
	if err != nil {
		return Source{}, err
	}
	data, sum, err := readBounded(r, most)
	if err != nil {
		return Source{}, err
	}
	key, err := objects.Key(objects.KindSource, fmt.Sprintf("%x", sum))
	if err != nil {
		return Source{}, fmt.Errorf("function: %w", err)
	}
	if _, err := s.o.Objects.Put(ctx, key, data, archiveMediaType); err != nil {
		return Source{}, fmt.Errorf("function: store a source archive: %w", err)
	}
	var out Source
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = s.insertSource(ctx, tx, p, appID, environmentID, name, key, sum[:], int64(len(data)))
		return err
	})
	return out, err
}

// readBounded reads r to its end, refusing more than most bytes, and
// returns the bytes with their SHA-256.
func readBounded(r io.Reader, most int64) ([]byte, [sha256.Size]byte, error) {
	var sum [sha256.Size]byte
	h := sha256.New()
	var buf bytes.Buffer
	n, err := io.Copy(io.MultiWriter(&buf, h), io.LimitReader(r, most+1))
	if err != nil {
		return nil, sum, fmt.Errorf("function: read a source archive: %w", err)
	}
	switch {
	case n > most:
		return nil, sum, plxerr.New(plxerr.FunctionBuildLimitExceeded, "the source archive is above function.sourceArchiveSize = %d bytes", most)
	case n == 0:
		return nil, sum, plxerr.New(plxerr.InvalidFormat, "the source archive is empty")
	}
	h.Sum(sum[:0])
	return buf.Bytes(), sum, nil
}

// insertSource records a stored archive.
func (s *Service) insertSource(ctx context.Context, tx pgx.Tx, p auth.Principal, appID, environmentID, name, key string, sum []byte, size int64) (Source, error) {
	q := dbgen.New(tx)
	env, err := environmentOf(ctx, q, appID, environmentID)
	if err != nil {
		return Source{}, err
	}
	id, err := s.newID()
	if err != nil {
		return Source{}, err
	}
	row, err := q.InsertFunctionSource(ctx, dbgen.InsertFunctionSourceParams{
		ID: storage.MustUUID(id), OrganizationID: env.OrganizationID, AppID: env.AppID, EnvironmentID: env.ID,
		Name: name, ObjectKey: key, Sha256: sum, SizeBytes: size, CreatedBy: createdBy(p),
	})
	if err != nil {
		return Source{}, failure(err, "function source")
	}
	detail := fmt.Sprintf("app %s, environment %s, function %s, %d bytes, sha256 %x", appID, environmentID, name, size, sum)
	if err := s.record(ctx, tx, p, audit.FunctionSourceUploaded, "function_source", id, detail); err != nil {
		return Source{}, err
	}
	return sourceOf(row), nil
}

// sourceOf converts a row.
func sourceOf(row dbgen.FunctionSource) Source {
	return Source{
		ID: storage.ID(row.ID), AppID: storage.ID(row.AppID), EnvironmentID: storage.ID(row.EnvironmentID),
		Name: row.Name, ObjectKey: row.ObjectKey, SHA256: row.Sha256, Size: row.SizeBytes, CreatedAt: storage.Time(row.CreatedAt),
	}
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package document

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// Upload is the progress of one chunked upload (SRV-060).
type Upload struct {
	ID        string
	ChunkSize int64
	Size      int64
	Received  int64
	ExpiresAt time.Time
}

// uploadTTL is how long an upload lives past its last chunk.
func (s *Service) uploadTTL() time.Duration {
	return time.Duration(s.o.Limits.Get(limits.AssetUploadSessionTtl)) * time.Millisecond
}

// BeginAssetUpload opens a chunked upload of one file whose size and
// SHA-256 are declared up front. Bytes accumulate in the upload session;
// FinishAssetUpload verifies them and hands the file to UploadAsset, so
// the streaming and chunked paths validate and store identically.
func (s *Service) BeginAssetUpload(ctx context.Context, p auth.Principal, appID, pluginID, session, name string, size int64, sum []byte) (Upload, error) {
	if err := s.assetsRequired(); err != nil {
		return Upload{}, err
	}
	if err := authorize(p, auth.AppManage, appID); err != nil {
		return Upload{}, err
	}
	app, err := parseID(appID, "app")
	if err != nil {
		return Upload{}, err
	}
	if len(sum) != sha256.Size {
		return Upload{}, plxerr.New(plxerr.InvalidFormat, "sha256 is the 32-byte digest of the whole file")
	}
	lim, err := s.appLimits(ctx, p, appID)
	if err != nil {
		return Upload{}, err
	}
	if max := lim.Get(limits.AssetFileSize); size <= 0 || size > max {
		return Upload{}, plxerr.New(plxerr.AssetUploadTooLarge, "the file is above asset.fileSize = %d bytes", max)
	}
	id, err := s.newID()
	if err != nil {
		return Upload{}, err
	}
	now := s.now()
	u := Upload{ID: id, ChunkSize: s.o.Limits.Get(limits.AssetUploadChunkBytes), Size: size, ExpiresAt: now.Add(s.uploadTTL())}
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM upload_sessions WHERE expires_at <= $1`, now); err != nil {
			return fmt.Errorf("document: sweep uploads: %w", err)
		}
		_, err := tx.Exec(ctx, `INSERT INTO upload_sessions (id, organization_id, user_id, app_id, plugin_id, editor_session, name, size, sha256, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`, id, p.OrganizationID, p.UserID, app, pluginID, session, name, size, sum, u.ExpiresAt)
		if err != nil {
			return fmt.Errorf("document: begin upload: %w", err)
		}
		return nil
	})
	return u, err
}

// uploadRow is a live upload session of the caller.
type uploadRow struct {
	appID, pluginID, session, name string
	size, received                 int64
	sum, data                      []byte
	user                           string
	expires                        time.Time
}

var errNoUpload = plxerr.New(plxerr.AssetUploadNotFound, "no such upload, or it has expired")

// loadUpload reads a live upload of the caller, after checking the caller may
// manage its app; withData also reads its bytes.
func (s *Service) loadUpload(ctx context.Context, tx pgx.Tx, p auth.Principal, id string, withData bool) (uploadRow, error) {
	var r uploadRow
	cols := "app_id::text, plugin_id, editor_session, name, size, received, sha256, expires_at, ''::bytea, user_id::text"
	if withData {
		cols = "app_id::text, plugin_id, editor_session, name, size, received, sha256, expires_at, data, user_id::text"
	}
	err := tx.QueryRow(ctx, `SELECT `+cols+` FROM upload_sessions WHERE id::text = $1 AND expires_at > $2`,
		id, s.now()).Scan(&r.appID, &r.pluginID, &r.session, &r.name, &r.size, &r.received, &r.sum, &r.expires, &r.data, &r.user)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, errNoUpload
	}
	if err != nil {
		return r, fmt.Errorf("document: read upload: %w", err)
	}
	if err := authorize(p, auth.AppManage, r.appID); err != nil {
		return uploadRow{}, err
	}
	if r.user != p.UserID {
		return uploadRow{}, errNoUpload
	}
	return r, nil
}

// GetAssetUpload reports how many bytes an upload has, for resuming.
func (s *Service) GetAssetUpload(ctx context.Context, p auth.Principal, id string) (Upload, error) {
	var u Upload
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		r, err := s.loadUpload(ctx, tx, p, id, false)
		u = Upload{ID: id, Size: r.size, Received: r.received, ExpiresAt: r.expires}
		return err
	})
	return u, err
}

// UploadAssetChunk appends a chunk at the offset the upload has reached;
// any other offset is refused, so chunks cannot arrive out of order.
func (s *Service) UploadAssetChunk(ctx context.Context, p auth.Principal, id string, offset int64, chunk []byte) (Upload, error) {
	if max := s.o.Limits.Get(limits.AssetUploadChunkBytes); int64(len(chunk)) > max {
		return Upload{}, plxerr.New(plxerr.AssetUploadChunkTooLarge, "the chunk is above asset.uploadChunkBytes = %d bytes", max)
	}
	var u Upload
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		r, err := s.loadUpload(ctx, tx, p, id, false)
		if err != nil {
			return err
		}
		if offset != r.received {
			return plxerr.New(plxerr.AssetUploadOutOfOrder, "the chunk is at offset %d but the upload has %d bytes", offset, r.received)
		}
		if r.received+int64(len(chunk)) > r.size {
			return plxerr.New(plxerr.AssetUploadSizeMismatch, "the chunk goes past the declared size of %d bytes", r.size)
		}
		u = Upload{ID: id, Size: r.size, Received: r.received + int64(len(chunk)), ExpiresAt: s.now().Add(s.uploadTTL())}
		tag, err := tx.Exec(ctx, `UPDATE upload_sessions SET data = data || $2, received = received + $3, expires_at = $4
			WHERE id::text = $1 AND received = $5`, id, chunk, len(chunk), u.ExpiresAt, offset)
		if err != nil {
			return fmt.Errorf("document: store chunk: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return plxerr.New(plxerr.AssetUploadOutOfOrder, "the upload moved on; ask for its progress")
		}
		return nil
	})
	return u, err
}

// FinishAssetUpload verifies the whole file against its declared size and
// SHA-256, then stores it through UploadAsset.
func (s *Service) FinishAssetUpload(ctx context.Context, p auth.Principal, id string) (Asset, error) {
	var r uploadRow
	if err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		r, err = s.loadUpload(ctx, tx, p, id, true)
		return err
	}); err != nil {
		return Asset{}, err
	}
	drop := func() error {
		return s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `DELETE FROM upload_sessions WHERE id::text = $1`, id)
			return err //nolint:wrapcheck // wrapped by the caller's path
		})
	}
	if r.received != r.size {
		return Asset{}, plxerr.New(plxerr.AssetUploadSizeMismatch, "the upload has %d of %d bytes", r.received, r.size)
	}
	if sum := sha256.Sum256(r.data); !bytes.Equal(sum[:], r.sum) {
		if err := drop(); err != nil {
			return Asset{}, fmt.Errorf("document: drop upload: %w", err)
		}
		return Asset{}, plxerr.New(plxerr.AssetUploadHashMismatch, "the SHA-256 of the received bytes is not the one declared")
	}
	a, err := s.UploadAsset(ctx, p, r.appID, r.pluginID, r.session, r.name, r.data)
	if err != nil {
		return Asset{}, err
	}
	if err := drop(); err != nil {
		return Asset{}, fmt.Errorf("document: drop upload: %w", err)
	}
	return a, nil
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"image"
	"image/png"
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// Verifies: SRV-060.
// A file is uploaded through unary chunked calls: out-of-order chunks, a
// wrong digest, an oversize declaration and an expired upload are refused,
// and a finished upload stores the same asset the streaming path does.
func TestChunkedAssetUpload(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	ctx := context.Background()
	_, invitation, err := w.auth.Bootstrap(ctx, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	must(w.identity.AcceptInvitation(ctx, connect.NewRequest(&pluxv1.AcceptInvitationRequest{
		Invitation: invitation, DisplayName: "Admin", Password: "correct horse battery",
	})))(t)
	admin := w.signIn(t, "admin@example.com")
	admin.org = must(w.org.CreateOrganization(ctx, req(admin, &pluxv1.CreateOrganizationRequest{Key: "acme", Name: "Acme"})))(t).GetOrganization().GetId()
	app := must(w.app.CreateApp(ctx, req(admin, &pluxv1.CreateAppRequest{Key: "demo", Name: "Demo"})))(t).GetApp().GetId()
	must(w.plugin.AcquireLock(ctx, req(admin, &pluxv1.AcquireLockRequest{AppId: app, Session: "s"})))(t)

	var img bytes.Buffer
	if err := png.Encode(&img, image.NewNRGBA(image.Rect(0, 0, 16, 16))); err != nil {
		t.Fatal(err)
	}
	data := img.Bytes()
	sum := sha256.Sum256(data)
	begin := func(name string, size int64, digest []byte) (*connect.Response[pluxv1.BeginAssetUploadResponse], error) {
		return w.asset.BeginAssetUpload(ctx, req(admin, &pluxv1.BeginAssetUploadRequest{AppId: app, Name: name, Session: "s", Size: size, Sha256: digest}))
	}
	chunk := func(id string, off int64, b []byte) (*connect.Response[pluxv1.UploadAssetChunkResponse], error) {
		return w.asset.UploadAssetChunk(ctx, req(admin, &pluxv1.UploadAssetChunkRequest{UploadId: id, Offset: off, Chunk: b}))
	}
	half := len(data) / 2

	id := must(begin("images/a.png", int64(len(data)), sum[:]))(t).GetUploadId()
	if _, err := chunk(id, int64(half), data[half:]); codeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a chunk out of order: %v", err)
	}
	if reason := uploadReasonOf(t, err); reason != "ASSET_UPLOAD_OUT_OF_ORDER" {
		t.Errorf("a chunk out of order reason = %s", reason)
	}
	if r := must(chunk(id, 0, data[:half]))(t); r.GetReceived() != int64(half) {
		t.Errorf("received = %d", r.GetReceived())
	}
	if _, err := chunk(id, 0, data[:half]); codeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a repeated chunk: %v", err)
	}
	if reason := uploadReasonOf(t, err); reason != "ASSET_UPLOAD_OUT_OF_ORDER" {
		t.Errorf("a repeated chunk reason = %s", reason)
	}
	if _, err := w.asset.FinishAssetUpload(ctx, req(admin, &pluxv1.FinishAssetUploadRequest{UploadId: id})); codeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("an unfinished upload: %v", err)
	}
	if reason := uploadReasonOf(t, err); reason != "ASSET_UPLOAD_SIZE_MISMATCH" {
		t.Errorf("an unfinished upload reason = %s", reason)
	}
	if st := must(w.asset.GetAssetUpload(ctx, req(admin, &pluxv1.GetAssetUploadRequest{UploadId: id})))(t); st.GetReceived() != int64(half) {
		t.Errorf("progress = %d", st.GetReceived())
	}
	must(chunk(id, int64(half), data[half:]))(t)
	a := must(w.asset.FinishAssetUpload(ctx, req(admin, &pluxv1.FinishAssetUploadRequest{UploadId: id})))(t).GetAsset()
	if a.GetMediaType() != "image/png" || a.GetName() != "images/a.png" || a.GetWidth() != 16 {
		t.Errorf("finished asset %+v", a)
	}
	if _, err := w.asset.GetAssetUpload(ctx, req(admin, &pluxv1.GetAssetUploadRequest{UploadId: id})); codeOf(err) != connect.CodeNotFound {
		t.Errorf("a finished upload: %v", err)
	}
	if reason := uploadReasonOf(t, err); reason != "ASSET_UPLOAD_NOT_FOUND" {
		t.Errorf("a finished upload reason = %s", reason)
	}

	bad := sha256.Sum256([]byte("other"))
	id = must(begin("images/b.png", int64(len(data)), bad[:]))(t).GetUploadId()
	must(chunk(id, 0, data))(t)
	if _, err := w.asset.FinishAssetUpload(ctx, req(admin, &pluxv1.FinishAssetUploadRequest{UploadId: id})); codeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a digest mismatch: %v", err)
	}
	if reason := uploadReasonOf(t, err); reason != "ASSET_UPLOAD_HASH_MISMATCH" {
		t.Errorf("a digest mismatch reason = %s", reason)
	}

	if _, err := begin("images/c.png", 1<<40, sum[:]); codeOf(err) != connect.CodeResourceExhausted {
		t.Errorf("an oversize declaration: %v", err)
	}
	if reason := uploadReasonOf(t, err); reason != "ASSET_UPLOAD_TOO_LARGE" {
		t.Errorf("an oversize declaration reason = %s", reason)
	}
	if _, err := begin("images/c.png", 10, sum[:4]); codeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a short digest: %v", err)
	}
	b := must(begin("images/d.png", int64(len(data)), sum[:]))(t)
	if _, err := chunk(b.GetUploadId(), 0, make([]byte, b.GetChunkSize()+1)); codeOf(err) != connect.CodeResourceExhausted {
		t.Errorf("an oversize chunk: %v", err)
	}
	if reason := uploadReasonOf(t, err); reason != "ASSET_UPLOAD_CHUNK_TOO_LARGE" {
		t.Errorf("an oversize chunk reason = %s", reason)
	}
	if _, err := chunk(b.GetUploadId(), 0, append(data, 0)); codeOf(err) != connect.CodeResourceExhausted {
		t.Errorf("a chunk past the declared size: %v", err)
	}
	if reason := uploadReasonOf(t, err); reason != "ASSET_UPLOAD_SIZE_MISMATCH" {
		t.Errorf("a chunk past the declared size reason = %s", reason)
	}

	err = w.db.InTx(ctx, storage.Tenant{OrganizationID: admin.org}, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE upload_sessions SET expires_at = now() - interval '1 second'`)
		return err //nolint:wrapcheck // a test
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := chunk(b.GetUploadId(), 0, data); codeOf(err) != connect.CodeNotFound {
		t.Errorf("an expired upload: %v", err)
	}
	if reason := uploadReasonOf(t, err); reason != "ASSET_UPLOAD_NOT_FOUND" {
		t.Errorf("an expired upload reason = %s", reason)
	}
}

// reasonOf returns the registered reason an error carries, or "" for none.
func uploadReasonOf(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		return ""
	}
	return errorInfo(t, err).GetReason()
}

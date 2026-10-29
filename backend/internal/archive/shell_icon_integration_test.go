// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package archive_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/generator"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/shellupdate"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
	"github.com/nightCode42/plux3/backend/internal/storage/objects"
)

func squarePNG(t *testing.T, side int, c color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, side, side))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// addAsset stores a file in the object store and lists it as an asset of an
// app, as the document service's upload does.
func (f *fixture) addAsset(t *testing.T, p auth.Principal, appID, file, mediaType string, data []byte) (assetID string, sha string) {
	t.Helper()
	ctx := context.Background()
	sum := sha256.Sum256(data)
	sha = hex.EncodeToString(sum[:])
	key, err := objects.Key(objects.KindAsset, sha)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.objects.Put(ctx, key, data, mediaType); err != nil {
		t.Fatal(err)
	}
	assetID, rowID := mustID(t, f), mustID(t, f)
	err = f.db.InTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID, UserID: p.UserID}, func(ctx context.Context, tx pgx.Tx) error {
		_, err := dbgen.New(tx).InsertAsset(ctx, dbgen.InsertAssetParams{
			ID: storage.MustUUID(rowID), AssetID: storage.MustUUID(assetID), OrganizationID: storage.MustUUID(p.OrganizationID),
			AppID: storage.MustUUID(appID), File: file, MediaType: mediaType, Sha256: sum[:], Size: int64(len(data)),
			Processing: "ready", UploadedByKind: "user", UploadedByID: p.UserID, UploadedBy: "test",
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return assetID, sha
}

func mustID(t *testing.T, f *fixture) string {
	t.Helper()
	id, err := f.gen.New()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// runJob asks for a project and runs the queued job.
func (f *fixture) runJob(t *testing.T, appID, format string) shellupdate.ProjectJob {
	t.Helper()
	ctx := context.Background()
	job, err := f.shells.Generate(ctx, f.owner, appID, "development", "production", format)
	if err != nil {
		t.Fatalf("Generate %s: %v", format, err)
	}
	if err := f.shells.Work(ctx, f.queue.jobs[len(f.queue.jobs)-1]); err != nil {
		t.Fatalf("Work: %v", err)
	}
	done, err := f.shells.Job(ctx, f.owner, appID, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	return done
}

func readZip(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]byte{}
	for _, e := range zr.File {
		r, err := e.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(r)
		_ = r.Close()
		if err != nil {
			t.Fatal(err)
		}
		out[e.Name] = b
	}
	return out
}

// TestShellIconAndPatch_GEN_004 covers the app icon (its checks under row-level
// security, its change detection and its use in the project) and the patch
// format (its content, its preconditions).
func TestShellIconAndPatch_GEN_004(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	app, err := f.tenancy.CreateApp(ctx, f.owner, "shop", "Shop")
	if err != nil {
		t.Fatal(err)
	}
	sibling, err := f.tenancy.CreateApp(ctx, f.owner, "sibling", "Sibling")
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := f.tenancy.CreateApp(ctx, f.other, "foreign", "Foreign")
	if err != nil {
		t.Fatal(err)
	}
	red := squarePNG(t, 1024, color.NRGBA{R: 200, A: 255})
	blue := squarePNG(t, 1024, color.NRGBA{B: 200, A: 255})
	redID, redSHA := f.addAsset(t, f.owner, app.ID, "images/red.png", "image/png", red)
	blueID, blueSHA := f.addAsset(t, f.owner, app.ID, "images/blue.png", "image/png", blue)
	siblingID, _ := f.addAsset(t, f.owner, sibling.ID, "images/red.png", "image/png", red)
	foreignID, _ := f.addAsset(t, f.other, foreign.ID, "images/red.png", "image/png", red)
	smallID, _ := f.addAsset(t, f.owner, app.ID, "images/small.png", "image/png", squarePNG(t, 512, color.NRGBA{A: 255}))
	jpegID, _ := f.addAsset(t, f.owner, app.ID, "images/photo.jpg", "image/jpeg", []byte("jpeg"))

	st, err := f.shells.Status(ctx, f.owner, app.ID)
	if err != nil {
		t.Fatal(err)
	}
	// An icon that is not the app's own, not a PNG or not usable is refused
	// with one answer, so another app's or organisation's asset is not confirmed.
	var first error
	for name, id := range map[string]string{
		"another app of the organisation": siblingID, "another organisation": foreignID, "unknown": mustID(t, f),
		"not an identifier": "nope", "too small": smallID, "not a PNG": jpegID,
	} {
		spec := st.Current
		spec.IconAssetID = id
		_, err := f.shells.SetSettings(ctx, f.owner, app.ID, spec)
		if code(err) != plxerr.InvalidFormat {
			t.Errorf("%s: %v", name, err)
		}
		if name == "another organisation" || name == "unknown" {
			if first == nil {
				first = err
			} else if err.Error() != first.Error() {
				t.Errorf("an unknown asset and another organisation's asset answer differently: %q %q", first, err)
			}
		}
	}
	if _, err := f.shells.Generate(ctx, f.owner, app.ID, "development", "production", "patch"); code(err) != plxerr.PreconditionFailed {
		t.Errorf("a patch before any generation: %v", err)
	}
	if _, err := f.shells.Generate(ctx, f.owner, app.ID, "development", "production", "tar"); code(err) != plxerr.InvalidEnumValue {
		t.Errorf("an unknown format: %v", err)
	}

	set := st.Current
	set.IconAssetID = redID
	if _, err := f.shells.SetSettings(ctx, f.owner, app.ID, set); err != nil {
		t.Fatalf("a valid icon: %v", err)
	}
	if done := f.runJob(t, app.ID, ""); done.Status != shellupdate.StatusSucceeded {
		t.Fatalf("zip job: %+v", done)
	}
	st, err = f.shells.Status(ctx, f.owner, app.ID)
	if err != nil || st.Generated == nil || st.Generated.IconSHA256 != redSHA || st.Current.IconSHA256 != redSHA || len(st.Changes) != 0 {
		t.Fatalf("status after generating with the icon: %+v %v", st, err)
	}

	// A new icon is a change, with a reason, and the patch carries it.
	set.IconAssetID, set.AppVersion = blueID, "1.1.0+2"
	if _, err := f.shells.SetSettings(ctx, f.owner, app.ID, set); err != nil {
		t.Fatal(err)
	}
	if st, err = f.shells.Status(ctx, f.owner, app.ID); err != nil {
		t.Fatal(err)
	}
	var icon *shellupdate.Change
	for i := range st.Changes {
		if st.Changes[i].Field == "icon" {
			icon = &st.Changes[i]
		}
	}
	if icon == nil || icon.Old != redSHA || icon.New != blueSHA || icon.Reason == "" || len(st.Changes) != 2 {
		t.Fatalf("changes: %+v", st.Changes)
	}
	if _, err := f.shells.Generate(ctx, f.owner, app.ID, "development", "production", "patch"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.shells.Generate(ctx, f.owner, app.ID, "development", "production", "zip"); code(err) != plxerr.PreconditionFailed {
		t.Errorf("a zip while a patch is queued: %v", err)
	}
	if err := f.shells.Work(ctx, f.queue.jobs[len(f.queue.jobs)-1]); err != nil {
		t.Fatal(err)
	}
	patchJob := f.lastJob(t, app.ID)
	if patchJob.Status != shellupdate.StatusSucceeded || patchJob.Format != shellupdate.FormatPatch {
		t.Fatalf("patch job: %+v", patchJob)
	}
	_, data, err := f.shells.Download(ctx, f.owner, app.ID, patchJob.ID)
	if err != nil {
		t.Fatal(err)
	}
	files := readZip(t, data)
	var m generator.PatchManifest
	if err := json.Unmarshal(files["shop/"+generator.PatchFile], &m); err != nil {
		t.Fatalf("manifest: %v", err)
	}
	changed := func() []string {
		var out []string
		for _, e := range m.Changed {
			out = append(out, e.Path)
		}
		return out
	}()
	if m.From.AppVersion != "1.0.0+1" || m.To.AppVersion != "1.1.0+2" || m.To.IconSHA256 != blueSHA || m.From.IconSHA256 != redSHA {
		t.Errorf("manifest specs: %+v %+v", m.From, m.To)
	}
	for _, want := range []string{"pubspec.yaml", "plux.shell.json", "android/app/src/main/res/mipmap-hdpi/ic_launcher.png"} {
		if !slices.Contains(changed, want) {
			t.Errorf("%s is not in the patch: %v", want, changed)
		}
		if _, ok := files["shop/"+want]; !ok {
			t.Errorf("%s is listed but not in the archive", want)
		}
	}
	if len(m.Added) != 0 || len(m.Deleted) != 0 || len(files) != len(m.Changed)+1 {
		t.Errorf("a patch of two settings added or deleted files: %+v, %d files", m, len(files))
	}
	for _, e := range m.Changed {
		if sum := sha256.Sum256(files["shop/"+e.Path]); hex.EncodeToString(sum[:]) != e.SHA256 {
			t.Errorf("%s: the listed hash is wrong", e.Path)
		}
	}
	// The patch is the new state of the project.
	if st, err = f.shells.Status(ctx, f.owner, app.ID); err != nil || len(st.Changes) != 0 || st.Generated.IconSHA256 != blueSHA {
		t.Fatalf("status after the patch: %+v %v", st, err)
	}

	// An icon that is no longer available fails the job with a registered code.
	if err := f.objects.Delete(ctx, mustKey(t, blueSHA)); err != nil {
		t.Fatal(err)
	}
	set.IconAssetID, set.AppVersion = redID, "1.2.0+3"
	if _, err := f.shells.SetSettings(ctx, f.owner, app.ID, set); err != nil {
		t.Fatal(err)
	}
	failed := f.runJob(t, app.ID, "patch")
	if failed.Status != shellupdate.StatusFailed || failed.ErrorCode != plxerr.PreconditionFailed.String() {
		t.Errorf("a patch whose old icon is gone: %+v", failed)
	}
	// The same holds when the setting names an asset that was deleted since.
	goneID, _ := f.addAsset(t, f.owner, app.ID, "images/gone.png", "image/png", squarePNG(t, 1024, color.NRGBA{G: 200, A: 255}))
	set.IconAssetID = goneID
	if _, err := f.shells.SetSettings(ctx, f.owner, app.ID, set); err != nil {
		t.Fatal(err)
	}
	err = f.db.InTx(ctx, storage.Tenant{OrganizationID: f.owner.OrganizationID, UserID: f.owner.UserID}, func(ctx context.Context, tx pgx.Tx) error {
		_, err := dbgen.New(tx).RetireAssetFile(ctx, dbgen.RetireAssetFileParams{AppID: storage.MustUUID(app.ID), File: "images/gone.png"})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if failed = f.runJob(t, app.ID, "zip"); failed.Status != shellupdate.StatusFailed || failed.ErrorCode != plxerr.PreconditionFailed.String() {
		t.Errorf("a deleted icon asset: %+v", failed)
	}
	// The default icon needs no asset.
	set.IconAssetID = ""
	if _, err := f.shells.SetSettings(ctx, f.owner, app.ID, set); err != nil {
		t.Fatal(err)
	}
	if ok := f.runJob(t, app.ID, "zip"); ok.Status != shellupdate.StatusSucceeded {
		t.Errorf("the default icon: %+v", ok)
	}
	if st, err = f.shells.Status(ctx, f.owner, app.ID); err != nil || st.Generated.IconSHA256 != "" || len(st.Changes) != 0 {
		t.Errorf("status with the default icon: %+v %v", st, err)
	}
}

func mustKey(t *testing.T, sha string) string {
	t.Helper()
	key, err := objects.Key(objects.KindAsset, sha)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

// lastJob returns the newest job of an app.
func (f *fixture) lastJob(t *testing.T, appID string) shellupdate.ProjectJob {
	t.Helper()
	job := f.queue.jobs[len(f.queue.jobs)-1]
	j, err := f.shells.Job(context.Background(), f.owner, appID, job.JobID)
	if err != nil {
		t.Fatal(err)
	}
	return j
}

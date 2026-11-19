// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/function"

	"github.com/nightCode42/plux3/backend/internal/function/functiontest"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/objects"
)

// Verifies: FN-003, FN-016.
// A crate archive is kept only under the SHA-256 of its bytes, once per
// organisation; ListMissingCrates names exactly those not yet sent, and only
// function.deploy may ask or send.
func TestCrateArchivesAreSentOnce_FN_003(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t, nil, nil)
	serde, itoa := functiontest.NewCrate("serde", "1.0.229", "// serde\n"), functiontest.NewCrate("itoa", "1.0.18", "// itoa\n")
	asked := []string{serde.SHA256(), itoa.SHA256(), serde.SHA256()}
	missing, err := w.svc.ListMissingCrates(ctx, w.developer, w.app, asked)
	if err != nil || !slices.Equal(missing, []string{serde.SHA256(), itoa.SHA256()}) {
		t.Fatalf("missing %v, %v", missing, err)
	}
	stored, err := w.svc.UploadCrate(ctx, w.developer, w.app, serde.SHA256(), bytes.NewReader(serde.Archive))
	if err != nil || !stored {
		t.Fatalf("upload: %v %v", stored, err)
	}
	if stored, err = w.svc.UploadCrate(ctx, w.developer, w.app, serde.SHA256(), bytes.NewReader(serde.Archive)); err != nil || stored {
		t.Errorf("a second upload: %v %v", stored, err)
	}
	if missing, err = w.svc.ListMissingCrates(ctx, w.developer, w.app, asked); err != nil || !slices.Equal(missing, []string{itoa.SHA256()}) {
		t.Errorf("missing after the upload %v, %v", missing, err)
	}
	if _, err := w.svc.UploadCrate(ctx, w.developer, w.app, itoa.SHA256(), bytes.NewReader(serde.Archive)); code(err) != plxerr.InvalidFormat {
		t.Errorf("bytes that do not have the SHA-256 named: %v", err)
	}
	if _, err := w.svc.UploadCrate(ctx, w.developer, w.app, strings.ToUpper(itoa.SHA256()), bytes.NewReader(itoa.Archive)); code(err) != plxerr.InvalidFormat {
		t.Errorf("an upper-case checksum: %v", err)
	}
	if _, err := w.svc.ListMissingCrates(ctx, w.developer, w.app, []string{"c1"}); code(err) != plxerr.InvalidFormat {
		t.Errorf("a checksum that is not a SHA-256: %v", err)
	}
	if _, err := w.svc.UploadCrate(ctx, w.viewer, w.app, itoa.SHA256(), bytes.NewReader(itoa.Archive)); code(err) != plxerr.PermissionDenied {
		t.Errorf("a viewer sent a crate: %v", err)
	}
	if _, err := w.svc.ListMissingCrates(ctx, w.viewer, w.app, asked); code(err) != plxerr.PermissionDenied {
		t.Errorf("a viewer asked: %v", err)
	}
	other := newWorld(t, nil, nil)
	if missing, err := other.svc.ListMissingCrates(ctx, other.developer, other.app, []string{serde.SHA256()}); err != nil || len(missing) != 1 {
		t.Errorf("another organisation sees a crate it never sent: %v %v", missing, err)
	}
}

// Verifies: FN-003, FN-016.
// A source is stored only when every crate its Cargo.lock names was sent, it
// holds no vendor/ directory, and it stays within function.sourceCrates and,
// with its crates, function.sourceArchiveSize; a build is given exactly its
// crates.
func TestSourceNeedsItsCrates_FN_003(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	serde := functiontest.NewCrate("serde", "1.0.229", "// serde\n")
	archive := functiontest.SourceArchive("needs serde", serde)
	w := newWorld(t, nil, func(l *limits.Set) {
		tight, err := l.Tighten(limits.FunctionSourceArchiveSize, limits.ScopeApp, int64(len(archive)+len(serde.Archive)))
		if err != nil {
			t.Fatal(err)
		}
		if tight, err = tight.Tighten(limits.FunctionSourceCrates, limits.ScopeApp, 1); err != nil {
			t.Fatal(err)
		}
		*l = tight
	})
	dev := w.envs["development"]
	if _, err := w.svc.UploadSource(ctx, w.developer, w.app, dev, fnName, bytes.NewReader(archive)); code(err) != plxerr.FunctionDependencyNotVendored {
		t.Errorf("a source whose crate was not sent: %v", err)
	}
	if _, err := w.svc.UploadCrate(ctx, w.developer, w.app, serde.SHA256(), bytes.NewReader(serde.Archive)); err != nil {
		t.Fatal(err)
	}
	src, err := w.svc.UploadSource(ctx, w.developer, w.app, dev, fnName, bytes.NewReader(archive))
	if err != nil {
		t.Fatalf("a source with its crate: %v", err)
	}
	vendored := functiontest.TarGz("", map[string]string{"Cargo.lock": "version = 4\n", "vendor/serde/Cargo.toml": "[package]\n"})
	if _, err := w.svc.UploadSource(ctx, w.developer, w.app, dev, fnName, bytes.NewReader(vendored)); code(err) != plxerr.FunctionDependencyNotVendored {
		t.Errorf("a source with vendor/: %v", err)
	}
	itoa := functiontest.NewCrate("itoa", "1.0.18", "// itoa\n")
	if _, err := w.svc.UploadSource(ctx, w.developer, w.app, dev, fnName, bytes.NewReader(functiontest.SourceArchive("two", serde, itoa))); code(err) != plxerr.LimitExceeded {
		t.Errorf("more crates than function.sourceCrates: %v", err)
	}
	bigger := functiontest.SourceArchive("a longer seed that makes the archive larger than the limit allows", serde)
	if len(bigger) > len(archive) {
		if _, err := w.svc.UploadSource(ctx, w.developer, w.app, dev, fnName, bytes.NewReader(bigger)); code(err) != plxerr.FunctionBuildLimitExceeded {
			t.Errorf("a source that, with its crates, is above function.sourceArchiveSize: %v", err)
		}
	}
	w.builder.set(goodOutput(fnName), nil)
	if _, err := w.svc.RequestBuild(ctx, w.developer, src.ID, ""); err != nil {
		t.Fatal(err)
	}
	if err := w.svc.RunBuild(ctx, w.queue.last(t)); err != nil {
		t.Fatal(err)
	}
	calls := w.builder.calls
	if len(calls) != 1 || len(calls[0].Crates) != 1 || calls[0].Crates[0].SHA256 != serde.SHA256() {
		t.Errorf("the build was given %+v", calls)
	}
}

// Verifies: FN-003.
// The crate archives an organisation no longer uses — no stored source names
// them and nobody needed them for a week — are removed; their bytes go from
// the object store unless another organisation still holds them (build log
// #163). An archive a source names is kept however old.
func TestUnusedCrateArchivesAreRemoved_FN_003(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t, nil, nil)
	used := functiontest.NewCrate("serde", "1.0.229", "// serde\n")
	shared := functiontest.NewCrate("itoa", "1.0.18", "// itoa\n")
	alone := functiontest.NewCrate("memchr", "2.8.3", "// memchr\n")
	for _, c := range []functiontest.Crate{used, shared, alone} {
		if _, err := w.svc.UploadCrate(ctx, w.developer, w.app, c.SHA256(), bytes.NewReader(c.Archive)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := w.svc.UploadSource(ctx, w.developer, w.app, w.envs["development"], fnName, bytes.NewReader(functiontest.SourceArchive("uses serde", used))); err != nil {
		t.Fatal(err)
	}
	o2, err := w.tenancy.CreateOrganization(ctx, w.adminID, "beta", "Beta")
	if err != nil {
		t.Fatal(err)
	}
	key := func(c functiontest.Crate) string {
		k, err := objects.Key(objects.KindCrate, c.SHA256())
		if err != nil {
			t.Fatal(err)
		}
		return k
	}
	err = w.db.InTx(ctx, storage.Tenant{OrganizationID: o2.ID}, func(ctx context.Context, tx pgx.Tx) error {
		sum, _ := hex.DecodeString(shared.SHA256())
		_, err := tx.Exec(ctx, `INSERT INTO function_crates (organization_id, sha256, object_key, size_bytes) VALUES ($1, $2, $3, $4)`,
			o2.ID, sum, key(shared), len(shared.Archive))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	if n, err := w.svc.PurgeCrates(ctx, w.org); err != nil || n != 0 {
		t.Fatalf("archives used today were removed: %d %v", n, err)
	}
	later := w.service(t, func(o *function.Options) { o.Now = func() time.Time { return time.Now().Add(8 * 24 * time.Hour) } })
	if n, err := later.PurgeCrates(ctx, w.org); err != nil || n != 2 {
		t.Fatalf("PurgeCrates removed %d archives (%v); want the two no source names", n, err)
	}
	missing, err := w.svc.ListMissingCrates(ctx, w.developer, w.app, []string{used.SHA256(), shared.SHA256(), alone.SHA256()})
	if err != nil || !slices.Equal(missing, []string{shared.SHA256(), alone.SHA256()}) {
		t.Errorf("after the clean-up the organisation lacks %v (%v); want itoa and memchr", missing, err)
	}
	for c, kept := range map[string]bool{key(used): true, key(shared): true, key(alone): false} {
		_, err := w.store.Stat(ctx, c)
		if kept && err != nil || !kept && !errors.Is(err, objects.ErrNotFound) {
			t.Errorf("%s: kept %v, stat %v", c, kept, err)
		}
	}
}

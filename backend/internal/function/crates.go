// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/builder"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
	"github.com/nightCode42/plux3/backend/internal/storage/objects"
)

// The dependencies of a function source are the `.crate` archives crates.io
// publishes, sent once per organisation and stored once in the object store
// under their SHA-256 (FN-003, FN-016, ADR-0105). A source names them through
// its Cargo.lock; builds, links and editor sessions send exactly those to the
// builder, which checks each against its checksum again.

// crateMediaType is how a crate archive is stored.
const crateMediaType = "application/gzip"

// CrateRef is a stored crate archive a source depends on.
type CrateRef struct {
	// SHA256 is the lower-case hex SHA-256 of the archive.
	SHA256 string
	// ObjectKey is where the archive is stored.
	ObjectKey string
}

// ListMissingCrates returns, of the checksums asked about, those whose crate
// archive the organisation of the app has not sent, in the order asked and
// each once. The caller needs function.deploy on the app; at most
// function.sourceCrates checksums may be asked about.
func (s *Service) ListMissingCrates(ctx context.Context, p auth.Principal, appID string, checksums []string) ([]string, error) {
	if err := authorize(p, auth.FunctionDeploy, appID); err != nil {
		return nil, err
	}
	order, sums, err := parseChecksums(checksums)
	if err != nil {
		return nil, err
	}
	var missing []string
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		org, set, err := s.appLimits(ctx, tx, q, appID)
		if err != nil {
			return err
		}
		if most := set.Get(limits.FunctionSourceCrates); int64(len(order)) > most {
			return plxerr.New(plxerr.LimitExceeded, "%d crate checksums are above function.sourceCrates = %d", len(order), most)
		}
		rows, err := q.ListFunctionCrates(ctx, dbgen.ListFunctionCratesParams{OrganizationID: org, Sums: sums})
		if err != nil {
			return failure(err, "function crates")
		}
		have := make(map[string]bool, len(rows))
		for _, r := range rows {
			have[hex.EncodeToString(r.Sha256)] = true
		}
		for _, sum := range order {
			if !have[sum] {
				missing = append(missing, sum)
			}
		}
		return nil
	})
	return missing, err
}

// UploadCrate stores one crate archive of the organisation of the app, read
// from r, if its bytes have the SHA-256 sha it was sent as. It is bounded by
// function.sourceArchiveSize. stored is false when the organisation had
// already sent it; then r is not read. The caller needs function.deploy on
// the app, and the first upload of an archive by the organisation is audited.
func (s *Service) UploadCrate(ctx context.Context, p auth.Principal, appID, sha string, r io.Reader) (stored bool, err error) {
	if err := authorize(p, auth.FunctionDeploy, appID); err != nil {
		return false, err
	}
	_, sums, err := parseChecksums([]string{sha})
	if err != nil {
		return false, err
	}
	var (
		org  pgtype.UUID
		most int64
		have bool
	)
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		o, set, err := s.appLimits(ctx, tx, q, appID)
		if err != nil {
			return err
		}
		rows, err := q.ListFunctionCrates(ctx, dbgen.ListFunctionCratesParams{OrganizationID: o, Sums: sums})
		if err != nil {
			return failure(err, "function crates")
		}
		org, most, have = o, set.Get(limits.FunctionSourceArchiveSize), len(rows) > 0
		return nil
	})
	if err != nil || have {
		return false, err
	}
	data, sum, err := readBoundedAs(r, most, "crate archive")
	if err != nil {
		return false, err
	}
	if got := hex.EncodeToString(sum[:]); got != sha {
		return false, plxerr.New(plxerr.InvalidFormat, "the crate archive has the SHA-256 %s, not the %s it was sent as", got, sha)
	}
	key, err := objects.Key(objects.KindCrate, sha)
	if err != nil {
		return false, fmt.Errorf("function: %w", err)
	}
	if _, err := s.o.Objects.Put(ctx, key, data, crateMediaType); err != nil {
		return false, fmt.Errorf("function: store a crate archive: %w", err)
	}
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		n, err := dbgen.New(tx).InsertFunctionCrate(ctx, dbgen.InsertFunctionCrateParams{
			OrganizationID: org, Sha256: sum[:], ObjectKey: key, SizeBytes: int64(len(data)), CreatedBy: createdBy(p),
		})
		if err != nil {
			return failure(err, "function crate")
		}
		if stored = n == 1; !stored {
			return nil
		}
		return s.record(ctx, tx, p, audit.FunctionCrateUploaded, "function_crate", sha,
			fmt.Sprintf("app %s, crate archive sha256 %s, %d bytes", appID, sha, len(data)))
	})
	return stored, err
}

// appLimits reads the organisation of an app and the app's limits.
func (s *Service) appLimits(ctx context.Context, tx pgx.Tx, q *dbgen.Queries, appID string) (pgtype.UUID, limits.Set, error) {
	id, err := parseID(appID, "app")
	if err != nil {
		return pgtype.UUID{}, limits.Set{}, err
	}
	app, err := q.GetApp(ctx, id)
	if err != nil {
		return pgtype.UUID{}, limits.Set{}, failure(err, "app")
	}
	set, err := s.o.Limits(ctx, tx, storage.ID(app.OrganizationID), appID)
	if err != nil {
		return pgtype.UUID{}, limits.Set{}, fmt.Errorf("function: %w", err)
	}
	return app.OrganizationID, set, nil
}

// parseChecksums checks that every checksum is a lower-case hex SHA-256 and
// returns them in order, each once, as text and as bytes.
func parseChecksums(checksums []string) ([]string, [][]byte, error) {
	seen := make(map[string]bool, len(checksums))
	order := make([]string, 0, len(checksums))
	sums := make([][]byte, 0, len(checksums))
	for _, c := range checksums {
		b, err := hex.DecodeString(c)
		if err != nil || len(b) != 32 || strings.ToLower(c) != c {
			return nil, nil, plxerr.New(plxerr.InvalidFormat, "%q is not a lower-case hex SHA-256", c)
		}
		if seen[c] {
			continue
		}
		seen[c] = true
		order, sums = append(order, c), append(sums, b)
	}
	return order, sums, nil
}

// lockedCratesOf reads the crates.io packages that the Cargo.lock of a source
// archive names, refusing an archive with a vendor/ directory, without a lock
// file, with a package from another source or with more than count crates
// (FN-003, ADR-0105). The archive is extracted within the registry maximum of
// function.sourceArchiveSize, the bound the builder extracts it within; its
// own size was checked against the app's limit as it arrived.
func lockedCratesOf(data []byte, count int64) ([]builder.LockedCrate, error) {
	def, _ := limits.Lookup(limits.FunctionSourceArchiveSize)
	files, err := builder.ExtractArchive(bytes.NewReader(data), def.Max)
	if err != nil {
		return nil, err //nolint:wrapcheck // a Plux error, returned as is
	}
	if _, err := fs.Stat(files, "vendor"); err == nil {
		return nil, plxerr.New(plxerr.FunctionDependencyNotVendored,
			"the source holds a vendor/ directory; send the dependencies as their published crate archives (plux fn deploy does), and the builder makes vendor/ from them")
	}
	lock, err := fs.ReadFile(files, "Cargo.lock")
	if errors.Is(err, fs.ErrNotExist) {
		return nil, plxerr.New(plxerr.FunctionDependencyNotVendored, "the source has no Cargo.lock; a lock file is required (FN-003)")
	}
	if err != nil {
		return nil, fmt.Errorf("function: read Cargo.lock: %w", err)
	}
	locked, err := builder.LockedCrates(lock)
	if err != nil {
		return nil, err //nolint:wrapcheck // a Plux error, returned as is
	}
	if int64(len(locked)) > count {
		return nil, plxerr.New(plxerr.LimitExceeded, "Cargo.lock names %d crates, above function.sourceCrates = %d", len(locked), count)
	}
	return locked, nil
}

// checkSourceCrates refuses a source whose lock file names a crate archive the
// organisation has not sent, or whose archive and crates together are above
// most bytes (function.sourceArchiveSize).
func checkSourceCrates(ctx context.Context, q *dbgen.Queries, org pgtype.UUID, locked []builder.LockedCrate, archiveBytes, most int64) error {
	sums := make([][]byte, 0, len(locked))
	for _, c := range locked {
		b, err := hex.DecodeString(c.Checksum)
		if err != nil {
			return plxerr.New(plxerr.FunctionDependencyNotVendored, "crate %s %s has the checksum %q", c.Name, c.Version, c.Checksum)
		}
		sums = append(sums, b)
	}
	rows, err := q.ListFunctionCrates(ctx, dbgen.ListFunctionCratesParams{OrganizationID: org, Sums: sums})
	if err != nil {
		return failure(err, "function crates")
	}
	size := map[string]int64{}
	for _, r := range rows {
		size[hex.EncodeToString(r.Sha256)] = r.SizeBytes
	}
	total := archiveBytes
	for _, c := range locked {
		n, ok := size[c.Checksum]
		if !ok {
			return plxerr.New(plxerr.FunctionDependencyNotVendored,
				"crate %s %s (sha256 %s) has not been sent; send its published archive first (plux fn deploy does)", c.Name, c.Version, c.Checksum)
		}
		total += n
	}
	if total > most {
		return plxerr.New(plxerr.FunctionBuildLimitExceeded,
			"the source and its crates are %d bytes, above function.sourceArchiveSize = %d bytes", total, most)
	}
	return nil
}

// insertSourceCrates records the crates a stored source depends on.
func insertSourceCrates(ctx context.Context, q *dbgen.Queries, src dbgen.FunctionSource, locked []builder.LockedCrate) error {
	done := map[string]bool{}
	for _, c := range locked {
		if done[c.Checksum] {
			continue
		}
		done[c.Checksum] = true
		sum, err := hex.DecodeString(c.Checksum)
		if err != nil {
			return plxerr.New(plxerr.FunctionDependencyNotVendored, "crate %s %s has the checksum %q", c.Name, c.Version, c.Checksum)
		}
		err = q.InsertFunctionSourceCrate(ctx, dbgen.InsertFunctionSourceCrateParams{SourceID: src.ID, OrganizationID: src.OrganizationID, Sha256: sum})
		if err != nil {
			return failure(err, "function source crate")
		}
	}
	return nil
}

// sourceCrates lists the stored crate archives a source depends on.
func sourceCrates(ctx context.Context, q *dbgen.Queries, sourceID pgtype.UUID) ([]CrateRef, error) {
	rows, err := q.ListFunctionSourceCrates(ctx, sourceID)
	if err != nil {
		return nil, failure(err, "function source crates")
	}
	out := make([]CrateRef, 0, len(rows))
	for _, r := range rows {
		out = append(out, CrateRef{SHA256: hex.EncodeToString(r.Sha256), ObjectKey: r.ObjectKey})
	}
	return out, nil
}

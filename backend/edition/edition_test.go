// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package edition_test

import (
	"context"
	"errors"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

type fixed edition.Status

func (f fixed) Status() edition.Status { return edition.Status(f) }

func with(st edition.Status) context.Context {
	return edition.WithProvider(context.Background(), fixed(st))
}

// Verifies: GOV-033.
// Without a registered provider every enterprise feature is off, and using
// one reports PLX-8600.
func TestNoLicenceMeansEveryFeatureIsOff_GOV_033(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	if st := edition.From(ctx).Status(); st.State != edition.StateAbsent {
		t.Fatalf("default state = %s", st.State)
	}
	if st := edition.Community().Status(); st.State != edition.StateAbsent || st.Message == "" {
		t.Errorf("Community = %+v", st)
	}
	for _, f := range edition.Features() {
		if edition.Enabled(ctx, f) || edition.EnabledForWrite(ctx, f) || edition.Readable(ctx, f) {
			t.Errorf("%s is on without a licence", f)
		}
		for _, err := range []error{edition.Require(ctx, f), edition.RequireRead(ctx, f)} {
			var pe *plxerr.Error
			if !errors.As(err, &pe) || pe.Code != plxerr.EnterpriseFeatureNotLicensed {
				t.Errorf("%s: %v, want PLX-8600", f, err)
			}
		}
	}
	// A nil provider in the context also reads as no licence.
	if edition.Enabled(edition.WithProvider(ctx, nil), edition.FeatureSAML) {
		t.Error("a nil provider enables a feature")
	}
}

// Verifies: GOV-030.
func TestStatesAllowWhatTheyShould_GOV_030(t *testing.T) {
	t.Parallel()
	listed := []edition.Feature{edition.FeatureSAML}
	tests := []struct {
		state       edition.State
		write, read bool
	}{
		{edition.StateValid, true, true},
		{edition.StateGrace, true, true},
		{edition.StateExpired, false, true},
		{edition.StateInvalid, false, false},
		{edition.StateAbsent, false, false},
		{edition.State("unknown"), false, false},
	}
	for _, tc := range tests {
		ctx := with(edition.Status{State: tc.state, Features: listed})
		if got := edition.Enabled(ctx, edition.FeatureSAML); got != tc.write {
			t.Errorf("%s: Enabled = %v", tc.state, got)
		}
		if got := edition.EnabledForWrite(ctx, edition.FeatureSAML); got != tc.write {
			t.Errorf("%s: EnabledForWrite = %v", tc.state, got)
		}
		if got := edition.Readable(ctx, edition.FeatureSAML); got != tc.read {
			t.Errorf("%s: Readable = %v", tc.state, got)
		}
		if got := edition.Enabled(ctx, edition.FeatureSCIM); got {
			t.Errorf("%s: an unlisted feature is enabled", tc.state)
		}
		if (edition.Require(ctx, edition.FeatureSAML) == nil) != tc.write || (edition.RequireRead(ctx, edition.FeatureSAML) == nil) != tc.read {
			t.Errorf("%s: Require", tc.state)
		}
	}
}

// Verifies: GOV-030.
func TestFeatureNamesAreUniqueAndLicenceSafe_GOV_030(t *testing.T) {
	t.Parallel()
	seen := map[edition.Feature]bool{}
	for _, f := range edition.Features() {
		if seen[f] || f == "" || strings.ToLower(string(f)) != string(f) || strings.ContainsAny(string(f), " _") {
			t.Errorf("feature %q", f)
		}
		seen[f] = true
	}
	for _, want := range []edition.Feature{
		edition.FeatureSAML, edition.FeatureSCIM, edition.FeatureCustomRoles, edition.FeatureMultiStageApprovals,
		edition.FeatureChangeFreezes, edition.FeatureComplianceReports, edition.FeatureClickHouse,
		edition.FeatureParquetExport, edition.FeatureWebhookExport, edition.FeatureSIEMExtended, edition.FeatureAirGap,
	} {
		if !seen[want] {
			t.Errorf("%s is not listed by Features", want)
		}
	}
}

// Verifies: GOV-033.
// The core builds, tests and runs without ee/: no Go file under backend/
// imports the enterprise module, and the core module does not require it.
func TestCoreNeverImportsEnterpriseCode_GOV_033(t *testing.T) {
	t.Parallel()
	root := ".."
	gomod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(gomod), "plux3/ee") {
		t.Error("backend/go.mod mentions the enterprise module")
	}
	fset := token.NewFileSet()
	var checked int
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			return perr //nolint:wrapcheck // reported by the test
		}
		checked++
		for _, imp := range f.Imports {
			if strings.Contains(imp.Path.Value, "/plux3/ee/") {
				t.Errorf("%s imports %s", path, imp.Path.Value)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 100 {
		t.Errorf("only %d Go files checked; the walk is wrong", checked)
	}
}

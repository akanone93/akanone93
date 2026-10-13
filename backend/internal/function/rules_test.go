// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

func code(err error) plxerr.Code {
	c, _ := plxerr.CodeOf(err)
	return c
}

// Verifies: FN-006.
// Names and aliases follow the patterns of a plugin's function grant.
func TestNamesAndAliases_FN_006(t *testing.T) {
	t.Parallel()
	for _, ok := range []string{"payments.charge", "a.b.c", "loans.calc2"} {
		if err := ValidateName(ok); err != nil {
			t.Errorf("ValidateName(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"", "charge", "Payments.charge", "a..b", "a.1b", strings.Repeat("a", 130) + ".b"} {
		if code(ValidateName(bad)) != plxerr.InvalidFormat {
			t.Errorf("ValidateName(%q) was accepted", bad)
		}
	}
	for _, ok := range []string{"prod", "stable-2", "a"} {
		if err := ValidateAlias(ok); err != nil {
			t.Errorf("ValidateAlias(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"", "Prod", "2fast", "a_b", "-x", strings.Repeat("a", 33)} {
		if code(ValidateAlias(bad)) != plxerr.InvalidFormat {
			t.Errorf("ValidateAlias(%q) was accepted", bad)
		}
	}
}

// Verifies: FN-007.
// Production needs the production permission, which requires a second
// factor; any other environment needs function.deploy.
func TestDeployPermission_FN_007(t *testing.T) {
	t.Parallel()
	if deployPermission(true) != auth.FunctionDeployProduction || deployPermission(false) != auth.FunctionDeploy {
		t.Fatal("the deploy permission does not follow the environment")
	}
	if !auth.NeedsSecondFactor(auth.FunctionDeployProduction) {
		t.Error("function.deploy.production does not need a second factor")
	}
	if auth.NeedsSecondFactor(auth.FunctionDeploy) || auth.NeedsSecondFactor(auth.FunctionRead) {
		t.Error("function.deploy and function.read need a second factor")
	}
}

func signedBuild() Build {
	return Build{State: BuildSucceeded, Signed: true, KeyID: "fn-1", TestResults: []TestResult{{Name: "ok", Passed: true}}}
}

// Verifies: FN-009, FN-031.
// Only a succeeded build with the builder's signature and passing tests is
// deployable.
func TestDeployable_FN_009_FN_031(t *testing.T) {
	t.Parallel()
	if err := checkDeployable(signedBuild()); err != nil {
		t.Fatalf("a good build: %v", err)
	}
	cases := map[string]struct {
		edit func(*Build)
		want plxerr.Code
	}{
		"failed":   {func(b *Build) { b.State = BuildFailed }, plxerr.FunctionBuildFailed},
		"queued":   {func(b *Build) { b.State = BuildQueued }, plxerr.PreconditionFailed},
		"running":  {func(b *Build) { b.State = BuildRunning }, plxerr.PreconditionFailed},
		"unsigned": {func(b *Build) { b.Signed = false }, plxerr.FunctionArtifactSignatureInvalid},
		"no key":   {func(b *Build) { b.KeyID = "" }, plxerr.FunctionArtifactSignatureInvalid},
		"a failing test": {func(b *Build) {
			b.TestResults = append(b.TestResults, TestResult{Name: "bad", Passed: false})
		}, plxerr.FunctionTestsFailed},
	}
	for name, c := range cases {
		b := signedBuild()
		c.edit(&b)
		if got := code(checkDeployable(b)); got != c.want {
			t.Errorf("%s: code %v, want %v", name, got, c.want)
		}
	}
}

// Verifies: FN-018.
// A write is refused when it would pass function.kvValueBytes,
// function.kvKeys or function.kvBytes; an overwrite counts the difference.
func TestKVLimits_FN_018(t *testing.T) {
	t.Parallel()
	lim := limits.Defaults()
	for k, v := range map[limits.Key]int64{limits.FunctionKVValueBytes: 10, limits.FunctionKVKeys: 3, limits.FunctionKVBytes: 30} {
		var err error
		if lim, err = lim.Tighten(k, limits.ScopeApp, v); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		name                 string
		use                  kvUsage
		replaced, key, value int64
		fail                 bool
	}{
		{"fits", kvUsage{Keys: 2, Bytes: 20}, -1, 2, 5, false},
		{"value too big", kvUsage{}, -1, 1, 11, true},
		{"fourth key", kvUsage{Keys: 3, Bytes: 9}, -1, 1, 1, true},
		{"overwrite at the key limit", kvUsage{Keys: 3, Bytes: 9}, 4, 1, 1, false},
		{"over the bytes", kvUsage{Keys: 2, Bytes: 28}, -1, 2, 5, true},
		{"overwrite shrinks", kvUsage{Keys: 2, Bytes: 30}, 15, 2, 10, false},
		{"overwrite grows past the bytes", kvUsage{Keys: 2, Bytes: 30}, 5, 2, 10, true},
	}
	for _, c := range cases {
		err := checkKVPut(lim, c.use, c.replaced, c.key, c.value)
		if c.fail != (err != nil) || (err != nil && code(err) != plxerr.FunctionKVLimitExceeded) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
}

// Verifies: FN-009, FN-016, FN-010.
// The worker refuses a module that is too large, declares another
// placement than asked for, or lacks the function.
func TestJudgeOutput_FN_016(t *testing.T) {
	t.Parallel()
	row := dbgen.FunctionBuild{Name: "a.b", Placement: "server"}
	out := BuildOutput{Module: []byte("wasm"), Placement: PlacementServer, Functions: []Export{{Name: "a.b"}}}
	if err := judge(row, out, 100); err != nil {
		t.Fatalf("a good output: %v", err)
	}
	if code(judge(row, out, 3)) != plxerr.FunctionBuildLimitExceeded {
		t.Error("an oversized module was accepted")
	}
	wrong := out
	wrong.Placement = PlacementDevice
	if code(judge(row, wrong, 100)) != plxerr.FunctionDeclarationMismatch {
		t.Error("another placement was accepted")
	}
	row.Placement = ""
	if err := judge(row, wrong, 100); err != nil {
		t.Errorf("a placement left to the module: %v", err)
	}
	other := out
	other.Functions = []Export{{Name: "x.y"}}
	if code(judge(row, other, 100)) != plxerr.FunctionDeclarationMismatch {
		t.Error("a module without the function was accepted")
	}
	if err := judge(row, BuildOutput{}, 100); err != nil {
		t.Errorf("a failed build is judged by its diagnostics: %v", err)
	}
}

// Verifies: FN-005, FN-009.
// The artifact statement is canonical JSON, so the same build always
// yields the bytes the signature covers.
func TestStatementIsCanonical_FN_005(t *testing.T) {
	t.Parallel()
	st := statement{
		Version: statementVersion, Name: "a.b", Placement: PlacementServer, ABIVersion: 1,
		ModuleSHA256: strings.Repeat("a", 64), InputSchemaSHA: digest([]byte("{}")), OutputSchemaSHA: digest([]byte("{}")),
		Provenance: Provenance{SourceSHA256: strings.Repeat("b", 64), SDKSHA256: strings.Repeat("c", 64), Dependencies: []Dependency{}, Toolchain: "1.97.0"},
	}
	first, err := encodeStatement(st)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := encodeStatement(st)
	if !bytes.Equal(first, second) || bytes.ContainsAny(first, " \n") {
		t.Errorf("statement = %s", first)
	}
	if !bytes.HasPrefix(first, []byte(`{"abiVersion":1,"inputSchemaSha256":`)) {
		t.Errorf("keys are not in canonical order: %s", first)
	}
	if !bytes.Contains(first, []byte(`"sdkSha256":"`+strings.Repeat("c", 64)+`"`)) {
		t.Errorf("the statement does not carry the SDK hash: %s", first)
	}
}

// Verifies: FN-016.
// An archive above function.sourceArchiveSize is refused with PLX-7203
// after at most one byte more than the limit was read; an empty one is
// refused too.
func TestReadBounded_FN_016(t *testing.T) {
	t.Parallel()
	data, sum, err := readBounded(strings.NewReader("hello"), 5)
	if err != nil || string(data) != "hello" {
		t.Fatalf("readBounded: %q %v", data, err)
	}
	if got := digest(data); got != strings.ToLower(hexString(sum[:])) {
		t.Errorf("sum = %x, want %s", sum, got)
	}
	r := &countingReader{r: strings.NewReader(strings.Repeat("x", 1000))}
	if _, _, err := readBounded(r, 5); code(err) != plxerr.FunctionBuildLimitExceeded {
		t.Errorf("an oversized archive: %v", err)
	}
	if r.n > 6 {
		t.Errorf("read %d bytes of an archive limited to 5", r.n)
	}
	if _, _, err := readBounded(strings.NewReader(""), 5); code(err) != plxerr.InvalidFormat {
		t.Errorf("an empty archive: %v", err)
	}
}

type countingReader struct {
	r interface{ Read([]byte) (int, error) }
	n int
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += n
	return n, err
}

func hexString(b []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, c := range b {
		out = append(out, digits[c>>4], digits[c&0xf])
	}
	return string(out)
}

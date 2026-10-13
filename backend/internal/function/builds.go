// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/jcs"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
	"github.com/nightCode42/plux3/backend/internal/storage/objects"
)

// BuildJob asks the worker to run a build. Its Kind is "function.build".
type BuildJob struct {
	OrganizationID string `json:"organizationId"`
	BuildID        string `json:"buildId"`
}

// Kind names the job.
func (BuildJob) Kind() string { return "function.build" }

// Builder compiles a source archive into a module on the builder role
// (FN-016, ADR-0063). Implementations are safe for concurrent use. A
// compile failure is a BuildOutput with error diagnostics and no module;
// an error is a failure of the builder itself, which the job retries,
// unless it carries a PLX code, which fails the build with that code.
type Builder interface {
	Build(ctx context.Context, in BuildInput) (BuildOutput, error)
}

// BuildInput is what the builder is given. The builder reads the archive
// from the object store itself; nothing else of the tenant reaches it.
type BuildInput struct {
	BuildID         string
	SourceObjectKey string
	SourceSHA256    string
	Name            string
	// Placement is the one asked for, or "" for the one the function's
	// document declares.
	Placement Placement
	// Timeout, MemoryBytes and ModuleBytes are function.buildTimeout,
	// function.buildMemory and function.moduleSize (FN-010).
	Timeout     time.Duration
	MemoryBytes int64
	ModuleBytes int64
}

// Export describes one function a module exports (FN-004).
type Export struct {
	Name         string `json:"name"`
	InputSchema  string `json:"inputSchema"`
	OutputSchema string `json:"outputSchema"`
}

// BuildOutput is what the builder returns.
type BuildOutput struct {
	// Module is the optimised module; empty when the build failed.
	Module      []byte
	ABIVersion  int32
	Placement   Placement
	Functions   []Export
	Provenance  Provenance
	Diagnostics plxerr.Diagnostics
	TestResults []TestResult
}

// ArtifactSigner signs an artifact statement with the function-artifact
// key (FN-009). The worker's adapter over signing/ implements it; this
// package never sees a key (L-3). Implementations are safe for
// concurrent use.
type ArtifactSigner interface {
	SignStatement(ctx context.Context, statement []byte) (signature []byte, keyID string, err error)
}

// statementVersion is the format of the artifact statement.
const statementVersion = 1

// statement is the part of a build the signature covers (FN-005, FN-009):
// the module's SHA-256, the SHA-256s of its schemas and its provenance.
type statement struct {
	Version         int        `json:"version"`
	Name            string     `json:"name"`
	Placement       Placement  `json:"placement"`
	ABIVersion      int32      `json:"abiVersion"`
	ModuleSHA256    string     `json:"moduleSha256"`
	InputSchemaSHA  string     `json:"inputSchemaSha256"`
	OutputSchemaSHA string     `json:"outputSchemaSha256"`
	Provenance      Provenance `json:"provenance"`
}

// encodeStatement returns the RFC 8785 canonical JSON of a statement, so
// that the same build always yields the same bytes to sign.
func encodeStatement(st statement) ([]byte, error) {
	raw, err := json.Marshal(st)
	if err != nil {
		return nil, fmt.Errorf("function: encode a statement: %w", err)
	}
	canonical, err := jcs.Canonicalize(raw, 16)
	if err != nil {
		return nil, fmt.Errorf("function: canonicalise a statement: %w", err)
	}
	return canonical, nil
}

// digest returns the hex SHA-256 of b.
func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// RequestBuild queues a build of an uploaded source. The build row and its
// job commit together (FN-016, SRV-024). placement is the one to build
// for, or "" for the one the function's document declares.
func (s *Service) RequestBuild(ctx context.Context, p auth.Principal, sourceID string, placement Placement) (Build, error) {
	if placement != "" && !placement.Valid() {
		return Build{}, plxerr.New(plxerr.InvalidEnumValue, "%q is not a placement; use server or device", placement)
	}
	id, err := parseID(sourceID, "source")
	if err != nil {
		return Build{}, err
	}
	var out Build
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		src, err := q.GetFunctionSource(ctx, id)
		if err != nil {
			return failure(err, "function source")
		}
		if err := authorize(p, auth.FunctionDeploy, storage.ID(src.AppID)); err != nil {
			return err
		}
		buildID, err := s.newID()
		if err != nil {
			return err
		}
		row, err := q.InsertFunctionBuild(ctx, dbgen.InsertFunctionBuildParams{
			ID: storage.MustUUID(buildID), OrganizationID: src.OrganizationID, AppID: src.AppID, EnvironmentID: src.EnvironmentID,
			SourceID: src.ID, Name: src.Name, Placement: string(placement), CreatedBy: createdBy(p),
		})
		if err != nil {
			return failure(err, "function build")
		}
		if s.o.Jobs == nil {
			return fmt.Errorf("function: this process cannot queue builds")
		}
		if err := s.o.Jobs.Enqueue(ctx, tx, BuildJob{OrganizationID: p.OrganizationID, BuildID: buildID}); err != nil {
			return fmt.Errorf("function: %w", err)
		}
		detail := fmt.Sprintf("function %s, source %s", src.Name, sourceID)
		if err := s.record(ctx, tx, p, audit.FunctionBuildRequested, "function_build", buildID, detail); err != nil {
			return err
		}
		out, err = buildOf(row)
		return err
	})
	return out, err
}

// GetBuild returns a build with its diagnostics and test results.
func (s *Service) GetBuild(ctx context.Context, p auth.Principal, buildID string) (Build, error) {
	id, err := parseID(buildID, "build")
	if err != nil {
		return Build{}, err
	}
	var out Build
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		row, err := dbgen.New(tx).GetFunctionBuild(ctx, id)
		if err != nil {
			return failure(err, "function build")
		}
		if err := authorize(p, auth.FunctionRead, storage.ID(row.AppID)); err != nil {
			return err
		}
		out, err = buildOf(row)
		return err
	})
	return out, err
}

// buildOf converts a row.
func buildOf(row dbgen.FunctionBuild) (Build, error) {
	b := Build{
		ID: storage.ID(row.ID), SourceID: storage.ID(row.SourceID), AppID: storage.ID(row.AppID),
		EnvironmentID: storage.ID(row.EnvironmentID), Name: row.Name, Placement: Placement(row.Placement),
		State: BuildState(row.State), ModuleSHA256: row.ModuleSha256, Signed: len(row.Signature) > 0,
		CreatedAt: storage.Time(row.CreatedAt), FinishedAt: storage.Time(row.FinishedAt),
	}
	if row.KeyID != nil {
		b.KeyID = *row.KeyID
	}
	if row.AbiVersion != nil {
		b.ABIVersion = *row.AbiVersion
	}
	if err := json.Unmarshal(row.Diagnostics, &b.Diagnostics); err != nil {
		return Build{}, fmt.Errorf("function: read the diagnostics of a build: %w", err)
	}
	if err := json.Unmarshal(row.TestResults, &b.TestResults); err != nil {
		return Build{}, fmt.Errorf("function: read the test results of a build: %w", err)
	}
	if len(row.Provenance) > 0 {
		b.Provenance = new(Provenance)
		if err := json.Unmarshal(row.Provenance, b.Provenance); err != nil {
			return Build{}, fmt.Errorf("function: read the provenance of a build: %w", err)
		}
	}
	return b, nil
}

// claimed is a build the worker took, with what the builder needs.
type claimed struct {
	build  dbgen.FunctionBuild
	source dbgen.FunctionSource
	lim    limits.Set
}

// RunBuild is the job: it takes the build, runs the Builder, has the
// artifact statement signed and stores the module (FN-009, FN-016). A
// build that already finished is left alone, so a retried job is safe. A
// failure of the builder or the signer returns an error and leaves the
// build running for the retry; a build the builder refuses with a PLX
// code, or one whose result breaks a rule, fails with that diagnostic.
func (s *Service) RunBuild(ctx context.Context, job BuildJob) error {
	if s.o.Builder == nil || s.o.Signer == nil {
		return fmt.Errorf("function: this process cannot run builds: it has no builder or no signer")
	}
	c, ok, err := s.claim(ctx, job)
	if err != nil || !ok {
		return err
	}
	in := BuildInput{
		BuildID: job.BuildID, SourceObjectKey: c.source.ObjectKey, SourceSHA256: hex.EncodeToString(c.source.Sha256),
		Name: c.build.Name, Placement: Placement(c.build.Placement),
		Timeout:     time.Duration(c.lim.Get(limits.FunctionBuildTimeout)) * time.Second,
		MemoryBytes: c.lim.Get(limits.FunctionBuildMemory), ModuleBytes: c.lim.Get(limits.FunctionModuleSize),
	}
	out, err := s.o.Builder.Build(ctx, in)
	if err != nil {
		if _, domain := plxerr.CodeOf(err); !domain {
			return fmt.Errorf("function: build %s: %w", job.BuildID, err)
		}
		return s.fail(ctx, job, plxerr.Diagnostics{diagnosticOf(err)}, nil)
	}
	if refusal := judge(c.build, out, in.ModuleBytes); refusal != nil {
		return s.fail(ctx, job, append(out.Diagnostics, diagnosticOf(refusal)), out.TestResults)
	}
	if out.Diagnostics.HasErrors() || len(out.Module) == 0 {
		return s.fail(ctx, job, out.Diagnostics, out.TestResults)
	}
	return s.succeed(ctx, job, c, out)
}

// diagnosticOf describes an error that carries a PLX code.
func diagnosticOf(err error) plxerr.Diagnostic {
	code, _ := plxerr.CodeOf(err)
	return plxerr.NewDiagnostic(code, plxerr.Location{}, "%s", err.Error())
}

// judge applies the rules a builder's output must keep: the module fits
// function.moduleSize (FN-010), the placement is the one asked for, and
// the module exports the function (FN-004, PLX-7205).
func judge(b dbgen.FunctionBuild, out BuildOutput, moduleMost int64) error {
	if len(out.Module) == 0 {
		return nil
	}
	if int64(len(out.Module)) > moduleMost {
		return plxerr.New(plxerr.FunctionBuildLimitExceeded, "the module is %d bytes, above function.moduleSize = %d", len(out.Module), moduleMost)
	}
	if !out.Placement.Valid() || (b.Placement != "" && Placement(b.Placement) != out.Placement) {
		return plxerr.New(plxerr.FunctionDeclarationMismatch, "the module declares placement %q, not the %q asked for", out.Placement, b.Placement)
	}
	if _, ok := infoOf(out.Functions, b.Name); !ok {
		return plxerr.New(plxerr.FunctionDeclarationMismatch, "the module does not export the function %s", b.Name)
	}
	return nil
}

// infoOf finds a function in a module's manifest.
func infoOf(fns []Export, name string) (Export, bool) {
	for _, f := range fns {
		if f.Name == name {
			return f, true
		}
	}
	return Export{}, false
}

// claim moves a build to running and reads what the builder needs. ok is
// false when there is nothing to run.
func (s *Service) claim(ctx context.Context, job BuildJob) (claimed, bool, error) {
	id, err := parseID(job.BuildID, "build")
	if err != nil {
		return claimed{}, false, err
	}
	var c claimed
	ok := false
	err = s.inTenant(ctx, storage.Tenant{OrganizationID: job.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, err := q.GetFunctionBuildForUpdate(ctx, id)
		if err != nil {
			return failure(err, "function build")
		}
		if BuildState(row.State) == BuildSucceeded || BuildState(row.State) == BuildFailed {
			return nil
		}
		if c.build, err = q.StartFunctionBuild(ctx, dbgen.StartFunctionBuildParams{ID: id, StartedAt: storage.Timestamp(s.o.Now())}); err != nil {
			return failure(err, "function build")
		}
		if c.source, err = q.GetFunctionSource(ctx, c.build.SourceID); err != nil {
			return failure(err, "function source")
		}
		if c.lim, err = s.o.Limits(ctx, tx, job.OrganizationID, storage.ID(c.build.AppID)); err != nil {
			return fmt.Errorf("function: %w", err)
		}
		ok = true
		return nil
	})
	return c, ok, err
}

// fail records a build that did not produce a deployable module.
func (s *Service) fail(ctx context.Context, job BuildJob, diags plxerr.Diagnostics, tests []TestResult) error {
	id, err := parseID(job.BuildID, "build")
	if err != nil {
		return err
	}
	if diags == nil {
		diags = plxerr.Diagnostics{}
	}
	diagJSON, err := json.Marshal(diags)
	if err != nil {
		return fmt.Errorf("function: encode diagnostics: %w", err)
	}
	testJSON, err := encodeTests(tests)
	if err != nil {
		return err
	}
	return s.inTenant(ctx, storage.Tenant{OrganizationID: job.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		row, err := dbgen.New(tx).FailFunctionBuild(ctx, dbgen.FailFunctionBuildParams{
			ID: id, Diagnostics: diagJSON, TestResults: testJSON, FinishedAt: storage.Timestamp(s.o.Now()),
		})
		if err != nil {
			return failure(err, "function build")
		}
		return s.recordFinished(ctx, tx, job, row.Name, "failed")
	})
}

// encodeTests encodes test results; no results is an empty list.
func encodeTests(tests []TestResult) ([]byte, error) {
	if tests == nil {
		tests = []TestResult{}
	}
	raw, err := json.Marshal(tests)
	if err != nil {
		return nil, fmt.Errorf("function: encode test results: %w", err)
	}
	return raw, nil
}

// recordFinished audits the end of a build as the server's own act.
func (s *Service) recordFinished(ctx context.Context, tx pgx.Tx, job BuildJob, name, outcome string) error {
	return s.record(ctx, tx, auth.System(job.OrganizationID), audit.FunctionBuildFinished, "function_build", job.BuildID,
		fmt.Sprintf("function %s, %s", name, outcome))
}

// succeed signs a built module, stores it and marks the build.
func (s *Service) succeed(ctx context.Context, job BuildJob, c claimed, out BuildOutput) error {
	id, err := parseID(job.BuildID, "build")
	if err != nil {
		return err
	}
	info, _ := infoOf(out.Functions, c.build.Name)
	sum := sha256.Sum256(out.Module)
	out.Provenance.SourceSHA256 = hex.EncodeToString(c.source.Sha256)
	if out.Provenance.Dependencies == nil {
		out.Provenance.Dependencies = []Dependency{}
	}
	stmt, err := encodeStatement(statement{
		Version: statementVersion, Name: c.build.Name, Placement: out.Placement, ABIVersion: out.ABIVersion,
		ModuleSHA256: hex.EncodeToString(sum[:]), InputSchemaSHA: digest([]byte(info.InputSchema)),
		OutputSchemaSHA: digest([]byte(info.OutputSchema)), Provenance: out.Provenance,
	})
	if err != nil {
		return err
	}
	signature, keyID, err := s.o.Signer.SignStatement(ctx, stmt)
	if err != nil {
		return fmt.Errorf("function: sign the artifact of build %s: %w", job.BuildID, err)
	}
	if len(signature) == 0 || keyID == "" {
		return fmt.Errorf("function: the signer returned no signature for build %s", job.BuildID)
	}
	key, err := objects.Key(objects.KindModule, hex.EncodeToString(sum[:]))
	if err != nil {
		return fmt.Errorf("function: %w", err)
	}
	if _, err := s.o.Objects.Put(ctx, key, out.Module, "application/wasm"); err != nil {
		return fmt.Errorf("function: store a module: %w", err)
	}
	params, err := succeedParams(id, out, key, sum[:], stmt, signature, keyID, s.o.Now())
	if err != nil {
		return err
	}
	return s.inTenant(ctx, storage.Tenant{OrganizationID: job.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := dbgen.New(tx).SucceedFunctionBuild(ctx, params); err != nil {
			return failure(err, "function build")
		}
		return s.recordFinished(ctx, tx, job, c.build.Name, "succeeded, module "+hex.EncodeToString(sum[:]))
	})
}

// succeedParams builds the row update of a succeeded build.
func succeedParams(id pgtype.UUID, out BuildOutput, key string, sum, stmt, signature []byte, keyID string, now time.Time) (dbgen.SucceedFunctionBuildParams, error) {
	diags, err := json.Marshal(append(plxerr.Diagnostics{}, out.Diagnostics...))
	if err != nil {
		return dbgen.SucceedFunctionBuildParams{}, fmt.Errorf("function: encode diagnostics: %w", err)
	}
	tests, err := encodeTests(out.TestResults)
	if err != nil {
		return dbgen.SucceedFunctionBuildParams{}, err
	}
	manifest, err := json.Marshal(out.Functions)
	if err != nil {
		return dbgen.SucceedFunctionBuildParams{}, fmt.Errorf("function: encode the function manifest: %w", err)
	}
	prov, err := json.Marshal(out.Provenance)
	if err != nil {
		return dbgen.SucceedFunctionBuildParams{}, fmt.Errorf("function: encode provenance: %w", err)
	}
	return dbgen.SucceedFunctionBuildParams{
		ID: id, Placement: string(out.Placement), Diagnostics: diags, TestResults: tests, ModuleKey: &key,
		ModuleSha256: sum, AbiVersion: &out.ABIVersion, FunctionsManifest: manifest, Provenance: prov,
		Statement: stmt, Signature: signature, KeyID: &keyID, FinishedAt: storage.Timestamp(now),
	}, nil
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package builderrole

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/builder"
	"github.com/nightCode42/plux3/backend/internal/fnhost"
	"github.com/nightCode42/plux3/backend/internal/fnpb"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// Build implements fnpbconnect.BuilderServiceHandler (FN-016).
func (s *Service) Build(ctx context.Context, req *connect.Request[fnpb.BuildRequest]) (*connect.Response[fnpb.BuildResponse], error) {
	done, err := s.acquire()
	if err != nil {
		return nil, err
	}
	defer done()
	in := req.Msg
	log := s.log.With(slog.String("build", in.GetBuildId()), slog.String("function", in.GetName()))
	if s.o.Unsandboxed {
		log.WarnContext(ctx, unsandboxedWarning)
	}
	start := time.Now()
	resp, err := s.build(ctx, in)
	if err != nil {
		log.ErrorContext(ctx, "the build failed in the builder", slog.String("error", err.Error()))
		return nil, err
	}
	log.InfoContext(ctx, "build finished", slog.Int("moduleBytes", len(resp.GetModule())),
		slog.Bool("diagnostics", len(resp.GetDiagnosticsJson()) > 0), slog.Duration("took", time.Since(start)))
	return connect.NewResponse(resp), nil
}

// acquire takes one of the role's build slots, or refuses when all are in use.
// The returned function gives the slot back.
func (s *Service) acquire() (func(), error) {
	select {
	case s.slots <- struct{}{}:
		return func() { <-s.slots }, nil
	default:
		return nil, connect.NewError(connect.CodeResourceExhausted, errors.New("the builder is running as many builds as it may; retry later"))
	}
}

// build runs one request. A source that does not build is a response with
// diagnostics; an error is a failure of the role.
func (s *Service) build(ctx context.Context, in *fnpb.BuildRequest) (*fnpb.BuildResponse, error) {
	if err := checkRequest(in); err != nil {
		return nil, err
	}
	files, err := builder.ExtractArchive(bytes.NewReader(in.GetSource()), maxSource())
	if err != nil {
		return refusal(err)
	}
	decl, diags := builder.ReadDeclaration(s.loader, files, in.GetName(), builder.Placement(in.GetPlacement()))
	if decl == nil {
		return diagnosed(diags)
	}
	b, err := builder.New(s.toolConfig(in.GetLimits()))
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("the builder is not configured to build: %w", err))
	}
	artifact, err := b.Build(ctx, []builder.FunctionSource{decl.Source(files)}, builder.Options{})
	if err != nil {
		return refusal(err)
	}
	tests, err := builder.RunTests(ctx, s.engine, artifact.Module, decl, s.o.Limits)
	if err != nil {
		return refusal(err)
	}
	return responseOf(artifact, decl, tests, in.GetSourceSha256())
}

// checkRequest refuses a request that is wrong rather than a source that does
// not build.
func checkRequest(in *fnpb.BuildRequest) error {
	if len(in.GetSource()) == 0 || in.GetName() == "" {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("a build needs a source archive and a function name"))
	}
	sum := sha256.Sum256(in.GetSource())
	if !strings.EqualFold(hex.EncodeToString(sum[:]), in.GetSourceSha256()) {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("the source archive does not match its SHA-256"))
	}
	return nil
}

// toolConfig is the pipeline configuration of one build: the role's tools
// under the smaller of the request's limits and the registry maximum.
func (s *Service) toolConfig(req *fnpb.BuildLimits) builder.Config {
	cfg := s.o.Tools
	cfg.Schemas = builder.NewWasmSchemaReader(s.engine, fnhost.Limits{
		Timeout:     time.Duration(s.o.Limits.Get(limits.FunctionTimeout)) * time.Millisecond,
		MemoryBytes: uint64(s.o.Limits.Get(limits.FunctionMemory)), //nolint:gosec // G115: registry values are non-negative.
		OutputBytes: int(s.o.Limits.Get(limits.FunctionOutputSize)),
		LogBytes:    int(s.o.Limits.Get(limits.FunctionLogBytes)),
	})
	wall := ceiling(limits.FunctionBuildTimeout, int64(req.GetTimeoutMs()))
	memory := ceiling(limits.FunctionBuildMemory, int64(min(req.GetMemoryBytes(), 1<<62)))
	cfg.Limits = builder.Limits{Wall: time.Duration(wall) * time.Millisecond, Memory: uint64(memory), OutputBytes: buildOutputBytes} //nolint:gosec // G115: a ceiling of the registry is positive.
	cfg.SourceBytes = maxSource()
	return cfg
}

// ceiling returns the smaller of the requested value and the registry maximum
// of the key; a request of zero asks for the maximum.
func ceiling(k limits.Key, requested int64) int64 {
	def, _ := limits.Lookup(k)
	if requested <= 0 {
		return def.Max
	}
	return min(requested, def.Max)
}

// refusal turns the error of a stage into the response or the RPC error: an
// error with a PLX code is the source's fault and becomes a diagnostic; the
// sandbox the kernel refuses is a failed precondition; anything else is the
// role's own failure, which the worker retries.
func refusal(err error) (*fnpb.BuildResponse, error) {
	if pe, ok := errors.AsType[*plxerr.Error](err); ok {
		msg := pe.Message
		if pe.Err != nil {
			msg += ": " + pe.Err.Error()
		}
		return diagnosed(plxerr.Diagnostics{plxerr.NewDiagnostic(pe.Code, plxerr.Location{}, "%s", msg)})
	}
	switch {
	case errors.Is(err, builder.ErrSandboxUnavailable):
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("the kernel refuses the build sandbox and builder.sandbox is not none: %w", err))
	case errors.Is(err, context.Canceled):
		return nil, connect.NewError(connect.CodeCanceled, err)
	case errors.Is(err, context.DeadlineExceeded):
		return nil, connect.NewError(connect.CodeDeadlineExceeded, err)
	}
	return nil, connect.NewError(connect.CodeInternal, err)
}

// diagnosed is the response of a source that does not build.
func diagnosed(diags plxerr.Diagnostics) (*fnpb.BuildResponse, error) {
	raw, err := json.Marshal(diags)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("encode diagnostics: %w", err))
	}
	return &fnpb.BuildResponse{DiagnosticsJson: raw}, nil
}

// responseOf converts the artifact of a build into the wire form.
func responseOf(a *builder.Artifact, decl *builder.Declaration, tests []builder.TestResult, sourceSHA256 string) (*fnpb.BuildResponse, error) {
	if len(a.Functions) != 1 {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("the build produced %d functions; the role builds one", len(a.Functions)))
	}
	p := a.Provenance
	out := &fnpb.BuildResponse{
		Module: a.Module, AbiVersion: int32(a.ABIVersion), //nolint:gosec // G115: the ABI version is 1.
		Placement:   string(a.Functions[0].Placement),
		Declaration: decl.Document,
		Functions: []*fnpb.BuiltFunction{{
			Name: a.Functions[0].Name, InputSchema: string(a.Functions[0].InputSchema), OutputSchema: string(a.Functions[0].OutputSchema),
		}},
		Provenance: &fnpb.BuildProvenance{
			SourceSha256: sourceSHA256, SdkSha256: p.SDKSHA256, Toolchain: p.Toolchain, WasmOptVersion: p.WasmOpt,
			BuildTime: p.BuildTime, BuilderIdentity: p.BuilderIdentity,
		},
	}
	for _, d := range p.Dependencies {
		out.Provenance.Dependencies = append(out.Provenance.Dependencies, &fnpb.BuildDependency{Name: d.Name, Version: d.Version, Checksum: d.Checksum})
	}
	for _, r := range tests {
		out.TestResults = append(out.TestResults, &fnpb.BuildTestResult{Name: r.Name, Passed: r.Passed, Message: r.Message})
	}
	return out, nil
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package builderrole

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/builder"
	"github.com/nightCode42/plux3/backend/internal/fnpb"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Link implements fnpbconnect.BuilderServiceHandler: it links the sources of
// a plugin's device-placed functions into one module (FN-051).
func (s *Service) Link(ctx context.Context, req *connect.Request[fnpb.LinkRequest]) (*connect.Response[fnpb.LinkResponse], error) {
	done, err := s.acquire()
	if err != nil {
		return nil, err
	}
	defer done()
	in := req.Msg
	log := s.log.With(slog.String("link", in.GetLinkId()), slog.Int("functions", len(in.GetSources())))
	if s.o.Unsandboxed {
		log.WarnContext(ctx, unsandboxedWarning)
	}
	start := time.Now()
	resp, err := s.link(ctx, in)
	if err != nil {
		log.ErrorContext(ctx, "the link failed in the builder", slog.String("error", err.Error()))
		return nil, err
	}
	log.InfoContext(ctx, "link finished", slog.Int("moduleBytes", len(resp.GetModule())),
		slog.Bool("diagnostics", len(resp.GetDiagnosticsJson()) > 0), slog.Duration("took", time.Since(start)))
	return connect.NewResponse(resp), nil
}

// link runs one request. Sources that do not link are a response with
// diagnostics; an error is a failure of the role.
func (s *Service) link(ctx context.Context, in *fnpb.LinkRequest) (*fnpb.LinkResponse, error) {
	if err := checkLinkRequest(in); err != nil {
		return nil, err
	}
	sources := make([]builder.FunctionSource, 0, len(in.GetSources()))
	var diags plxerr.Diagnostics
	for _, src := range in.GetSources() {
		files, err := builder.ExtractArchive(bytes.NewReader(src.GetSource()), maxSource())
		if err != nil {
			return linkRefusal(err)
		}
		decl, found := builder.ReadDeclaration(s.loader, files, src.GetName(), builder.PlacementDevice)
		if decl == nil {
			diags = append(diags, found...)
			continue
		}
		sources = append(sources, decl.Source(files))
	}
	if len(diags) > 0 {
		return linkDiagnosed(diags)
	}
	b, err := builder.New(s.toolConfig(in.GetLimits()))
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("the builder is not configured to build: %w", err))
	}
	// FN-051: each function was built, tested and verified on its own, and
	// linking is deterministic, so the link does not build twice.
	artifact, err := b.Build(ctx, sources, builder.Options{})
	if err != nil {
		return linkRefusal(err)
	}
	return linkResponseOf(artifact), nil
}

// checkLinkRequest refuses a request that is wrong rather than sources that
// do not link.
func checkLinkRequest(in *fnpb.LinkRequest) error {
	if len(in.GetSources()) == 0 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("a link needs at least one source archive"))
	}
	var total int64
	seen := map[string]bool{}
	for _, src := range in.GetSources() {
		if len(src.GetSource()) == 0 || src.GetName() == "" {
			return connect.NewError(connect.CodeInvalidArgument, errors.New("a link source needs an archive and a function name"))
		}
		if seen[src.GetName()] {
			return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("the function %q is listed twice", src.GetName()))
		}
		seen[src.GetName()] = true
		sum := sha256.Sum256(src.GetSource())
		if !strings.EqualFold(hex.EncodeToString(sum[:]), src.GetSourceSha256()) {
			return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("the source archive of %q does not match its SHA-256", src.GetName()))
		}
		total += int64(len(src.GetSource()))
	}
	if total > maxSource() {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("the source archives total %d bytes, above function.sourceArchiveSize = %d", total, maxSource()))
	}
	return nil
}

// linkRefusal turns the error of a stage into a link response or the RPC
// error, as refusal does for a build.
func linkRefusal(err error) (*fnpb.LinkResponse, error) {
	resp, rpcErr := refusal(err)
	if rpcErr != nil {
		return nil, rpcErr
	}
	return &fnpb.LinkResponse{DiagnosticsJson: resp.GetDiagnosticsJson()}, nil
}

// linkDiagnosed is the response of sources that do not link.
func linkDiagnosed(diags plxerr.Diagnostics) (*fnpb.LinkResponse, error) {
	resp, err := diagnosed(diags)
	if err != nil {
		return nil, err
	}
	return &fnpb.LinkResponse{DiagnosticsJson: resp.GetDiagnosticsJson()}, nil
}

// linkResponseOf converts the artifact of a link into the wire form.
func linkResponseOf(a *builder.Artifact) *fnpb.LinkResponse {
	p := a.Provenance
	out := &fnpb.LinkResponse{
		Module: a.Module, AbiVersion: int32(a.ABIVersion), //nolint:gosec // G115: the ABI version is 1.
		Provenance: &fnpb.BuildProvenance{
			SourceSha256: p.SourceSHA256, SdkSha256: p.SDKSHA256, Toolchain: p.Toolchain, WasmOptVersion: p.WasmOpt,
			BuildTime: p.BuildTime, BuilderIdentity: p.BuilderIdentity,
		},
	}
	for _, f := range a.Functions {
		out.Functions = append(out.Functions, &fnpb.LinkedFunction{Name: f.Name, Index: uint32(f.Index)}) //nolint:gosec // G115: an index is below the number of functions.
	}
	for _, d := range p.Dependencies {
		out.Provenance.Dependencies = append(out.Provenance.Dependencies, &fnpb.BuildDependency{Name: d.Name, Version: d.Version, Checksum: d.Checksum})
	}
	return out
}

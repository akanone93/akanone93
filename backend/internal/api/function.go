// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"errors"
	"io"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// FunctionAdmin serves FunctionAdminService: sources, builds, versions and
// aliases of Plux Functions (FN-006, FN-016). ListLogs and GetMetrics
// answer Unimplemented until the observability milestone (FN-032).
type FunctionAdmin struct {
	pluxv1connect.UnimplementedFunctionAdminServiceHandler
	h *Handlers
}

var _ pluxv1connect.FunctionAdminServiceHandler = FunctionAdmin{}

// FunctionAdmin returns the FunctionAdminService handler.
func (h *Handlers) FunctionAdmin() FunctionAdmin { return FunctionAdmin{h: h} }

// UploadSource receives a source archive in chunks; the first message names
// the app, the environment and the function and carries no bytes
// (FN-016). The service bounds the archive by function.sourceArchiveSize.
func (s FunctionAdmin) UploadSource(ctx context.Context, stream *connect.ClientStream[pluxv1.UploadSourceRequest]) (*connect.Response[pluxv1.UploadSourceResponse], error) {
	if s.h.Functions == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("this server runs no function service"))
	}
	p, err := s.h.principal(ctx, stream.RequestHeader(), "")
	if err != nil {
		return nil, err
	}
	if !stream.Receive() {
		if err := stream.Err(); err != nil && !errors.Is(err, io.EOF) {
			return nil, err //nolint:wrapcheck // the stream's own error
		}
		return nil, plxerr.New(plxerr.MissingProperty, "the first message must name the function")
	}
	first := stream.Msg()
	r := &chunkReader{stream: stream, pending: first.GetChunk()}
	src, err := s.h.Functions.UploadSource(ctx, p, first.GetAppId(), first.GetEnvironmentId(), first.GetName(), r)
	if err != nil {
		return nil, err //nolint:wrapcheck // a domain error
	}
	return connect.NewResponse(&pluxv1.UploadSourceResponse{SourceId: src.ID, SourceSha256: hexOf(src.SHA256)}), nil
}

// chunkReader presents the chunks of an upload as a reader. It is used by
// one goroutine.
type chunkReader struct {
	stream  *connect.ClientStream[pluxv1.UploadSourceRequest]
	pending []byte
}

// Read returns the bytes of the next chunks.
func (r *chunkReader) Read(b []byte) (int, error) {
	for len(r.pending) == 0 {
		if !r.stream.Receive() {
			if err := r.stream.Err(); err != nil {
				return 0, err //nolint:wrapcheck // the stream's own error
			}
			return 0, io.EOF
		}
		r.pending = r.stream.Msg().GetChunk()
	}
	n := copy(b, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}

// Build queues a build of an uploaded source (FN-016).
func (s FunctionAdmin) Build(ctx context.Context, req *connect.Request[pluxv1.BuildRequest]) (*connect.Response[pluxv1.BuildResponse], error) {
	if s.h.Functions == nil {
		return s.UnimplementedFunctionAdminServiceHandler.Build(ctx, req) //nolint:wrapcheck // the generated Unimplemented answer
	}
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.BuildResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		b, err := s.h.Functions.RequestBuild(ctx, p, req.Msg.GetSourceId(), "")
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.BuildResponse{Build: buildProto(b)}, nil
	})
}

// GetBuild returns the state, diagnostics and test results of a build.
func (s FunctionAdmin) GetBuild(ctx context.Context, req *connect.Request[pluxv1.GetBuildRequest]) (*connect.Response[pluxv1.GetBuildResponse], error) {
	if s.h.Functions == nil {
		return s.UnimplementedFunctionAdminServiceHandler.GetBuild(ctx, req) //nolint:wrapcheck // the generated Unimplemented answer
	}
	return read(ctx, func(ctx context.Context) (*pluxv1.GetBuildResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		b, err := s.h.Functions.GetBuild(ctx, p, req.Msg.GetBuildId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.GetBuildResponse{Build: buildProto(b)}, nil
	})
}

// GetFunction returns a function with its aliases and latest version.
func (s FunctionAdmin) GetFunction(ctx context.Context, req *connect.Request[pluxv1.GetFunctionRequest]) (*connect.Response[pluxv1.GetFunctionResponse], error) {
	if s.h.Functions == nil {
		return s.UnimplementedFunctionAdminServiceHandler.GetFunction(ctx, req) //nolint:wrapcheck // the generated Unimplemented answer
	}
	return read(ctx, func(ctx context.Context) (*pluxv1.GetFunctionResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		f, err := s.h.Functions.GetFunction(ctx, p, m.GetAppId(), m.GetEnvironmentId(), m.GetName())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.GetFunctionResponse{Latest: functionVersionProto(f.Latest), Aliases: aliasProtos(f.Aliases)}, nil
	})
}

// ListFunctions lists the functions of an environment with their latest
// version, by name.
func (s FunctionAdmin) ListFunctions(ctx context.Context, req *connect.Request[pluxv1.ListFunctionsRequest]) (*connect.Response[pluxv1.ListFunctionsResponse], error) {
	if s.h.Functions == nil {
		return s.UnimplementedFunctionAdminServiceHandler.ListFunctions(ctx, req) //nolint:wrapcheck // the generated Unimplemented answer
	}
	return read(ctx, func(ctx context.Context) (*pluxv1.ListFunctionsResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		q := Query{OrderBy: "name", Filters: map[string]func(Item) string{
			"name": func(m Item) string { return m.(*pluxv1.FunctionVersion).GetName() },
		}}
		page := req.Msg.GetPage()
		clauses, err := q.Parse(page)
		if err != nil {
			return nil, err
		}
		scope := req.Msg.GetAppId() + "/" + req.Msg.GetEnvironmentId()
		after, size, err := s.h.Pages.Request(req.Spec().Procedure, scope, page)
		if err != nil {
			return nil, err
		}
		versions, err := s.h.Functions.ListFunctions(ctx, p, req.Msg.GetAppId(), req.Msg.GetEnvironmentId(), after.Key, size)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListFunctionsResponse{Page: &pluxv1.PageResult{}}
		var last storage.Cursor
		for _, v := range versions {
			last = storage.Cursor{Key: v.Name}
			if item := functionVersionProto(v); q.Keep(clauses, item) {
				out.Functions = append(out.Functions, item)
			}
		}
		out.Page.NextPageToken = s.h.Pages.Next(req.Spec().Procedure, scope, page, len(versions), size, last)
		return out, Mask(page.GetReadMask(), out.Functions)
	})
}

// ListVersions lists the versions of a function, newest first.
func (s FunctionAdmin) ListVersions(ctx context.Context, req *connect.Request[pluxv1.ListVersionsRequest]) (*connect.Response[pluxv1.ListVersionsResponse], error) {
	if s.h.Functions == nil {
		return s.UnimplementedFunctionAdminServiceHandler.ListVersions(ctx, req) //nolint:wrapcheck // the generated Unimplemented answer
	}
	return read(ctx, func(ctx context.Context) (*pluxv1.ListVersionsResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		q := Query{OrderBy: "version", Filters: map[string]func(Item) string{
			"placement": func(m Item) string { return m.(*pluxv1.FunctionVersion).GetPlacement() },
		}}
		page := req.Msg.GetPage()
		clauses, err := q.Parse(page)
		if err != nil {
			return nil, err
		}
		m := req.Msg
		scope := m.GetAppId() + "/" + m.GetEnvironmentId() + "/" + m.GetName()
		after, size, err := s.h.Pages.Request(req.Spec().Procedure, scope, page)
		if err != nil {
			return nil, err
		}
		versions, err := s.h.Functions.ListVersions(ctx, p, m.GetAppId(), m.GetEnvironmentId(), m.GetName(), int32(after.Sequence), size) //nolint:gosec // a version number
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListVersionsResponse{Page: &pluxv1.PageResult{}}
		var last storage.Cursor
		for _, v := range versions {
			last = storage.Cursor{Sequence: int64(v.Version)}
			if item := functionVersionProto(v); q.Keep(clauses, item) {
				out.Versions = append(out.Versions, item)
			}
		}
		out.Page.NextPageToken = s.h.Pages.Next(req.Spec().Procedure, scope, page, len(versions), size, last)
		return out, Mask(page.GetReadMask(), out.Versions)
	})
}

// Deploy turns a successful build into a new immutable version and may
// point an alias at it (FN-006, FN-007, FN-031).
func (s FunctionAdmin) Deploy(ctx context.Context, req *connect.Request[pluxv1.DeployRequest]) (*connect.Response[pluxv1.DeployResponse], error) {
	if s.h.Functions == nil {
		return s.UnimplementedFunctionAdminServiceHandler.Deploy(ctx, req) //nolint:wrapcheck // the generated Unimplemented answer
	}
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.DeployResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		v, err := s.h.Functions.Deploy(ctx, p, req.Msg.GetBuildId(), req.Msg.GetAlias())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.DeployResponse{Version: functionVersionProto(v)}, nil
	})
}

// SetAlias points an alias at an existing version (FN-006).
func (s FunctionAdmin) SetAlias(ctx context.Context, req *connect.Request[pluxv1.SetAliasRequest]) (*connect.Response[pluxv1.SetAliasResponse], error) {
	if s.h.Functions == nil {
		return s.UnimplementedFunctionAdminServiceHandler.SetAlias(ctx, req) //nolint:wrapcheck // the generated Unimplemented answer
	}
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.SetAliasResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		aliases, err := s.h.Functions.SetAlias(ctx, p, m.GetAppId(), m.GetEnvironmentId(), m.GetName(), m.GetAlias().GetName(), int32(m.GetAlias().GetVersion())) //nolint:gosec // bounded by the version column
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.SetAliasResponse{Aliases: aliasProtos(aliases)}, nil
	})
}

// hexOf renders a digest as lower-case hex.
func hexOf(b []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, c := range b {
		out = append(out, digits[c>>4], digits[c&0x0f])
	}
	return string(out)
}

// buildProto converts a build for the wire.
func buildProto(b function.Build) *pluxv1.Build {
	out := &pluxv1.Build{Id: b.ID, State: buildStateProto(b.State), Diagnostics: diagnosticProtos(b.Diagnostics)}
	for _, t := range b.TestResults {
		out.TestResults = append(out.TestResults, &pluxv1.FunctionTestResult{Name: t.Name, Passed: t.Passed, Message: t.Message})
	}
	return out
}

// buildStateProto converts a build state.
func buildStateProto(s function.BuildState) pluxv1.BuildState {
	switch s {
	case function.BuildQueued:
		return pluxv1.BuildState_BUILD_STATE_QUEUED
	case function.BuildRunning:
		return pluxv1.BuildState_BUILD_STATE_RUNNING
	case function.BuildSucceeded:
		return pluxv1.BuildState_BUILD_STATE_SUCCEEDED
	case function.BuildFailed:
		return pluxv1.BuildState_BUILD_STATE_FAILED
	}
	return pluxv1.BuildState_BUILD_STATE_UNSPECIFIED
}

// functionVersionProto converts a version for the wire.
func functionVersionProto(v function.Version) *pluxv1.FunctionVersion {
	prov := &pluxv1.Provenance{
		SourceSha256: v.Provenance.SourceSHA256, Toolchain: v.Provenance.Toolchain, WasmOptVersion: v.Provenance.WasmOptVersion,
		BuildTime: ts(v.Provenance.BuildTime), BuilderIdentity: v.Provenance.BuilderIdentity,
	}
	for _, d := range v.Provenance.Dependencies {
		prov.Dependencies = append(prov.Dependencies, &pluxv1.Dependency{Name: d.Name, Version: d.Version, Checksum: d.Checksum})
	}
	return &pluxv1.FunctionVersion{
		AppId: v.AppID, EnvironmentId: v.EnvironmentID, Name: v.Name, Version: uint32(v.Version), //nolint:gosec // positive by the version column
		Placement: string(v.Placement), ModuleSha256: hexOf(v.ModuleSHA256), AbiVersion: uint32(v.ABIVersion), //nolint:gosec // positive
		InputSchema: v.InputSchema, OutputSchema: v.OutputSchema, Provenance: prov, Signature: v.Signature,
		TestsPassed: v.TestsPassed, CreatedAt: ts(v.CreatedAt),
	}
}

// aliasProtos converts aliases for the wire.
func aliasProtos(as []function.Alias) []*pluxv1.Alias {
	out := make([]*pluxv1.Alias, 0, len(as))
	for _, a := range as {
		out = append(out, &pluxv1.Alias{Name: a.Name, Version: uint32(a.Version)}) //nolint:gosec // positive by the version column
	}
	return out
}

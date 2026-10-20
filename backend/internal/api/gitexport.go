// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/gitsync"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

// GitExport serves GitExportService (GOV-011).
type GitExport struct{ h *Handlers }

var _ pluxv1connect.GitExportServiceHandler = GitExport{}

// GitExport returns the GitExportService handler.
func (h *Handlers) GitExport() GitExport { return GitExport{h: h} }

func gitSettingsProto(s gitsync.Settings) *pluxv1.GitSettings {
	return &pluxv1.GitSettings{
		AppId: s.AppID, Enabled: s.Enabled, RemoteUrl: s.RemoteURL, Branch: s.Branch, Username: s.Username,
		CredentialEnvironmentId: s.CredentialEnvironmentID, CredentialKey: s.CredentialKey, UpdatedAt: ts(s.UpdatedAt),
	}
}

func gitExportProto(e gitsync.Export) *pluxv1.GitExport {
	return &pluxv1.GitExport{
		Id: e.ID, AppId: e.AppID, VersionId: e.VersionID, State: e.State, Commit: e.Commit, Attempts: e.Attempts,
		ErrorCode: e.ErrorCode, CreatedAt: ts(e.CreatedAt), UpdatedAt: ts(e.UpdatedAt),
	}
}

// GetSettings returns the export settings of an app.
func (s GitExport) GetSettings(ctx context.Context, req *connect.Request[pluxv1.GetGitSettingsRequest]) (*connect.Response[pluxv1.GetGitSettingsResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.GetGitSettingsResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		got, err := s.h.GitExports.Get(ctx, p, req.Msg.GetAppId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.GetGitSettingsResponse{Settings: gitSettingsProto(got)}, nil
	})
}

// ConfigureExport sets the export of an app.
func (s GitExport) ConfigureExport(ctx context.Context, req *connect.Request[pluxv1.ConfigureGitExportRequest]) (*connect.Response[pluxv1.ConfigureGitExportResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.ConfigureGitExportResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		in := req.Msg.GetSettings()
		got, err := s.h.GitExports.Configure(ctx, p, gitsync.Settings{
			AppID: in.GetAppId(), Enabled: in.GetEnabled(), RemoteURL: in.GetRemoteUrl(), Branch: in.GetBranch(),
			Username: in.GetUsername(), CredentialEnvironmentID: in.GetCredentialEnvironmentId(), CredentialKey: in.GetCredentialKey(),
		})
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.ConfigureGitExportResponse{Settings: gitSettingsProto(got)}, nil
	})
}

// ListExports lists the newest exports of an app.
func (s GitExport) ListExports(ctx context.Context, req *connect.Request[pluxv1.ListGitExportsRequest]) (*connect.Response[pluxv1.ListGitExportsResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListGitExportsResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		rows, err := s.h.GitExports.List(ctx, p, req.Msg.GetAppId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListGitExportsResponse{}
		for _, r := range rows {
			out.Exports = append(out.Exports, gitExportProto(r))
		}
		return out, nil
	})
}

// RunExport queues the export of the newest published version.
func (s GitExport) RunExport(ctx context.Context, req *connect.Request[pluxv1.RunGitExportRequest]) (*connect.Response[pluxv1.RunGitExportResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.RunGitExportResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		got, err := s.h.GitExports.Run(ctx, p, req.Msg.GetAppId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.RunGitExportResponse{Export: gitExportProto(got)}, nil
	})
}

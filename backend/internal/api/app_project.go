// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/archive"
	"github.com/nightCode42/plux3/backend/internal/generator"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/shellupdate"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
)

// AppProject serves AppProjectService (STU-022 to STU-024, STU-015, GEN-004).
type AppProject struct{ h *Handlers }

var _ pluxv1connect.AppProjectServiceHandler = AppProject{}

// AppProject returns the AppProjectService handler.
func (h *Handlers) AppProject() AppProject { return AppProject{h: h} }

func summaryProto(s tenancy.AppSummary) *pluxv1.AppSummary {
	out := &pluxv1.AppSummary{
		App: appProto(s.App), LastPublishedAt: ts(s.LastPublishedAt), ActiveDevices: s.ActiveDevices, PluginCount: s.PluginCount,
		CrashFreeRate: s.CrashFreeRate, CrashFreeTrend: s.CrashFreeTrend,
	}
	if w := s.LastPublisher; w != nil {
		out.LastPublisher = &pluxv1.Actor{Kind: w.Kind, Id: w.ID, Display: w.Display}
	}
	for _, r := range s.Releases {
		out.Releases = append(out.Releases, &pluxv1.EnvironmentRelease{EnvironmentKey: r.Environment, LiveSequence: r.Sequence, PublishedAt: ts(r.PublishedAt)})
	}
	for _, l := range s.Limits {
		out.Limits = append(out.Limits, &pluxv1.AppLimitUsage{Key: l.Key, Used: l.Used, Limit: l.Limit})
	}
	return out
}

// ListAppSummaries returns the apps grid in one call.
func (s AppProject) ListAppSummaries(ctx context.Context, req *connect.Request[pluxv1.ListAppSummariesRequest]) (*connect.Response[pluxv1.ListAppSummariesResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListAppSummariesResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), req.Msg.GetOrganizationId())
		if err != nil {
			return nil, err
		}
		sums, next, err := s.h.Tenancy.ListAppSummaries(ctx, p, req.Msg.GetAfterKey(), req.Msg.GetLimit())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListAppSummariesResponse{NextAfterKey: next}
		for _, sum := range sums {
			out.Summaries = append(out.Summaries, summaryProto(sum))
		}
		return out, nil
	})
}

// ExportApp returns an app as a .plux archive.
func (s AppProject) ExportApp(ctx context.Context, req *connect.Request[pluxv1.ExportAppRequest]) (*connect.Response[pluxv1.ExportAppResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.ExportAppResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		x, err := s.h.Archives.Export(ctx, p, req.Msg.GetAppId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.ExportAppResponse{Filename: x.Filename, Archive: x.Data, Sha256: x.SHA256}, nil
	})
}

// ImportApp creates an app from a .plux archive.
func (s AppProject) ImportApp(ctx context.Context, req *connect.Request[pluxv1.ImportAppRequest]) (*connect.Response[pluxv1.ImportAppResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.ImportAppResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), req.Msg.GetOrganizationId())
		if err != nil {
			return nil, err
		}
		in, err := s.h.Archives.Import(ctx, p, req.Msg.GetArchive(), req.Msg.GetKey(), req.Msg.GetName())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.ImportAppResponse{App: appProto(in.App), Documents: int32(in.Documents), ErrorDiagnostics: int32(in.ErrorDiagnostics)}, nil //nolint:gosec // bounded by archive.entries
	})
}

// DuplicateApp creates an app from an export of another.
func (s AppProject) DuplicateApp(ctx context.Context, req *connect.Request[pluxv1.DuplicateAppRequest]) (*connect.Response[pluxv1.DuplicateAppResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.DuplicateAppResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		in, err := s.h.Archives.Duplicate(ctx, p, req.Msg.GetAppId(), req.Msg.GetKey(), req.Msg.GetName())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.DuplicateAppResponse{App: appProto(in.App), Documents: int32(in.Documents)}, nil //nolint:gosec // bounded by archive.entries
	})
}

// ListSampleApps lists the embedded sample apps.
func (s AppProject) ListSampleApps(ctx context.Context, req *connect.Request[pluxv1.ListSampleAppsRequest]) (*connect.Response[pluxv1.ListSampleAppsResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListSampleAppsResponse, error) {
		if _, err := s.h.principal(ctx, req.Header(), ""); err != nil {
			return nil, err
		}
		out := &pluxv1.ListSampleAppsResponse{}
		for _, sample := range s.h.Archives.ListSamples() {
			out.Samples = append(out.Samples, sampleProto(sample))
		}
		return out, nil
	})
}

func sampleProto(s archive.Sample) *pluxv1.SampleApp {
	return &pluxv1.SampleApp{Id: s.ID, Key: s.Key, Name: s.Name, Description: s.Description, Size: int64(len(s.Data)), Sha256: s.SHA256}
}

// ImportSampleApp imports a sample app.
func (s AppProject) ImportSampleApp(ctx context.Context, req *connect.Request[pluxv1.ImportSampleAppRequest]) (*connect.Response[pluxv1.ImportSampleAppResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.ImportSampleAppResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), req.Msg.GetOrganizationId())
		if err != nil {
			return nil, err
		}
		in, err := s.h.Archives.ImportSample(ctx, p, req.Msg.GetSampleId(), req.Msg.GetKey(), req.Msg.GetName())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.ImportSampleAppResponse{App: appProto(in.App), Documents: int32(in.Documents)}, nil //nolint:gosec // bounded by archive.entries
	})
}

func shellProto(s generator.ShellSpec) *pluxv1.ShellSettings {
	return &pluxv1.ShellSettings{
		Name: s.Name, Package: s.Package, ApplicationId: s.ApplicationID, BundleId: s.BundleID, AppVersion: s.AppVersion,
		SplashColor: s.SplashColor, Router: s.Router, DeviceApis: s.DeviceAPIs, Push: s.Push,
		DeepLinkHosts: s.DeepLinkHosts, DeepLinkSchemes: s.DeepLinkSchemes, Packages: s.Packages, EntryRoute: s.Plux.EntryRoute,
	}
}

func shellSpec(m *pluxv1.ShellSettings) generator.ShellSpec {
	return generator.ShellSpec{
		Name: m.GetName(), Package: m.GetPackage(), ApplicationID: m.GetApplicationId(), BundleID: m.GetBundleId(), AppVersion: m.GetAppVersion(),
		SplashColor: m.GetSplashColor(), Router: m.GetRouter(), DeviceAPIs: m.GetDeviceApis(), Push: m.GetPush(),
		DeepLinkHosts: m.GetDeepLinkHosts(), DeepLinkSchemes: m.GetDeepLinkSchemes(), Packages: m.GetPackages(),
		Plux: generator.PluxSource{EntryRoute: m.GetEntryRoute()},
	}
}

// GetShellStatus says whether the app's shell needs an update.
func (s AppProject) GetShellStatus(ctx context.Context, req *connect.Request[pluxv1.GetShellStatusRequest]) (*connect.Response[pluxv1.GetShellStatusResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.GetShellStatusResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		st, err := s.h.Shells.Status(ctx, p, req.Msg.GetAppId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.GetShellStatusResponse{Current: shellProto(st.Current), GeneratedAt: ts(st.GeneratedAt), UpdateRequired: len(st.Changes) > 0}
		if st.Generated != nil {
			out.Generated = shellProto(*st.Generated)
		}
		for _, c := range st.Changes {
			out.Changes = append(out.Changes, &pluxv1.ShellChange{Field: c.Field, OldValue: c.Old, NewValue: c.New, Reason: c.Reason})
		}
		return out, nil
	})
}

// SetShellSettings stores the app's shell settings.
func (s AppProject) SetShellSettings(ctx context.Context, req *connect.Request[pluxv1.SetShellSettingsRequest]) (*connect.Response[pluxv1.SetShellSettingsResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.SetShellSettingsResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		spec, err := s.h.Shells.SetSettings(ctx, p, req.Msg.GetAppId(), shellSpec(req.Msg.GetSettings()))
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.SetShellSettingsResponse{Settings: shellProto(spec)}, nil
	})
}

func projectJobProto(j shellupdate.ProjectJob) *pluxv1.ProjectJob {
	return &pluxv1.ProjectJob{
		Id: j.ID, AppId: j.AppID, EnvironmentKey: j.Environment, ChannelKey: j.Channel, Format: j.Format, Status: j.Status,
		ErrorCode: j.ErrorCode, ErrorMessage: j.ErrorMessage, Size: j.Size, Sha256: j.SHA256, CreatedAt: ts(j.CreatedAt), FinishedAt: ts(j.FinishedAt),
	}
}

// GenerateProject queues the generation of the host project.
func (s AppProject) GenerateProject(ctx context.Context, req *connect.Request[pluxv1.GenerateProjectRequest]) (*connect.Response[pluxv1.GenerateProjectResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.GenerateProjectResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		j, err := s.h.Shells.Generate(ctx, p, m.GetAppId(), m.GetEnvironmentKey(), m.GetChannelKey(), m.GetFormat())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.GenerateProjectResponse{Job: projectJobProto(j)}, nil
	})
}

// GetProjectJob returns a generation job.
func (s AppProject) GetProjectJob(ctx context.Context, req *connect.Request[pluxv1.GetProjectJobRequest]) (*connect.Response[pluxv1.GetProjectJobResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.GetProjectJobResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		j, err := s.h.Shells.Job(ctx, p, req.Msg.GetAppId(), req.Msg.GetJobId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.GetProjectJobResponse{Job: projectJobProto(j)}, nil
	})
}

// DownloadProject returns the zip of a succeeded job.
func (s AppProject) DownloadProject(ctx context.Context, req *connect.Request[pluxv1.DownloadProjectRequest]) (*connect.Response[pluxv1.DownloadProjectResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.DownloadProjectResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		j, data, err := s.h.Shells.Download(ctx, p, req.Msg.GetAppId(), req.Msg.GetJobId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.DownloadProjectResponse{Filename: j.ID + ".zip", Archive: data, Sha256: j.SHA256}, nil
	})
}

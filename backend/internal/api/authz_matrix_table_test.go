// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api_test

import (
	"context"
	"errors"
	"io"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
)

// atPerm is shorthand for a needs entry.
func atPerm(p auth.Permission, build func(t *testing.T, f *matrixFixture) proto.Message) entry {
	return entry{kind: needs, perm: p, build: build}
}

// fixed is a builder for a request that needs nothing made first.
func fixed(f func(f *matrixFixture) proto.Message) func(*testing.T, *matrixFixture) proto.Message {
	return func(_ *testing.T, m *matrixFixture) proto.Message { return f(m) }
}

// anyone is a builder for an RPC that takes an empty request.
func anyone(msg proto.Message) func(*testing.T, *matrixFixture) proto.Message {
	return func(*testing.T, *matrixFixture) proto.Message { return proto.Clone(msg) }
}

// matrix declares, for every RPC of every service, who may call it. The
// permission is the one the handler's domain code checks; the entry is the
// specification the test holds the code to.
var matrix = map[string]entry{
	// AppService
	"AppService/CreateApp": atPerm(auth.AppManage, func(t *testing.T, f *matrixFixture) proto.Message {
		k := f.name()
		return &pluxv1.CreateAppRequest{Key: k, Name: k}
	}),
	"AppService/GetApp":   atPerm(auth.AppRead, fixed(func(f *matrixFixture) proto.Message { return &pluxv1.GetAppRequest{Id: f.app} })),
	"AppService/ListApps": atPerm(auth.AppRead, anyone(&pluxv1.ListAppsRequest{})),
	"AppService/UpdateApp": atPerm(auth.AppManage, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.UpdateAppRequest{Id: f.app, Name: "Shop"}
	})),
	"AppService/DeleteApp": atPerm(auth.AppManage, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.DeleteAppRequest{Id: f.freshApp(t)}
	}),
	"AppService/CreateEnvironment": atPerm(auth.EnvironmentManage, func(t *testing.T, f *matrixFixture) proto.Message {
		k := f.name()
		return &pluxv1.CreateEnvironmentRequest{AppId: f.app, Key: k, Name: k}
	}),
	"AppService/ListEnvironments": atPerm(auth.AppRead, fixed(func(f *matrixFixture) proto.Message { return &pluxv1.ListEnvironmentsRequest{AppId: f.app} })),
	"AppService/UpdateEnvironment": atPerm(auth.EnvironmentManage, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.UpdateEnvironmentRequest{Id: f.dev, Name: "Development"}
	})),
	"AppService/DeleteEnvironment": atPerm(auth.EnvironmentManage, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.DeleteEnvironmentRequest{Id: f.freshEnvironment(t)}
	}),
	"AppService/SetVariable": atPerm(auth.EnvironmentManage, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.SetVariableRequest{EnvironmentId: f.dev, Key: "API", Value: "https://api.example"}
	})),
	"AppService/ListVariables": atPerm(auth.AppRead, fixed(func(f *matrixFixture) proto.Message { return &pluxv1.ListVariablesRequest{EnvironmentId: f.dev} })),
	"AppService/SetSecret": atPerm(auth.SecretsManage, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.SetSecretRequest{EnvironmentId: f.dev, Key: "TOKEN", Value: "abcdefghijkl"}
	})),
	"AppService/ListSecrets": atPerm(auth.EnvironmentManage, fixed(func(f *matrixFixture) proto.Message { return &pluxv1.ListSecretsRequest{EnvironmentId: f.dev} })),
	"AppService/DeleteSecret": atPerm(auth.SecretsManage, func(t *testing.T, f *matrixFixture) proto.Message {
		must(f.w.app.SetSecret(context.Background(), req(f.owner(), &pluxv1.SetSecretRequest{EnvironmentId: f.dev, Key: "DOOMED", Value: "abcdefghijkl"})))(t)
		return &pluxv1.DeleteSecretRequest{EnvironmentId: f.dev, Key: "DOOMED"}
	}),
	"AppService/CreateChannel": atPerm(auth.EnvironmentManage, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.CreateChannelRequest{EnvironmentId: f.dev, Key: f.name()}
	}),
	"AppService/ListChannels": atPerm(auth.AppRead, fixed(func(f *matrixFixture) proto.Message { return &pluxv1.ListChannelsRequest{EnvironmentId: f.prod} })),
	"AppService/DeleteChannel": atPerm(auth.EnvironmentManage, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.DeleteChannelRequest{Id: f.freshChannel(t)}
	}),
	"AppService/GrantAccess": atPerm(auth.MembersManage, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.GrantAccessRequest{AppId: f.app, TeamId: f.freshTeam(t), Role: "viewer"}
	}),
	"AppService/RevokeAccess": atPerm(auth.MembersManage, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.RevokeAccessRequest{Id: f.freshGrant(t)}
	}),
	"AppService/ListAccess": atPerm(auth.AppRead, fixed(func(f *matrixFixture) proto.Message { return &pluxv1.ListAccessRequest{AppId: f.app} })),
	"AppService/ListTrash":  atPerm(auth.TrashManage, anyone(&pluxv1.ListTrashRequest{})),
	"AppService/RestoreFromTrash": atPerm(auth.TrashManage, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.RestoreFromTrashRequest{Id: f.freshTrash(t)}
	}),
	"AppService/PurgeFromTrash": atPerm(auth.TrashManage, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.PurgeFromTrashRequest{Id: f.freshTrash(t)}
	}),
	"AppService/ListAppLimits": atPerm(auth.AppRead, fixed(func(f *matrixFixture) proto.Message { return &pluxv1.ListAppLimitsRequest{AppId: f.app} })),
	"AppService/SetAppLimit": atPerm(auth.LimitsManage, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.SetAppLimitRequest{AppId: f.app, Key: "app.plugins", Value: 100}
	})),

	// OrgService
	"OrgService/CreateOrganization": {kind: installation, build: func(t *testing.T, f *matrixFixture) proto.Message {
		k := f.name()
		return &pluxv1.CreateOrganizationRequest{Key: k, Name: k}
	}},
	"OrgService/GetOrganization":   atPerm(auth.OrganizationRead, fixed(func(f *matrixFixture) proto.Message { return &pluxv1.GetOrganizationRequest{Id: f.org} })),
	"OrgService/ListOrganizations": {kind: self, build: anyone(&pluxv1.ListOrganizationsRequest{})},
	"OrgService/UpdateOrganization": atPerm(auth.OrganizationManage, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.UpdateOrganizationRequest{Id: f.org, Name: "Acme"}
	})),
	"OrgService/CreateTeam": atPerm(auth.MembersManage, func(t *testing.T, f *matrixFixture) proto.Message {
		k := f.name()
		return &pluxv1.CreateTeamRequest{Key: k, Name: k}
	}),
	"OrgService/ListTeams": atPerm(auth.OrganizationRead, anyone(&pluxv1.ListTeamsRequest{})),
	"OrgService/UpdateTeam": atPerm(auth.MembersManage, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.UpdateTeamRequest{Id: f.team, Name: "Web"}
	})),
	"OrgService/DeleteTeam": atPerm(auth.MembersManage, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.DeleteTeamRequest{Id: f.freshTeam(t)}
	}),
	"OrgService/AddMember": atPerm(auth.MembersManage, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.AddMemberRequest{Email: f.name() + "@example.com", Role: "viewer"}
	}),
	"OrgService/RemoveMember": atPerm(auth.MembersManage, fixed(func(*matrixFixture) proto.Message {
		return &pluxv1.RemoveMemberRequest{UserId: unknownID}
	})),
	"OrgService/ListMembers":            atPerm(auth.OrganizationRead, anyone(&pluxv1.ListMembersRequest{})),
	"OrgService/ListOrganizationLimits": atPerm(auth.OrganizationRead, anyone(&pluxv1.ListOrganizationLimitsRequest{})),
	"OrgService/SetOrganizationLimit": atPerm(auth.LimitsManage, anyone(&pluxv1.SetOrganizationLimitRequest{
		Key: "app.plugins", Value: 100,
	})),

	// PluginService
	"PluginService/CreatePlugin": atPerm(auth.AppManage, func(t *testing.T, f *matrixFixture) proto.Message {
		k := f.name()
		return &pluxv1.CreatePluginRequest{AppId: f.app, Key: k, Name: k}
	}),
	"PluginService/GetPlugin":   atPerm(auth.PluginRead, fixed(func(f *matrixFixture) proto.Message { return &pluxv1.GetPluginRequest{Id: f.plugin} })),
	"PluginService/ListPlugins": atPerm(auth.PluginRead, fixed(func(f *matrixFixture) proto.Message { return &pluxv1.ListPluginsRequest{AppId: f.app} })),
	"PluginService/UpdatePlugin": atPerm(auth.AppManage, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.UpdatePluginRequest{Id: f.plugin, Name: "Loans"}
	})),
	"PluginService/DeletePlugin": atPerm(auth.AppManage, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.DeletePluginRequest{Id: f.freshPlugin(t)}
	}),
	"PluginService/AcquireLock": atPerm(auth.PluginEdit, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.AcquireLockRequest{AppId: f.app, PluginId: f.plugin, Session: "tab-2"}
	})),
	"PluginService/RenewLock": atPerm(auth.PluginRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.RenewLockRequest{AppId: f.app, PluginId: f.plugin, Session: "tab-2"}
	})),
	"PluginService/ReleaseLock": atPerm(auth.PluginRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ReleaseLockRequest{AppId: f.app, PluginId: f.plugin, Session: "tab-2"}
	})),
	"PluginService/RequestLock": atPerm(auth.PluginEdit, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.RequestLockRequest{AppId: f.app, PluginId: f.plugin}
	})),
	"PluginService/GetLock": atPerm(auth.PluginRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.GetLockRequest{AppId: f.app, PluginId: f.plugin}
	})),
	"PluginService/ListPluginLimits": atPerm(auth.PluginRead, fixed(func(f *matrixFixture) proto.Message { return &pluxv1.ListPluginLimitsRequest{PluginId: f.plugin} })),
	"PluginService/SetPluginLimit": atPerm(auth.LimitsManage, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.SetPluginLimitRequest{PluginId: f.plugin, Key: "plugin.pages", Value: 50}
	})),

	// DocumentService
	"DocumentService/GetDocument": atPerm(auth.PluginRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.GetDocumentRequest{AppId: f.app, PluginId: f.plugin, Path: "plugins/loans/plugin.json"}
	})),
	"DocumentService/ListDocuments": atPerm(auth.PluginRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ListDocumentsRequest{AppId: f.app, PluginId: f.plugin}
	})),
	"DocumentService/PutDocument": atPerm(auth.PluginEdit, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.PutDocumentRequest{AppId: f.app, PluginId: f.plugin, Path: "plugins/loans/pages/home.page.json", Content: []byte(pageDoc), Session: "tab-2"}
	})),
	"DocumentService/PatchDocument": atPerm(auth.PluginEdit, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.PatchDocumentRequest{AppId: f.app, PluginId: f.plugin, Path: "plugins/loans/pages/home.page.json", IfRevision: 1, Session: "tab-2"}
	})),
	"DocumentService/DeleteDocument": atPerm(auth.PluginEdit, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.DeleteDocumentRequest{AppId: f.app, PluginId: f.plugin, Path: "plugins/loans/pages/home.page.json", IfRevision: 1, Session: "tab-2"}
	})),
	"DocumentService/ValidateDraft": atPerm(auth.PluginRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ValidateDraftRequest{AppId: f.app, PluginId: f.plugin}
	})),
	"DocumentService/ValidatePage": atPerm(auth.PluginRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ValidatePageRequest{AppId: f.app, PluginId: f.plugin, Path: "plugins/loans/pages/home.page.json", Content: []byte(pageDoc)}
	})),
	"DocumentService/ListSnapshots": atPerm(auth.PluginRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ListSnapshotsRequest{AppId: f.app, PluginId: f.plugin}
	})),
	"DocumentService/GetSnapshot": atPerm(auth.PluginRead, fixed(func(f *matrixFixture) proto.Message { return &pluxv1.GetSnapshotRequest{Id: f.snaps[0]} })),
	"DocumentService/CompareSnapshots": atPerm(auth.PluginRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.CompareSnapshotsRequest{FromId: f.snaps[1], ToId: f.snaps[0]}
	})),
	"DocumentService/RestoreSnapshot": atPerm(auth.PluginEdit, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.RestoreSnapshotRequest{Id: f.snaps[1], Session: "tab-2"}
	})),
	"DocumentService/ExportDraft": {kind: needs, perm: auth.PluginRead, call: func(t *testing.T, f *matrixFixture, c caller) error {
		return drain(f.w.document.ExportDraft(context.Background(), req(c, &pluxv1.ExportDraftRequest{AppId: f.app, PluginId: f.plugin})))
	}},
	"DocumentService/ImportDraft": {kind: needs, perm: auth.PluginEdit, call: func(t *testing.T, f *matrixFixture, c caller) error {
		stream := f.w.document.ImportDraft(context.Background())
		withHeaders(stream.RequestHeader(), c)
		err := stream.Send(&pluxv1.ImportDraftRequest{AppId: f.app, PluginId: f.plugin, Session: "tab-2", Path: "plugins/loans/plugin.json", Content: []byte(pluginDoc)})
		if err != nil && !errors.Is(err, io.EOF) {
			t.Fatal(err)
		}
		_, err = stream.CloseAndReceive()
		return err //nolint:wrapcheck // the code is what the test reads
	}},

	// AssetService
	"AssetService/UploadAsset": {kind: needs, perm: auth.AppManage, call: func(t *testing.T, f *matrixFixture, c caller) error {
		stream := f.w.asset.UploadAsset(context.Background())
		withHeaders(stream.RequestHeader(), c)
		err := stream.Send(&pluxv1.UploadAssetRequest{AppId: f.app, Name: "images/" + f.name() + ".png", Session: "tab-2", Chunk: f.png(t)})
		if err != nil && !errors.Is(err, io.EOF) {
			t.Fatal(err)
		}
		_, err = stream.CloseAndReceive()
		return err //nolint:wrapcheck // the code is what the test reads
	}},
	"AssetService/GetAsset": atPerm(auth.PluginRead, anyone(&pluxv1.GetAssetRequest{Id: unknownID})),
	"AssetService/ListAssets": atPerm(auth.PluginRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ListAssetsRequest{AppId: f.app}
	})),
	"AssetService/DeleteAsset": atPerm(auth.AppManage, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.DeleteAssetRequest{Id: f.freshAsset(t), Session: "tab-2"}
	}),

	// ComponentService and TemplateService
	"ComponentService/ListComponents": atPerm(auth.PluginRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ListComponentsRequest{AppId: f.app, PluginId: f.plugin}
	})),
	"ComponentService/GetComponent": atPerm(auth.PluginRead, anyone(&pluxv1.GetComponentRequest{Id: unknownID})),
	"ComponentService/ListUsages": atPerm(auth.PluginRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ListUsagesRequest{AppId: f.app, EntityKind: "component", EntityId: unknownID}
	})),
	"TemplateService/ListTemplates": atPerm(auth.PluginRead, fixed(func(f *matrixFixture) proto.Message { return &pluxv1.ListTemplatesRequest{AppId: f.app} })),
	"TemplateService/GetTemplate":   atPerm(auth.PluginRead, fixed(func(f *matrixFixture) proto.Message { return &pluxv1.GetTemplateRequest{Id: f.template} })),
	"TemplateService/Instantiate": atPerm(auth.PluginEdit, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.InstantiateRequest{TemplateId: f.template, AppId: f.app, PluginId: f.plugin, Session: "tab-2", Key: "copy"}
	})),

	// LocalisationService
	"LocalisationService/ExportTranslations": atPerm(auth.AppRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ExportTranslationsRequest{AppId: f.app, Locale: "de", Format: "csv"}
	})),
	"LocalisationService/ImportTranslations": atPerm(auth.TranslationsEdit, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ImportTranslationsRequest{AppId: f.app, Locale: "de", Format: "csv", Content: []byte("keyId,target\n"), DryRun: true}
	})),
	"LocalisationService/GetStatus": atPerm(auth.AppRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.GetStatusRequest{AppId: f.app}
	})),
	"LocalisationService/SetStates": atPerm(auth.TranslationsEdit, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.SetStatesRequest{AppId: f.app, Locale: "de", States: map[string]string{unknownID: "reviewed"}}
	})),

	// PublishService and ReleaseService
	"PublishService/Publish": atPerm(auth.ReleasePublish, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.PublishRequest{AppId: f.app, PluginId: f.plugin, EnvironmentId: f.dev}
	})),
	"PublishService/WatchPublish": {kind: needs, perm: auth.PluginRead, call: func(t *testing.T, f *matrixFixture, c caller) error {
		return drain(f.w.publish.WatchPublish(context.Background(), req(c, &pluxv1.WatchPublishRequest{JobId: f.job})))
	}},
	"PublishService/GetPublishJob": atPerm(auth.PluginRead, fixed(func(f *matrixFixture) proto.Message { return &pluxv1.GetPublishJobRequest{JobId: f.job} })),
	"PublishService/ListPublishJobs": atPerm(auth.PluginRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ListPublishJobsRequest{AppId: f.app, PluginId: f.plugin}
	})),
	"PublishService/CancelPublishJob": atPerm(auth.ReleasePublish, fixed(func(f *matrixFixture) proto.Message { return &pluxv1.CancelPublishJobRequest{JobId: f.job} })),
	"ReleaseService/GetPluginVersion": atPerm(auth.PluginRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.GetPluginVersionRequest{PluginId: f.plugin, Version: 1}
	})),
	"ReleaseService/ListPluginVersions": atPerm(auth.PluginRead, fixed(func(f *matrixFixture) proto.Message { return &pluxv1.ListPluginVersionsRequest{PluginId: f.plugin} })),
	"ReleaseService/CreateRelease": atPerm(auth.ReleasePublish, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.CreateReleaseRequest{AppId: f.app, EnvironmentId: f.dev}
	})),
	"ReleaseService/GetRelease": atPerm(auth.PluginRead, fixed(func(f *matrixFixture) proto.Message { return &pluxv1.GetReleaseRequest{AppId: f.app, Sequence: 1} })),
	"ReleaseService/ListReleases": atPerm(auth.PluginRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ListReleasesRequest{AppId: f.app, EnvironmentId: f.dev}
	})),
	"ReleaseService/PromoteRelease": atPerm(auth.ReleasePromote, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.PromoteReleaseRequest{AppId: f.app, Sequence: 1, EnvironmentId: f.dev, ChannelKey: f.channel}
	})),
	"ReleaseService/RollbackRelease": atPerm(auth.ReleasePromote, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.RollbackReleaseRequest{AppId: f.app, EnvironmentId: f.dev, ChannelKey: f.channel, ToSequence: 1}
	})),
	"ReleaseService/GetChangelog": atPerm(auth.PluginRead, fixed(func(f *matrixFixture) proto.Message { return &pluxv1.GetChangelogRequest{AppId: f.app, Sequence: 1} })),
	"ReleaseService/GetReport": atPerm(auth.PluginRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.GetReportRequest{AppId: f.app, Sequence: 1, Kind: pluxv1.ReportKind_REPORT_KIND_ACCESSIBILITY}
	})),
	"ReleaseService/GetCompatibility": atPerm(auth.PluginRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.GetCompatibilityRequest{AppId: f.app, Sequence: 1}
	})),

	// ControlService, NativeCatalogueService, SecurityAdminService
	"ControlService/GetControl": atPerm(auth.AppRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.GetControlRequest{AppId: f.app, EnvironmentId: f.prod, ChannelKey: f.channel}
	})),
	"ControlService/SetControl": atPerm(auth.ReleasePromote, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.SetControlRequest{AppId: f.app, EnvironmentId: f.prod, ChannelKey: f.channel}
	})),
	"NativeCatalogueService/UploadNativeCatalogue": atPerm(auth.ReleasePublish, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.UploadNativeCatalogueRequest{AppId: f.app, HostBuild: "1.0.0+1", Catalogue: []byte(`{}`)}
	})),
	"NativeCatalogueService/GetNativeCatalogue": atPerm(auth.AppRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.GetNativeCatalogueRequest{AppId: f.app, HostBuild: "1.0.0+1"}
	})),
	"NativeCatalogueService/ListHostBuilds": atPerm(auth.AppRead, fixed(func(f *matrixFixture) proto.Message { return &pluxv1.ListHostBuildsRequest{AppId: f.app} })),
	"SecurityAdminService/SetSecurityConfig": atPerm(auth.AppManage, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.SetSecurityConfigRequest{AppId: f.app, EnvironmentId: f.dev, Profile: "standard"}
	})),
	"SecurityAdminService/GetEffectiveSecurityConfig": atPerm(auth.AppRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.GetEffectiveSecurityConfigRequest{AppId: f.app, EnvironmentId: f.dev}
	})),
	"SecurityAdminService/CreateUpstream": atPerm(auth.AppManage, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.CreateUpstreamRequest{Upstream: f.upstream("")}
	}),
	"SecurityAdminService/GetUpstream": atPerm(auth.AppRead, anyone(&pluxv1.GetUpstreamRequest{Id: unknownID})),
	"SecurityAdminService/ListUpstreams": atPerm(auth.AppRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ListUpstreamsRequest{AppId: f.app, EnvironmentId: f.dev}
	})),
	"SecurityAdminService/UpdateUpstream": atPerm(auth.AppManage, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.UpdateUpstreamRequest{Upstream: f.upstream(f.freshUpstream(t).GetId())}
	}),
	"SecurityAdminService/DeleteUpstream": atPerm(auth.AppManage, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.DeleteUpstreamRequest{Id: f.freshUpstream(t).GetId()}
	}),
	"SecurityAdminService/SetUserIdentityProvider": atPerm(auth.AppManage, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.SetUserIdentityProviderRequest{Provider: &pluxv1.UserIdentityProvider{AppId: f.app, EnvironmentId: f.dev}}
	})),
	"SecurityAdminService/GetUserIdentityProvider": atPerm(auth.AppRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.GetUserIdentityProviderRequest{AppId: f.app, EnvironmentId: f.dev}
	})),
	"SecurityAdminService/DeleteUserIdentityProvider": atPerm(auth.AppManage, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.DeleteUserIdentityProviderRequest{AppId: f.app, EnvironmentId: f.dev}
	})),
	"SecurityAdminService/SetConfigValue": atPerm(auth.AppManage, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.SetConfigValueRequest{AppId: f.app, EnvironmentId: f.dev, Value: &pluxv1.ConfigValue{
			Key: "matrix", Tier: pluxv1.ConfigTier_CONFIG_TIER_LIVE, Type: pluxv1.ConfigType_CONFIG_TYPE_STRING, Value: "v",
		}}
	})),
	"SecurityAdminService/GetConfigValue": atPerm(auth.AppRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.GetConfigValueRequest{AppId: f.app, EnvironmentId: f.dev, Key: "matrix"}
	})),
	"SecurityAdminService/ListConfigValues": atPerm(auth.AppRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ListConfigValuesRequest{AppId: f.app, EnvironmentId: f.dev}
	})),
	"SecurityAdminService/DeleteConfigValue": atPerm(auth.AppManage, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.DeleteConfigValueRequest{AppId: f.app, EnvironmentId: f.dev, Key: "absent"}
	})),
	"ConfigService/GetLiveConfig":               {kind: deviceRPC, build: anyone(&pluxv1.GetLiveConfigRequest{})},
	"DeviceService/EnrolScaKey":                 {kind: deviceRPC, build: anyone(&pluxv1.EnrolScaKeyRequest{})},
	"SecurityAdminService/ListAuditCheckpoints": atPerm(auth.AuditRead, anyone(&pluxv1.ListAuditCheckpointsRequest{})),

	// FunctionAdminService
	"FunctionAdminService/UploadSource": {kind: needs, perm: auth.FunctionDeploy, call: func(t *testing.T, f *matrixFixture, c caller) error {
		_, err := f.w.uploadSource(t, c, f.app, f.dev, "matrix.fn", "archive")
		return err
	}},
	"FunctionAdminService/Build": atPerm(auth.FunctionDeploy, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.BuildRequest{SourceId: f.freshSource(t)}
	}),
	"FunctionAdminService/GetBuild": atPerm(auth.FunctionRead, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.GetBuildRequest{BuildId: f.freshFunctionBuild(t, f.dev)}
	}),
	"FunctionAdminService/GetFunction": atPerm(auth.FunctionRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.GetFunctionRequest{AppId: f.app, EnvironmentId: f.dev, Name: "matrix.fn"}
	})),
	"FunctionAdminService/ListFunctions": atPerm(auth.FunctionRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ListFunctionsRequest{AppId: f.app, EnvironmentId: f.dev}
	})),
	"FunctionAdminService/ListVersions": atPerm(auth.FunctionRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ListVersionsRequest{AppId: f.app, EnvironmentId: f.dev, Name: "matrix.fn"}
	})),
	"FunctionAdminService/Deploy": atPerm(auth.FunctionDeploy, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.DeployRequest{BuildId: f.freshFunctionBuild(t, f.dev)}
	}),
	"FunctionAdminService/PromoteFunction": atPerm(auth.FunctionDeploy, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.PromoteFunctionRequest{AppId: f.app, FromEnvironmentId: f.prod, ToEnvironmentId: f.dev, Name: "matrix.fn", Version: 1}
	})),
	"FunctionAdminService/SetAlias": atPerm(auth.FunctionDeploy, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.SetAliasRequest{AppId: f.app, EnvironmentId: f.dev, Name: "matrix.fn", Alias: &pluxv1.Alias{Name: "stable", Version: 1}}
	})),
	"FunctionAdminService/ListLogs": atPerm(auth.FunctionRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ListLogsRequest{AppId: f.app, EnvironmentId: f.dev, Name: "matrix.fn"}
	})),
	"FunctionAdminService/GetMetrics": atPerm(auth.FunctionRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.GetMetricsRequest{AppId: f.app, EnvironmentId: f.dev, Name: "matrix.fn"}
	})),

	// DeviceService, TelemetryService: the people's side
	"DeviceService/GetDevice": atPerm(auth.AppRead, fixed(func(f *matrixFixture) proto.Message { return &pluxv1.GetDeviceRequest{Id: f.device} })),
	"DeviceService/ListDevices": atPerm(auth.AppRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ListDevicesRequest{AppId: f.app, EnvironmentId: f.dev}
	})),
	"DeviceService/RevokeDevice": atPerm(auth.AppManage, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.RevokeDeviceRequest{DeviceId: f.device, Reason: "test"}
	})),
	"TelemetryService/ListEvents": atPerm(auth.AppRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ListEventsRequest{AppId: f.app, Environment: "development"}
	})),

	// IdentityService: sign-in, the person's own account, tokens, audit
	"IdentityService/AcceptInvitation":           {kind: public, build: anyone(&pluxv1.AcceptInvitationRequest{})},
	"IdentityService/StartPasswordLogin":         {kind: public, build: anyone(&pluxv1.StartPasswordLoginRequest{})},
	"IdentityService/CompleteMfa":                {kind: public, build: anyone(&pluxv1.CompleteMfaRequest{})},
	"IdentityService/StartOidcLogin":             {kind: public, build: anyone(&pluxv1.StartOidcLoginRequest{})},
	"IdentityService/CompleteOidcLogin":          {kind: public, build: anyone(&pluxv1.CompleteOidcLoginRequest{})},
	"IdentityService/BeginWebAuthnLogin":         {kind: public, build: anyone(&pluxv1.BeginWebAuthnLoginRequest{})},
	"IdentityService/CompleteWebAuthnLogin":      {kind: public, build: anyone(&pluxv1.CompleteWebAuthnLoginRequest{})},
	"IdentityService/StartDeviceAuthorization":   {kind: public, build: anyone(&pluxv1.StartDeviceAuthorizationRequest{Client: "plux-cli"})},
	"IdentityService/PollDeviceAuthorization":    {kind: public, build: anyone(&pluxv1.PollDeviceAuthorizationRequest{})},
	"IdentityService/ExchangeWorkloadIdentity":   {kind: public, build: anyone(&pluxv1.ExchangeWorkloadIdentityRequest{})},
	"IdentityService/GetCurrentUser":             {kind: self, build: anyone(&pluxv1.GetCurrentUserRequest{})},
	"IdentityService/ChangePassword":             {kind: self, ownSession: true, build: anyone(&pluxv1.ChangePasswordRequest{CurrentPassword: "correct horse battery", NewPassword: "another good passphrase"})},
	"IdentityService/Logout":                     {kind: self, ownSession: true, build: anyone(&pluxv1.LogoutRequest{})},
	"IdentityService/VerifySecondFactor":         {kind: self, ownSession: true, answersUnauthenticated: true, build: anyone(&pluxv1.VerifySecondFactorRequest{})},
	"IdentityService/EnrollTotp":                 {kind: self, build: anyone(&pluxv1.EnrollTotpRequest{Label: "spare"})},
	"IdentityService/BeginWebAuthnRegistration":  {kind: self, build: anyone(&pluxv1.BeginWebAuthnRegistrationRequest{})},
	"IdentityService/FinishWebAuthnRegistration": {kind: self, build: anyone(&pluxv1.FinishWebAuthnRegistrationRequest{})},
	"IdentityService/ConfirmFactor":              {kind: self, build: anyone(&pluxv1.ConfirmFactorRequest{})},
	"IdentityService/ListFactors":                {kind: self, build: anyone(&pluxv1.ListFactorsRequest{})},
	"IdentityService/DeleteFactor":               {kind: self, build: anyone(&pluxv1.DeleteFactorRequest{FactorId: unknownID})},
	"IdentityService/ApproveDeviceAuthorization": {kind: self, build: anyone(&pluxv1.ApproveDeviceAuthorizationRequest{})},
	"IdentityService/DenyDeviceAuthorization":    {kind: self, build: anyone(&pluxv1.DenyDeviceAuthorizationRequest{})},
	"IdentityService/CreateAccessToken":          {kind: self, build: anyone(&pluxv1.CreateAccessTokenRequest{Name: "t", Scopes: []string{"app.read"}, TtlSeconds: 60})},
	"IdentityService/ListAccessTokens":           {kind: self, build: anyone(&pluxv1.ListAccessTokensRequest{})},
	"IdentityService/RevokeAccessToken": atPerm(auth.TokensManage, func(t *testing.T, f *matrixFixture) proto.Message {
		// The owner's token: others may revoke it only with tokens.manage.
		minted := must(f.w.identity.CreateAccessToken(context.Background(), req(f.owner(), &pluxv1.CreateAccessTokenRequest{Name: f.name(), Scopes: []string{"app.read"}, TtlSeconds: 60})))(t)
		return &pluxv1.RevokeAccessTokenRequest{Id: minted.GetToken().GetId()}
	}),
	"IdentityService/CreateWorkloadIdentity": atPerm(auth.TokensManage, anyone(&pluxv1.CreateWorkloadIdentityRequest{
		Issuer: "https://issuer.example", Audience: "x", SubjectPattern: "repo:a/b", Scopes: []string{"app.read"},
	})),
	"IdentityService/ListWorkloadIdentities": atPerm(auth.OrganizationRead, anyone(&pluxv1.ListWorkloadIdentitiesRequest{})),
	"IdentityService/DeleteWorkloadIdentity": atPerm(auth.TokensManage, anyone(&pluxv1.DeleteWorkloadIdentityRequest{Id: unknownID})),
	"IdentityService/ListAuditEntries":       atPerm(auth.AuditRead, anyone(&pluxv1.ListAuditEntriesRequest{})),

	// The device's side: a person's session never satisfies these.
	"DeviceService/RegisterDevice":              {kind: deviceRPC, build: anyone(&pluxv1.RegisterDeviceRequest{})},
	"DeviceService/ReportInstalled":             {kind: deviceRPC, build: anyone(&pluxv1.ReportInstalledRequest{})},
	"DeviceService/CreateRegistrationChallenge": {kind: deviceRPC, build: anyone(&pluxv1.CreateRegistrationChallengeRequest{})},
	"DeviceService/RegisterAttestedDevice":      {kind: deviceRPC, build: anyone(&pluxv1.RegisterAttestedDeviceRequest{})},
	"DeviceService/ReattestDevice":              {kind: deviceRPC, build: anyone(&pluxv1.ReattestDeviceRequest{})},
	"TelemetryService/IngestEvents":             {kind: deviceRPC, build: anyone(&pluxv1.IngestEventsRequest{})},
	"TokenService/IssueDeviceToken":             {kind: deviceRPC, build: anyone(&pluxv1.IssueDeviceTokenRequest{})},
	"TokenService/RefreshDeviceToken":           {kind: deviceRPC, build: anyone(&pluxv1.RefreshDeviceTokenRequest{})},
	"TokenService/ExchangeUserToken":            {kind: deviceRPC, build: anyone(&pluxv1.ExchangeUserTokenRequest{})},
	"ManifestService/GetManifest":               {kind: deviceRPC, build: anyone(&pluxv1.GetManifestRequest{})},
	"ManifestService/GetRootKeys":               {kind: deviceRPC, build: anyone(&pluxv1.GetRootKeysRequest{})},
	"GatewayService/Call":                       {kind: deviceRPC, build: anyone(&pluxv1.CallRequest{})},
	"ScaService/CreateTransactionChallenge":     {kind: deviceRPC, build: anyone(&pluxv1.CreateTransactionChallengeRequest{})},
	"FunctionService/Invoke":                    {kind: deviceRPC, build: anyone(&pluxv1.FunctionServiceInvokeRequest{})},
}

// variants are calls of an RPC that need more than the RPC's own entry
// declares.
var variants = map[string]entry{
	// Deploying to a production environment is the stand-in for an
	// approval until P9, with a second factor (FN-007).
	"FunctionAdminService/Deploy (production)": atPerm(auth.FunctionDeployProduction, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.DeployRequest{BuildId: f.freshFunctionBuild(t, f.prod)}
	}),
	"FunctionAdminService/PromoteFunction (production)": atPerm(auth.FunctionDeployProduction, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.PromoteFunctionRequest{AppId: f.app, FromEnvironmentId: f.dev, ToEnvironmentId: f.prod, Name: "matrix.fn", Version: 1}
	})),
	"FunctionAdminService/SetAlias (production)": atPerm(auth.FunctionDeployProduction, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.SetAliasRequest{AppId: f.app, EnvironmentId: f.prod, Name: "matrix.fn", Alias: &pluxv1.Alias{Name: "stable", Version: 1}}
	})),
	// Taking a lock another session holds is a separate permission.
	"PluginService/AcquireLock (force)": atPerm(auth.PluginLockOverride, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.AcquireLockRequest{AppId: f.app, PluginId: f.plugin, Session: "tab-3", Force: true}
	})),
}

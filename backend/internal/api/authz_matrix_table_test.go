// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"

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

// matrixWebhook is the endpoint the notification-channel matrix entry names.
const matrixWebhook = "https://hooks.slack.com/services/T0/B0/xxxxxxxx"

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
	"OrgService/CreateTeam": atPerm(auth.TeamsManage, func(t *testing.T, f *matrixFixture) proto.Message {
		k := f.name()
		return &pluxv1.CreateTeamRequest{Key: k, Name: k}
	}),
	"OrgService/ListTeams": atPerm(auth.OrganizationRead, anyone(&pluxv1.ListTeamsRequest{})),
	"OrgService/UpdateTeam": atPerm(auth.TeamsManage, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.UpdateTeamRequest{Id: f.team, Name: "Web"}
	})),
	"OrgService/DeleteTeam": atPerm(auth.TeamsManage, func(t *testing.T, f *matrixFixture) proto.Message {
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
	"PluginService/DuplicatePlugin": atPerm(auth.AppManage, func(t *testing.T, f *matrixFixture) proto.Message {
		k := f.name()
		return &pluxv1.DuplicatePluginRequest{Id: f.plugin, Key: k, Name: k, Session: "tab-3"}
	}),
	"PluginService/ArchivePlugin": atPerm(auth.AppManage, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ArchivePluginRequest{Id: f.plugin, Archived: false}
	})),
	"PluginService/ListPluginSummaries": atPerm(auth.PluginRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ListPluginSummariesRequest{AppId: f.app}
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
	"DocumentService/ApplyChangeSet": atPerm(auth.PluginEdit, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ApplyChangeSetRequest{AppId: f.app, Drafts: []*pluxv1.ChangeSetDraft{{
			PluginId: f.plugin, Session: "tab-2",
			Edits: []*pluxv1.ChangeSetEdit{{Path: "plugins/loans/pages/home.page.json", Content: []byte(pageDoc)}},
		}}}
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
	"DocumentService/ListEditorSessionEntries": atPerm(auth.PluginRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ListEditorSessionEntriesRequest{AppId: f.app, PluginId: f.plugin, EditorSessionId: "tab-1"}
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
	"AssetService/BeginAssetUpload": atPerm(auth.AppManage, func(t *testing.T, f *matrixFixture) proto.Message {
		png := f.png(t)
		sum := sha256.Sum256(png)
		return &pluxv1.BeginAssetUploadRequest{AppId: f.app, Name: "images/" + f.name() + ".png", Session: "tab-2", Size: int64(len(png)), Sha256: sum[:]}
	}),
	"AssetService/UploadAssetChunk": atPerm(auth.AppManage, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.UploadAssetChunkRequest{UploadId: f.freshUpload(t), Chunk: f.png(t)}
	}),
	"AssetService/GetAssetUpload": atPerm(auth.AppManage, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.GetAssetUploadRequest{UploadId: f.freshUpload(t)}
	}),
	"AssetService/FinishAssetUpload": atPerm(auth.AppManage, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.FinishAssetUploadRequest{UploadId: f.freshUpload(t)}
	}),
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

	// The library's authoring calls (STU-080 to STU-086, ADR-0098). Items the
	// calls act on are made by the owner and shared with the organisation, so
	// that a refusal is the role's and not a hidden item's.
	"TemplateService/CreateTemplate": atPerm(auth.PluginEdit, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.CreateTemplateRequest{Draft: &pluxv1.LibraryDraft{Content: []byte(f.libraryDoc("template"))}}
	}),
	"TemplateService/ReviseTemplate": atPerm(auth.LibraryManage, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.ReviseTemplateRequest{Id: f.libraryItem(t, "template"), ExpectedVersion: 1, Draft: &pluxv1.LibraryDraft{Content: []byte(f.libraryDocOf("template", f.libraryItem(t, "template")))}}
	}),
	"TemplateService/ListTemplateVersions": atPerm(auth.PluginRead, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.ListTemplateVersionsRequest{Id: f.libraryItem(t, "template")}
	}),
	"TemplateService/ListLibraryTemplates": atPerm(auth.PluginRead, anyone(&pluxv1.ListLibraryTemplatesRequest{})),
	"TemplateService/SetTemplateVisibility": atPerm(auth.LibraryPublish, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.SetTemplateVisibilityRequest{Id: f.libraryItem(t, "template"), Visibility: "public"}
	}),
	"TemplateService/ExportTemplate": atPerm(auth.PluginRead, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.ExportTemplateRequest{Id: f.libraryItem(t, "template")}
	}),
	"TemplateService/ImportTemplate": atPerm(auth.PluginEdit, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.ImportTemplateRequest{File: []byte("{}")}
	}),
	"ComponentService/CreateComponent": atPerm(auth.PluginEdit, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.CreateComponentRequest{Draft: &pluxv1.LibraryDraft{Content: []byte(f.libraryDoc("component"))}}
	}),
	"ComponentService/ReviseComponent": atPerm(auth.LibraryManage, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.ReviseComponentRequest{Id: f.libraryItem(t, "component"), ExpectedVersion: 1, Draft: &pluxv1.LibraryDraft{Content: []byte(f.libraryDocOf("component", f.libraryItem(t, "component")))}}
	}),
	"ComponentService/ListComponentVersions": atPerm(auth.PluginRead, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.ListComponentVersionsRequest{Id: f.libraryItem(t, "component")}
	}),
	"ComponentService/ListLibraryComponents": atPerm(auth.PluginRead, anyone(&pluxv1.ListLibraryComponentsRequest{})),
	"ComponentService/SetComponentVisibility": atPerm(auth.LibraryPublish, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.SetComponentVisibilityRequest{Id: f.libraryItem(t, "component"), Visibility: "public"}
	}),
	"ComponentService/ExportComponent": atPerm(auth.PluginRead, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.ExportComponentRequest{Id: f.libraryItem(t, "component")}
	}),
	"ComponentService/ImportComponent": atPerm(auth.PluginEdit, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.ImportComponentRequest{File: []byte("{}")}
	}),
	"ComponentService/AdoptComponentVersion": atPerm(auth.PluginEdit, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.AdoptComponentVersionRequest{AppId: f.app, Id: f.libraryItem(t, "component"), Version: 1}
	}),
	"ComponentService/ListComponentAdoptions": atPerm(auth.PluginRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ListComponentAdoptionsRequest{AppId: f.app}
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

	// GovernanceService (GOV-002, GOV-003)
	"GovernanceService/ListPermissions": atPerm(auth.OrganizationRead, anyone(&pluxv1.ListPermissionsRequest{})),
	"AnalyticsService/QueryRollups": atPerm(auth.AnalyticsRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.QueryRollupsRequest{
			AppId: f.app, Metric: "screen_view", Granularity: "1h",
			From: timestamppb.New(time.Unix(0, 0)), To: timestamppb.New(time.Unix(3600, 0)),
		}
	})),
	"AnalyticsService/GetRetention":          atPerm(auth.AnalyticsRead, anyone(&pluxv1.GetRetentionRequest{})),
	"AnalyticsService/ListRetentionChanges":  atPerm(auth.AnalyticsRead, anyone(&pluxv1.ListRetentionChangesRequest{})),
	"AnalyticsService/GetExport":             atPerm(auth.AnalyticsRead, anyone(&pluxv1.GetExportRequest{})),
	"AnalyticsService/SetRetention":          atPerm(auth.RetentionManage, anyone(&pluxv1.SetRetentionRequest{DataClass: "raw_events", Days: 90})),
	"AnalyticsService/CancelRetentionChange": atPerm(auth.RetentionManage, anyone(&pluxv1.CancelRetentionChangeRequest{Id: unknownID})),
	"AnalyticsService/SetExport":             atPerm(auth.OrganizationManage, anyone(&pluxv1.SetExportRequest{Endpoint: "https://127.0.0.1:1"})),
	"AnalyticsService/GetFileExport":         atPerm(auth.AnalyticsRead, anyone(&pluxv1.GetFileExportRequest{})),
	"AnalyticsService/SetFileExport":         atPerm(auth.OrganizationManage, anyone(&pluxv1.SetFileExportRequest{Settings: &pluxv1.FileExportSettings{ParquetGranularity: "7d"}})),
	"AnalyticsService/DeleteExport":          atPerm(auth.OrganizationManage, anyone(&pluxv1.DeleteExportRequest{})),
	"AnalyticsService/CreateAlertRule": atPerm(auth.AlertsManage, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.CreateAlertRuleRequest{Rule: &pluxv1.AlertRule{
			AppId: f.app, Name: f.name(), Metric: "fatal_error", Aggregation: "count", WindowSeconds: 300,
			Comparison: ">", Threshold: 5, Severity: "warning",
		}}
	}),
	"AnalyticsService/UpdateAlertRule": atPerm(auth.AlertsManage, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.UpdateAlertRuleRequest{Rule: f.sharedAlertRule(t)}
	}),
	"AnalyticsService/DeleteAlertRule": atPerm(auth.AlertsManage, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.DeleteAlertRuleRequest{Id: f.freshAlertRule(t).GetId()}
	}),
	"AnalyticsService/ListAlertRules":  atPerm(auth.AnalyticsRead, anyone(&pluxv1.ListAlertRulesRequest{})),
	"AnalyticsService/ListAlertStates": atPerm(auth.AnalyticsRead, anyone(&pluxv1.ListAlertStatesRequest{})),
	"AnalyticsService/ListErrorGroups": atPerm(auth.AnalyticsRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ListErrorGroupsRequest{AppId: f.app}
	})),
	"AnalyticsService/GetErrorGroup": atPerm(auth.AnalyticsRead, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.GetErrorGroupRequest{Id: f.errorGroup(t)}
	}),
	"GovernanceService/ListRoles": atPerm(auth.OrganizationRead, anyone(&pluxv1.ListRolesRequest{})),
	"GovernanceService/GetRole": atPerm(auth.OrganizationRead, fixed(func(*matrixFixture) proto.Message {
		return &pluxv1.GetRoleRequest{Key: "viewer"}
	})),
	"GovernanceService/CreateRole": atPerm(auth.RolesManage, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.CreateRoleRequest{Key: "role-" + f.name(), Name: "Role", Permissions: []string{"app.read"}}
	}),
	"GovernanceService/UpdateRole": atPerm(auth.RolesManage, fixed(func(*matrixFixture) proto.Message {
		return &pluxv1.UpdateRoleRequest{Key: "no-such-role", Name: "Role", Permissions: []string{"app.read"}}
	})),
	"GovernanceService/DeleteRole": atPerm(auth.RolesManage, fixed(func(*matrixFixture) proto.Message {
		return &pluxv1.DeleteRoleRequest{Key: "no-such-role"}
	})),
	"GovernanceService/ListGrants": atPerm(auth.OrganizationRead, anyone(&pluxv1.ListGrantsRequest{})),
	"GovernanceService/CreateGrant": atPerm(auth.MembersManage, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.CreateGrantRequest{TeamId: f.freshTeam(t), Role: "viewer", Scope: "organization"}
	}),
	"GovernanceService/UpdateGrant": atPerm(auth.MembersManage, fixed(func(*matrixFixture) proto.Message {
		return &pluxv1.UpdateGrantRequest{Id: unknownID, Role: "viewer"}
	})),
	"GovernanceService/DeleteGrant": atPerm(auth.MembersManage, fixed(func(*matrixFixture) proto.Message {
		return &pluxv1.DeleteGrantRequest{Id: unknownID}
	})),

	// Restrictions and effective permissions (GOV-006, GOV-007): managing
	// restrictions needs restrictions.manage; a person's own effective
	// permissions, change notices and explanation need nothing.
	"GovernanceService/CreateRestriction": atPerm(auth.RestrictionsManage, fixed(func(*matrixFixture) proto.Message {
		return &pluxv1.CreateRestrictionRequest{SubjectKind: "user", SubjectId: unknownID, Patterns: []string{"app.read"}}
	})),
	"GovernanceService/ListRestrictions": atPerm(auth.RestrictionsManage, anyone(&pluxv1.ListRestrictionsRequest{})),
	"GovernanceService/DeleteRestriction": atPerm(auth.RestrictionsManage, fixed(func(*matrixFixture) proto.Message {
		return &pluxv1.DeleteRestrictionRequest{Id: unknownID}
	})),
	"GovernanceService/GetEffectivePermissions": {kind: self, build: anyone(&pluxv1.GetEffectivePermissionsRequest{})},
	"GovernanceService/WatchPermissions": {kind: self, call: func(t *testing.T, f *matrixFixture, c caller) error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		stream, err := f.w.govern.WatchPermissions(ctx, req(c, &pluxv1.WatchPermissionsRequest{}))
		if err != nil {
			return err //nolint:wrapcheck // the code is what the test reads
		}
		defer func() { _ = stream.Close() }()
		if !stream.Receive() {
			return stream.Err() //nolint:wrapcheck // the code is what the test reads
		}
		return nil
	}},
	"GovernanceService/ExplainAccess": {kind: self, build: anyone(&pluxv1.ExplainAccessRequest{Permission: "app.read"})},

	// ApprovalService (GOV-020 to GOV-023): reading policies and requests
	// needs the app, writing a policy needs policies.manage and deciding
	// needs approvals.decide.
	"ApprovalService/PutPolicy": atPerm(auth.PoliciesManage, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.PutPolicyRequest{AppId: f.app, Document: []byte(`{}`)}
	})),
	"ApprovalService/GetPolicy": atPerm(auth.AppRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.GetPolicyRequest{AppId: f.app, Key: "no-such-policy"}
	})),
	"ApprovalService/ListPolicies": atPerm(auth.AppRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ListPoliciesRequest{AppId: f.app}
	})),
	"ApprovalService/DeletePolicy": atPerm(auth.PoliciesManage, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.DeletePolicyRequest{AppId: f.app, Key: "no-such-policy"}
	})),
	"ApprovalService/ExportPolicies": atPerm(auth.AppRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ExportPoliciesRequest{AppId: f.app}
	})),
	"ApprovalService/ListApprovals": atPerm(auth.AppRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ListApprovalsRequest{AppId: f.app}
	})),
	"ApprovalService/GetApproval": atPerm(auth.AppRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.GetApprovalRequest{AppId: f.app, Id: unknownID}
	})),
	"ApprovalService/GetApprovalReview": atPerm(auth.AppRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.GetApprovalReviewRequest{AppId: f.app, Id: unknownID}
	})),
	"ApprovalService/DecideApproval": atPerm(auth.ApprovalsDecide, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.DecideApprovalRequest{AppId: f.app, Id: unknownID, Decision: "approve"}
	})),

	// BreakGlassService (GOV-024): the list needs audit.read and the review
	// approvals.decide.
	"BreakGlassService/ListBreakGlass": atPerm(auth.AuditRead, anyone(&pluxv1.ListBreakGlassRequest{})),
	"BreakGlassService/ReviewBreakGlass": atPerm(auth.ApprovalsDecide, fixed(func(*matrixFixture) proto.Message {
		return &pluxv1.ReviewBreakGlassRequest{Id: unknownID}
	})),

	// FreezeService (GOV-025): reading windows needs organization.read and
	// changing them freezes.manage. The requests are ones the domain refuses
	// for their form after the permission is checked, so that no window is
	// written.
	"FreezeService/ListFreezes": atPerm(auth.OrganizationRead, anyone(&pluxv1.ListFreezesRequest{})),
	"FreezeService/PutFreeze":   atPerm(auth.FreezesManage, anyone(&pluxv1.PutFreezeRequest{Freeze: &pluxv1.ChangeFreeze{}})),
	"FreezeService/DeleteFreeze": atPerm(auth.FreezesManage, fixed(func(*matrixFixture) proto.Message {
		return &pluxv1.DeleteFreezeRequest{Id: unknownID}
	})),

	// ComplianceService (GOV-026): every call needs compliance.report.
	"ComplianceService/CreateComplianceReport": atPerm(auth.ComplianceReport, anyone(&pluxv1.CreateComplianceReportRequest{})),
	"ComplianceService/GetComplianceReport": atPerm(auth.ComplianceReport, fixed(func(*matrixFixture) proto.Message {
		return &pluxv1.GetComplianceReportRequest{Id: unknownID, Format: "json"}
	})),
	"ComplianceService/ListComplianceReports": atPerm(auth.ComplianceReport, anyone(&pluxv1.ListComplianceReportsRequest{})),

	// SearchService
	"SearchService/Search": atPerm(auth.SearchRead, anyone(&pluxv1.SearchRequest{Query: "demo"})),
	// StudioEventsService
	"StudioEventsService/Watch": {kind: self, call: func(t *testing.T, f *matrixFixture, c caller) error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		stream, err := f.w.studioEvents.Watch(ctx, req(c, &pluxv1.StudioEventsServiceWatchRequest{}))
		if err != nil {
			return err //nolint:wrapcheck // the code is what the test reads
		}
		defer func() { _ = stream.Close() }()
		stream.Receive()
		return stream.Err() //nolint:wrapcheck // the code is what the test reads
	}},
	// PreferencesService
	"PreferencesService/GetPreferences": {kind: self, build: anyone(&pluxv1.GetPreferencesRequest{})},
	"PreferencesService/UpdatePreferences": {kind: self, build: anyone(&pluxv1.UpdatePreferencesRequest{
		Preferences: &pluxv1.Preferences{Theme: "dark"}, UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"theme"}},
	})},
	"PreferencesService/GetOnboarding": {kind: self, build: anyone(&pluxv1.GetOnboardingRequest{})},

	// GitExportService
	"GitExportService/GetSettings": atPerm(auth.GitExport, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.GetSettingsRequest{AppId: f.app}
	})),
	"GitExportService/ConfigureExport": atPerm(auth.GitExport, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ConfigureExportRequest{Settings: &pluxv1.GitSettings{AppId: f.app, RemoteUrl: "not a url"}}
	})),
	"GitExportService/ListExports": atPerm(auth.GitExport, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ListExportsRequest{AppId: f.app}
	})),
	"GitExportService/RunExport": atPerm(auth.GitExport, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.RunExportRequest{AppId: f.app}
	})),

	// FederationService (GOV-004): the sign-in page's calls carry no
	// credential; every call that configures federation needs sso.manage.
	"FederationService/ListSignInProviders":   {kind: public, build: anyone(&pluxv1.ListSignInProvidersRequest{})},
	"FederationService/StartSamlLogin":        {kind: public, build: anyone(&pluxv1.StartSamlLoginRequest{})},
	"FederationService/CompleteSamlLogin":     {kind: public, build: anyone(&pluxv1.CompleteSamlLoginRequest{})},
	"FederationService/GetSamlMetadata":       {kind: public, build: anyone(&pluxv1.GetSamlMetadataRequest{})},
	"FederationService/ListIdentityProviders": atPerm(auth.SSOManage, anyone(&pluxv1.ListIdentityProvidersRequest{})),
	"FederationService/GetIdentityProvider": atPerm(auth.SSOManage, fixed(func(*matrixFixture) proto.Message {
		return &pluxv1.GetIdentityProviderRequest{Id: unknownID}
	})),
	"FederationService/CreateIdentityProvider": atPerm(auth.SSOManage, anyone(&pluxv1.CreateIdentityProviderRequest{})),
	"FederationService/UpdateIdentityProvider": atPerm(auth.SSOManage, fixed(func(*matrixFixture) proto.Message {
		return &pluxv1.UpdateIdentityProviderRequest{Provider: &pluxv1.IdentityProvider{Id: unknownID}}
	})),
	"FederationService/DeleteIdentityProvider": atPerm(auth.SSOManage, fixed(func(*matrixFixture) proto.Message {
		return &pluxv1.DeleteIdentityProviderRequest{Id: unknownID}
	})),
	"FederationService/GetGroupMapping": atPerm(auth.SSOManage, fixed(func(*matrixFixture) proto.Message {
		return &pluxv1.GetGroupMappingRequest{ProviderId: unknownID}
	})),
	"FederationService/SetGroupMapping": atPerm(auth.SSOManage, fixed(func(*matrixFixture) proto.Message {
		return &pluxv1.SetGroupMappingRequest{ProviderId: unknownID, Mapping: &pluxv1.GroupMapping{}}
	})),
	"FederationService/ListScimTokens": atPerm(auth.SSOManage, anyone(&pluxv1.ListScimTokensRequest{})),
	"FederationService/CreateScimToken": atPerm(auth.SSOManage, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.CreateScimTokenRequest{Name: "scim-" + f.name()}
	}),
	"FederationService/RotateScimToken": atPerm(auth.SSOManage, fixed(func(*matrixFixture) proto.Message {
		return &pluxv1.RotateScimTokenRequest{Id: unknownID}
	})),
	"FederationService/RevokeScimToken": atPerm(auth.SSOManage, fixed(func(*matrixFixture) proto.Message {
		return &pluxv1.RevokeScimTokenRequest{Id: unknownID}
	})),

	// GoldenService
	"GoldenService/GetBaselines": atPerm(auth.PluginRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.GetBaselinesRequest{AppId: f.app}
	})),
	"GoldenService/UploadRun": atPerm(auth.PluginEdit, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.UploadRunRequest{AppId: f.app}
	})),
	"GoldenService/ListDifferences": atPerm(auth.PluginRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ListDifferencesRequest{AppId: f.app, RunId: unknownID}
	})),
	"GoldenService/Approve": atPerm(auth.GoldensApprove, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ApproveRequest{AppId: f.app, RunId: unknownID}
	})),
	"GoldenService/GetRunStatus": atPerm(auth.PluginRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.GetRunStatusRequest{AppId: f.app}
	})),

	// ReleaseRequestService
	"ReleaseRequestService/CreateReleaseRequest": atPerm(auth.ReleasePublish, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.CreateReleaseRequestRequest{AppId: f.app, EnvironmentId: f.dev, Plugins: []*pluxv1.SelectedPlugin{{PluginKey: "unknown", Reason: "changed"}}}
	})),
	"ReleaseRequestService/GetReleaseRequest": atPerm(auth.PluginRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.GetReleaseRequestRequest{AppId: f.app, Id: unknownID}
	})),
	"ReleaseRequestService/ListReleaseRequests": atPerm(auth.PluginRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ListReleaseRequestsRequest{AppId: f.app}
	})),
	"ReleaseRequestService/AdvanceReleaseRequest": atPerm(auth.ReleasePublish, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.AdvanceReleaseRequestRequest{AppId: f.app, Id: unknownID}
	})),
	"ReleaseRequestService/RefreezeReleaseRequest": atPerm(auth.ReleasePublish, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.RefreezeReleaseRequestRequest{AppId: f.app, Id: unknownID}
	})),
	"ReleaseRequestService/RequestVisualDiff": atPerm(auth.ReleasePublish, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.RequestVisualDiffRequest{AppId: f.app, Id: unknownID}
	})),
	"ReleaseRequestService/RequestReleaseApproval": atPerm(auth.ReleasePublish, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.RequestReleaseApprovalRequest{AppId: f.app, Id: unknownID}
	})),
	"ReleaseRequestService/CompleteReleaseRequest": atPerm(auth.ReleasePromote, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.CompleteReleaseRequestRequest{AppId: f.app, Id: unknownID}
	})),
	"ReleaseRequestService/CancelReleaseRequest": atPerm(auth.ReleasePublish, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.CancelReleaseRequestRequest{AppId: f.app, Id: unknownID}
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
	"ControlService/GetControlDocument": atPerm(auth.AppRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.GetControlDocumentRequest{AppId: f.app, EnvironmentId: f.prod, ChannelKey: f.channel}
	})),
	"ControlService/SetKillSwitch": atPerm(auth.KillswitchManage, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.SetKillSwitchRequest{AppId: f.app, EnvironmentId: f.prod, ChannelKey: f.channel, KillSwitch: &pluxv1.KillSwitchEntry{Scope: pluxv1.KillSwitchScope_KILL_SWITCH_SCOPE_PLUGIN, Target: "loans"}}
	})),
	"ControlService/ClearKillSwitch": atPerm(auth.KillswitchManage, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ClearKillSwitchRequest{AppId: f.app, EnvironmentId: f.prod, ChannelKey: f.channel, KillSwitch: &pluxv1.KillSwitchEntry{Scope: pluxv1.KillSwitchScope_KILL_SWITCH_SCOPE_PLUGIN, Target: "loans"}}
	})),
	"ControlService/SetMandatoryUpdate": atPerm(auth.KillswitchManage, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.SetMandatoryUpdateRequest{AppId: f.app, EnvironmentId: f.prod, ChannelKey: f.channel, ReleaseSequence: 1}
	})),
	"ControlService/ClearMandatoryUpdate": atPerm(auth.KillswitchManage, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ClearMandatoryUpdateRequest{AppId: f.app, EnvironmentId: f.prod, ChannelKey: f.channel}
	})),
	"ControlService/SetForcedRollback": atPerm(auth.KillswitchManage, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.SetForcedRollbackRequest{AppId: f.app, EnvironmentId: f.prod, ChannelKey: f.channel, ReleaseSequence: 1}
	})),
	"ControlService/ClearForcedRollback": atPerm(auth.KillswitchManage, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ClearForcedRollbackRequest{AppId: f.app, EnvironmentId: f.prod, ChannelKey: f.channel}
	})),
	"ManifestService/GetSignedControl": {kind: deviceRPC, build: anyone(&pluxv1.GetSignedControlRequest{})},
	"DeviceService/RegisterPushToken":  {kind: deviceRPC, build: anyone(&pluxv1.RegisterPushTokenRequest{})},
	"DeviceService/ClearPushToken":     {kind: deviceRPC, build: anyone(&pluxv1.ClearPushTokenRequest{})},
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
	"ConfigService/GetLiveConfig": {kind: deviceRPC, build: anyone(&pluxv1.GetLiveConfigRequest{})},
	"DeviceService/EnrolScaKey":   {kind: deviceRPC, build: anyone(&pluxv1.EnrolScaKeyRequest{})},
	"AuditService/CreateExportDestination": atPerm(auth.AuditExport, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.CreateExportDestinationRequest{Name: "d" + f.name(), Kind: "syslog", Address: "siem.example.com:6514"}
	}),
	"AuditService/GetExportDestination": atPerm(auth.AuditExport, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.GetExportDestinationRequest{Id: f.sharedDestination(t)}
	}),
	"AuditService/ListExportDestinations": atPerm(auth.AuditExport, anyone(&pluxv1.ListExportDestinationsRequest{})),
	"AuditService/UpdateExportDestination": atPerm(auth.AuditExport, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.UpdateExportDestinationRequest{Id: f.sharedDestination(t), Address: "siem.example.com:6514", Enabled: true}
	}),
	"AuditService/DeleteExportDestination": {kind: needs, perm: auth.AuditExport, call: func(t *testing.T, f *matrixFixture, c caller) error {
		return f.deleteFreshDestination(t, c)
	}},
	"RolloutService/CreateRollout": atPerm(auth.RolloutManage, anyone(&pluxv1.CreateRolloutRequest{
		AppId: unknownID, EnvironmentId: unknownID, CandidateSequence: 2, Stages: []*pluxv1.Stage{{BasisPoints: 10000}},
	})),
	"RolloutService/GetRollout":    atPerm(auth.RolloutManage, anyone(&pluxv1.GetRolloutRequest{Id: unknownID})),
	"RolloutService/ListRollouts":  atPerm(auth.RolloutManage, anyone(&pluxv1.ListRolloutsRequest{AppId: unknownID})),
	"RolloutService/PauseRollout":  atPerm(auth.RolloutManage, anyone(&pluxv1.PauseRolloutRequest{Id: unknownID})),
	"RolloutService/ResumeRollout": atPerm(auth.RolloutManage, anyone(&pluxv1.ResumeRolloutRequest{Id: unknownID})),
	"RolloutService/AbortRollout":  atPerm(auth.RolloutManage, anyone(&pluxv1.AbortRolloutRequest{Id: unknownID})),

	"AuditService/Reconstruct": atPerm(auth.ReconstructionView, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ReconstructRequest{AppId: f.app, DeviceId: unknownID, At: timestamppb.Now()}
	})),
	"AuditService/GetReconstruction": atPerm(auth.ReconstructionView, anyone(&pluxv1.GetReconstructionRequest{Id: unknownID})),
	"AuditService/ListReconstructions": atPerm(auth.ReconstructionView, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ListReconstructionsRequest{AppId: f.app}
	})),
	"DataSubjectService/ExportSubject": atPerm(auth.DSRManage, anyone(&pluxv1.ExportSubjectRequest{
		Kind: pluxv1.SubjectKind_SUBJECT_KIND_DEVICE, SubjectId: unknownID,
	})),
	"DataSubjectService/EraseSubject": atPerm(auth.DSRManage, anyone(&pluxv1.EraseSubjectRequest{
		Kind: pluxv1.SubjectKind_SUBJECT_KIND_DEVICE, SubjectId: unknownID,
	})),
	"DataSubjectService/GetRequest":         atPerm(auth.DSRManage, anyone(&pluxv1.GetRequestRequest{Id: unknownID})),
	"DataSubjectService/ListRequests":       atPerm(auth.DSRManage, anyone(&pluxv1.ListRequestsRequest{})),
	"DataSubjectService/GetPersonalDataMap": atPerm(auth.DSRManage, anyone(&pluxv1.GetPersonalDataMapRequest{})),
	"FlagService/CreateFlag": atPerm(auth.FlagsManage, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.CreateFlagRequest{AppId: f.app, EnvironmentId: f.prod, Key: "matrixFlag", Spec: &pluxv1.FlagSpec{Type: pluxv1.FlagType_FLAG_TYPE_BOOL, DefaultJson: "false"}}
	})),
	"FlagService/UpdateFlag": atPerm(auth.FlagsManage, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.UpdateFlagRequest{AppId: f.app, EnvironmentId: f.prod, Key: "absentFlag", Spec: &pluxv1.FlagSpec{DefaultJson: "false"}}
	})),
	"FlagService/DeleteFlag": atPerm(auth.FlagsManage, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.DeleteFlagRequest{AppId: f.app, EnvironmentId: f.prod, Key: "absentFlag"}
	})),
	"FlagService/GetFlag": atPerm(auth.AppRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.GetFlagRequest{AppId: f.app, EnvironmentId: f.prod, Key: "absentFlag"}
	})),
	"FlagService/ListFlags": atPerm(auth.AppRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ListFlagsRequest{AppId: f.app, EnvironmentId: f.prod}
	})),
	"ExperimentService/CreateExperiment": atPerm(auth.ExperimentsManage, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.CreateExperimentRequest{AppId: f.app, EnvironmentId: f.prod, Document: []byte("{}")}
	})),
	"ExperimentService/GetExperiment":    atPerm(auth.AppRead, anyone(&pluxv1.GetExperimentRequest{Id: unknownID})),
	"ExperimentService/DeleteExperiment": atPerm(auth.ExperimentsManage, anyone(&pluxv1.DeleteExperimentRequest{Id: unknownID})),
	"ExperimentService/ListExperiments": atPerm(auth.AppRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ListExperimentsRequest{AppId: f.app}
	})),
	"ExperimentService/StartExperiment": atPerm(auth.ExperimentsManage, anyone(&pluxv1.StartExperimentRequest{Id: unknownID})),
	"ExperimentService/StopExperiment":  atPerm(auth.ExperimentsManage, anyone(&pluxv1.StopExperimentRequest{Id: unknownID})),
	"ExperimentService/SetHoldout": atPerm(auth.ExperimentsManage, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.SetHoldoutRequest{AppId: f.app, EnvironmentId: f.prod, Percent: 1}
	})),
	"ExperimentService/GetHoldout": atPerm(auth.AppRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.GetHoldoutRequest{AppId: f.app, EnvironmentId: f.prod}
	})),
	"ExperimentService/GetResults":              atPerm(auth.AnalyticsRead, anyone(&pluxv1.GetResultsRequest{Id: unknownID})),
	"ExperimentService/PromoteExperiment":       atPerm(auth.ExperimentsManage, anyone(&pluxv1.PromoteExperimentRequest{Id: unknownID, Variant: "b"})),
	"LicenseService/GetLicenseState":            atPerm(auth.OrganizationManage, anyone(&pluxv1.GetLicenseStateRequest{})),
	"SecurityAdminService/ListAuditCheckpoints": atPerm(auth.AuditRead, anyone(&pluxv1.ListAuditCheckpointsRequest{})),

	// NotificationService, WebhookService (SRV-062, SRV-063)
	"NotificationService/ListEventTypes":    {kind: self, build: anyone(&pluxv1.ListEventTypesRequest{})},
	"NotificationService/ListInbox":         {kind: self, build: anyone(&pluxv1.ListInboxRequest{})},
	"NotificationService/MarkRead":          {kind: self, build: anyone(&pluxv1.MarkReadRequest{})},
	"NotificationService/Subscribe":         {kind: self, build: anyone(&pluxv1.SubscribeRequest{Subscription: &pluxv1.NotificationSubscription{EventType: "release.published"}})},
	"NotificationService/Unsubscribe":       {kind: self, build: anyone(&pluxv1.UnsubscribeRequest{Id: unknownID})},
	"NotificationService/ListSubscriptions": {kind: self, build: anyone(&pluxv1.ListSubscriptionsRequest{})},
	"NotificationService/CreateNotificationChannel": atPerm(auth.NotificationsManage, anyone(&pluxv1.CreateNotificationChannelRequest{
		Name: "matrix", Kind: "slack", Secret: matrixWebhook,
	})),
	"NotificationService/ListNotificationChannels":  atPerm(auth.NotificationsManage, anyone(&pluxv1.ListNotificationChannelsRequest{})),
	"NotificationService/DeleteNotificationChannel": atPerm(auth.NotificationsManage, anyone(&pluxv1.DeleteNotificationChannelRequest{Id: unknownID})),
	"NotificationService/TestNotificationChannel":   atPerm(auth.NotificationsManage, anyone(&pluxv1.TestNotificationChannelRequest{Id: unknownID})),
	"NotificationService/ListDeliveries":            atPerm(auth.NotificationsManage, anyone(&pluxv1.ListDeliveriesRequest{})),
	"WebhookService/CreateWebhook":                  atPerm(auth.WebhooksManage, anyone(&pluxv1.CreateWebhookRequest{Name: "matrix", Url: "https://example.invalid/hook"})),
	"WebhookService/ListWebhooks":                   atPerm(auth.WebhooksManage, anyone(&pluxv1.ListWebhooksRequest{})),
	"WebhookService/DeleteWebhook":                  atPerm(auth.WebhooksManage, anyone(&pluxv1.DeleteWebhookRequest{Id: unknownID})),
	"WebhookService/RotateWebhookSecret":            atPerm(auth.WebhooksManage, anyone(&pluxv1.RotateWebhookSecretRequest{Id: unknownID})),
	"WebhookService/ListWebhookDeliveries":          atPerm(auth.WebhooksManage, anyone(&pluxv1.ListWebhookDeliveriesRequest{})),

	// FunctionTriggerService (FN-025)
	"FunctionTriggerService/CreateTrigger": atPerm(auth.FunctionDeploy, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.CreateTriggerRequest{AppId: f.app, EnvironmentId: f.dev, Function: "matrix.fn", Alias: "stable", Expression: "@daily"}
	})),
	"FunctionTriggerService/ListTriggers": atPerm(auth.FunctionRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ListTriggersRequest{AppId: f.app}
	})),
	"FunctionTriggerService/DeleteTrigger": atPerm(auth.FunctionDeploy, anyone(&pluxv1.DeleteTriggerRequest{Id: unknownID})),
	"FunctionTriggerService/CreateHook": atPerm(auth.FunctionDeploy, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.CreateHookRequest{AppId: f.app, EnvironmentId: f.dev, Function: "matrix.fn", Alias: "stable"}
	})),
	"FunctionTriggerService/ListHooks": atPerm(auth.FunctionRead, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.ListHooksRequest{AppId: f.app}
	})),
	"FunctionTriggerService/DeleteHook":       atPerm(auth.FunctionDeploy, anyone(&pluxv1.DeleteHookRequest{Id: unknownID})),
	"FunctionTriggerService/RotateHookSecret": atPerm(auth.FunctionDeploy, anyone(&pluxv1.RotateHookSecretRequest{Id: unknownID})),

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
	"FunctionAdminService/RunTests": {kind: needs, perm: auth.FunctionDeploy, call: func(t *testing.T, f *matrixFixture, c caller) error {
		return drain(f.w.functionAdmin.RunTests(context.Background(), req(c, &pluxv1.RunTestsRequest{SourceId: f.freshSource(t)})))
	}},
	// LanguageServerService: the sandbox runs at the organisation's cost on
	// a source that Build accepts, so both calls need what Build needs.
	"LanguageServerService/Open": {kind: needs, perm: auth.FunctionDeploy, call: func(t *testing.T, f *matrixFixture, c caller) error {
		return drain(f.w.languageServer.Open(context.Background(), req(c, &pluxv1.LanguageServerServiceOpenRequest{SourceId: f.freshSource(t)})))
	}},
	"LanguageServerService/Send": atPerm(auth.FunctionDeploy, anyone(&pluxv1.LanguageServerServiceSendRequest{SessionId: "no-such-session", Sequence: 1, Message: []byte("{}")})),
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
	"IdentityService/BeginStepUp":            {kind: self, build: anyone(&pluxv1.BeginStepUpRequest{})},
	"IdentityService/CompleteStepUp":         {kind: self, build: anyone(&pluxv1.CompleteStepUpRequest{RequestId: unknownID})},
	"IdentityService/ListSessions":           atPerm(auth.SessionsManage, anyone(&pluxv1.ListSessionsRequest{})),
	"IdentityService/RevokeSession":          atPerm(auth.SessionsManage, anyone(&pluxv1.RevokeSessionRequest{Id: unknownID})),
	"IdentityService/ListAuditEntries":       atPerm(auth.AuditRead, anyone(&pluxv1.ListAuditEntriesRequest{})),

	// DevSessionService: the device's own call is Pair; the permission is
	// checked where the person creates the session and its pairing token.
	"DevSessionService/CreateSession": atPerm(auth.DevSessionCreate, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.DevSessionServiceCreateSessionRequest{AppId: f.app, EnvironmentId: f.dev}
	})),
	"DevSessionService/GetPairing":  atPerm(auth.DevSessionCreate, anyone(&pluxv1.DevSessionServiceGetPairingRequest{SessionId: unknownID})),
	"DevSessionService/ListDevices": atPerm(auth.DevSessionObserve, anyone(&pluxv1.DevSessionServiceListDevicesRequest{SessionId: unknownID})),
	"DevSessionService/EndSession":  atPerm(auth.DevSessionCreate, anyone(&pluxv1.DevSessionServiceEndSessionRequest{SessionId: unknownID})),
	"DevSessionService/PushPatch":   atPerm(auth.DevSessionCreate, anyone(&pluxv1.DevSessionServicePushPatchRequest{SessionId: unknownID})),
	"DevSessionService/SendCommand": atPerm(auth.DevSessionCreate, anyone(&pluxv1.DevSessionServiceSendCommandRequest{
		SessionId: unknownID,
		Command:   &pluxv1.DevSessionServiceSendCommandRequest_CaptureScreenshot{CaptureScreenshot: &pluxv1.CaptureScreenshotCommand{}},
	})),
	"DevSessionService/ExportTrace": atPerm(auth.DevSessionObserve, anyone(&pluxv1.DevSessionServiceExportTraceRequest{SessionId: unknownID})),
	// WatchEvents streams; the call reads the stream as WatchPermissions
	// does.
	"DevSessionService/WatchEvents": {kind: needs, perm: auth.DevSessionObserve, call: func(t *testing.T, f *matrixFixture, c caller) error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		stream, err := f.w.devSession.WatchEvents(ctx, req(c, &pluxv1.DevSessionServiceWatchEventsRequest{SessionId: unknownID}))
		if err != nil {
			return err //nolint:wrapcheck // the code is what the test reads
		}
		defer func() { _ = stream.Close() }()
		if !stream.Receive() {
			return stream.Err() //nolint:wrapcheck // the code is what the test reads
		}
		return nil
	}},

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
	"DevSessionService/Pair":                    {kind: deviceRPC, build: anyone(&pluxv1.DevSessionServicePairRequest{})},
	"DevSessionService/RenewSocketTicket":       {kind: deviceRPC, build: anyone(&pluxv1.DevSessionServiceRenewSocketTicketRequest{})},
}

// variants are calls of an RPC that need more than the RPC's own entry
// declares.
var variants = map[string]entry{
	// Deploying to a production environment needs a fresh step-up
	// with a security key (FN-007, GOV-005).
	"FunctionAdminService/Deploy (production)": atPerm(auth.FunctionDeployProduction, func(t *testing.T, f *matrixFixture) proto.Message {
		return &pluxv1.DeployRequest{BuildId: f.freshFunctionBuild(t, f.prod)}
	}),
	"FunctionAdminService/PromoteFunction (production)": atPerm(auth.FunctionDeployProduction, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.PromoteFunctionRequest{AppId: f.app, FromEnvironmentId: f.dev, ToEnvironmentId: f.prod, Name: "matrix.fn", Version: 1}
	})),
	"FunctionAdminService/SetAlias (production)": atPerm(auth.FunctionDeployProduction, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.SetAliasRequest{AppId: f.app, EnvironmentId: f.prod, Name: "matrix.fn", Alias: &pluxv1.Alias{Name: "stable", Version: 1}}
	})),
	// An emergency action needs break-glass on top of the call's own
	// permission, with a fresh step-up (GOV-024, SEC-103).
	"ControlService/SetKillSwitch (break-glass)": atPerm(auth.BreakglassInvoke, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.SetKillSwitchRequest{
			AppId: f.app, EnvironmentId: f.prod, ChannelKey: f.channel, BreakGlass: true, Justification: "the matrix",
			KillSwitch: &pluxv1.KillSwitchEntry{Scope: pluxv1.KillSwitchScope_KILL_SWITCH_SCOPE_PLUGIN, Target: "loans"},
		}
	})),
	// Taking a lock another session holds is a separate permission.
	"PluginService/AcquireLock (force)": atPerm(auth.PluginLockOverride, fixed(func(f *matrixFixture) proto.Message {
		return &pluxv1.AcquireLockRequest{AppId: f.app, PluginId: f.plugin, Session: "tab-3", Force: true}
	})),
}

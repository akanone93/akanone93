// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	"github.com/nightCode42/plux3/backend/internal/analytics"
	"github.com/nightCode42/plux3/backend/internal/api"
	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
)

// The authorisation matrix (QA-008): every RPC of every service, called as
// each role, must be allowed exactly when the role holds the permission the
// RPC declares. The table below is that declaration; the RPC list is read
// from the service descriptors, so an RPC added without an entry fails
// TestEveryRPCHasAnAuthorisationEntry.

// kind says who may call an RPC at all.
type kind int

const (
	// needs is an RPC for a signed-in person holding a permission.
	needs kind = iota
	// self is an RPC for any signed-in person, about themselves.
	self
	// installation is an RPC for the installation administrator only.
	installation
	// public is an RPC that takes no credential: it authenticates by what
	// it carries.
	public
	// deviceRPC is an RPC for a device: a human session must not satisfy it.
	deviceRPC
)

// entry declares one RPC.
type entry struct {
	kind kind
	// perm is the permission a needs RPC requires.
	perm auth.Permission
	// build makes the request for a call; it may create the resources the
	// call consumes, so that every call finds them.
	build func(t *testing.T, f *matrixFixture) proto.Message
	// call replaces build for streaming RPCs, which the JSON POST cannot
	// drive; it returns the error the stream ended with.
	call func(t *testing.T, f *matrixFixture, c caller) error
	// ownSession makes each role call as a new person of that role, because
	// the RPC ends or changes the sessions of the person it is called by.
	ownSession bool
	// answersUnauthenticated marks an RPC that answers a request it cannot
	// accept with Unauthenticated whoever asks, as a wrong one-time code
	// does; only PermissionDenied is then a role's refusal.
	answersUnauthenticated bool
}

// roles are the callers of the matrix, in the order they are checked.
var matrixRoles = []auth.Role{
	auth.RoleViewer, auth.RoleTranslator, auth.RoleDeveloper, auth.RoleAdmin, auth.RoleOwner,
	auth.RoleReleaseManager, auth.RoleDesigner, auth.RoleReviewer, auth.RoleComplianceOfficer, auth.RoleAuditor,
}

// matrixFixture is an organisation with one member per role and the
// resources the RPCs act on.
type matrixFixture struct {
	w       *world
	callers map[auth.Role]caller
	// spares are second sessions, for the RPCs that end the one they use.
	org      string
	app      string
	prod     string
	dev      string
	channel  string
	plugin   string
	team     string
	template string
	// destination is the fixture's shared export destination (sharedDestination).
	destination string
	snaps       []string
	device      string
	job         string
	count       int
	// library holds the items the library entries act on, by kind.
	library     map[string]string
	libraryDocs map[string]string
	// rule and group are the alert rule and error group the read and update
	// entries share, made on first use to spare the rate limit.
	rule  *pluxv1.AlertRule
	group string
}

func (f *matrixFixture) owner() caller { return f.callers[auth.RoleOwner] }

// name returns a key no earlier call used.
func (f *matrixFixture) name() string {
	f.count++
	return fmt.Sprintf("k%d", f.count)
}

// withSecondFactor enrols and verifies a TOTP factor for a signed-in
// person, so that the permissions behind a second factor are not refused
// for the lack of one: a refusal is then always the role's.
func (w *world) withSecondFactor(t *testing.T, c caller) {
	t.Helper()
	ctx := context.Background()
	enrolled := must(w.identity.EnrollTotp(ctx, req(c, &pluxv1.EnrollTotpRequest{Label: "phone"})))(t)
	code, _ := auth.TOTPCode(enrolled.GetSecret(), time.Now())
	must(w.identity.ConfirmFactor(ctx, req(c, &pluxv1.ConfirmFactorRequest{FactorId: enrolled.GetFactor().GetId(), Code: code})))(t)
	next, _ := auth.TOTPCode(enrolled.GetSecret(), time.Now().Add(30*time.Second))
	must(w.identity.VerifySecondFactor(ctx, req(c, &pluxv1.VerifySecondFactorRequest{Code: next})))(t)
}

// newMember adds a person with a role and signs them in, for the RPCs
// that end or change the session they are called with.
func (f *matrixFixture) newMember(t *testing.T, role auth.Role) caller {
	t.Helper()
	ctx := context.Background()
	email := f.name() + "@example.com"
	added := must(f.w.org.AddMember(ctx, req(f.owner(), &pluxv1.AddMemberRequest{Email: email, Role: string(role)})))(t)
	must(f.w.identity.AcceptInvitation(ctx, connect.NewRequest(&pluxv1.AcceptInvitationRequest{Invitation: added.GetInvitation(), DisplayName: "temporary", Password: "correct horse battery"})))(t)
	c := f.w.signIn(t, email)
	c.org = f.org
	return c
}

// newMatrixFixture builds the organisation: an owner who is also the
// installation administrator, and a member for each other role.
func newMatrixFixture(t *testing.T) *matrixFixture {
	t.Helper()
	ctx := context.Background()
	w := newWorldWith(t, worldConfig{Checkpoints: emptyCheckpoints{}})
	f := &matrixFixture{w: w, callers: map[auth.Role]caller{}}
	_, invitation, err := w.auth.Bootstrap(ctx, "owner@example.com")
	if err != nil {
		t.Fatal(err)
	}
	const password = "correct horse battery"
	must(w.identity.AcceptInvitation(ctx, connect.NewRequest(&pluxv1.AcceptInvitationRequest{Invitation: invitation, DisplayName: "owner", Password: password})))(t)
	owner := w.signIn(t, "owner@example.com")
	f.org = must(w.org.CreateOrganization(ctx, req(owner, &pluxv1.CreateOrganizationRequest{Key: "acme", Name: "Acme"})))(t).GetOrganization().GetId()
	owner.org = f.org
	w.withSecondFactor(t, owner)
	f.callers[auth.RoleOwner] = owner
	for _, role := range matrixRoles {
		if role == auth.RoleOwner {
			continue
		}
		email := string(role) + "@example.com"
		added := must(w.org.AddMember(ctx, req(owner, &pluxv1.AddMemberRequest{Email: email, Role: string(role)})))(t)
		must(w.identity.AcceptInvitation(ctx, connect.NewRequest(&pluxv1.AcceptInvitationRequest{Invitation: added.GetInvitation(), DisplayName: string(role), Password: password})))(t)
		c := w.signIn(t, email)
		c.org = f.org
		w.withSecondFactor(t, c)
		f.callers[role] = c
	}

	f.app = must(w.app.CreateApp(ctx, req(owner, &pluxv1.CreateAppRequest{Key: "shop", Name: "Shop"})))(t).GetApp().GetId()
	for _, e := range must(w.app.ListEnvironments(ctx, req(owner, &pluxv1.ListEnvironmentsRequest{AppId: f.app})))(t).GetEnvironments() {
		switch e.GetKey() {
		case "production":
			f.prod = e.GetId()
		case "development":
			f.dev = e.GetId()
		}
	}
	if f.prod == "" || f.dev == "" {
		t.Fatal("the app has no production and development environment")
	}
	f.channel = must(w.app.ListChannels(ctx, req(owner, &pluxv1.ListChannelsRequest{EnvironmentId: f.prod})))(t).GetChannels()[0].GetKey()
	f.team = must(w.org.CreateTeam(ctx, req(owner, &pluxv1.CreateTeamRequest{Key: "web", Name: "Web"})))(t).GetTeam().GetId()
	must(w.plugin.AcquireLock(ctx, req(owner, &pluxv1.AcquireLockRequest{AppId: f.app, Session: "tab-1"})))(t)
	f.plugin = must(w.plugin.CreatePlugin(ctx, req(owner, &pluxv1.CreatePluginRequest{AppId: f.app, Key: "loans", Name: "Loans"})))(t).GetPlugin().GetId()
	must(w.plugin.AcquireLock(ctx, req(owner, &pluxv1.AcquireLockRequest{AppId: f.app, PluginId: f.plugin, Session: "tab-1"})))(t)
	for path, content := range map[string]string{
		"plugins/loans/plugin.json":          pluginDoc,
		"plugins/loans/pages/home.page.json": pageDoc,
	} {
		must(w.document.PutDocument(ctx, req(owner, &pluxv1.PutDocumentRequest{AppId: f.app, PluginId: f.plugin, Path: path, Content: []byte(content), Session: "tab-1"})))(t)
	}
	for _, s := range must(w.document.ListSnapshots(ctx, req(owner, &pluxv1.ListSnapshotsRequest{AppId: f.app, PluginId: f.plugin})))(t).GetSnapshots() {
		f.snaps = append(f.snaps, s.GetId())
	}
	if len(f.snaps) < 2 {
		t.Fatalf("snapshots = %v", f.snaps)
	}
	must(w.document.PutDocument(ctx, req(owner, &pluxv1.PutDocumentRequest{AppId: f.app, Path: "templates/greeting.template.json", Content: []byte(templateDoc), Session: "tab-1"})))(t)
	templates := must(w.template.ListTemplates(ctx, req(owner, &pluxv1.ListTemplatesRequest{AppId: f.app})))(t).GetTemplates()
	if len(templates) == 0 {
		t.Fatal("the app has no template")
	}
	f.template = templates[0].GetId()
	f.job = must(w.publish.Publish(ctx, req(owner, &pluxv1.PublishRequest{AppId: f.app, PluginId: f.plugin, EnvironmentId: f.dev})))(t).GetJob().GetId()
	f.device = w.registerDevelopment(t, f.app).id
	return f
}

// emptyCheckpoints serves ListAuditCheckpoints with no checkpoints.
type emptyCheckpoints struct{}

func (emptyCheckpoints) List(context.Context, string, int64, int32) ([]audit.Checkpoint, error) {
	return nil, nil
}

// freshApp makes an app only the call that asked for it uses.
func (f *matrixFixture) freshApp(t *testing.T) string {
	t.Helper()
	k := f.name()
	return must(f.w.app.CreateApp(context.Background(), req(f.owner(), &pluxv1.CreateAppRequest{Key: k, Name: k})))(t).GetApp().GetId()
}

func (f *matrixFixture) freshDestination(t *testing.T) string {
	t.Helper()
	return must(f.w.audit.CreateExportDestination(context.Background(), req(f.owner(), &pluxv1.CreateExportDestinationRequest{
		Name: "d" + f.name(), Kind: "syslog", Address: "siem.example.com:6514",
	})))(t).GetDestination().GetId()
}

// sharedDestination returns the fixture's destination, creating it on first
// use. Read and update calls share it so the matrix stays under the org's
// destination limit (audit.exportDestinationsPerOrg).
func (f *matrixFixture) sharedDestination(t *testing.T) string {
	t.Helper()
	if f.destination == "" {
		f.destination = f.freshDestination(t)
	}
	return f.destination
}

// deleteFreshDestination deletes a new destination as c and returns the
// outcome. A refused delete leaves its destination behind, so the owner
// removes it.
func (f *matrixFixture) deleteFreshDestination(t *testing.T, c caller) error {
	t.Helper()
	id := f.freshDestination(t)
	_, err := f.w.audit.DeleteExportDestination(context.Background(), req(c, &pluxv1.DeleteExportDestinationRequest{Id: id}))
	if err != nil {
		must(f.w.audit.DeleteExportDestination(context.Background(), req(f.owner(), &pluxv1.DeleteExportDestinationRequest{Id: id})))(t)
	}
	return err //nolint:wrapcheck // the code is what the test reads
}

func (f *matrixFixture) freshEnvironment(t *testing.T) string {
	t.Helper()
	k := f.name()
	return must(f.w.app.CreateEnvironment(context.Background(), req(f.owner(), &pluxv1.CreateEnvironmentRequest{AppId: f.app, Key: k, Name: k})))(t).GetEnvironment().GetId()
}

func (f *matrixFixture) freshChannel(t *testing.T) string {
	t.Helper()
	return must(f.w.app.CreateChannel(context.Background(), req(f.owner(), &pluxv1.CreateChannelRequest{EnvironmentId: f.prod, Key: f.name()})))(t).GetChannel().GetId()
}

// freshAlertRule creates an alert rule as the owner and returns it.
func (f *matrixFixture) freshAlertRule(t *testing.T) *pluxv1.AlertRule {
	t.Helper()
	rule := &pluxv1.AlertRule{
		AppId: f.app, Name: f.name(), Metric: "fatal_error", Aggregation: "count", WindowSeconds: 300,
		Comparison: ">", Threshold: 5, Severity: "warning",
	}
	return must(f.w.analytics.CreateAlertRule(context.Background(), req(f.owner(), &pluxv1.CreateAlertRuleRequest{Rule: rule})))(t).GetRule()
}

func (f *matrixFixture) freshFunnel(t *testing.T) *pluxv1.Funnel {
	t.Helper()
	fn := &pluxv1.Funnel{AppId: f.app, Name: f.name(), Steps: []string{"screen_view", "action_run"}}
	return must(f.w.analytics.CreateFunnel(context.Background(), req(f.owner(), &pluxv1.CreateFunnelRequest{Funnel: fn})))(t).GetFunnel()
}

// errorGroup records one error event, which is delivered once however often
// this runs, and returns the group it falls into.
func (f *matrixFixture) errorGroup(t *testing.T) string {
	t.Helper()
	if f.group != "" {
		return f.group
	}
	ctx := context.Background()
	b := analytics.Batch{OrganizationID: f.org, AppID: f.app, DeviceID: f.device, Events: []analytics.Event{{
		ID: "018f0000-0000-7000-8000-0000000000e1", Name: analytics.EventError, Time: time.Now(), ReleaseSequence: 1, PluginKey: "main",
		Fields: []byte(`{"code":"PLX-4101","node_path":"root/0"}`),
	}}}
	if err := f.w.analyticsSvc.RecordErrors(ctx, b); err != nil {
		t.Fatalf("RecordErrors: %v", err)
	}
	groups := must(f.w.analytics.ListErrorGroups(ctx, req(f.owner(), &pluxv1.ListErrorGroupsRequest{AppId: f.app})))(t).GetGroups()
	if len(groups) == 0 {
		t.Fatal("no error group was recorded")
	}
	f.group = groups[0].GetId()
	return f.group
}

// sharedAlertRule is an alert rule that entries may change but not remove.
func (f *matrixFixture) sharedAlertRule(t *testing.T) *pluxv1.AlertRule {
	t.Helper()
	if f.rule == nil {
		f.rule = f.freshAlertRule(t)
	}
	return &pluxv1.AlertRule{
		Id: f.rule.GetId(), Name: f.rule.GetName(), Metric: f.rule.GetMetric(), Aggregation: f.rule.GetAggregation(),
		WindowSeconds: f.rule.GetWindowSeconds(), Comparison: f.rule.GetComparison(), Threshold: 9, Severity: f.rule.GetSeverity(),
	}
}

func (f *matrixFixture) freshPlugin(t *testing.T) string {
	t.Helper()
	k := f.name()
	return must(f.w.plugin.CreatePlugin(context.Background(), req(f.owner(), &pluxv1.CreatePluginRequest{AppId: f.app, Key: k, Name: k})))(t).GetPlugin().GetId()
}

func (f *matrixFixture) freshTeam(t *testing.T) string {
	t.Helper()
	k := f.name()
	return must(f.w.org.CreateTeam(context.Background(), req(f.owner(), &pluxv1.CreateTeamRequest{Key: k, Name: k})))(t).GetTeam().GetId()
}

func (f *matrixFixture) freshGrant(t *testing.T) string {
	t.Helper()
	return must(f.w.app.GrantAccess(context.Background(), req(f.owner(), &pluxv1.GrantAccessRequest{AppId: f.app, TeamId: f.freshTeam(t), Role: "viewer"})))(t).GetGrant().GetId()
}

// png returns a small valid image.
func (*matrixFixture) png(t *testing.T) []byte {
	t.Helper()
	var img bytes.Buffer
	if err := png.Encode(&img, image.NewNRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	return img.Bytes()
}

// freshAsset uploads an image to the app's draft as its owner.
func (f *matrixFixture) freshAsset(t *testing.T) string {
	t.Helper()
	stream := f.w.asset.UploadAsset(context.Background())
	withHeaders(stream.RequestHeader(), f.owner())
	if err := stream.Send(&pluxv1.UploadAssetRequest{AppId: f.app, Name: "images/" + f.name() + ".png", Session: "tab-1", Chunk: f.png(t)}); err != nil {
		t.Fatal(err)
	}
	res, err := stream.CloseAndReceive()
	return must(res, err)(t).GetAsset().GetId()
}

// freshUpload opens a chunked upload of the fixture's PNG as the owner.
func (f *matrixFixture) freshUpload(t *testing.T) string {
	t.Helper()
	png := f.png(t)
	sum := sha256.Sum256(png)
	req := connect.NewRequest(&pluxv1.BeginAssetUploadRequest{AppId: f.app, Name: "images/" + f.name() + ".png", Session: "tab-1", Size: int64(len(png)), Sha256: sum[:]})
	withHeaders(req.Header(), f.owner())
	res, err := f.w.asset.BeginAssetUpload(context.Background(), req)
	return must(res, err)(t).GetUploadId()
}

// upstream returns a valid upstream of the development environment.
func (f *matrixFixture) upstream(id string) *pluxv1.Upstream {
	return &pluxv1.Upstream{
		Id: id, AppId: f.app, EnvironmentId: f.dev, Name: "bank" + strings.ReplaceAll(f.name(), "k", ""),
		BaseUrl: "https://bank.example.com/v1", AllowedMethods: []string{"GET"},
	}
}

// freshUpstream registers an upstream as the owner and returns it.
func (f *matrixFixture) freshUpstream(t *testing.T) *pluxv1.Upstream {
	t.Helper()
	return must(f.w.secAdmin.CreateUpstream(context.Background(), req(f.owner(), &pluxv1.CreateUpstreamRequest{Upstream: f.upstream("")})))(t).GetUpstream()
}

// freshTrash puts a new app in the trash and returns the item.
func (f *matrixFixture) freshTrash(t *testing.T) string {
	t.Helper()
	app := f.freshApp(t)
	return must(f.w.app.DeleteApp(context.Background(), req(f.owner(), &pluxv1.DeleteAppRequest{Id: app})))(t).GetTrash().GetId()
}

// withHeaders sets a caller's credential on a stream.
func withHeaders(h http.Header, c caller) {
	if c.cookie != "" {
		h.Set("Cookie", c.cookie)
		h.Set(api.CSRFHeader, c.csrf)
	}
	if c.bearer != "" {
		h.Set("Authorization", "Bearer "+c.bearer)
	}
	if c.org != "" {
		h.Set(api.OrganizationHeader, c.org)
	}
}

// drain reads a server stream to its end and returns how it ended.
func drain[T any](s *connect.ServerStreamForClient[T], err error) error {
	if err != nil {
		return err //nolint:wrapcheck // the code is what the test reads
	}
	for s.Receive() {
	}
	return s.Err() //nolint:wrapcheck // the code is what the test reads
}

// post calls a unary procedure with the JSON encoding of Connect and
// returns the code it failed with, or 0.
func (w *world) post(t *testing.T, c caller, procedure string, msg proto.Message) connect.Code {
	t.Helper()
	body, err := protojson.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	r, err := http.NewRequestWithContext(context.Background(), http.MethodPost, w.base+procedure, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Connect-Protocol-Version", "1")
	withHeaders(r.Header, c)
	res, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode == http.StatusOK {
		return 0
	}
	if res.StatusCode == http.StatusNotFound && !json.Valid(raw) {
		return connect.CodeUnimplemented // no handler is mounted for the service
	}
	var e struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	var code connect.Code
	if err := json.Unmarshal(raw, &e); err != nil || code.UnmarshalText([]byte(e.Code)) != nil {
		t.Fatalf("%s: status %d, body %q", procedure, res.StatusCode, raw)
	}
	if code != connect.CodePermissionDenied && code != connect.CodeUnauthenticated {
		t.Logf("%s: %v: %s", procedure, code, e.Message)
	}
	return code
}

// Verifies: SEC-102, QA-008.
// The RPC list is the service descriptors', so an RPC added to a proto
// file without a decision about who may call it fails here.
func TestEveryRPCHasAnAuthorisationEntry(t *testing.T) {
	t.Parallel()
	declared := map[string]bool{}
	for key := range matrix {
		declared[key] = true
	}
	seen := map[string]bool{}
	protoregistry.GlobalFiles.RangeFilesByPackage("plux.v1", func(fd protoreflect.FileDescriptor) bool {
		services := fd.Services()
		for i := range services.Len() {
			s := services.Get(i)
			for j := range s.Methods().Len() {
				key := string(s.Name()) + "/" + string(s.Methods().Get(j).Name())
				seen[key] = true
				if !declared[key] {
					t.Errorf("%s has no entry in the authorisation matrix: decide who may call it", key)
				}
			}
		}
		return true
	})
	for key := range declared {
		if !seen[key] {
			t.Errorf("the matrix has an entry for %s, which no service declares", key)
		}
	}
	if len(seen) < 100 {
		t.Errorf("only %d RPCs found in the descriptors; the registry is not loaded", len(seen))
	}
}

// Verifies: SEC-102, QA-008.
// Every permission in the catalogue is required by some RPC, or is named
// as one no RPC needs yet.
func TestEveryPermissionGuardsAnRPC(t *testing.T) {
	t.Parallel()
	used := map[auth.Permission]bool{}
	for _, m := range []map[string]entry{matrix, variants} {
		for _, e := range m {
			if e.kind == needs {
				used[e.perm] = true
			}
		}
	}
	for _, p := range auth.Permissions() {
		if !used[p] && !slices.Contains(permissionsWithoutRPC, p) {
			t.Errorf("no RPC in the matrix requires %s", p)
		}
	}
	for _, p := range permissionsWithoutRPC {
		if used[p] {
			t.Errorf("%s is listed as guarding no RPC, but the matrix uses it", p)
		}
	}
}

// permissionsWithoutRPC are permissions the catalogue defines that no
// ConnectRPC procedure checks: key management is a command-line and
// operator act (SEC-120). The P8 permissions are checked inside the document
// service (translations) or arrive with their RPCs (waivers).
var permissionsWithoutRPC = []auth.Permission{
	auth.A11yWaive, auth.KeysManage, auth.TranslationsApprove,
	// The P9 permissions whose RPCs a later milestone adds; each milestone
	// removes its permissions from this list when its first call checks
	// them (plan §2.4).
	auth.LicenseManage,
	auth.BackupManage,
}

// Verifies: SEC-100, SEC-102, GOV-001, QA-008.
// Each RPC is called as an anonymous caller and as each role. A role that
// holds the declared permission is not refused for its role; one that
// does not is refused with PermissionDenied and nothing else.
func TestAuthorisationMatrix_QA_008(t *testing.T) {
	f := newMatrixFixture(t)
	all := map[string]entry{}
	for _, m := range []map[string]entry{matrix, variants} {
		for key, e := range m {
			all[key] = e
		}
	}
	keys := make([]string, 0, len(all))
	for key := range all {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		e := all[key]
		procedure, _, _ := strings.Cut(key, " ")
		t.Run(key, func(t *testing.T) {
			if procedure == "DevSessionService/PushPatch" {
				t.Skip("PushPatch is declared by the contract and served by the push slice of milestone D2 (plan P10 §5.2), which removes this skip")
			}
			call := func(c caller) connect.Code {
				if e.call != nil {
					return codeOf(e.call(t, f, c))
				}
				return f.w.post(t, c, "/plux.v1."+procedure, e.build(t, f))
			}

			// Without a credential.
			anon := call(caller{})
			switch e.kind {
			case public:
				// These authenticate by what they carry; a refusal of an
				// empty request is theirs to make.
				return
			case deviceRPC:
				if anon == 0 {
					t.Error("anonymous: served a device RPC")
				}
			default:
				if anon != connect.CodeUnauthenticated {
					t.Errorf("anonymous: %v, want Unauthenticated", anon)
				}
			}

			for _, role := range matrixRoles {
				c := f.callers[role]
				if e.ownSession {
					c = f.newMember(t, role)
				}
				got := call(c)
				var allowed bool
				switch e.kind {
				case needs:
					allowed = slices.Contains(auth.RolePermissions(role), e.perm)
				case installation:
					allowed = role == auth.RoleOwner
				case self:
					allowed = true
				case deviceRPC:
					if got == 0 {
						t.Errorf("%s: a person's session was served a device RPC", role)
					}
					continue
				}
				switch {
				case allowed && (got == connect.CodePermissionDenied || (got == connect.CodeUnauthenticated && !e.answersUnauthenticated)):
					t.Errorf("%s holds %q and was refused: %v", role, e.perm, got)
				case !allowed && got != connect.CodePermissionDenied:
					t.Errorf("%s does not hold %q and got %v, want PermissionDenied", role, e.perm, got)
				}
			}
		})
	}
}

// unknownID is a well-formed identifier no row has.
const unknownID = "018f0000-0000-7000-8000-000000000000"

// Verifies: SEC-100, QA-008.
// The RPCs the matrix calls public are those the server mounts without
// authentication, no more and no fewer.
func TestPublicRPCsAreTheServersPublicRPCs(t *testing.T) {
	t.Parallel()
	got := map[string]bool{}
	for key, e := range matrix {
		if e.kind == public {
			got["/plux.v1."+key] = true
		}
	}
	for procedure := range api.IdentityPublic {
		if !got[procedure] {
			t.Errorf("%s is served without authentication but the matrix does not call it public", procedure)
		}
	}
	for procedure := range got {
		if !api.IdentityPublic[procedure] {
			t.Errorf("the matrix calls %s public, but the server authenticates it", procedure)
		}
	}
}

// TestMatrixCallsEveryRole keeps the role catalogue and the matrix's idea of it
// in step: a new role must be added to the roles the matrix calls.
func TestMatrixCallsEveryRole(t *testing.T) {
	t.Parallel()
	var catalogue []string
	for _, r := range auth.Roles() {
		catalogue = append(catalogue, string(r))
	}
	var called []string
	for _, r := range matrixRoles {
		called = append(called, string(r))
	}
	sort.Strings(called)
	if strings.Join(catalogue, ",") != strings.Join(called, ",") {
		t.Errorf("roles are %v; the matrix calls %v", catalogue, called)
	}
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
	"github.com/nightCode42/plux3/backend/internal/updatemeta"
)

// controlView is what `plux control ... --json` prints.
type controlView struct {
	Version       int64
	SignedVersion int64
	KillSwitches  []struct{ Scope, Target string }
	Mandatory     *struct{ ReleaseSequence int64 } `json:"mandatoryUpdate"`
	Rollback      *struct{ ReleaseSequence int64 } `json:"forcedRollback"`
}

// delegateControl stores a root for the app's environment that lists the
// environment's online keys, the control key among them, as an offline
// ceremony would (SEC-051, ADR-0004 Revision).
func delegateControl(t *testing.T, st *stack, environment string) {
	t.Helper()
	ctx := t.Context()
	envs, err := st.svc.Tenancy.ListEnvironments(ctx, st.owner, st.app.ID, tenancy.Page{Size: 100})
	if err != nil {
		t.Fatal(err)
	}
	var envID string
	for _, e := range envs {
		if e.Key == environment {
			envID = e.ID
		}
	}
	releases, err := BuildReleases(ctx, st.cfg, st.db)
	if err != nil {
		t.Fatal(err)
	}
	online, err := releases.OnlineKeys(ctx, st.org, envID)
	if err != nil {
		t.Fatal(err)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	spec := updatemeta.RootSpec{Version: 1, Expires: time.Now().Add(24 * time.Hour), Root: updatemeta.RoleSpec{
		Keys: []updatemeta.Key{{Alg: updatemeta.AlgEd25519, Public: base64.StdEncoding.EncodeToString(pub)}}, Threshold: 1,
	}}
	for _, k := range online {
		role := updatemeta.RoleSpec{Keys: []updatemeta.Key{{Alg: k.Algorithm, Public: base64.StdEncoding.EncodeToString(k.PublicKey)}}, Threshold: 1}
		switch k.Role {
		case updatemeta.RoleTargets:
			spec.Targets = role
		case updatemeta.RoleSnapshot:
			spec.Snapshot = role
		case updatemeta.RoleTimestamp:
			spec.Timestamp = role
		case updatemeta.RoleControl:
			spec.Control = &role
		}
	}
	root, err := updatemeta.NewRoot(spec)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := updatemeta.Canonical(root)
	if err != nil {
		t.Fatal(err)
	}
	file, err := updatemeta.Marshal(root, []updatemeta.Signature{{
		KeyID: updatemeta.KeyID(pub), Alg: updatemeta.AlgEd25519, Sig: base64.StdEncoding.EncodeToString(ed25519.Sign(priv, canonical)),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := releases.UploadRoot(ctx, st.org, envID, file); err != nil {
		t.Fatalf("UploadRoot: %v", err)
	}
}

// signedControl waits until the CLI shows the newest control document
// signed, which is when devices are served it.
func signedControl(t *testing.T, st *stack, project string) controlView {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		var v controlView
		if err := json.Unmarshal(st.run(t, 0, "control", "get", "-C", project, "--env", "staging", "--json"), &v); err != nil {
			t.Fatal(err)
		}
		if v.Version > 0 && v.SignedVersion == v.Version {
			return v
		}
		if time.Now().After(deadline) {
			t.Fatalf("the control document was not signed: %+v", v)
		}
		time.Sleep(time.Second)
	}
}

// Verifies: SYN-060, REL-060, REL-070.
// Over a running server: the CLI sets a kill switch, a mandatory update
// and a forced rollback; a device fetches the signed document with DPoP,
// a repeated request with its ETag is answered not modified, a change
// gives a newer version and a new ETag, a request without DPoP is refused,
// a forced rollback to a release that is not earlier is refused, and the
// device's push token is stored only when silent push is on.
func TestControlDocumentOverTheWire_SYN_060(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the CLI")
	}
	st := startStackWith(t, "127.0.0.1:0", stackOptions{extraYAML: "push:\n  enabled: true\n"})
	project := st.project(t, "loan-calculator")
	delegateControl(t, st, "staging")
	st.run(t, 0, "publish", "-C", project, "--promote", "staging", "--json")

	st.run(t, 0, "control", "kill", "-C", project, "--env", "staging", "--json", "plugin", "loans")
	first := signedControl(t, st, project)
	if len(first.KillSwitches) != 1 || first.KillSwitches[0].Scope != "plugin" || first.KillSwitches[0].Target != "loans" {
		t.Fatalf("document = %+v", first)
	}

	hc := &http.Client{Timeout: 30 * time.Second}
	dev := registerDevice(t, hc, st.server, st.app.ID, "staging")
	manifests := pluxv1connect.NewManifestServiceClient(hc, st.server, connect.WithInterceptors(dev))
	got, err := manifests.GetSignedControl(t.Context(), connect.NewRequest(&pluxv1.GetSignedControlRequest{}))
	if err != nil {
		t.Fatalf("GetSignedControl: %v", err)
	}
	if got.Msg.GetNotModified() || len(got.Msg.GetSigned()) == 0 || got.Msg.GetVersion() != first.SignedVersion ||
		got.Msg.GetEtag() == "" || got.Header().Get("ETag") != got.Msg.GetEtag() {
		t.Fatalf("first fetch = %+v", got.Msg)
	}
	again, err := manifests.GetSignedControl(t.Context(), connect.NewRequest(&pluxv1.GetSignedControlRequest{IfNoneMatch: got.Msg.GetEtag()}))
	if err != nil || !again.Msg.GetNotModified() || len(again.Msg.GetSigned()) != 0 {
		t.Fatalf("with its ETag: %v %+v", err, again.Msg)
	}

	// No DPoP, no document.
	anonymous := pluxv1connect.NewManifestServiceClient(hc, st.server)
	if _, err := anonymous.GetSignedControl(t.Context(), connect.NewRequest(&pluxv1.GetSignedControlRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("without DPoP: %v", err)
	}

	// A change gives a newer version and a new ETag.
	st.run(t, 0, "control", "mandatory", "-C", project, "--env", "staging", "--release", "1", "--message", "update.now", "--json")
	second := signedControl(t, st, project)
	if second.Version <= first.Version || second.Mandatory == nil || second.Mandatory.ReleaseSequence != 1 || len(second.KillSwitches) != 1 {
		t.Fatalf("after the mandatory update: %+v", second)
	}
	changed, err := manifests.GetSignedControl(t.Context(), connect.NewRequest(&pluxv1.GetSignedControlRequest{IfNoneMatch: got.Msg.GetEtag()}))
	if err != nil || changed.Msg.GetNotModified() || changed.Msg.GetEtag() == got.Msg.GetEtag() || changed.Msg.GetVersion() != second.SignedVersion {
		t.Fatalf("after a change: %v %+v", err, changed.Msg)
	}

	// The only release is the current one: it is not an earlier release.
	st.run(t, 1, "control", "rollback", "-C", project, "--env", "staging", "--release", "1")
	st.run(t, 1, "control", "rollback", "-C", project, "--env", "staging", "--release", "99")
	st.run(t, 0, "control", "unkill", "-C", project, "--env", "staging", "--json", "plugin", "loans")
	st.run(t, 0, "control", "mandatory", "-C", project, "--env", "staging", "--clear", "--json")
	cleared := signedControl(t, st, project)
	if len(cleared.KillSwitches) != 0 || cleared.Mandatory != nil || cleared.Rollback != nil || cleared.Version <= second.Version {
		t.Errorf("after withdrawing: %+v", cleared)
	}

	// The push token: stored for the device, refused when malformed.
	devices := pluxv1connect.NewDeviceServiceClient(hc, st.server, connect.WithInterceptors(dev))
	if _, err := devices.RegisterPushToken(t.Context(), connect.NewRequest(&pluxv1.RegisterPushTokenRequest{Provider: pluxv1.PushProvider_PUSH_PROVIDER_FCM, Token: "fcm:token_1-A"})); err != nil {
		t.Errorf("RegisterPushToken: %v", err)
	}
	if _, err := devices.RegisterPushToken(t.Context(), connect.NewRequest(&pluxv1.RegisterPushTokenRequest{Provider: pluxv1.PushProvider_PUSH_PROVIDER_APNS, Token: "not hex!"})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a malformed token: %v", err)
	}
	if _, err := devices.RegisterPushToken(t.Context(), connect.NewRequest(&pluxv1.RegisterPushTokenRequest{Token: "x"})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("no provider: %v", err)
	}
	if _, err := devices.ClearPushToken(t.Context(), connect.NewRequest(&pluxv1.ClearPushTokenRequest{})); err != nil {
		t.Errorf("ClearPushToken: %v", err)
	}
}

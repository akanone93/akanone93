// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/nightCode42/plux3/backend/edition/licensefile"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

// writeLicence signs a payload with a test key derived from a constant and
// writes the file; it returns the path and the public key in base64.
func writeLicence(t *testing.T, mutate func(*licensefile.Payload)) (string, string) {
	t.Helper()
	priv := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{7}, ed25519.SeedSize))
	pub, _ := priv.Public().(ed25519.PublicKey)
	grace := 30
	p := licensefile.Payload{
		LicenseID: "LIC-9", Organization: "Acme AG", Edition: licensefile.Edition,
		IssuedAt: "2026-01-01T00:00:00Z", NotBefore: "2026-01-01T00:00:00Z", ExpiresAt: "2027-01-01T00:00:00Z",
		GraceDays: &grace, Features: []string{"saml", "scim"}, Limits: map[string]int64{"page.nodes": 100},
	}
	if mutate != nil {
		mutate(&p)
	}
	canonical, err := licensefile.Canonical(p)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := licensefile.Marshal(p, []licensefile.Signature{{
		KeyID: licensefile.KeyID(pub), Alg: licensefile.AlgEd25519, Sig: base64.StdEncoding.EncodeToString(ed25519.Sign(priv, canonical)),
	}})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "license.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path, base64.StdEncoding.EncodeToString(pub)
}

func runLicense(args ...string) (int, string, string) {
	var out, errOut bytes.Buffer
	e := newEnv(&out, &errOut)
	code := e.license(args)
	return code, out.String(), errOut.String()
}

// Verifies: GOV-030.
// plux license inspect shows the payload and whether a trusted key signed
// it; a build without the production key trusts nothing unless told.
func TestLicenseInspect_GOV_030(t *testing.T) { //nolint:paralleltest // newEnv reads process-wide state
	path, pub := writeLicence(t, nil)

	code, out, stderr := runLicense("inspect", "--public-key", pub, path)
	if code != exitOK || !strings.Contains(out, "Verified: signed by a trusted key") ||
		!strings.Contains(out, "Acme AG") || !strings.Contains(out, "saml, scim") || !strings.Contains(out, "page.nodes=100") {
		t.Fatalf("inspect = %d\n%s%s", code, out, stderr)
	}

	code, out, _ = runLicense("inspect", path)
	if code != exitFailed || !strings.Contains(out, "NOT VERIFIED") {
		t.Errorf("without a trusted key: %d\n%s", code, out)
	}

	code, out, _ = runLicense("inspect", "--json", "--public-key", pub, path)
	var got licenseView
	if err := json.Unmarshal([]byte(out), &got); code != exitOK || err != nil || !got.Verified || got.Payload.Organization != "Acme AG" || len(got.SignedBy) != 1 {
		t.Errorf("--json = %d %v\n%s", code, err, out)
	}

	bound, _ := writeLicence(t, func(p *licensefile.Payload) {
		p.InstallationID = "inst-1"
		p.ExpiresAt = "2020-01-01T00:00:00Z"
		p.IssuedAt = "2019-01-01T00:00:00Z"
		p.NotBefore = "2019-01-01T00:00:00Z"
	})
	if code, out, _ := runLicense("inspect", bound); code != exitFailed || !strings.Contains(out, "Installation:") || !strings.Contains(out, "days ago") {
		t.Errorf("a bound, expired licence: %d\n%s", code, out)
	}
}

// Verifies: GOV-030.
func TestLicenseInspectRefusals_GOV_030(t *testing.T) { //nolint:paralleltest // newEnv reads process-wide state
	path, _ := writeLicence(t, nil)
	garbage := filepath.Join(t.TempDir(), "x")
	if err := os.WriteFile(garbage, []byte("nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, args := range map[string][]string{
		"no subcommand": {},
		"no file":       {"inspect"},
		"bad key":       {"inspect", "--public-key", "!!", path},
		"missing file":  {"inspect", filepath.Join(t.TempDir(), "none")},
		"garbage":       {"inspect", garbage},
	} {
		if code, _, _ := runLicense(args...); code == exitOK {
			t.Errorf("%s: exit 0", name)
		}
	}
	if code, _, _ := runLicense("inspect", "-h"); code != exitOK {
		t.Errorf("-h: %d", code)
	}
	if code, _, _ := runLicense("inspect", "--public-key", "!!", path); code != exitUsage {
		t.Errorf("a bad key is a usage error: %d", code)
	}
	if got := licenseCountdown("garbage", time.Now()); got != "" {
		t.Errorf("countdown = %q", got)
	}
}

type fakeLicense struct {
	pluxv1connect.UnimplementedLicenseServiceHandler
	state *pluxv1.GetLicenseStateResponse
}

func (f fakeLicense) GetLicenseState(context.Context, *connect.Request[pluxv1.GetLicenseStateRequest]) (*connect.Response[pluxv1.GetLicenseStateResponse], error) {
	return connect.NewResponse(f.state), nil
}

// Verifies: GOV-030.
// plux admin license prints the server's state.
func TestAdminLicense_GOV_030(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	t.Setenv(tokenEnv, "plux_pat_lic")
	t.Setenv(serverEnv, "")
	t.Setenv(orgEnv, "")
	state := &pluxv1.GetLicenseStateResponse{State: "absent", Message: "no licence"}
	mux := http.NewServeMux()
	mux.Handle(pluxv1connect.NewLicenseServiceHandler(fakeLicense{state: state}))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	run := func(args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		e := newEnv(&out, &errOut)
		e.creds = credentials{dir: t.TempDir()}
		code := e.admin(append([]string{"license", "--server", srv.URL}, args...))
		return code, out.String(), errOut.String()
	}

	if code, out, stderr := run(); code != exitOK || !strings.Contains(out, "State:    absent") || strings.Contains(out, "License:") {
		t.Fatalf("absent: %d %q %s", code, out, stderr)
	}
	exp := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	state.State, state.LicenseId, state.Organization, state.DaysRemaining = "valid", "LIC-9", "Acme AG", 74
	state.ExpiresAt, state.GraceEndsAt = timestamppb.New(exp), timestamppb.New(exp.AddDate(0, 0, 30))
	state.Features = []string{"saml"}
	code, out, _ := run()
	if code != exitOK || !strings.Contains(out, "LIC-9 (Acme AG)") || !strings.Contains(out, "2027-01-01 (74 days)") || !strings.Contains(out, "saml") {
		t.Errorf("valid: %d %q", code, out)
	}
	code, out, _ = run("--json")
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); code != exitOK || err != nil || got["state"] != "valid" || got["expiresAt"] != "2027-01-01T00:00:00Z" {
		t.Errorf("--json: %d %v %s", code, err, out)
	}

	var errOut bytes.Buffer
	e := newEnv(&bytes.Buffer{}, &errOut)
	if code := e.admin(nil); code != exitUsage {
		t.Errorf("no subcommand: %d", code)
	}
	if code := e.admin([]string{"license", "--server", "ftp://x"}); code != exitUsage {
		t.Errorf("bad server: %d", code)
	}
}

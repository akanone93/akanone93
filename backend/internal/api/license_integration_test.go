// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
)

// Verifies: GOV-030, GOV-033.
// The open-source core has no licence: an administrator reads the state
// "absent" with nothing enabled.
func TestLicenseServiceReportsAbsentWithoutEdition_GOV_030(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	admin, _, _ := w.appOnly(t)

	got := must(w.license.GetLicenseState(t.Context(), req(admin, &pluxv1.GetLicenseStateRequest{})))(t)
	if got.GetState() != "absent" || len(got.GetFeatures()) != 0 || got.GetLicenseId() != "" || got.GetExpiresAt() != nil {
		t.Errorf("GetLicenseState = %+v", got)
	}
}

// installer is an edition that takes a licence file.
type installer struct{ installed [][]byte }

func (*installer) Status() edition.Status { return edition.Status{State: edition.StateAbsent} }

func (i *installer) Install(_ context.Context, file []byte) (edition.Status, error) {
	if string(file) == "bad" {
		return edition.Status{}, errors.New("signature does not verify")
	}
	i.installed = append(i.installed, file)
	return edition.Status{State: edition.StateValid, LicenseID: "lic-1", Organization: "Acme"}, nil
}

// Verifies: GOV-030, STU-116.
// The installation administrator installs a licence through the API (P11
// §4.3 #23): the edition verifies it, the change reaches the installation's
// audit chain, and a file that fails verification is refused without being
// quoted back.
func TestInstallLicenseVerifiesAndAudits_GOV_030(t *testing.T) {
	t.Parallel()
	ed := &installer{}
	w := newWorldWith(t, worldConfig{Edition: ed})
	admin, _, _ := w.appOnly(t)

	got := must(w.license.InstallLicense(t.Context(), req(admin, &pluxv1.InstallLicenseRequest{License: []byte("good")})))(t)
	if got.GetState().GetLicenseId() != "lic-1" || got.GetState().GetState() != "valid" || len(ed.installed) != 1 {
		t.Errorf("InstallLicense = %+v, installed %d", got, len(ed.installed))
	}
	_, err := w.license.InstallLicense(t.Context(), req(admin, &pluxv1.InstallLicenseRequest{License: []byte("bad")}))
	if err == nil || strings.Contains(err.Error(), "signature") {
		t.Errorf("a file that fails verification got %v, want a refusal that does not quote the cause", err)
	}
	_, err = w.license.InstallLicense(t.Context(), req(admin, &pluxv1.InstallLicenseRequest{}))
	if err == nil {
		t.Errorf("an empty file was accepted")
	}
	installationLevel := admin
	installationLevel.org = ""
	entries := must(w.identity.ListAuditEntries(t.Context(), req(installationLevel, &pluxv1.ListAuditEntriesRequest{})))(t)
	found := false
	for _, e := range entries.GetEntries() {
		found = found || e.GetAction() == "license.state_changed"
	}
	if !found {
		t.Errorf("the installation's audit chain has no license.state_changed entry: %+v", entries.GetEntries())
	}
}

// Verifies: GOV-030.
// The open-source core cannot hold a licence: the call is refused as a failed
// precondition, never silently dropped.
func TestInstallLicenseRefusedByTheCommunityEdition_GOV_030(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	admin, _, _ := w.appOnly(t)

	_, err := w.license.InstallLicense(t.Context(), req(admin, &pluxv1.InstallLicenseRequest{License: []byte("x")}))
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("InstallLicense on the community edition = %v, want FailedPrecondition", err)
	}
}

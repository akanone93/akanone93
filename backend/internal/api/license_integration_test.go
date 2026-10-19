// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api_test

import (
	"testing"

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

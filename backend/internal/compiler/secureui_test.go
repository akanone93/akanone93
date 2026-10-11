// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"slices"
	"testing"
	"testing/fstest"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// secureUI edits the forms project to use one part of the secure UI.
var secureUI = map[string]func(t *testing.T, m fstest.MapFS){
	"secure page": func(t *testing.T, m fstest.MapFS) {
		edit(t, m, formsPage, func(doc map[string]any) {
			doc["security"] = map[string]any{"secure": true}
		})
	},
	"secure widget": func(t *testing.T, m fstest.MapFS) {
		edit(t, m, formsPage, func(doc map[string]any) {
			body := at(t, doc, "root/slots/body")
			body["children"] = append(body["children"].([]any), raw(t, `{"id": "01f0c450-6c00-7000-8000-000000009001",
				"type": "BiometricButton", "props": {"label": "Approve", "reason": "Approve the transfer"}}`))
		})
	},
	"inactivity lock": func(t *testing.T, m fstest.MapFS) {
		edit(t, m, formsApp, func(doc map[string]any) {
			doc["security"] = map[string]any{"settings": map[string]any{"inactivityLock": true}}
		})
	},
	"screenshot blocking": func(t *testing.T, m fstest.MapFS) {
		edit(t, m, formsApp, func(doc map[string]any) {
			doc["security"] = map[string]any{"settings": map[string]any{"screenshotBlockingDefault": true}}
		})
	},
}

// TestSecureUIFeatureIsRaised checks that a secure widget, a secure page
// and an app that turns on the inactivity lock or screenshot blocking need
// runtime 0.4.0: rejected under an older minRuntimeVersion, or raised as
// security.secureui.v1 (SEC-090 to SEC-094, BND-008).
// Verifies: SEC-090, SEC-091, SEC-094, BND-008.
func TestSecureUIFeatureIsRaised(t *testing.T) {
	t.Parallel()
	for name, use := range secureUI {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := project(t, formsDir)
			use(t, m)
			res := compileFS(m)
			var found bool
			for _, d := range res.Diagnostics {
				found = found || d.Code == plxerr.RuntimeTooOld
			}
			if !found {
				t.Fatalf("no %s; got:\n%s", plxerr.RuntimeTooOld, list(res.Diagnostics))
			}

			m = project(t, formsDir)
			use(t, m)
			edit(t, m, formsApp, func(doc map[string]any) { doc["requiredFeatures"] = "raise" })
			res = compileFS(m)
			if res.Diagnostics.HasErrors() {
				t.Fatalf("diagnostics:\n%s", list(res.Diagnostics))
			}
			features := slices.Concat(res.App.Features, res.Plugins[0].Features)
			if !slices.Contains(features, "security.secureui.v1") {
				t.Errorf("features %v lack security.secureui.v1", features)
			}

			m = project(t, formsDir)
			use(t, m)
			edit(t, m, formsApp, func(doc map[string]any) { doc["minRuntimeVersion"] = "0.4.0" })
			if res := compileFS(m); res.Diagnostics.HasErrors() {
				t.Fatalf("diagnostics:\n%s", list(res.Diagnostics))
			}
		})
	}
}

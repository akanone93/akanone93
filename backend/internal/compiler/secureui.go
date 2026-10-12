// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

// secureUIFeature is required by a bundle that uses the secure UI: a
// secure widget, a secure page, or an app that turns on the inactivity lock
// or screenshot blocking (SEC-090 to SEC-094, ADR-0059). A runtime without
// it would show the page unprotected, so it refuses the bundle instead.
const secureUIFeature = "security.secureui"

// secureUIRuntimes lists the runtime each revision of secureUIFeature
// first shipped in.
var secureUIRuntimes = []string{"0.4.0"}

// secureWidgets are the widgets of the secure UI (SEC-090).
var secureWidgets = map[string]bool{
	"SecureTextField":  true,
	"SecurePinPad":     true,
	"OtpInput":         true,
	"BiometricButton":  true,
	"KycCameraCapture": true,
}

// checkAppSecureUI raises the secure UI feature for an app whose security
// settings turn on the inactivity lock or screenshot blocking.
func (u *unit) checkAppSecureUI() {
	sec := u.project.App.Doc.Security
	if sec == nil || sec.Settings == nil {
		return
	}
	s := sec.Settings
	if (s.InactivityLock != nil && *s.InactivityLock) || (s.ScreenshotBlockingDefault != nil && *s.ScreenshotBlockingDefault) {
		u.useRevision(secureUIFeature, secureUIRuntimes, 1, vctx{file: "app.json", ptr: "/security/settings"})
	}
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package shellupdate detects when an app's shell settings differ from those
// its host project was generated from ("shell update required", GEN-004, F18)
// and generates the project as a queued job (GEN-001). Publishing continues
// meanwhile (GEN-005): only what needs a new store build waits.
package shellupdate

import (
	"slices"
	"strings"

	"github.com/nightCode42/plux3/backend/internal/generator"
)

// Change is one setting that differs, with why it needs a store build.
type Change struct {
	Field  string
	Old    string
	New    string
	Reason string
}

// field describes one comparable setting.
type field struct {
	name   string
	reason string
	get    func(generator.ShellSpec) string
}

func list(v []string) string { return strings.Join(v, ",") }

// fields are the settings that live in the native shell, in the order the
// changes are reported.
var fields = []field{
	{"name", "The name is the launcher label and the store listing, both compiled into the app.", func(s generator.ShellSpec) string { return s.Name }},
	{"package", "The Dart package name is the project's identity; renaming it needs a regenerated project.", func(s generator.ShellSpec) string { return s.Package }},
	{"applicationId", "The Android application ID identifies the app in Google Play and cannot change in place.", func(s generator.ShellSpec) string { return s.ApplicationID }},
	{"bundleId", "The iOS bundle ID identifies the app in the App Store and cannot change in place.", func(s generator.ShellSpec) string { return s.BundleID }},
	{"appVersion", "The version is shown by the stores and is set in pubspec.yaml.", func(s generator.ShellSpec) string { return s.AppVersion }},
	{"splashColor", "The launch screen is native and is drawn before Plux starts.", func(s generator.ShellSpec) string { return s.SplashColor }},
	{"router", "The router adapter is a compiled dependency of the shell.", func(s generator.ShellSpec) string { return s.Router }},
	{"deviceApis", "Device APIs add Android permissions and iOS usage descriptions to the native manifests.", func(s generator.ShellSpec) string { return list(s.DeviceAPIs) }},
	{"push", "Push adds the notification permission and the iOS entitlement to the native project.", func(s generator.ShellSpec) string {
		if s.Push {
			return "true"
		}
		return "false"
	}},
	{"deepLinkHosts", "Deep-link hosts are declared in the native manifests and entitlements.", func(s generator.ShellSpec) string { return list(s.DeepLinkHosts) }},
	{"deepLinkSchemes", "Deep-link schemes are declared in the native manifests.", func(s generator.ShellSpec) string { return list(s.DeepLinkSchemes) }},
	{"packages", "Optional packages are dependencies of the shell and need a build.", func(s generator.ShellSpec) string { return list(s.Packages) }},
	{"entryRoute", "The entry route is compiled into the shell's options.", func(s generator.ShellSpec) string { return s.Plux.EntryRoute }},
}

// Normalise puts the list settings in the order the generator writes them,
// so that a reordering is not a change.
func Normalise(s generator.ShellSpec) generator.ShellSpec {
	s.DeviceAPIs = sortedUnique(s.DeviceAPIs)
	s.DeepLinkHosts = sortedUnique(s.DeepLinkHosts)
	s.DeepLinkSchemes = sortedUnique(s.DeepLinkSchemes)
	s.Packages = sortedUnique(s.Packages)
	if s.Router == "none" {
		s.Router = ""
	}
	return s
}

func sortedUnique(in []string) []string {
	out := slices.Clone(in)
	slices.Sort(out)
	return slices.Compact(out)
}

// Compare returns what differs between the settings a project was generated
// from and the current settings, in a fixed order. No change means the
// project is current. The result depends only on its arguments.
func Compare(generated, current generator.ShellSpec) []Change {
	generated, current = Normalise(generated), Normalise(current)
	var out []Change
	for _, f := range fields {
		if o, n := f.get(generated), f.get(current); o != n {
			out = append(out, Change{Field: f.name, Old: o, New: n, Reason: f.reason})
		}
	}
	return out
}

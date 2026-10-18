// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package codegen

import (
	"slices"
	"strings"
)

// Package is an optional Plux package a host app adds when its Plux
// project uses what the package provides (ADR-0051, HST-032), and how the
// generated configuration registers it.
type Package struct {
	// Name is the pub package name.
	Name string
	// Import is the library the configuration imports.
	Import string
	// Constraint is the pub constraint plux init adds for the package; empty
	// for the default of the packages at their first release.
	Constraint string
	// Device registers the package in PluxConfig.devicePackages; the
	// placeholder KEY stands for the app's root navigator key.
	Device string
	// Widgets registers the package's widgets in PluxConfig.widgetPackages
	// (RT-060): a project that shows one of them needs the package
	// registered, or the node renders the fallback and reports PLX-4403.
	Widgets string
	// Slots adds the package's native slots to PluxConfig.nativeSlots.
	Slots string
	// Adapter sets PluxConfig.databaseAdapter.
	Adapter string
}

// OptionalPackages are the optional Plux packages, sorted by name.
func OptionalPackages() []Package {
	return []Package{
		{Name: "plux_charts", Import: "package:plux_charts/plux_charts.dart", Constraint: "^0.5.0", Widgets: "PluxCharts()"},
		{Name: "plux_components", Import: "package:plux_components/plux_components.dart", Widgets: "PluxComponents()"},
		{Name: "plux_db_drift", Import: "package:plux_db_drift/plux_db_drift.dart", Adapter: "PluxDriftAdapter()"},
		{Name: "plux_location", Import: "package:plux_location/plux_location.dart", Device: "PluxLocation()"},
		{Name: "plux_lottie", Import: "package:plux_lottie/plux_lottie.dart", Widgets: "PluxLottieWidgets()", Slots: "PluxLottie.slots"},
		{Name: "plux_maps", Import: "package:plux_maps/plux_maps.dart", Widgets: "PluxMaps()"},
		{Name: "plux_markdown", Import: "package:plux_markdown/plux_markdown.dart", Constraint: "^0.5.0", Widgets: "PluxMarkdown()"},
		{Name: "plux_media", Import: "package:plux_media/plux_media.dart", Device: "PluxMedia()", Widgets: "PluxMediaWidgets()"},
		{Name: "plux_player", Import: "package:plux_player/plux_player.dart", Constraint: "^0.5.0", Widgets: "PluxPlayer()"},
		{Name: "plux_rive", Import: "package:plux_rive/plux_rive.dart", Widgets: "PluxRiveWidgets()", Slots: "PluxRive.slots"},
		{Name: "plux_scanner", Import: "package:plux_scanner/plux_scanner.dart", Device: "PluxScanner(navigatorKey: KEY)", Widgets: "PluxScannerWidgets()"},
	}
}

// PackageByName returns an optional package.
func PackageByName(name string) (Package, bool) {
	all := OptionalPackages()
	i := slices.IndexFunc(all, func(p Package) bool { return p.Name == name })
	if i < 0 {
		return Package{}, false
	}
	return all[i], true
}

// packageSpecs resolves the names of an OptionsSpec to packages, sorted by
// name and without repeats; unknown names are ignored (the CLI refuses
// them before).
func packageSpecs(names []string) []Package {
	var out []Package
	for _, n := range slices.Sorted(slices.Values(names)) {
		p, ok := PackageByName(n)
		if ok && (len(out) == 0 || out[len(out)-1].Name != n) {
			out = append(out, p)
		}
	}
	return out
}

// needsNavigatorKey reports whether a package opens pages of its own on
// the app's root navigator.
func needsNavigatorKey(pkgs []Package) bool {
	return slices.ContainsFunc(pkgs, func(p Package) bool { return strings.Contains(p.Device, "KEY") })
}

// configFields are the PluxConfig arguments the packages add, one per
// line, and the navigator key expression they use.
func configFields(pkgs []Package, key string) []string {
	var devices, widgets, slots, adapters []string
	for _, p := range pkgs {
		if p.Device != "" {
			devices = append(devices, strings.ReplaceAll(p.Device, "KEY", key))
		}
		if p.Widgets != "" {
			widgets = append(widgets, p.Widgets)
		}
		if p.Slots != "" {
			slots = append(slots, "..."+p.Slots)
		}
		if p.Adapter != "" {
			adapters = append(adapters, p.Adapter)
		}
	}
	var out []string
	if len(devices) > 0 {
		out = append(out, "devicePackages: ["+strings.Join(devices, ", ")+"],")
	}
	if len(widgets) > 0 {
		out = append(out, "widgetPackages: ["+strings.Join(widgets, ", ")+"],")
	}
	if len(slots) > 0 {
		out = append(out, "nativeSlots: {"+strings.Join(slots, ", ")+"},")
	}
	if len(adapters) > 0 {
		out = append(out, "databaseAdapter: "+adapters[0]+",")
	}
	return out
}

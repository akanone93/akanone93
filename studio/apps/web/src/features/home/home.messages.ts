// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the organisation home page (STU-012). */
export function homeMessages(i18n: I18n) {
  return {
    title: (name: string) =>
      i18n._({
        id: "home.title",
        message: "Welcome to {name}",
        values: { name },
        comment: "Heading of the organisation home; name is the organisation.",
      }),
    caption: i18n._({
      id: "home.caption",
      message: "Apps, plugins and approvals appear here as they become available.",
      comment: "Caption on the organisation home.",
    }),
    appsTitle: i18n._({
      id: "home.apps.title",
      message: "Apps",
      comment: "Heading of the list of the organisation's apps.",
    }),
    appsEmpty: i18n._({
      id: "home.apps.empty",
      message: "This organisation has no apps yet.",
      comment: "Shown on the organisation home when it has no apps.",
    }),
    appsEmptyAsk: i18n._({
      id: "home.apps.empty-ask",
      message: "Ask an administrator to create one.",
      comment: "Second sentence of the empty apps state for a person who may not create apps.",
    }),
    appsLoadFailed: i18n._({
      id: "home.apps.load-failed",
      message: "The apps could not be loaded.",
      comment: "Shown when listing the organisation's apps failed.",
    }),
    openApp: (name: string) =>
      i18n._({
        id: "home.apps.open",
        message: "Open {name}",
        values: { name },
        comment: "Accessible name of an app's link in the apps list; name is the app.",
      }),
    createApp: i18n._({
      id: "home.apps.create",
      message: "Create app",
      comment: "Button, dialog title and submit label for creating an app; shown only with the app-manage permission.",
    }),
    eyebrow: i18n._({
      id: "home.eyebrow",
      message: "Organisation home",
      comment: "Small label above the organisation home's headline.",
    }),
    heroCount: (apps: number, devices: string) =>
      i18n._({
        id: "home.hero.count",
        message: "{apps, plural, one {# app} other {# apps}} on {devices} devices.",
        values: { apps, devices },
        comment: "Second line of the home headline; devices is the formatted number of active devices.",
      }),
    heroEmpty: i18n._({
      id: "home.hero.empty",
      message: "Your first app starts here.",
      comment: "Second line of the home headline while the organisation has no apps.",
    }),
    lead: i18n._({
      id: "home.lead",
      message: "Design screens, ship plugins and roll out releases to every device, signed and native.",
      comment: "Lead sentence under the home headline.",
    }),
    search: i18n._({
      id: "home.search",
      message: "Search or run a command",
      comment: "Button on the home hero that opens the command palette.",
    }),
    pulseLabel: i18n._({
      id: "home.pulse.label",
      message: "At a glance",
      comment: "Accessible name of the row of organisation figures on the home page.",
    }),
    pulseDevices: i18n._({
      id: "home.pulse.devices",
      message: "Active devices",
      comment: "Label of the organisation's total of active devices.",
    }),
    pulseDevicesNote: (apps: number) =>
      i18n._({
        id: "home.pulse.devices-note",
        message: "{apps, plural, one {across # app} other {across # apps}}",
        values: { apps },
        comment: "Note under the total of active devices.",
      }),
    pulseLive: i18n._({
      id: "home.pulse.live",
      message: "Live in production",
      comment: "Label of the number of apps with a live release in production.",
    }),
    pulseLiveNote: (apps: number) =>
      i18n._({
        id: "home.pulse.live-note",
        message: "{apps, plural, one {of # app} other {of # apps}}",
        values: { apps },
        comment: "Note under the number of apps live in production.",
      }),
    pulseCrashFree: i18n._({
      id: "home.pulse.crash-free",
      message: "Crash-free",
      comment: "Label of the crash-free rate over all active devices.",
    }),
    pulseCrashFreeNote: i18n._({
      id: "home.pulse.crash-free-note",
      message: "weighted by devices",
      comment: "Note under the crash-free rate: each app counts by its active devices.",
    }),
    pulseNoRate: i18n._({
      id: "home.pulse.no-rate",
      message: "No devices yet",
      comment: "Shown in place of the crash-free rate while no app has an active device.",
    }),
    pulsePlugins: i18n._({
      id: "home.pulse.plugins",
      message: "Plugins",
      comment: "Label of the total of plugins over all apps.",
    }),
    pulsePluginsNote: i18n._({
      id: "home.pulse.plugins-note",
      message: "in use across apps",
      comment: "Note under the total of plugins.",
    }),
    stackLabel: i18n._({
      id: "home.stack.label",
      message: "The busiest apps of the organisation, stacked as layers.",
      comment: "Accessible name of the decorative picture on the home page.",
    }),
    stackEmptyLabel: i18n._({
      id: "home.stack.empty-label",
      message: "Empty layers, waiting for the first app.",
      comment: "Accessible name of the decorative picture while the organisation has no apps.",
    }),
    stackDevices: (devices: string) =>
      i18n._({
        id: "home.stack.devices",
        message: "{devices} devices",
        values: { devices },
        comment: "Active devices on a layer of the home picture; devices is a compact number such as 184K.",
      }),
    stackLive: (release: number) =>
      i18n._({
        id: "home.stack.live",
        message: "Release {release} live",
        values: { release },
        comment: "Chip on the top layer of the home picture: the release live in production.",
      }),
    stackCrashFree: (rate: string) =>
      i18n._({
        id: "home.stack.crash-free",
        message: "{rate} crash-free",
        values: { rate },
        comment: "Chip on the top layer of the home picture; rate is a formatted percentage.",
      }),
    pulseTrend: (points: string) =>
      i18n._({
        id: "home.pulse.trend",
        message: "{points} pts",
        values: { points },
        comment:
          "Badge on the crash-free figure: its change in percentage points against the week before, e.g. +0.2 pts.",
      }),
  };
}

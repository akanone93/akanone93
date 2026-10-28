// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the apps grid and the app settings page (STU-020 to STU-025). */
export function appsMessages(i18n: I18n) {
  return {
    searchLabel: i18n._({
      id: "apps.search.label",
      message: "Search apps",
      comment: "Label of the field that filters the apps by name, key, description and tag.",
    }),
    filtersLabel: i18n._({
      id: "apps.filters.label",
      message: "Filter and sort apps",
      comment: "Accessible name of the group of search, filter and sort controls above the apps.",
    }),
    teamLabel: i18n._({ id: "apps.team.label", message: "Team", comment: "Label of the owning-team filter." }),
    allTeams: i18n._({
      id: "apps.team.all",
      message: "All teams",
      comment: "Option of the team filter that keeps every app.",
    }),
    tagLabel: i18n._({ id: "apps.tag.label", message: "Tag", comment: "Label of the tag filter." }),
    allTags: i18n._({
      id: "apps.tag.all",
      message: "All tags",
      comment: "Option of the tag filter that keeps every app.",
    }),
    statusLabel: i18n._({
      id: "apps.status.label",
      message: "Status",
      comment: "Accessible name of the group of buttons that choose active, archived or all apps.",
    }),
    statusActive: i18n._({
      id: "apps.status.active",
      message: "Active",
      comment: "Status filter button: apps that are not archived.",
    }),
    statusArchived: i18n._({
      id: "apps.status.archived",
      message: "Archived",
      comment: "Status filter button and badge: archived apps.",
    }),
    statusAll: i18n._({
      id: "apps.status.all",
      message: "All",
      comment: "Status filter button: active and archived apps.",
    }),
    mine: i18n._({
      id: "apps.mine",
      message: "My apps",
      comment: "Toggle button that keeps only the apps the person holds a grant on, directly or through a team.",
    }),
    sortLabel: i18n._({ id: "apps.sort.label", message: "Sort by", comment: "Label of the sort control of the apps." }),
    sortName: i18n._({ id: "apps.sort.name", message: "Name", comment: "Sort option: by the app's name." }),
    sortActivity: i18n._({
      id: "apps.sort.activity",
      message: "Last activity",
      comment: "Sort option: newest publish first.",
    }),
    sortHealth: i18n._({
      id: "apps.sort.health",
      message: "Health",
      comment: "Sort option: lowest crash-free rate first, so apps that need attention come first.",
    }),
    viewLabel: i18n._({
      id: "apps.view.label",
      message: "View",
      comment: "Accessible name of the group of buttons that switch between the grid and the list of apps.",
    }),
    viewGrid: i18n._({ id: "apps.view.grid", message: "Grid", comment: "Button that shows the apps as cards." }),
    viewList: i18n._({ id: "apps.view.list", message: "List", comment: "Button that shows the apps as rows." }),
    count: (n: number) =>
      i18n._({
        id: "apps.count",
        message: "{n, plural, one {# app} other {# apps}}",
        values: { n },
        comment: "Number of apps shown after the filters; announced politely when it changes.",
      }),
    noMatch: i18n._({
      id: "apps.no-match",
      message: "No app matches these filters.",
      comment: "Shown when the search and filters leave no app.",
    }),
    clearFilters: i18n._({
      id: "apps.clear-filters",
      message: "Clear filters",
      comment: "Button that resets the search and every filter of the apps.",
    }),
    archivedLoadFailed: i18n._({
      id: "apps.archived-load-failed",
      message: "The archived apps could not be loaded.",
      comment: "Shown when listing the archived apps failed.",
    }),
    cardVisual: (name: string) =>
      i18n._({
        id: "apps.card.visual",
        message: "Screenshot of {name}",
        values: { name },
        comment: "Accessible name of the picture on an app's card; name is the app.",
      }),
    cardIcon: (name: string) =>
      i18n._({
        id: "apps.card.icon",
        message: "Image of {name}",
        values: { name },
        comment: "Accessible name of the uploaded image on an app's card; name is the app.",
      }),
    noTeam: i18n._({
      id: "apps.card.no-team",
      message: "No team",
      comment: "Shown on an app's card when no team owns the app.",
    }),
    owner: (team: string) =>
      i18n._({
        id: "apps.card.owner",
        message: "Team {team}",
        values: { team },
        comment: "The team that owns an app, shown on its card; team is the team's name.",
      }),
    environments: i18n._({
      id: "apps.card.environments",
      message: "Live releases",
      comment: "Accessible name of the badges that show which release each environment serves.",
    }),
    live: (environment: string, sequence: number) =>
      i18n._({
        id: "apps.card.live",
        message: "{environment} release {sequence}",
        values: { environment, sequence },
        comment: "Badge on an app's card: the release an environment serves; environment is its key.",
      }),
    notLive: (environment: string) =>
      i18n._({
        id: "apps.card.not-live",
        message: "{environment} not live",
        values: { environment },
        comment: "Badge on an app's card: an environment that serves no release yet; environment is its key.",
      }),
    noEnvironments: i18n._({
      id: "apps.card.no-environments",
      message: "Nothing published yet",
      comment: "Shown on an app's card when no environment serves a release.",
    }),
    published: (when: string) =>
      i18n._({
        id: "apps.card.published",
        message: "Last publish {when}",
        values: { when },
        comment: "Date and time of an app's newest release, on its card; when is already formatted.",
      }),
    neverPublished: i18n._({
      id: "apps.card.never-published",
      message: "Never published",
      comment: "Shown on an app's card when it has no release.",
    }),
    devices: i18n._({
      id: "apps.card.devices",
      message: "Active devices",
      comment: "Label of the number of devices seen in the last seven days.",
    }),
    plugins: i18n._({ id: "apps.card.plugins", message: "Plugins", comment: "Label of an app's number of plugins." }),
    crashFree: (percent: string) =>
      i18n._({
        id: "apps.card.crash-free",
        message: "{percent} crash-free",
        values: { percent },
        comment: "Value of the health badge: the share of active devices without a fatal error in the last seven days.",
      }),
    healthGood: i18n._({
      id: "apps.health.good",
      message: "Healthy",
      comment: "Health badge: crash-free rate is high.",
    }),
    healthWarn: i18n._({
      id: "apps.health.warn",
      message: "Degraded",
      comment: "Health badge: crash-free rate has dropped.",
    }),
    healthBad: i18n._({
      id: "apps.health.bad",
      message: "Unhealthy",
      comment: "Health badge: crash-free rate is low.",
    }),
    healthNone: i18n._({
      id: "apps.health.none",
      message: "No devices",
      comment: "Health badge: no device was active in the last seven days, so there is no rate.",
    }),
    trendUp: (points: string) =>
      i18n._({
        id: "apps.health.trend-up",
        message: "Errors falling, {points} points",
        values: { points },
        comment: "Error trend: crash-free rate rose against the seven days before; points is a number.",
      }),
    trendDown: (points: string) =>
      i18n._({
        id: "apps.health.trend-down",
        message: "Errors rising, {points} points",
        values: { points },
        comment: "Error trend: crash-free rate fell against the seven days before; points is a number.",
      }),
    trendFlat: i18n._({
      id: "apps.health.trend-flat",
      message: "Errors steady",
      comment: "Error trend: crash-free rate did not change against the seven days before.",
    }),
    limitsUsage: i18n._({
      id: "apps.card.limits",
      message: "Limits usage",
      comment: "Accessible name of the meters that show an app's usage against its limits.",
    }),
    pin: (name: string) =>
      i18n._({
        id: "apps.card.pin",
        message: "Pin {name}",
        values: { name },
        comment: "Button on an app's card that pins the app to the top; name is the app.",
      }),
    unpin: (name: string) =>
      i18n._({
        id: "apps.card.unpin",
        message: "Unpin {name}",
        values: { name },
        comment: "Button on an app's card that removes the pin; name is the app.",
      }),
    pinShort: i18n._({
      id: "apps.card.pin-short",
      message: "Pin",
      comment: "Visible text of the button that pins an app.",
    }),
    pinnedShort: i18n._({
      id: "apps.card.pinned-short",
      message: "Pinned",
      comment: "Visible text of the button of an app that is pinned.",
    }),
    pinFailed: i18n._({
      id: "apps.pin-failed",
      message: "The pin could not be saved.",
      comment: "Shown when saving the pinned apps failed.",
    }),
    actions: (name: string) =>
      i18n._({
        id: "apps.card.actions",
        message: "Actions for {name}",
        values: { name },
        comment: "Accessible name of the button that opens an app's action menu; name is the app.",
      }),
    actionOpen: i18n._({ id: "apps.action.open", message: "Open", comment: "Menu entry that opens the app's canvas." }),
    actionSettings: i18n._({
      id: "apps.action.settings",
      message: "Settings",
      comment: "Menu entry that opens the app's settings.",
    }),
    actionDuplicate: i18n._({
      id: "apps.action.duplicate",
      message: "Duplicate",
      comment: "Menu entry that creates a copy of the app.",
    }),
    actionExport: i18n._({
      id: "apps.action.export",
      message: "Export .plux archive",
      comment: "Menu entry that downloads the app as a .plux archive.",
    }),
    actionArchive: i18n._({
      id: "apps.action.archive",
      message: "Archive",
      comment: "Menu entry that archives the app.",
    }),
    actionUnarchive: i18n._({
      id: "apps.action.unarchive",
      message: "Unarchive",
      comment: "Menu entry that makes an archived app active again.",
    }),
    duplicateTitle: (name: string) =>
      i18n._({
        id: "apps.duplicate.title",
        message: "Duplicate {name}",
        values: { name },
        comment: "Title of the dialog that copies an app; name is the app.",
      }),
    duplicateSubmit: i18n._({
      id: "apps.duplicate.submit",
      message: "Duplicate app",
      comment: "Submit button of the dialog that copies an app.",
    }),
    actionFailed: i18n._({
      id: "apps.action-failed",
      message: "The action could not be completed.",
      comment: "Shown when an app action (archive, unarchive, export) failed.",
    }),
    archivedBadge: i18n._({
      id: "apps.card.archived",
      message: "Archived",
      comment: "Badge on the card of an archived app.",
    }),
    settingsTitle: (name: string) =>
      i18n._({
        id: "apps.settings.title",
        message: "{name} settings",
        values: { name },
        comment: "Heading of the app settings page; name is the app.",
      }),
    settingsCaption: i18n._({
      id: "apps.settings.caption",
      message: "The app's name and description, its limits usage and the fonts of its host app.",
      comment: "Caption under the heading of the app settings page.",
    }),
    settingsLoadFailed: i18n._({
      id: "apps.settings.load-failed",
      message: "The app could not be loaded.",
      comment: "Shown when reading the app on its settings page failed.",
    }),
    detailsTitle: i18n._({
      id: "apps.settings.details",
      message: "Details",
      comment: "Heading of the name and description form.",
    }),
    nameLabel: i18n._({ id: "apps.settings.name", message: "Name", comment: "Label of the app name field." }),
    nameInvalid: i18n._({
      id: "apps.settings.name-invalid",
      message: "Enter a name of 1 to 200 characters.",
      comment: "Shown when the app name is empty or too long.",
    }),
    descriptionLabel: i18n._({
      id: "apps.settings.description",
      message: "Description",
      comment: "Label of the app description field.",
    }),
    save: i18n._({
      id: "apps.settings.save",
      message: "Save changes",
      comment: "Button that saves the name and description.",
    }),
    saved: i18n._({
      id: "apps.settings.saved",
      message: "Saved.",
      comment: "Shown after the name and description were saved.",
    }),
    saveFailed: i18n._({
      id: "apps.settings.save-failed",
      message: "The changes could not be saved.",
      comment: "Shown when saving the name and description failed.",
    }),
    readOnly: i18n._({
      id: "apps.settings.read-only",
      message: "You may read these settings but not change them.",
      comment: "Shown on the app settings page to a person without the permission to change the app.",
    }),
    limitsTitle: i18n._({
      id: "apps.settings.limits",
      message: "Limits usage",
      comment: "Heading of the table of the app's limits and their usage.",
    }),
    limitsCaption: i18n._({
      id: "apps.settings.limits-caption",
      message: "What the app uses of each limit in force. A limit shows a warning from 80 percent.",
      comment: "Caption of the table of the app's limits and their usage.",
    }),
    limitsLoadFailed: i18n._({
      id: "apps.settings.limits-load-failed",
      message: "The limits could not be loaded.",
      comment: "Shown when reading the app's limits failed.",
    }),
    limitsEmpty: i18n._({
      id: "apps.settings.limits-empty",
      message: "No limits to show.",
      comment: "Shown when the app has no limits to list.",
    }),
    colLimit: i18n._({ id: "apps.limits.limit", message: "Limit", comment: "Column heading: the limit's name." }),
    colUsed: i18n._({
      id: "apps.limits.used",
      message: "Used",
      comment: "Column heading: what the app uses of the limit.",
    }),
    colEffective: i18n._({
      id: "apps.limits.effective",
      message: "Limit in force",
      comment: "Column heading: the limit that applies to the app.",
    }),
    colUsage: i18n._({
      id: "apps.limits.usage",
      message: "Usage",
      comment: "Column heading: meter of used against the limit in force.",
    }),
    limitWarning: i18n._({
      id: "apps.limits.warning",
      message: "Near limit",
      comment: "Badge on a limit whose usage has reached 80 percent.",
    }),
    limitOver: i18n._({
      id: "apps.limits.over",
      message: "At limit",
      comment: "Badge on a limit whose usage has reached the limit in force.",
    }),
    limitUsageText: (used: string, limit: string) =>
      i18n._({
        id: "apps.limits.usage-text",
        message: "{used} of {limit}",
        values: { used, limit },
        comment: "Usage reading of a meter: used and the limit in force, each with its unit.",
      }),
    limitName: (key: string): string | undefined => limitNames(i18n)[key],
    unitOperations: i18n._({
      id: "apps.unit.operations",
      message: "operations",
      comment: "Unit word after a number of operations.",
    }),
    unitCodepoints: i18n._({
      id: "apps.unit.codepoints",
      message: "characters",
      comment: "Unit word after a number of characters.",
    }),
    unitMicroseconds: i18n._({ id: "apps.unit.microseconds", message: "µs", comment: "Unit symbol for microseconds." }),
    unitMilliseconds: i18n._({ id: "apps.unit.milliseconds", message: "ms", comment: "Unit symbol for milliseconds." }),
    fontsTitle: i18n._({
      id: "apps.settings.fonts",
      message: "Host fonts",
      comment: "Heading of the section that links to the host app's fonts.",
    }),
    fontsCaption: i18n._({
      id: "apps.settings.fonts-caption",
      message: "The font files the host app bundles are managed with the app's typography.",
      comment: "Caption of the section that links to the host fonts.",
    }),
    fontsLink: i18n._({
      id: "apps.settings.fonts-link",
      message: "Manage host fonts",
      comment: "Link to the typography page of the design area, where the host fonts are managed.",
    }),
    adminTitle: i18n._({
      id: "apps.settings.admin",
      message: "Organisation administration",
      comment: "Heading of the section that links to the organisation's administration.",
    }),
    adminLink: i18n._({
      id: "apps.settings.admin-link",
      message: "Open organisation administration",
      comment:
        "Link from the app settings to the organisation's administration area; shown only with the permission the area needs.",
    }),
    loading: i18n._({
      id: "apps.loading",
      message: "Loading…",
      comment: "Shown while the apps or an app's settings load.",
    }),
    keyOf: (key: string) =>
      i18n._({
        id: "apps.settings.key",
        message: "Key {key}",
        values: { key },
        comment: "The app's immutable key under the settings heading; key is the key.",
      }),
  };
}

/** The friendly names of the limits the cards and settings feature (STU-025); other limits show their registry key. */
function limitNames(i18n: I18n): Readonly<Record<string, string>> {
  return {
    "release.appSize": i18n._({
      id: "apps.limit.release-size",
      message: "Release size",
      comment: "Name of the limit on the total size of one app release.",
    }),
    "app.plugins": i18n._({
      id: "apps.limit.plugins",
      message: "Plugins",
      comment: "Name of the limit on the number of plugins in an app.",
    }),
    "plugin.pages": i18n._({
      id: "apps.limit.pages",
      message: "Pages per plugin",
      comment: "Name of the limit on the number of pages in a plugin.",
    }),
    "plugin.assetBytes": i18n._({
      id: "apps.limit.assets",
      message: "Assets per plugin",
      comment: "Name of the limit on the total size of a plugin's assets.",
    }),
    "app.functions": i18n._({
      id: "apps.limit.functions",
      message: "Functions",
      comment: "Name of the limit on the number of functions in an app.",
    }),
  };
}

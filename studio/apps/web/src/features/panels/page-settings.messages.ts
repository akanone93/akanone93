// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the inspector's Page pane (PC-14, SCH-022 to SCH-025). */
export function pageSettingsMessages(i18n: I18n) {
  return {
    tab: i18n._({
      id: "pageSettings.tab",
      message: "Page",
      comment: "Inspector tab with the settings of the selected screen's page: title, route, parameters, transition.",
    }),
    none: i18n._({
      id: "pageSettings.none",
      message: "Select a screen, or anything on it, to edit its page.",
      comment: "Shown in the Page tab when nothing on a screen is selected.",
    }),
    sectionPage: i18n._({ id: "pageSettings.sectionPage", message: "Page", comment: "Section heading." }),
    sectionParams: i18n._({
      id: "pageSettings.sectionParams",
      message: "Parameters",
      comment: "Section heading: the typed values a page is opened with.",
    }),
    sectionData: i18n._({
      id: "pageSettings.sectionData",
      message: "State and data",
      comment: "Section heading: the page's state entries and data sources.",
    }),
    sectionLifecycle: i18n._({
      id: "pageSettings.sectionLifecycle",
      message: "Lifecycle",
      comment: "Section heading: what runs when the page is created, shown, resumed, left and disposed.",
    }),
    sectionRoute: i18n._({
      id: "pageSettings.sectionRoute",
      message: "Route",
      comment: "Section heading: how the page is opened — its transition and its guards.",
    }),
    sectionSecurity: i18n._({ id: "pageSettings.sectionSecurity", message: "Security", comment: "Section heading." }),
    title: i18n._({ id: "pageSettings.title", message: "Title", comment: "Label of the page's title field." }),
    titleKey: (key: string) =>
      i18n._({
        id: "pageSettings.titleKey",
        message: "Translated: {key}",
        values: { key },
        comment: "Shown in place of the title field when the title is a translation key; edit it in Localisation.",
      }),
    description: i18n._({
      id: "pageSettings.description",
      message: "Description",
      comment: "Label of the page's description field (for developers, not shown in the app).",
    }),
    kind: i18n._({ id: "pageSettings.kind", message: "Kind", comment: "Label of the page kind select." }),
    route: i18n._({
      id: "pageSettings.route",
      message: "Route name",
      comment: "Label of the page's route name field: the app-wide name other screens and native code open it by.",
    }),
    routeForm: i18n._({
      id: "pageSettings.routeForm",
      message: "Use lower-case letters, digits and single hyphens, starting with a letter.",
      comment: "Error under the route name field when its form is wrong.",
    }),
    routeTaken: i18n._({
      id: "pageSettings.routeTaken",
      message: "Another page of the app has this route name.",
      comment: "Error under the route name field: route names are unique across the app.",
    }),
    routeExternal: (plugins: string) =>
      i18n._({
        id: "pageSettings.routeExternal",
        message: "Other plugins open this route ({plugins}); change them first.",
        values: { plugins },
        comment:
          "Error under the route name field: pages of other plugins link to the route, so it cannot change here.",
      }),
    routeFollows: (count: number) =>
      i18n._({
        id: "pageSettings.routeFollows",
        message:
          "{count, plural, one {# link in this plugin follows the new name.} other {# links in this plugin follow the new name.}}",
        values: { count },
        comment: "Note under the route name field: the links to the page are updated with it.",
      }),
    key: i18n._({
      id: "pageSettings.key",
      message: "Key",
      comment: "Label of the page's key, which names its file and cannot change here.",
    }),
    entry: i18n._({
      id: "pageSettings.entry",
      message: "Entry page",
      comment: "Switch: the page is the one the plugin opens first.",
    }),
    entryNote: i18n._({
      id: "pageSettings.entryNote",
      message: "Make another page the entry to change it.",
      comment: "Note beside the entry switch of the entry page, which cannot be switched off.",
    }),
    result: i18n._({
      id: "pageSettings.result",
      message: "Result type",
      comment: "Label of the type of value a dialog or sheet page returns when it closes.",
    }),
    paramName: i18n._({ id: "pageSettings.paramName", message: "Name", comment: "Parameter field label." }),
    paramType: i18n._({ id: "pageSettings.paramType", message: "Type", comment: "Parameter field label." }),
    paramRequired: i18n._({
      id: "pageSettings.paramRequired",
      message: "Required",
      comment: "Parameter switch label.",
    }),
    paramDefault: i18n._({ id: "pageSettings.paramDefault", message: "Default", comment: "Parameter field label." }),
    paramMock: i18n._({
      id: "pageSettings.paramMock",
      message: "Mock",
      comment: "Parameter field label: the value Studio and tests use while designing (SCH-024).",
    }),
    paramNameForm: i18n._({
      id: "pageSettings.paramNameForm",
      message: "Start with a lower-case letter; letters and digits only.",
      comment: "Error under a parameter's name.",
    }),
    paramNameTaken: i18n._({
      id: "pageSettings.paramNameTaken",
      message: "Another parameter has this name.",
      comment: "Error under a parameter's name.",
    }),
    typeForm: i18n._({
      id: "pageSettings.typeForm",
      message: "Not a type, such as string, int?, list<Item>.",
      comment: "Error under a type field.",
    }),
    addParam: i18n._({ id: "pageSettings.addParam", message: "Add parameter", comment: "Button." }),
    removeParam: (name: string) =>
      i18n._({
        id: "pageSettings.removeParam",
        message: "Remove {name}",
        values: { name },
        comment: "Accessible name of the button that removes a parameter.",
      }),
    noParams: i18n._({
      id: "pageSettings.noParams",
      message: "No parameters: the page opens without values.",
      comment: "Shown when a page has no parameters.",
    }),
    stateCount: (count: number) =>
      i18n._({
        id: "pageSettings.stateCount",
        message: "{count, plural, =0 {No state entries} one {# state entry} other {# state entries}}",
        values: { count },
        comment: "How many state entries the page declares.",
      }),
    dataCount: (count: number) =>
      i18n._({
        id: "pageSettings.dataCount",
        message: "{count, plural, =0 {No data sources} one {# data source} other {# data sources}}",
        values: { count },
        comment: "How many data sources the page declares.",
      }),
    openState: i18n._({ id: "pageSettings.openState", message: "Open state designer", comment: "Button." }),
    openData: i18n._({ id: "pageSettings.openData", message: "Open data sources", comment: "Button." }),
    lifecycleEvent: (event: string) =>
      i18n._({
        id: "pageSettings.lifecycleEvent",
        message:
          "{event, select, onInit {When created} onEnter {When shown} onResume {When resumed} onLeave {When left} other {When disposed}}",
        values: { event },
        comment:
          "A page lifecycle event: created (onInit), shown (onEnter), resumed (onResume), left (onLeave), disposed (onDispose).",
      }),
    handlerSet: i18n._({
      id: "pageSettings.handlerSet",
      message: "Runs actions",
      comment: "A lifecycle event has a handler.",
    }),
    handlerNone: i18n._({
      id: "pageSettings.handlerNone",
      message: "Nothing",
      comment: "A lifecycle event has no handler.",
    }),
    editActions: i18n._({
      id: "pageSettings.editActions",
      message: "Edit in action graph",
      comment: "Button that opens the action graph editor for the page's lifecycle handlers.",
    }),
    transition: i18n._({
      id: "pageSettings.transition",
      message: "Transition",
      comment: "Label of the page transition select.",
    }),
    transitionName: (t: string) =>
      i18n._({
        id: "pageSettings.transitionName",
        message:
          "{t, select, platform {Platform default} fade {Fade} slideLeft {Slide from the end} slideRight {Slide from the start} slideUp {Slide up} slideDown {Slide down} scale {Scale} sharedAxis {Shared axis} custom {Custom timeline} other {None}}",
        values: { t },
        comment: "A page transition (NAV-010).",
      }),
    timeline: i18n._({
      id: "pageSettings.timeline",
      message: "Timeline",
      comment: "Label of the route timeline a custom transition plays.",
    }),
    guards: i18n._({
      id: "pageSettings.guards",
      message: "Guards",
      comment: "Label of the action graphs run before the page opens; any of them can stop or redirect it.",
    }),
    noGuards: i18n._({ id: "pageSettings.noGuards", message: "No guards", comment: "The page has no guards." }),
    addGuard: i18n._({ id: "pageSettings.addGuard", message: "Add a guard", comment: "Select placeholder." }),
    removeGuard: (name: string) =>
      i18n._({
        id: "pageSettings.removeGuard",
        message: "Remove guard {name}",
        values: { name },
        comment: "Accessible name of the button that removes a guard.",
      }),
    secure: i18n._({
      id: "pageSettings.secure",
      message: "Block screenshots",
      comment: "Switch: the page blocks screenshots and screen recording (SEC-090).",
    }),
    assurance: i18n._({
      id: "pageSettings.assurance",
      message: "Required assurance",
      comment: "Label of the authentication assurance level a user needs to open the page.",
    }),
    assuranceNone: i18n._({
      id: "pageSettings.assuranceNone",
      message: "None",
      comment: "No assurance level required.",
    }),
  };
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the library browser and its detail pages (STU-080 to STU-086). */
export function libraryMessages(i18n: I18n) {
  return {
    title: i18n._({
      id: "library.title",
      message: "Library",
      comment: "Heading of the library of components and templates.",
    }),
    searchLabel: i18n._({ id: "library.search", message: "Search", comment: "Label of the library search field." }),
    kindLabel: i18n._({
      id: "library.kind",
      message: "Kind",
      comment: "Label of the filter for components or templates.",
    }),
    kindAll: i18n._({ id: "library.kind.all", message: "All", comment: "Filter option: components and templates." }),
    kindComponent: i18n._({
      id: "library.kind.component",
      message: "Components",
      comment: "Filter option and kind name.",
    }),
    kindTemplate: i18n._({
      id: "library.kind.template",
      message: "Templates",
      comment: "Filter option and kind name.",
    }),
    visibilityLabel: i18n._({
      id: "library.visibility",
      message: "Visibility",
      comment: "Label of the visibility filter and field.",
    }),
    visibilityAll: i18n._({
      id: "library.visibility.all",
      message: "Any visibility",
      comment: "Filter option: every visibility.",
    }),
    visPrivate: i18n._({
      id: "library.visibility.private",
      message: "Private",
      comment: "Visibility: only the author.",
    }),
    visOrganization: i18n._({
      id: "library.visibility.organization",
      message: "Organisation",
      comment: "Visibility: every member of the organisation.",
    }),
    visPublic: i18n._({
      id: "library.visibility.public",
      message: "Public",
      comment: "Visibility: the whole installation.",
    }),
    ownerLabel: i18n._({ id: "library.owner", message: "Author", comment: "Label of the author filter." }),
    categoryLabel: i18n._({
      id: "library.category",
      message: "Category",
      comment: "Label of the template category filter (page or flow).",
    }),
    tagLabel: i18n._({ id: "library.tag", message: "Tag", comment: "Label of the tag filter." }),
    loading: i18n._({
      id: "library.loading",
      message: "Loading the library…",
      comment: "Shown while the library loads.",
    }),
    empty: i18n._({
      id: "library.empty",
      message: "Nothing in the library matches. Save a selection as a template or extract a component to add one.",
      comment: "Shown when the library list is empty.",
    }),
    loadFailed: i18n._({
      id: "library.loadFailed",
      message: "The library could not be loaded. Try again.",
      comment: "Error when the library list fails.",
    }),
    headVersion: (n: number) =>
      i18n._({
        id: "library.headVersion",
        message: "version {n}",
        values: { n },
        comment: "Head version of a library item.",
      }),
    importComponent: i18n._({
      id: "library.importComponent",
      message: "Import component",
      comment: "Button: upload a component library file.",
    }),
    importTemplate: i18n._({
      id: "library.importTemplate",
      message: "Import template",
      comment: "Button: upload a template library file.",
    }),
    imported: (name: string) =>
      i18n._({
        id: "library.imported",
        message: "Imported {name}.",
        values: { name },
        comment: "Confirmation after importing a library file.",
      }),
    importFailed: i18n._({
      id: "library.importFailed",
      message: "The file could not be imported. Check that it is a library file of this kind.",
      comment: "Error after a failed import.",
    }),
    exportVersion: i18n._({
      id: "library.export",
      message: "Export",
      comment: "Button: download a version as a library file.",
    }),
    exportFailed: i18n._({
      id: "library.exportFailed",
      message: "The file could not be exported.",
      comment: "Error after a failed export.",
    }),
    detailLoadFailed: i18n._({
      id: "library.detailLoadFailed",
      message: "This item could not be loaded.",
      comment: "Error on a library detail page.",
    }),
    back: i18n._({
      id: "library.back",
      message: "Back to the library",
      comment: "Link from a detail page to the library.",
    }),
    versionsHeading: i18n._({ id: "library.versions", message: "Versions", comment: "Heading of the version list." }),
    revisionsHeading: i18n._({
      id: "library.revisions",
      message: "Revisions",
      comment: "Heading of the template revision list.",
    }),
    versionRow: (n: number, who: string) =>
      i18n._({
        id: "library.versionRow",
        message: "Version {n} by {who}",
        values: { n, who },
        comment: "One version in the version list.",
      }),
    usageHeading: i18n._({
      id: "library.usage",
      message: "Instances across apps",
      comment: "Heading of the list of component instances per app.",
    }),
    usageNone: i18n._({
      id: "library.usage.none",
      message: "No app uses this component yet.",
      comment: "Shown when a component has no instances.",
    }),
    usageApp: (app: string, n: number) =>
      i18n._({
        id: "library.usage.app",
        message: "{app}: {n} instances",
        values: { app, n },
        comment: "Instances of a component in one app.",
      }),
    adoptedVersion: (app: string, n: number) =>
      i18n._({
        id: "library.adopted",
        message: "{app} adopted version {n}",
        values: { app, n },
        comment: "The version an app adopted.",
      }),
    notAdopted: (app: string) =>
      i18n._({
        id: "library.notAdopted",
        message: "{app} has not adopted a version",
        values: { app },
        comment: "An app that did not adopt any version of the component.",
      }),
    parametersHeading: i18n._({
      id: "library.parameters",
      message: "Parameters",
      comment: "Heading of a template's parameters.",
    }),
    parametersNone: i18n._({
      id: "library.parameters.none",
      message: "This template asks for no parameters.",
      comment: "Shown when a template has no parameters.",
    }),
    parameterRow: (name: string, type: string, required: boolean) =>
      required
        ? i18n._({
            id: "library.parameterRequired",
            message: "{name} ({type}), required",
            values: { name, type },
            comment: "One required template parameter with its type.",
          })
        : i18n._({
            id: "library.parameterRow",
            message: "{name} ({type})",
            values: { name, type },
            comment: "One optional template parameter with its type.",
          }),
    changeVisibility: i18n._({
      id: "library.changeVisibility",
      message: "Change visibility",
      comment: "Button: open the visibility dialog.",
    }),
    visibilityTitle: i18n._({
      id: "library.visibility.title",
      message: "Change visibility",
      comment: "Title of the visibility dialog.",
    }),
    visibilityNote: i18n._({
      id: "library.visibility.note",
      message: "Public items are visible to the whole installation and are scanned for secrets first.",
      comment: "Explains the public visibility.",
    }),
    visibilityApply: i18n._({
      id: "library.visibility.apply",
      message: "Apply",
      comment: "Button: apply the chosen visibility.",
    }),
    visibilityFailed: i18n._({
      id: "library.visibility.failed",
      message: "The visibility could not be changed.",
      comment: "Error after a failed visibility change.",
    }),
    scanTitle: i18n._({
      id: "library.scan.title",
      message: "The public scan found content that cannot be public",
      comment: "Heading of the public scan result.",
    }),
    scanHelp: i18n._({
      id: "library.scan.help",
      message: "Remove or replace the content at each location, save a new version and make that version public.",
      comment: "What to do about public scan findings.",
    }),
    scanFinding: (pointer: string, category: string) =>
      i18n._({
        id: "library.scan.finding",
        message: "{pointer} ({category})",
        values: { pointer, category },
        comment: "One finding of the public scan: JSON pointer and category.",
      }),
    close: i18n._({ id: "library.close", message: "Close", comment: "Close button of the library dialogs." }),
    cancel: i18n._({ id: "library.cancel", message: "Cancel", comment: "Cancel button of the library dialogs." }),
    adoptTitle: (app: string) =>
      i18n._({
        id: "library.adopt.title",
        message: "Adopt a version in {app}",
        values: { app },
        comment: "Title of the adopt dialog; app is the app name.",
      }),
    adoptDiff: i18n._({
      id: "library.adopt.diff",
      message: "What changes",
      comment: "Heading of the props and slots diff.",
    }),
    adoptNoDiff: i18n._({
      id: "library.adopt.noDiff",
      message: "Props and slots stay the same.",
      comment: "Shown when the diff is empty.",
    }),
    adoptAddedProps: i18n._({ id: "library.adopt.addedProps", message: "New props", comment: "Diff label." }),
    adoptRemovedProps: i18n._({ id: "library.adopt.removedProps", message: "Removed props", comment: "Diff label." }),
    adoptAddedSlots: i18n._({ id: "library.adopt.addedSlots", message: "New slots", comment: "Diff label." }),
    adoptRemovedSlots: i18n._({ id: "library.adopt.removedSlots", message: "Removed slots", comment: "Diff label." }),
    adoptConfirm: i18n._({
      id: "library.adopt.confirm",
      message: "Adopt version",
      comment: "Button: adopt the chosen version.",
    }),
    adoptFailed: i18n._({
      id: "library.adopt.failed",
      message: "The version could not be adopted.",
      comment: "Error after a failed adoption.",
    }),
    adoptDone: (n: number) =>
      i18n._({
        id: "library.adopt.done",
        message: "Version {n} is adopted.",
        values: { n },
        comment: "Confirmation after adoption.",
      }),
    adoptFlagged: i18n._({
      id: "library.adopt.flagged",
      message: "Overrides of removed props or slots, to fix in the instances",
      comment: "Heading of the flagged overrides after adoption.",
    }),
    adoptVersion: i18n._({
      id: "library.adopt",
      message: "Adopt a version",
      comment: "Button: adopt a component version in an app.",
    }),
  };
}

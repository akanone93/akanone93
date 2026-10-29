// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the build evidence of the publish flow: size, accessibility, document changes and screens (STU-100). */
export function evidenceMessages(i18n: I18n) {
  return {
    loading: i18n._({
      id: "publish.evidence.loading",
      message: "Loading the build details",
      comment: "Status while the size, accessibility and document changes load.",
    }),
    failed: i18n._({
      id: "publish.evidence.failed",
      message: "The build details could not be loaded.",
      comment: "Error when the build evidence cannot be read.",
    }),
    size: i18n._({
      id: "publish.evidence.size",
      message: "Bundle size",
      comment: "Label of the bundle size of a build.",
    }),
    sizeKib: (kib: string) =>
      i18n._({
        id: "publish.evidence.sizeKib",
        message: "{kib} KiB",
        values: { kib },
        comment: "Size of a bundle in kibibytes.",
      }),
    sizeDelta: (delta: string) =>
      i18n._({
        id: "publish.evidence.sizeDelta",
        message: "{delta} KiB against the version before",
        values: { delta },
        comment: "Change of the bundle size against the previous version.",
      }),
    sizeFirst: i18n._({
      id: "publish.evidence.sizeFirst",
      message: "First version",
      comment: "Shown when a plugin has no earlier version to compare the size with.",
    }),
    sizeUnknown: i18n._({
      id: "publish.evidence.sizeUnknown",
      message: "Size not reported",
      comment: "Shown when the build reports no size.",
    }),
    a11yNone: i18n._({
      id: "publish.evidence.a11yNone",
      message: "No accessibility findings",
      comment: "Shown when a build has no accessibility diagnostics.",
    }),
    a11yFindings: (count: string) =>
      i18n._({
        id: "publish.evidence.a11yFindings",
        message: "{count} accessibility findings",
        values: { count },
        comment: "Count of accessibility diagnostics of a build.",
      }),
    jsonDiff: i18n._({
      id: "publish.evidence.jsonDiff",
      message: "Changed documents",
      comment: "Heading of the JSON diff of a plugin.",
    }),
    jsonDiffNone: i18n._({
      id: "publish.evidence.jsonDiffNone",
      message: "No document changed.",
      comment: "Shown when a plugin has no changed document.",
    }),
    jsonDiffFirst: i18n._({
      id: "publish.evidence.jsonDiffFirst",
      message: "First version: every document is new.",
      comment: "Shown when a plugin has no earlier version to diff against.",
    }),
    jsonDiffSummary: (plugin: string, count: string) =>
      i18n._({
        id: "publish.evidence.jsonDiffSummary",
        message: "{plugin}: {count} changed documents",
        values: { plugin, count },
        comment: "Summary of the JSON diff of one plugin.",
      }),
    changeAdded: i18n._({
      id: "publish.evidence.changeAdded",
      message: "Added",
      comment: "A document that is new.",
    }),
    changeRemoved: i18n._({
      id: "publish.evidence.changeRemoved",
      message: "Removed",
      comment: "A document that was removed.",
    }),
    changeChanged: i18n._({
      id: "publish.evidence.changeChanged",
      message: "Changed",
      comment: "A document that changed.",
    }),
    opAdd: i18n._({
      id: "publish.evidence.opAdd",
      message: "add",
      comment: "JSON Patch operation that adds a value.",
    }),
    opRemove: i18n._({
      id: "publish.evidence.opRemove",
      message: "remove",
      comment: "JSON Patch operation that removes a value.",
    }),
    opReplace: i18n._({
      id: "publish.evidence.opReplace",
      message: "replace",
      comment: "JSON Patch operation that replaces a value.",
    }),
    opMove: i18n._({
      id: "publish.evidence.opMove",
      message: "move",
      comment: "JSON Patch operation that moves a value.",
    }),
    opCopy: i18n._({
      id: "publish.evidence.opCopy",
      message: "copy",
      comment: "JSON Patch operation that copies a value.",
    }),
    opTest: i18n._({
      id: "publish.evidence.opTest",
      message: "test",
      comment: "JSON Patch operation that tests a value.",
    }),
    thumbBefore: i18n._({
      id: "publish.evidence.thumbBefore",
      message: "Before",
      comment: "Label of the screen as it is released now.",
    }),
    thumbAfter: i18n._({
      id: "publish.evidence.thumbAfter",
      message: "After",
      comment: "Label of the screen as it will be released.",
    }),
    thumbStored: (sha: string) =>
      i18n._({
        id: "publish.evidence.thumbStored",
        message: "Image stored ({sha})",
        values: { sha },
        comment: "Caption under a rendered screen image: its short hash, which names the stored image.",
      }),
    thumbFailed: (sha: string) =>
      i18n._({
        id: "publish.evidence.thumbFailed",
        message: "Image {sha} could not be loaded",
        values: { sha },
        comment: "Shown instead of a thumbnail when the stored image could not be fetched; the short hash names it.",
      }),
    thumbAlt: (side: string) =>
      i18n._({
        id: "publish.evidence.thumbAlt",
        message: "Rendered screen: {side}",
        values: { side },
        comment: "Alternative text of a rendered screen thumbnail; side is Before or After.",
      }),
    thumbNone: i18n._({
      id: "publish.evidence.thumbNone",
      message: "No image",
      comment: "Shown when a screen has no image on this side.",
    }),
    thumbQueued: i18n._({
      id: "publish.evidence.thumbQueued",
      message: "Thumbnails follow when the render finishes.",
      comment: "Shown while the visual diff has no images.",
    }),
  };
}

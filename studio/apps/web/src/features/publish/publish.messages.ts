// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the publish flow (STU-100, F11). */
export function publishMessages(i18n: I18n) {
  return {
    title: i18n._({ id: "publish.title", message: "Publish", comment: "Heading of the publish flow." }),
    starting: i18n._({
      id: "publish.starting",
      message: "Opening the release request",
      comment: "Status while the publish flow looks for the open release request.",
    }),
    loading: i18n._({ id: "publish.loading", message: "Loading", comment: "Status while the release request loads." }),
    loadFailed: i18n._({
      id: "publish.loadFailed",
      message: "The release request could not be loaded.",
      comment: "Error when the release request cannot be read.",
    }),
    nothing: i18n._({
      id: "publish.nothing",
      message: "No plugin has unpublished changes.",
      comment: "Shown when there is nothing to publish.",
    }),
    startFailed: i18n._({
      id: "publish.startFailed",
      message: "The release request could not be opened.",
      comment: "Error when the release request cannot be created.",
    }),
    stepsLabel: i18n._({
      id: "publish.stepsLabel",
      message: "Publish steps",
      comment: "Accessible name of the list of publish steps.",
    }),
    stepSelect: i18n._({
      id: "publish.stepSelect",
      message: "Select plugins",
      comment: "Name of the first publish step.",
    }),
    stepBuild: i18n._({
      id: "publish.stepBuild",
      message: "Freeze and build",
      comment: "Name of the build publish step.",
    }),
    stepReports: i18n._({
      id: "publish.stepReports",
      message: "Reports",
      comment: "Name of the reports publish step.",
    }),
    stepDiffs: i18n._({
      id: "publish.stepDiffs",
      message: "Visual diffs",
      comment: "Name of the visual diff publish step.",
    }),
    stepChangelog: i18n._({
      id: "publish.stepChangelog",
      message: "Changelog",
      comment: "Name of the changelog publish step.",
    }),
    stepTarget: i18n._({ id: "publish.stepTarget", message: "Target", comment: "Name of the target publish step." }),
    stepApproval: i18n._({
      id: "publish.stepApproval",
      message: "Approval",
      comment: "Name of the approval publish step.",
    }),
    stepRollout: i18n._({
      id: "publish.stepRollout",
      message: "Rollout plan",
      comment: "Name of the rollout plan publish step.",
    }),
    stepRelease: i18n._({ id: "publish.stepRelease", message: "Release", comment: "Name of the final publish step." }),
    ended_released: i18n._({
      id: "publish.ended_released",
      message: "Released",
      comment: "State of a request that was released.",
    }),
    ended_rejected: i18n._({
      id: "publish.ended_rejected",
      message: "Rejected",
      comment: "State of a request whose approval was rejected.",
    }),
    ended_expired: i18n._({
      id: "publish.ended_expired",
      message: "Expired",
      comment: "State of a request whose approval expired.",
    }),
    ended_cancelled: i18n._({
      id: "publish.ended_cancelled",
      message: "Cancelled",
      comment: "State of a request that was cancelled.",
    }),
    othersPublishing: (name: string) =>
      i18n._({
        id: "publish.othersPublishing",
        message: "{name} is publishing. You can view this request.",
        values: { name },
        comment: "Banner when someone else owns the open release request.",
      }),
    viewOnly: i18n._({
      id: "publish.viewOnly",
      message: "You need the permission release.publish to change this request.",
      comment: "Banner when the person cannot change the request.",
    }),
    holdBlocked: i18n._({
      id: "publish.holdBlocked",
      message: "Blocked",
      comment: "Hold of a request that waits because a report has errors.",
    }),
    holdFailed: i18n._({
      id: "publish.holdFailed",
      message: "Failed",
      comment: "Hold of a request whose build failed.",
    }),
    holdAwaiting: i18n._({
      id: "publish.holdAwaiting",
      message: "Awaiting approval",
      comment: "Hold of a request that waits for approvers.",
    }),
    continue: i18n._({ id: "publish.continue", message: "Continue", comment: "Button that completes a publish step." }),
    reasonChanged: i18n._({
      id: "publish.reasonChanged",
      message: "Unpublished changes",
      comment: "Why a plugin is in the release request.",
    }),
    reasonDependency: i18n._({
      id: "publish.reasonDependency",
      message: "Added because another plugin needs it",
      comment: "Why a dependency plugin is in the release request.",
    }),
    startBuild: i18n._({
      id: "publish.startBuild",
      message: "Freeze and build",
      comment: "Button that starts the publish jobs.",
    }),
    refreeze: i18n._({
      id: "publish.refreeze",
      message: "Freeze again",
      comment: "Button that discards the failed builds and freezes the draft again.",
    }),
    buildQueued: i18n._({ id: "publish.buildQueued", message: "Queued", comment: "State of a publish job." }),
    buildRunning: i18n._({ id: "publish.buildRunning", message: "Running", comment: "State of a publish job." }),
    buildSucceeded: i18n._({ id: "publish.buildSucceeded", message: "Built", comment: "State of a publish job." }),
    buildFailed: i18n._({ id: "publish.buildFailed", message: "Failed", comment: "State of a publish job." }),
    buildCancelled: i18n._({ id: "publish.buildCancelled", message: "Cancelled", comment: "State of a publish job." }),
    buildProgress: (percent: string, stage: string) =>
      i18n._({
        id: "publish.buildProgress",
        message: "{percent}% {stage}",
        values: { percent, stage },
        comment: "Progress of a publish job: percent and pipeline stage.",
      }),
    buildVersion: (version: string) =>
      i18n._({
        id: "publish.buildVersion",
        message: "Version {version}",
        values: { version },
        comment: "Version a publish job built.",
      }),
    buildsHint: i18n._({
      id: "publish.buildsHint",
      message: "Editing may continue; the builds freeze the draft as it is now.",
      comment: "Hint under the build step.",
    }),
    reportValidation: i18n._({
      id: "publish.reportValidation",
      message: "Validation",
      comment: "Name of the validation report.",
    }),
    reportSize: i18n._({ id: "publish.reportSize", message: "Size", comment: "Name of the size report." }),
    reportAccessibility: i18n._({
      id: "publish.reportAccessibility",
      message: "Accessibility",
      comment: "Name of the accessibility report.",
    }),
    reportErrors: (count: string) =>
      i18n._({
        id: "publish.reportErrors",
        message: "{count} errors",
        values: { count },
        comment: "Error count of a plugin in the reports.",
      }),
    reportWarnings: (count: string) =>
      i18n._({
        id: "publish.reportWarnings",
        message: "{count} warnings",
        values: { count },
        comment: "Warning count of a plugin in the reports.",
      }),
    reportOk: i18n._({
      id: "publish.reportOk",
      message: "No findings",
      comment: "Shown when a plugin has no errors or warnings.",
    }),
    reportBundle: (sha: string) =>
      i18n._({
        id: "publish.reportBundle",
        message: "Bundle {sha}",
        values: { sha },
        comment: "Short bundle hash of a plugin build.",
      }),
    reportsBlocked: i18n._({
      id: "publish.reportsBlocked",
      message: "A report has errors. Open Problems, fix them and freeze again.",
      comment: "Why the reports step is blocked.",
    }),
    acknowledge: i18n._({
      id: "publish.acknowledge",
      message: "Acknowledge warnings",
      comment: "Button that opens the warnings dialog.",
    }),
    acknowledged: (count: string) =>
      i18n._({
        id: "publish.acknowledged",
        message: "{count} warnings acknowledged",
        values: { count },
        comment: "Count of acknowledged warnings.",
      }),
    warningsTitle: i18n._({
      id: "publish.warningsTitle",
      message: "Acknowledge warnings",
      comment: "Title of the warnings dialog.",
    }),
    warningsBody: i18n._({
      id: "publish.warningsBody",
      message: "Every warning needs an acknowledgement before the release can continue.",
      comment: "Description of the warnings dialog.",
    }),
    warningsConfirm: i18n._({
      id: "publish.warningsConfirm",
      message: "Acknowledge",
      comment: "Confirm button of the warnings dialog.",
    }),
    warningsCancel: i18n._({
      id: "publish.warningsCancel",
      message: "Cancel",
      comment: "Cancel button of the warnings dialog.",
    }),
    warningsClose: i18n._({
      id: "publish.warningsClose",
      message: "Close",
      comment: "Accessible name of the close button of the publish dialogs.",
    }),
    requestDiff: i18n._({
      id: "publish.requestDiff",
      message: "Render visual diff",
      comment: "Button that queues the visual diff.",
    }),
    diffNone: i18n._({
      id: "publish.diffNone",
      message: "The visual diff has not been requested.",
      comment: "Shown before a visual diff was requested.",
    }),
    diffQueued: i18n._({
      id: "publish.diffQueued",
      message: "Queued. Rendering waits for earlier jobs and is rate-limited.",
      comment: "Visual diff queued or deferred.",
    }),
    diffRunning: i18n._({ id: "publish.diffRunning", message: "Rendering", comment: "Visual diff running." }),
    diffSucceeded: i18n._({ id: "publish.diffSucceeded", message: "Rendered", comment: "Visual diff finished." }),
    diffFailed: i18n._({
      id: "publish.diffFailed",
      message: "Failed. Request it again.",
      comment: "Visual diff failed.",
    }),
    diffNew: i18n._({ id: "publish.diffNew", message: "New", comment: "A screen that has no earlier version." }),
    diffChanged: i18n._({ id: "publish.diffChanged", message: "Changed", comment: "A screen that changed." }),
    diffSame: i18n._({ id: "publish.diffSame", message: "Unchanged", comment: "A screen that did not change." }),
    diffScreen: (plugin: string, screen: string, device: string) =>
      i18n._({
        id: "publish.diffScreen",
        message: "{plugin} / {screen} on {device}",
        values: { plugin, screen, device },
        comment: "Row of the visual diff: plugin, screen and device.",
      }),
    notes: i18n._({ id: "publish.notes", message: "Release notes", comment: "Label of the notes field." }),
    label: i18n._({ id: "publish.label", message: "Label (optional)", comment: "Label of the release label field." }),
    environment: i18n._({
      id: "publish.environment",
      message: "Environment",
      comment: "Label of the environment of the release.",
    }),
    channel: i18n._({ id: "publish.channel", message: "Channel", comment: "Label of the channel field." }),
    hash: (hash: string) =>
      i18n._({
        id: "publish.hash",
        message: "Content {hash}",
        values: { hash },
        comment: "Short content hash of the release.",
      }),
    requestApproval: i18n._({
      id: "publish.requestApproval",
      message: "Request approval",
      comment: "Button that asks the approvers.",
    }),
    approvalWaiting: i18n._({
      id: "publish.approvalWaiting",
      message: "Waiting for approvers. You can close Studio; a notification brings you back.",
      comment: "Approval is pending.",
    }),
    approvalDone: (hash: string) =>
      i18n._({
        id: "publish.approvalDone",
        message: "Approved for content {hash}.",
        values: { hash },
        comment: "Approval granted.",
      }),
    approvalNone: i18n._({
      id: "publish.approvalNone",
      message: "No policy requires approval.",
      comment: "Shown when no approval is needed.",
    }),
    approvalRejected: i18n._({
      id: "publish.approvalRejected",
      message: "The approval was rejected.",
      comment: "Approval refused.",
    }),
    approvalExpired: i18n._({
      id: "publish.approvalExpired",
      message: "The approval expired.",
      comment: "Approval expired.",
    }),
    rolloutPlan: i18n._({
      id: "publish.rolloutPlan",
      message: "Rollout plan (JSON)",
      comment: "Label of the rollout plan field.",
    }),
    rolloutInvalid: i18n._({
      id: "publish.rolloutInvalid",
      message: "The plan is not a JSON object.",
      comment: "Error when the rollout plan is not valid.",
    }),
    release: i18n._({ id: "publish.release", message: "Release", comment: "Button that releases after the step-up." }),
    releasedAs: (sequence: string) =>
      i18n._({
        id: "publish.releasedAs",
        message: "Released as release {sequence}.",
        values: { sequence },
        comment: "Result of the release.",
      }),
    cancelRequest: i18n._({
      id: "publish.cancelRequest",
      message: "Cancel request",
      comment: "Button that opens the cancel dialog.",
    }),
    cancelTitle: i18n._({
      id: "publish.cancelTitle",
      message: "Cancel this release request?",
      comment: "Title of the cancel dialog.",
    }),
    cancelBody: i18n._({
      id: "publish.cancelBody",
      message: "Plugin versions already built stay; they are immutable and harmless.",
      comment: "Description of the cancel dialog.",
    }),
    cancelKeep: i18n._({
      id: "publish.cancelKeep",
      message: "Keep request",
      comment: "Button that keeps the request.",
    }),
    cancelConfirm: i18n._({
      id: "publish.cancelConfirm",
      message: "Cancel request",
      comment: "Button that cancels the request.",
    }),
    err8060: i18n._({
      id: "publish.err8060",
      message: "The request was at another step. It has been reloaded.",
      comment: "Error PLX-8060.",
    }),
    err8061: i18n._({
      id: "publish.err8061",
      message: "This request has ended. Open a new one.",
      comment: "Error PLX-8061.",
    }),
    err8062: i18n._({
      id: "publish.err8062",
      message: "The visual diff has not finished for this content. Request it and wait.",
      comment: "Error PLX-8062.",
    }),
    errGeneric: i18n._({
      id: "publish.errGeneric",
      message: "The step failed. Try again.",
      comment: "Generic publish step error.",
    }),
    stepUpFailed: i18n._({
      id: "publish.stepUpFailed",
      message: "The security check could not start.",
      comment: "Error when the step-up cannot start.",
    }),
  };
}

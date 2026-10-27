// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the Releases area (STU-101, REL-050 to REL-053, REL-060). */
export function releasesMessages(i18n: I18n) {
  return {
    timelineTitle: i18n._({
      id: "releases.timelineTitle",
      message: "Releases",
      comment: "Heading of the release timeline.",
    }),
    timelineCaption: i18n._({
      id: "releases.timelineCaption",
      message: "Every release of this environment, newest first, with the rollouts that serve it.",
      comment: "Caption under the release timeline heading.",
    }),
    channelLabel: i18n._({ id: "releases.channelLabel", message: "Channel", comment: "Label of the channel filter." }),
    colRelease: i18n._({ id: "releases.colRelease", message: "Release", comment: "Column heading." }),
    colCreated: i18n._({ id: "releases.colCreated", message: "Created", comment: "Column heading." }),
    colNotes: i18n._({ id: "releases.colNotes", message: "Notes", comment: "Column heading." }),
    colRollout: i18n._({ id: "releases.colRollout", message: "Rollout", comment: "Column heading." }),
    colShare: i18n._({
      id: "releases.colShare",
      message: "Share of devices",
      comment: "Column heading: the share of devices offered the release.",
    }),
    colState: i18n._({ id: "releases.colState", message: "State", comment: "Column heading." }),
    noReleases: i18n._({
      id: "releases.noReleases",
      message: "No releases yet.",
      comment: "Shown when the environment has no release.",
    }),
    noRollout: i18n._({
      id: "releases.noRollout",
      message: "No rollout",
      comment: "Shown when a release has no rollout.",
    }),
    rollbackOf: (sequence: string) =>
      i18n._({
        id: "releases.rollbackOf",
        message: "Rollback of release {sequence}",
        comment: "Says which release a release reproduces.",
        values: { sequence },
      }),
    killSwitchLink: i18n._({
      id: "releases.killSwitchLink",
      message: "Kill switch",
      comment: "Link to the kill-switch panel.",
    }),
    never: i18n._({ id: "releases.never", message: "never", comment: "Shown for a missing date." }),
    loading: i18n._({ id: "releases.loading", message: "Loading", comment: "Shown while a page loads." }),
    loadFailed: i18n._({
      id: "releases.loadFailed",
      message: "This could not be loaded.",
      comment: "Shown when a request fails.",
    }),
    actionFailed: i18n._({
      id: "releases.actionFailed",
      message: "The action failed. Nothing was changed.",
      comment: "Shown when a release action fails.",
    }),
    envMissing: i18n._({
      id: "releases.envMissing",
      message: "This environment does not exist.",
      comment: "Shown for an unknown environment.",
    }),
    detailTitle: (sequence: string) =>
      i18n._({
        id: "releases.detailTitle",
        message: "Release {sequence}",
        comment: "Heading of the release page.",
        values: { sequence },
      }),
    content: i18n._({
      id: "releases.content",
      message: "Content",
      comment: "Heading of the plugin versions of a release.",
    }),
    changelog: i18n._({
      id: "releases.changelog",
      message: "Changelog",
      comment: "Heading of the changes of a release.",
    }),
    noChanges: i18n._({
      id: "releases.noChanges",
      message: "No changes.",
      comment: "Shown when a release changed nothing.",
    }),
    bundle: i18n._({ id: "releases.bundle", message: "Bundle SHA-256", comment: "Label of the bundle hash." }),
    adoption: i18n._({
      id: "releases.adoption",
      message: "Adoption",
      comment: "Heading of the rollout adoption section.",
    }),
    health: i18n._({
      id: "releases.health",
      message: "Health against the previous release",
      comment: "Heading of the health comparison.",
    }),
    noHealth: i18n._({
      id: "releases.noHealth",
      message: "No health check has run yet.",
      comment: "Shown while a rollout has no health figures.",
    }),
    colMetric: i18n._({ id: "releases.colMetric", message: "Metric", comment: "Column heading." }),
    colCandidate: i18n._({ id: "releases.colCandidate", message: "This release", comment: "Column heading." }),
    colPrevious: i18n._({ id: "releases.colPrevious", message: "Previous release", comment: "Column heading." }),
    sessions: i18n._({ id: "releases.sessions", message: "Sessions", comment: "Metric name." }),
    crashed: i18n._({ id: "releases.crashed", message: "Crashed sessions", comment: "Metric name." }),
    errors: i18n._({ id: "releases.errors", message: "Errors", comment: "Metric name." }),
    regressions: (metrics: string) =>
      i18n._({
        id: "releases.regressions",
        message: "Regressed beyond the gate: {metrics}",
        comment: "Names the metrics that failed the health gate.",
        values: { metrics },
      }),
    stage: (index: string, total: string) =>
      i18n._({
        id: "releases.stage",
        message: "Stage {index} of {total}",
        comment: "Position of a rollout in its stages.",
        values: { index, total },
      }),
    controls: i18n._({
      id: "releases.controls",
      message: "Rollout controls",
      comment: "Heading of the rollout controls.",
    }),
    pause: i18n._({ id: "releases.pause", message: "Pause", comment: "Button: stop a rollout from advancing." }),
    resume: i18n._({ id: "releases.resume", message: "Resume", comment: "Button: continue a paused rollout." }),
    advance: i18n._({
      id: "releases.advance",
      message: "Advance to next stage",
      comment: "Button: move a rollout to its next stage now.",
    }),
    reschedule: i18n._({
      id: "releases.reschedule",
      message: "Change end",
      comment: "Button: change when a rollout ends.",
    }),
    endLabel: i18n._({
      id: "releases.endLabel",
      message: "End of the rollout",
      comment: "Label of the end date field.",
    }),
    clearEnd: i18n._({ id: "releases.clearEnd", message: "No end", comment: "Button: remove the end of a rollout." }),
    abort: i18n._({ id: "releases.abort", message: "Abort rollout", comment: "Button: end a rollout and revert." }),
    rollback: i18n._({
      id: "releases.rollback",
      message: "Roll back to this release",
      comment: "Button: make a new release that reproduces this one.",
    }),
    rollbackHint: i18n._({
      id: "releases.rollbackHint",
      message: "Creates a new release with this content on the channel. A security check is required.",
      comment: "Explains the rollback button.",
    }),
    abortHint: i18n._({
      id: "releases.abortHint",
      message: "Ends the rollout and returns the channel to its previous release. A security check is required.",
      comment: "Explains the abort button.",
    }),
    killTitle: i18n._({
      id: "releases.killTitle",
      message: "Kill switch",
      comment: "Heading of the kill-switch panel.",
    }),
    killCaption: i18n._({
      id: "releases.killCaption",
      message: "Switch a plugin off for every device of this environment without a new release.",
      comment: "Caption of the kill-switch panel.",
    }),
    killActive: i18n._({
      id: "releases.killActive",
      message: "Switched off",
      comment: "Heading of the plugins that are switched off.",
    }),
    killNone: i18n._({
      id: "releases.killNone",
      message: "No plugin is switched off.",
      comment: "Shown when no kill switch is set.",
    }),
    pluginLabel: i18n._({
      id: "releases.pluginLabel",
      message: "Plugin key",
      comment: "Label of the plugin key field.",
    }),
    killSet: i18n._({ id: "releases.killSet", message: "Switch off", comment: "Button: set a kill switch." }),
    killClear: i18n._({ id: "releases.killClear", message: "Switch on", comment: "Button: clear a kill switch." }),
    killAppWide: i18n._({
      id: "releases.killAppWide",
      message: "The whole app is switched off.",
      comment: "Shown when the app kill switch is set.",
    }),
    stateScheduled: i18n._({ id: "releases.stateScheduled", message: "Scheduled", comment: "State of a rollout." }),
    stateActive: i18n._({ id: "releases.stateActive", message: "Active", comment: "State of a rollout." }),
    statePaused: i18n._({ id: "releases.statePaused", message: "Paused", comment: "State of a rollout." }),
    stateClosing: i18n._({ id: "releases.stateClosing", message: "Closing", comment: "State of a rollout." }),
    stateCompleted: i18n._({ id: "releases.stateCompleted", message: "Completed", comment: "State of a rollout." }),
    stateEnded: i18n._({ id: "releases.stateEnded", message: "Ended", comment: "State of a rollout." }),
    stateAborted: i18n._({ id: "releases.stateAborted", message: "Aborted", comment: "State of a rollout." }),
    stateRolledBack: i18n._({ id: "releases.stateRolledBack", message: "Rolled back", comment: "State of a rollout." }),
  };
}

/** The name of a rollout state, indexed by the wire enum. */
export function stateNames(m: ReturnType<typeof releasesMessages>): readonly string[] {
  return [
    "",
    m.stateScheduled,
    m.stateActive,
    m.statePaused,
    m.stateClosing,
    m.stateCompleted,
    m.stateEnded,
    m.stateAborted,
    m.stateRolledBack,
  ];
}

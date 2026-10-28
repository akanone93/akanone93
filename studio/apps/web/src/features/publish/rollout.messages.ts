// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the rollout plan form (REL-050 to REL-053). */
export function rolloutMessages(i18n: I18n) {
  return {
    stagesLegend: i18n._({
      id: "publish.rollout.stagesLegend",
      message: "Stages",
      comment: "Legend of the rollout stages.",
    }),
    stage: (n: string) =>
      i18n._({
        id: "publish.rollout.stage",
        message: "Stage {n}",
        values: { n },
        comment: "Heading of one rollout stage.",
      }),
    stagePercent: i18n._({
      id: "publish.rollout.stagePercent",
      message: "Share of devices (%)",
      comment: "Label of the share of devices a rollout stage reaches.",
    }),
    stageMinutes: i18n._({
      id: "publish.rollout.stageMinutes",
      message: "Duration (minutes)",
      comment: "Label of how long a rollout stage lasts.",
    }),
    addStage: i18n._({
      id: "publish.rollout.addStage",
      message: "Add stage",
      comment: "Button that adds a rollout stage.",
    }),
    removeStage: (n: string) =>
      i18n._({
        id: "publish.rollout.removeStage",
        message: "Remove stage {n}",
        values: { n },
        comment: "Button that removes one rollout stage.",
      }),
    scheduleLegend: i18n._({
      id: "publish.rollout.scheduleLegend",
      message: "Schedule",
      comment: "Legend of the rollout start and end.",
    }),
    startAt: i18n._({
      id: "publish.rollout.startAt",
      message: "Start (empty: at once)",
      comment: "Label of the rollout start time.",
    }),
    endAt: i18n._({
      id: "publish.rollout.endAt",
      message: "End (empty: when complete)",
      comment: "Label of the rollout end time.",
    }),
    healthLegend: i18n._({
      id: "publish.rollout.healthLegend",
      message: "Health gate",
      comment: "Legend of the rollout health gate.",
    }),
    action: i18n._({
      id: "publish.rollout.action",
      message: "When a gate fails",
      comment: "Label of what a failed health gate does.",
    }),
    actionPause: i18n._({
      id: "publish.rollout.actionPause",
      message: "Pause the rollout",
      comment: "Health gate action that pauses the rollout.",
    }),
    actionRollback: i18n._({
      id: "publish.rollout.actionRollback",
      message: "Roll back the release",
      comment: "Health gate action that reverts the rollout.",
    }),
    minSample: i18n._({
      id: "publish.rollout.minSample",
      message: "Fewest sessions before a verdict",
      comment: "Label of the health gate sample size.",
    }),
    windowMinutes: i18n._({
      id: "publish.rollout.windowMinutes",
      message: "Comparison window (minutes, 0: six hours)",
      comment: "Label of the health gate window.",
    }),
    crashFreeDrop: i18n._({
      id: "publish.rollout.crashFreeDrop",
      message: "Largest drop in crash-free sessions (basis points, 0: off)",
      comment: "Label of the crash-free threshold.",
    }),
    errorRateIncrease: i18n._({
      id: "publish.rollout.errorRateIncrease",
      message: "Largest rise in error rate (basis points, 0: off)",
      comment: "Label of the error rate threshold.",
    }),
    actionErrorRateIncrease: i18n._({
      id: "publish.rollout.actionErrorRateIncrease",
      message: "Largest rise in action error rate (basis points, 0: off)",
      comment: "Label of the action error rate threshold.",
    }),
    functionErrorRateIncrease: i18n._({
      id: "publish.rollout.functionErrorRateIncrease",
      message: "Largest rise in function error rate (basis points, 0: off)",
      comment: "Label of the function error rate threshold.",
    }),
    customLegend: i18n._({
      id: "publish.rollout.customLegend",
      message: "Custom metric gates",
      comment: "Legend of the custom health gates.",
    }),
    customEvent: i18n._({
      id: "publish.rollout.customEvent",
      message: "Event name",
      comment: "Label of the event a custom gate counts.",
    }),
    customMax: i18n._({
      id: "publish.rollout.customMax",
      message: "Largest rise per session (basis points)",
      comment: "Label of the custom gate threshold.",
    }),
    addCustom: i18n._({
      id: "publish.rollout.addCustom",
      message: "Add custom gate",
      comment: "Button that adds a custom health gate.",
    }),
    removeCustom: (n: string) =>
      i18n._({
        id: "publish.rollout.removeCustom",
        message: "Remove custom gate {n}",
        values: { n },
        comment: "Button that removes one custom health gate.",
      }),
    noStages: i18n._({
      id: "publish.rollout.noStages",
      message: "Add at least one stage.",
      comment: "Error when the rollout plan has no stage.",
    }),
    percentRange: i18n._({
      id: "publish.rollout.percentRange",
      message: "Use a share from 0.01 to 100.",
      comment: "Error when a stage share is out of range.",
    }),
    percentFalls: i18n._({
      id: "publish.rollout.percentFalls",
      message: "A stage cannot lower the share of the stage before it.",
      comment: "Error when a stage share falls.",
    }),
    durationMissing: i18n._({
      id: "publish.rollout.durationMissing",
      message: "Give every stage but the last a duration of at least a minute.",
      comment: "Error when a stage has no valid duration.",
    }),
    lastStageDuration: i18n._({
      id: "publish.rollout.lastStageDuration",
      message: "The last stage holds until the rollout ends unless it reaches 100 %. Give it no duration.",
      comment: "Error when the last stage below 100 percent has a duration.",
    }),
    dateInvalid: i18n._({
      id: "publish.rollout.dateInvalid",
      message: "Enter a valid date and time.",
      comment: "Error when a rollout time is not a date.",
    }),
    endBeforeStart: i18n._({
      id: "publish.rollout.endBeforeStart",
      message: "The end must come after the start.",
      comment: "Error when the rollout ends before it starts.",
    }),
    negative: i18n._({
      id: "publish.rollout.negative",
      message: "Enter a number of zero or more.",
      comment: "Error when a health gate number is negative or missing.",
    }),
    customEventInvalid: i18n._({
      id: "publish.rollout.customEventInvalid",
      message: "Name an event of up to 64 bytes.",
      comment: "Error when a custom gate has no valid event name.",
    }),
    customThreshold: i18n._({
      id: "publish.rollout.customThreshold",
      message: "Enter a threshold above zero.",
      comment: "Error when a custom gate threshold is not positive.",
    }),
  };
}

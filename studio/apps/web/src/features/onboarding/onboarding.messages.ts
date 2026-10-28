// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the first-run onboarding checklist and the sample apps (STU-015). */
export function onboardingMessages(i18n: I18n) {
  return {
    title: i18n._({
      id: "onboarding.title",
      message: "Get started",
      comment: "Heading of the first-run checklist on the organisation home.",
    }),
    caption: i18n._({
      id: "onboarding.caption",
      message: "From an empty organisation to a first release. Progress follows what you actually do.",
      comment: "Caption of the first-run checklist; its steps are ticked by real events, never by hand.",
    }),
    progressLabel: i18n._({
      id: "onboarding.progress",
      message: "Getting started progress",
      comment: "Accessible name of the meter that shows how many checklist steps are done.",
    }),
    progressText: (done: number, total: number) =>
      i18n._({
        id: "onboarding.progress-text",
        message: "{done} of {total} steps",
        values: { done, total },
        comment: "Reading of the checklist meter: steps done of all steps.",
      }),
    stepsLabel: i18n._({
      id: "onboarding.steps",
      message: "Checklist",
      comment: "Accessible name of the list of onboarding steps.",
    }),
    stepCreate: i18n._({
      id: "onboarding.step.create",
      message: "Create or import an app",
      comment: "Onboarding step: the organisation needs its first app.",
    }),
    stepCreateHint: i18n._({
      id: "onboarding.step.create-hint",
      message: "Start from a blank app, a template or an archive, or import a sample app below.",
      comment: "Explains the first onboarding step.",
    }),
    stepPair: i18n._({
      id: "onboarding.step.pair",
      message: "Pair a device",
      comment: "Onboarding step: connect a phone or an emulator.",
    }),
    stepPairHint: i18n._({
      id: "onboarding.step.pair-hint",
      message: "Connect a phone or an emulator to see your app live.",
      comment: "Explains the pair-a-device onboarding step.",
    }),
    stepEdit: i18n._({
      id: "onboarding.step.edit",
      message: "Edit a screen",
      comment: "Onboarding step: change a screen on the canvas.",
    }),
    stepEditHint: i18n._({
      id: "onboarding.step.edit-hint",
      message: "Change a screen on the canvas. It is saved as you work.",
      comment: "Explains the edit-a-screen onboarding step.",
    }),
    stepPublish: i18n._({
      id: "onboarding.step.publish",
      message: "Publish a release",
      comment: "Onboarding step: publish the app to an environment.",
    }),
    stepPublishHint: i18n._({
      id: "onboarding.step.publish-hint",
      message: "Publish to an environment and watch your device pick it up.",
      comment: "Explains the publish-a-release onboarding step.",
    }),
    stepInvite: i18n._({
      id: "onboarding.step.invite",
      message: "Invite a teammate",
      comment: "Onboarding step: add a second member to the organisation.",
    }),
    stepInviteHint: i18n._({
      id: "onboarding.step.invite-hint",
      message: "Add a member so you can work together.",
      comment: "Explains the invite-a-teammate onboarding step.",
    }),
    statusDone: i18n._({ id: "onboarding.status.done", message: "Done", comment: "Status word: the step is done." }),
    statusTodo: i18n._({
      id: "onboarding.status.todo",
      message: "To do",
      comment: "Status word: the step is not done yet.",
    }),
    newApp: i18n._({
      id: "onboarding.new-app",
      message: "Start a new app",
      comment: "Button on the checklist that opens the new-app wizard.",
    }),
    continueIn: (name: string) =>
      i18n._({
        id: "onboarding.continue-in",
        message: "Continue in {name}",
        values: { name },
        comment: "Link on a checklist step to the organisation's first app; name is the app.",
      }),
    samplesTitle: i18n._({
      id: "onboarding.samples.title",
      message: "Sample apps",
      comment: "Heading of the list of sample apps on the checklist.",
    }),
    samplesCaption: i18n._({
      id: "onboarding.samples.caption",
      message: "Import a working app to explore. It becomes your own copy.",
      comment: "Explains importing a sample app.",
    }),
    samplesLabel: i18n._({
      id: "onboarding.samples.label",
      message: "Sample apps to import",
      comment: "Accessible name of the list of sample apps.",
    }),
    importSample: (name: string) =>
      i18n._({
        id: "onboarding.samples.import",
        message: "Import {name}",
        values: { name },
        comment: "Button that imports a sample app; name is the sample.",
      }),
    importing: i18n._({
      id: "onboarding.samples.importing",
      message: "Importing",
      comment: "Announced while a sample app is imported.",
    }),
    importFailed: i18n._({
      id: "onboarding.samples.failed",
      message: "The sample app could not be imported.",
      comment: "Shown when importing a sample app failed without a reason from the server.",
    }),
    dismiss: i18n._({
      id: "onboarding.dismiss",
      message: "Dismiss checklist",
      comment: "Button that hides the first-run checklist.",
    }),
    reopen: i18n._({
      id: "onboarding.reopen",
      message: "Show getting started",
      comment: "Button that shows the dismissed first-run checklist again.",
    }),
    saveFailed: i18n._({
      id: "onboarding.save-failed",
      message: "Your choice could not be saved.",
      comment: "Shown when dismissing or reopening the checklist failed.",
    }),
  };
}

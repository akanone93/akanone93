// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the Plux Test area (TST-001, TST-002). */
export function testsMessages(i18n: I18n) {
  return {
    listTitle: i18n._({
      id: "tests.listTitle",
      message: "Plux Test",
      comment: "Heading of the Plux Test page: scenarios and runs.",
    }),
    listCaption: i18n._({
      id: "tests.listCaption",
      message: "Declarative test scenarios of this app, and the runs that `plux test` and CI reported.",
      comment: "Caption under the Plux Test heading.",
    }),
    scenariosHeading: i18n._({
      id: "tests.scenariosHeading",
      message: "Scenarios",
      comment: "Heading of the scenarios table.",
    }),
    runsHeading: i18n._({ id: "tests.runsHeading", message: "Runs", comment: "Heading of the runs table." }),
    searchLabel: i18n._({
      id: "tests.searchLabel",
      message: "Filter scenarios",
      comment: "Label of the scenario filter box.",
    }),
    noScenarios: i18n._({
      id: "tests.noScenarios",
      message: "No scenarios yet.",
      comment: "Empty state of the scenarios table.",
    }),
    noRuns: i18n._({ id: "tests.noRuns", message: "No runs reported yet.", comment: "Empty state of the runs table." }),
    colActions: i18n._({ id: "tests.colActions", message: "Actions", comment: "Column heading of the row actions." }),
    colName: i18n._({ id: "tests.colName", message: "Scenario", comment: "Column heading." }),
    colTarget: i18n._({ id: "tests.colTarget", message: "Page or flow", comment: "Column heading." }),
    colFile: i18n._({ id: "tests.colFile", message: "File", comment: "Column heading." }),
    colSteps: i18n._({ id: "tests.colSteps", message: "Steps", comment: "Column heading." }),
    colExpectations: i18n._({ id: "tests.colExpectations", message: "Expectations", comment: "Column heading." }),
    colRun: i18n._({ id: "tests.colRun", message: "Run", comment: "Column heading." }),
    colLabel: i18n._({ id: "tests.colLabel", message: "Label", comment: "Column heading." }),
    colSource: i18n._({ id: "tests.colSource", message: "Source", comment: "Column heading." }),
    colResult: i18n._({ id: "tests.colResult", message: "Result", comment: "Column heading." }),
    colWhen: i18n._({ id: "tests.colWhen", message: "Reported", comment: "Column heading." }),
    colStatus: i18n._({ id: "tests.colStatus", message: "Status", comment: "Column heading." }),
    colDuration: i18n._({ id: "tests.colDuration", message: "Duration", comment: "Column heading." }),
    colStep: i18n._({ id: "tests.colStep", message: "Step", comment: "Column heading." }),
    colMessage: i18n._({ id: "tests.colMessage", message: "Message", comment: "Column heading." }),
    remove: i18n._({ id: "tests.remove", message: "Remove", comment: "Button: remove one scenario." }),
    removed: i18n._({
      id: "tests.removed",
      message: "Scenario removed.",
      comment: "Status after removing a scenario.",
    }),
    saved: i18n._({ id: "tests.saved", message: "Scenario saved.", comment: "Status after saving a scenario." }),
    saveFailed: i18n._({
      id: "tests.saveFailed",
      message: "The scenario could not be saved.",
      comment: "Error after a refused save.",
    }),
    readOnly: i18n._({
      id: "tests.readOnly",
      message: "You may read scenarios but not change them.",
      comment: "Shown when the person lacks plugin.edit.",
    }),
    newTitle: i18n._({ id: "tests.newTitle", message: "New scenario", comment: "Heading of the scenario form." }),
    fileLabel: i18n._({ id: "tests.fileLabel", message: "File", comment: "Label: the scenarios file name." }),
    nameLabel: i18n._({ id: "tests.nameLabel", message: "Name", comment: "Label: the scenario name." }),
    pageLabel: i18n._({
      id: "tests.pageLabel",
      message: "Page route",
      comment: "Label: the route of the page under test.",
    }),
    stepsHeading: i18n._({ id: "tests.stepsHeading", message: "Steps", comment: "Heading of the steps group." }),
    expectsHeading: i18n._({
      id: "tests.expectsHeading",
      message: "Expectations",
      comment: "Heading of the expectations group.",
    }),
    addStep: i18n._({ id: "tests.addStep", message: "Add step", comment: "Button." }),
    addExpect: i18n._({ id: "tests.addExpect", message: "Add expectation", comment: "Button." }),
    kindLabel: i18n._({ id: "tests.kindLabel", message: "Kind", comment: "Label of a step or expectation kind." }),
    testIdLabel: i18n._({ id: "tests.testIdLabel", message: "Test ID", comment: "Label of a node test ID." }),
    textLabel: i18n._({
      id: "tests.textLabel",
      message: "Text",
      comment: "Label of the text of a step or expectation.",
    }),
    routeLabel: i18n._({ id: "tests.routeLabel", message: "Route", comment: "Label of a route in an expectation." }),
    save: i18n._({ id: "tests.save", message: "Save scenario", comment: "Button: save the scenario." }),
    stepTap: i18n._({ id: "tests.stepTap", message: "Tap", comment: "Step kind." }),
    stepEnterText: i18n._({ id: "tests.stepEnterText", message: "Enter text", comment: "Step kind." }),
    stepWaitFor: i18n._({ id: "tests.stepWaitFor", message: "Wait for", comment: "Step kind." }),
    stepScroll: i18n._({ id: "tests.stepScroll", message: "Scroll", comment: "Step kind." }),
    expectVisible: i18n._({ id: "tests.expectVisible", message: "Is visible", comment: "Expectation kind." }),
    expectNotVisible: i18n._({ id: "tests.expectNotVisible", message: "Is not visible", comment: "Expectation kind." }),
    expectTextEquals: i18n._({ id: "tests.expectTextEquals", message: "Text equals", comment: "Expectation kind." }),
    expectNavigatedTo: i18n._({ id: "tests.expectNavigatedTo", message: "Navigated to", comment: "Expectation kind." }),
    problemFile: i18n._({
      id: "tests.problemFile",
      message: "The file name needs lowercase letters, digits and hyphens.",
      comment: "Validation.",
    }),
    problemName: i18n._({ id: "tests.problemName", message: "The scenario needs a name.", comment: "Validation." }),
    problemPage: i18n._({ id: "tests.problemPage", message: "The page needs a route name.", comment: "Validation." }),
    problemStep: i18n._({
      id: "tests.problemStep",
      message: "Every step needs a valid test ID, and entered text needs text.",
      comment: "Validation.",
    }),
    problemExpect: i18n._({
      id: "tests.problemExpect",
      message: "Add at least one complete expectation.",
      comment: "Validation.",
    }),
    runTitle: i18n._({ id: "tests.runTitle", message: "Test run", comment: "Heading of the run page." }),
    runCaption: i18n._({
      id: "tests.runCaption",
      message: "Results per scenario and step.",
      comment: "Caption of the run page.",
    }),
    back: i18n._({ id: "tests.back", message: "Back to Plux Test", comment: "Link back to the list." }),
    passed: i18n._({ id: "tests.passed", message: "Passed", comment: "Status." }),
    failed: i18n._({ id: "tests.failed", message: "Failed", comment: "Status." }),
    skipped: i18n._({ id: "tests.skipped", message: "Skipped", comment: "Status." }),
    summary: i18n._({
      id: "tests.summary",
      message: "Passed, failed and skipped scenarios",
      comment: "Accessible name of the run summary.",
    }),
    noSteps: i18n._({
      id: "tests.noSteps",
      message: "This scenario reported no steps.",
      comment: "Shown for a scenario without steps.",
    }),
    fromSession: i18n._({
      id: "tests.fromSession",
      message: "Create test from device session",
      comment: "Button that fills the new scenario form with the taps recorded in the device-loop session.",
    }),
    fromSessionSteps: (count: number) =>
      i18n._({
        id: "tests.fromSessionSteps",
        message:
          "{count, plural, one {# tap recorded in the device session} other {# taps recorded in the device session}}",
        values: { count },
        comment: "How many tap steps the device-loop session has recorded so far.",
      }),
    never: i18n._({ id: "tests.never", message: "not reported", comment: "Fallback of a missing time." }),
  };
}

export type TestsMessages = ReturnType<typeof testsMessages>;

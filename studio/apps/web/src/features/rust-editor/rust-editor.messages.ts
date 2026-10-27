// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the Rust editor overlay (FN-033, S5.4). */
export function rustEditorMessages(i18n: I18n) {
  return {
    open: i18n._({
      id: "rustEditor.open",
      message: "Rust editor",
      comment: "Button in the right panel that opens the full-screen editor for the Rust source of a function.",
    }),
    title: i18n._({
      id: "rustEditor.title",
      message: "Rust editor",
      comment: "Title of the full-screen overlay in which the Rust source of a function is edited.",
    }),
    close: i18n._({
      id: "rustEditor.close",
      message: "Close the Rust editor",
      comment: "Accessible name of the button that closes the full-screen Rust editor.",
    }),
    source: i18n._({
      id: "rustEditor.source",
      message: "Uploaded source",
      comment: "Label of the field that takes the identifier of an uploaded function source.",
    }),
    sourceHint: i18n._({
      id: "rustEditor.sourceHint",
      message:
        "The identifier of the function source already uploaded to the server. The language server and the tests run on that source.",
      comment: "Help text under the field for the identifier of an uploaded function source.",
    }),
    connect: i18n._({
      id: "rustEditor.connect",
      message: "Start the language server",
      comment: "Button that starts the language server for the uploaded source.",
    }),
    stop: i18n._({
      id: "rustEditor.stop",
      message: "Stop the language server",
      comment: "Button that ends the language-server session.",
    }),
    template: i18n._({
      id: "rustEditor.template",
      message: "Starter template",
      comment: "Label of the list of starter templates for a new function.",
    }),
    useTemplate: i18n._({
      id: "rustEditor.useTemplate",
      message: "Replace the source with the template",
      comment: "Button that puts the chosen starter template into the editor, replacing its text.",
    }),
    editorLabel: i18n._({
      id: "rustEditor.editorLabel",
      message: "Rust source of the function",
      comment: "Accessible name of the text editor that holds the function's Rust source.",
    }),
    idle: i18n._({
      id: "rustEditor.idle",
      message: "The language server is not running. Completion, hints and diagnostics need it.",
      comment: "Status shown before the language server was started.",
    }),
    starting: i18n._({
      id: "rustEditor.starting",
      message: "Starting the language server…",
      comment: "Status shown while the language-server sandbox starts.",
    }),
    ready: i18n._({
      id: "rustEditor.ready",
      message: "The language server is ready.",
      comment: "Status shown when the language server answers.",
    }),
    failed: (detail: string) =>
      i18n._({
        id: "rustEditor.failed",
        message: "The language server could not start: {detail}",
        values: { detail },
        comment: "Status shown when the language server did not start; {detail} is the reason.",
      }),
    closedTime: i18n._({
      id: "rustEditor.closedTime",
      message: "The language-server session reached its time limit. Start it again to continue.",
      comment: "Status shown when the language-server session ended because its time ran out.",
    }),
    closedExited: i18n._({
      id: "rustEditor.closedExited",
      message: "The language server stopped. Start it again to continue.",
      comment: "Status shown when the language server process exited.",
    }),
    closedClient: i18n._({
      id: "rustEditor.closedClient",
      message: "The language server was stopped.",
      comment: "Status shown after the person stopped the language server.",
    }),
    closedFailed: (detail: string) =>
      i18n._({
        id: "rustEditor.closedFailed",
        message: "The connection to the language server broke: {detail}",
        values: { detail },
        comment: "Status shown when the connection to the language server failed; {detail} is the reason.",
      }),
    readOnly: i18n._({
      id: "rustEditor.readOnly",
      message: "You can read this source but not change it.",
      comment: "Shown when the person may not edit.",
    }),
    tabTests: i18n._({
      id: "rustEditor.tabTests",
      message: "Tests",
      comment: "Name of the tab that runs the function's tests.",
    }),
    tabDependencies: i18n._({
      id: "rustEditor.tabDependencies",
      message: "Dependencies",
      comment: "Name of the tab that manages the crates the function depends on.",
    }),
    runTests: i18n._({
      id: "rustEditor.runTests",
      message: "Run the tests",
      comment: "Button that builds the source and runs its stored test cases.",
    }),
    runningTests: i18n._({
      id: "rustEditor.runningTests",
      message: "Running the tests…",
      comment: "Status shown while the tests run.",
    }),
    testsPassed: (passed: number, failed: number) =>
      i18n._({
        id: "rustEditor.testsPassed",
        message:
          "{passed, plural, one {# test passed} other {# tests passed}}, {failed, plural, one {# failed} other {# failed}}.",
        values: { passed, failed },
        comment: "Summary of a finished test run: the number of tests that passed and failed.",
      }),
    testPass: i18n._({
      id: "rustEditor.testPass",
      message: "Passed",
      comment: "Result of one test case that passed.",
    }),
    testFail: i18n._({
      id: "rustEditor.testFail",
      message: "Failed",
      comment: "Result of one test case that failed.",
    }),
    testsFailedToRun: (detail: string) =>
      i18n._({
        id: "rustEditor.testsFailedToRun",
        message: "The tests could not run: {detail}",
        values: { detail },
        comment: "Shown when the test run itself failed; {detail} is the reason.",
      }),
    noTests: i18n._({
      id: "rustEditor.noTests",
      message: "Run the tests to see their results here.",
      comment: "Shown in the tests tab before any run.",
    }),
    buildDiagnostics: i18n._({
      id: "rustEditor.buildDiagnostics",
      message: "Build output",
      comment: "Heading of the compiler output shown when the source did not build.",
    }),
    crate: i18n._({
      id: "rustEditor.crate",
      message: "Crate name",
      comment: "Label of the field for the name of a crate to depend on.",
    }),
    version: i18n._({
      id: "rustEditor.version",
      message: "Version requirement",
      comment: "Label of the field for the version requirement of a crate, such as 1.0.",
    }),
    addDependency: i18n._({
      id: "rustEditor.addDependency",
      message: "Add or update the dependency",
      comment: "Button that adds a crate to the manifest or updates its version.",
    }),
    removeDependency: (name: string) =>
      i18n._({
        id: "rustEditor.removeDependency",
        message: "Remove {name}",
        values: { name },
        comment: "Accessible name of the button that removes the crate {name} from the dependencies.",
      }),
    crateInvalid: i18n._({
      id: "rustEditor.crateInvalid",
      message: "Crate names start with a letter and use letters, digits, - and _.",
      comment: "Error under the crate-name field when the name is not a valid crate name.",
    }),
    versionInvalid: i18n._({
      id: "rustEditor.versionInvalid",
      message: "Use a version requirement such as 1.0 or ^0.8.2.",
      comment: "Error under the version field when the text is not a Cargo version requirement.",
    }),
    noDependencies: i18n._({
      id: "rustEditor.noDependencies",
      message: "The function has no dependencies.",
      comment: "Shown when the manifest declares no dependencies.",
    }),
    vendorNote: i18n._({
      id: "rustEditor.vendorNote",
      message: "A crate added here takes effect when the source is uploaded again together with its vendored sources.",
      comment: "Note under the dependency list: the builder works from vendored sources only.",
    }),
    manifest: i18n._({
      id: "rustEditor.manifest",
      message: "Cargo manifest",
      comment: "Heading of the Cargo.toml text the dependencies are written to.",
    }),
  };
}

export type RustEditorMessages = ReturnType<typeof rustEditorMessages>;

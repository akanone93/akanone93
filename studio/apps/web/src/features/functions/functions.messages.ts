// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the Functions area (STU-110, FN-033 to FN-035). */
export function functionsMessages(i18n: I18n) {
  return {
    listTitle: i18n._({ id: "functions.listTitle", message: "Functions", comment: "Heading of the functions list." }),
    listCaption: i18n._({
      id: "functions.listCaption",
      message: "The functions of this environment with their latest version.",
      comment: "Caption under the functions heading.",
    }),
    noFunctions: i18n._({
      id: "functions.noFunctions",
      message: "No function is deployed in this environment.",
      comment: "Shown when there is no function.",
    }),
    colName: i18n._({ id: "functions.colName", message: "Function", comment: "Column heading." }),
    colVersion: i18n._({ id: "functions.colVersion", message: "Latest version", comment: "Column heading." }),
    colPlacement: i18n._({ id: "functions.colPlacement", message: "Placement", comment: "Column heading." }),
    colTests: i18n._({ id: "functions.colTests", message: "Tests", comment: "Column heading." }),
    colCreated: i18n._({ id: "functions.colCreated", message: "Created", comment: "Column heading." }),
    colAlias: i18n._({ id: "functions.colAlias", message: "Alias", comment: "Column heading." }),
    testsPassedYes: i18n._({
      id: "functions.testsPassedYes",
      message: "Passed",
      comment: "Shown when the tests of a version passed.",
    }),
    testsPassedNo: i18n._({
      id: "functions.testsPassedNo",
      message: "Failed",
      comment: "Shown when the tests of a version failed.",
    }),
    tabVersions: i18n._({ id: "functions.tabVersions", message: "Versions", comment: "Tab label." }),
    tabAssurance: i18n._({ id: "functions.tabAssurance", message: "Assurance", comment: "Tab label." }),
    tabCapabilities: i18n._({ id: "functions.tabCapabilities", message: "Capabilities", comment: "Tab label." }),
    tabTests: i18n._({ id: "functions.tabTests", message: "Tests", comment: "Tab label." }),
    tabMetrics: i18n._({ id: "functions.tabMetrics", message: "Metrics", comment: "Tab label." }),
    tabLogs: i18n._({ id: "functions.tabLogs", message: "Logs", comment: "Tab label." }),
    openEditor: i18n._({
      id: "functions.openEditor",
      message: "Edit in the Rust editor",
      comment: "Link to the full-page Rust editor.",
    }),
    back: i18n._({ id: "functions.back", message: "All functions", comment: "Link back to the list." }),
    promoteAlias: i18n._({
      id: "functions.promoteAlias",
      message: "Point alias at this version",
      comment: "Button that points an alias at a version.",
    }),
    aliasLabel: i18n._({ id: "functions.aliasLabel", message: "Alias", comment: "Label of the alias field." }),
    aliasFailed: i18n._({
      id: "functions.aliasFailed",
      message: "The alias could not be changed.",
      comment: "Shown when SetAlias fails.",
    }),
    assuranceSigned: i18n._({
      id: "functions.assuranceSigned",
      message: "Signed by the builder",
      comment: "Says the version carries a builder signature.",
    }),
    assuranceUnsigned: i18n._({
      id: "functions.assuranceUnsigned",
      message: "Not signed",
      comment: "Says the version carries no signature.",
    }),
    assuranceTests: i18n._({
      id: "functions.assuranceTests",
      message: "Stored tests",
      comment: "Label of the stored tests state.",
    }),
    assuranceSource: i18n._({ id: "functions.assuranceSource", message: "Source hash", comment: "Label." }),
    assuranceModule: i18n._({ id: "functions.assuranceModule", message: "Module hash", comment: "Label." }),
    assuranceToolchain: i18n._({ id: "functions.assuranceToolchain", message: "Toolchain", comment: "Label." }),
    assuranceBuilder: i18n._({ id: "functions.assuranceBuilder", message: "Builder", comment: "Label." }),
    assuranceDeps: i18n._({ id: "functions.assuranceDeps", message: "Dependencies", comment: "Label." }),
    noDeps: i18n._({ id: "functions.noDeps", message: "No dependencies.", comment: "Shown when a build has none." }),
    capInput: i18n._({ id: "functions.capInput", message: "Input schema", comment: "Label." }),
    capOutput: i18n._({ id: "functions.capOutput", message: "Output schema", comment: "Label." }),
    capAbi: i18n._({ id: "functions.capAbi", message: "Host ABI version", comment: "Label." }),
    capPlacement: i18n._({
      id: "functions.capPlacement",
      message: "Runs on",
      comment: "Label of where the function runs.",
    }),
    placementServer: i18n._({ id: "functions.placementServer", message: "the server", comment: "Placement value." }),
    placementDevice: i18n._({ id: "functions.placementDevice", message: "the device", comment: "Placement value." }),
    sourceLabel: i18n._({
      id: "functions.sourceLabel",
      message: "Uploaded source",
      comment: "Label of the source id field.",
    }),
    metricInvocations: i18n._({ id: "functions.metricInvocations", message: "Invocations", comment: "Metric label." }),
    metricP50: i18n._({ id: "functions.metricP50", message: "Latency p50 (microseconds)", comment: "Metric label." }),
    metricP95: i18n._({ id: "functions.metricP95", message: "Latency p95 (microseconds)", comment: "Metric label." }),
    metricP99: i18n._({ id: "functions.metricP99", message: "Latency p99 (microseconds)", comment: "Metric label." }),
    metricMemory: i18n._({ id: "functions.metricMemory", message: "Memory peak (bytes)", comment: "Metric label." }),
    metricCache: i18n._({ id: "functions.metricCache", message: "Cache hits", comment: "Metric label." }),
    metricErrors: i18n._({ id: "functions.metricErrors", message: "Errors by kind", comment: "Metric label." }),
    noErrors: i18n._({ id: "functions.noErrors", message: "No errors.", comment: "Shown when no error occurred." }),
    logsTitle: i18n._({ id: "functions.logsTitle", message: "Log lines", comment: "Caption of the log table." }),
    colTime: i18n._({ id: "functions.colTime", message: "Time", comment: "Column heading." }),
    colLevel: i18n._({ id: "functions.colLevel", message: "Level", comment: "Column heading." }),
    colMessage: i18n._({ id: "functions.colMessage", message: "Message", comment: "Column heading." }),
    noLogs: i18n._({ id: "functions.noLogs", message: "No log lines.", comment: "Shown when there are none." }),
    envLabel: i18n._({ id: "functions.envLabel", message: "Environment", comment: "Label of the environment filter." }),
    loading: i18n._({ id: "functions.loading", message: "Loading", comment: "Shown while a page loads." }),
    loadFailed: i18n._({
      id: "functions.loadFailed",
      message: "This could not be loaded.",
      comment: "Shown when a request fails.",
    }),
    editTitle: i18n._({
      id: "functions.editTitle",
      message: "Rust editor",
      comment: "Heading of the full-page editor.",
    }),
    versionN: (version: string) =>
      i18n._({
        id: "functions.versionN",
        message: "Version {version}",
        comment: "Names a version.",
        values: { version },
      }),
    aliasAt: (alias: string, version: string) =>
      i18n._({
        id: "functions.aliasAt",
        message: "{alias} points at version {version}",
        comment: "Says where an alias points.",
        values: { alias, version },
      }),
    detailTitle: (name: string) =>
      i18n._({
        id: "functions.detailTitle",
        message: "Function {name}",
        comment: "Heading of one function.",
        values: { name },
      }),
  };
}

export type FunctionsMessages = ReturnType<typeof functionsMessages>;

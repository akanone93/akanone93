// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the Experiments and flags area (STU-103, ABT-001, ABT-003). */
export function experimentsMessages(i18n: I18n) {
  return {
    listTitle: i18n._({
      id: "experiments.listTitle",
      message: "Experiments",
      comment: "Heading of the experiments list.",
    }),
    listCaption: i18n._({
      id: "experiments.listCaption",
      message: "Experiments of this environment, with their state and the share of subjects they reach.",
      comment: "Caption under the experiments heading.",
    }),
    newLink: i18n._({
      id: "experiments.newLink",
      message: "New experiment",
      comment: "Link and heading of the experiment wizard.",
    }),
    flagsLink: i18n._({
      id: "experiments.flagsLink",
      message: "Feature flags",
      comment: "Link to and heading of the flags page.",
    }),
    statusLabel: i18n._({
      id: "experiments.statusLabel",
      message: "State",
      comment: "Label of the state filter and a column heading.",
    }),
    anyState: i18n._({
      id: "experiments.anyState",
      message: "Any state",
      comment: "Filter option: every experiment state.",
    }),
    colKey: i18n._({ id: "experiments.colKey", message: "Key", comment: "Column heading." }),
    colStarted: i18n._({ id: "experiments.colStarted", message: "Started", comment: "Column heading." }),
    colWinner: i18n._({ id: "experiments.colWinner", message: "Winner", comment: "Column heading." }),
    noExperiments: i18n._({
      id: "experiments.noExperiments",
      message: "No experiments yet.",
      comment: "Shown when no experiment matches.",
    }),
    never: i18n._({ id: "experiments.never", message: "never", comment: "Shown for a missing date." }),
    loading: i18n._({ id: "experiments.loading", message: "Loading", comment: "Shown while a page loads." }),
    loadFailed: i18n._({
      id: "experiments.loadFailed",
      message: "This could not be loaded.",
      comment: "Shown when a request fails.",
    }),
    actionFailed: i18n._({
      id: "experiments.actionFailed",
      message: "The action failed. Nothing was changed.",
      comment: "Shown when an action fails.",
    }),
    stateDraft: i18n._({ id: "experiments.stateDraft", message: "Draft", comment: "State of an experiment." }),
    stateRunning: i18n._({ id: "experiments.stateRunning", message: "Running", comment: "State of an experiment." }),
    stateStopped: i18n._({ id: "experiments.stateStopped", message: "Stopped", comment: "State of an experiment." }),
    stateConcluded: i18n._({
      id: "experiments.stateConcluded",
      message: "Concluded",
      comment: "State of an experiment.",
    }),
    wizardTitle: i18n._({
      id: "experiments.wizardTitle",
      message: "New experiment",
      comment: "Heading of the experiment wizard.",
    }),
    wizardCaption: i18n._({
      id: "experiments.wizardCaption",
      message: "Vary a selection of the canvas, split the audience and name the metrics that decide.",
      comment: "Caption of the wizard.",
    }),
    stepTarget: i18n._({ id: "experiments.stepTarget", message: "Target", comment: "Wizard step." }),
    stepVariants: i18n._({ id: "experiments.stepVariants", message: "Variants", comment: "Wizard step and heading." }),
    stepAllocation: i18n._({ id: "experiments.stepAllocation", message: "Allocation", comment: "Wizard step." }),
    stepTargeting: i18n._({ id: "experiments.stepTargeting", message: "Targeting", comment: "Wizard step." }),
    stepMetrics: i18n._({ id: "experiments.stepMetrics", message: "Metrics", comment: "Wizard step." }),
    stepSchedule: i18n._({ id: "experiments.stepSchedule", message: "Schedule", comment: "Wizard step." }),
    back: i18n._({ id: "experiments.back", message: "Back", comment: "Button: previous wizard step." }),
    next: i18n._({ id: "experiments.next", message: "Next", comment: "Button: next wizard step." }),
    create: i18n._({
      id: "experiments.create",
      message: "Create experiment",
      comment: "Button that creates the experiment.",
    }),
    keyLabel: i18n._({ id: "experiments.keyLabel", message: "Experiment key", comment: "Label of the key field." }),
    targetLabel: i18n._({
      id: "experiments.targetLabel",
      message: "Subject of the experiment",
      comment: "Label of the field that names the varied nodes, page or prop.",
    }),
    scopeLabel: i18n._({ id: "experiments.scopeLabel", message: "Scope", comment: "Label of the scope select." }),
    scopePage: i18n._({ id: "experiments.scopePage", message: "Page", comment: "Scope option." }),
    scopeSubtree: i18n._({ id: "experiments.scopeSubtree", message: "Component subtree", comment: "Scope option." }),
    scopeProp: i18n._({ id: "experiments.scopeProp", message: "Prop value", comment: "Scope option." }),
    scopeFlow: i18n._({ id: "experiments.scopeFlow", message: "Flow", comment: "Scope option." }),
    scopePluginVersion: i18n._({
      id: "experiments.scopePluginVersion",
      message: "Plugin version",
      comment: "Scope option.",
    }),
    variantName: i18n._({
      id: "experiments.variantName",
      message: "Variant name",
      comment: "Label of a variant name field.",
    }),
    variantControl: i18n._({
      id: "experiments.variantControl",
      message: "Control",
      comment: "Marks the control variant.",
    }),
    addVariant: i18n._({
      id: "experiments.addVariant",
      message: "Add variant",
      comment: "Button that adds a variant.",
    }),
    removeVariant: i18n._({
      id: "experiments.removeVariant",
      message: "Remove variant",
      comment: "Button that removes a variant.",
    }),
    weightLabel: i18n._({
      id: "experiments.weightLabel",
      message: "Share in percent",
      comment: "Label of a variant weight field.",
    }),
    weightsSum: i18n._({
      id: "experiments.weightsSum",
      message: "The shares must add up to 100.",
      comment: "Error: variant shares do not sum to 100.",
    }),
    needControl: i18n._({
      id: "experiments.needControl",
      message: "Exactly one variant must be the control.",
      comment: "Error: not exactly one control.",
    }),
    holdoutLabel: i18n._({
      id: "experiments.holdoutLabel",
      message: "Holdout in percent",
      comment: "Label of the holdout field.",
    }),
    segmentLabel: i18n._({
      id: "experiments.segmentLabel",
      message: "Audience segment",
      comment: "Label of the segment field.",
    }),
    segmentHelp: i18n._({
      id: "experiments.segmentHelp",
      message: "Leave empty to reach every subject.",
      comment: "Help under the segment field.",
    }),
    primaryEvent: i18n._({
      id: "experiments.primaryEvent",
      message: "Primary metric event",
      comment: "Label of the primary event field.",
    }),
    primaryKind: i18n._({
      id: "experiments.primaryKind",
      message: "Metric kind",
      comment: "Label of the metric kind select.",
    }),
    kindProportion: i18n._({
      id: "experiments.kindProportion",
      message: "Share of subjects",
      comment: "Metric kind option.",
    }),
    kindMean: i18n._({ id: "experiments.kindMean", message: "Mean of a property", comment: "Metric kind option." }),
    directionLabel: i18n._({
      id: "experiments.directionLabel",
      message: "Improvement",
      comment: "Label of the metric direction select.",
    }),
    directionIncrease: i18n._({
      id: "experiments.directionIncrease",
      message: "Increase",
      comment: "Direction option.",
    }),
    directionDecrease: i18n._({
      id: "experiments.directionDecrease",
      message: "Decrease",
      comment: "Direction option.",
    }),
    guardrailEvent: i18n._({
      id: "experiments.guardrailEvent",
      message: "Guardrail event",
      comment: "Label of the guardrail event field.",
    }),
    minimumSample: i18n._({
      id: "experiments.minimumSample",
      message: "Subjects per variant before a result",
      comment: "Label of the minimum sample field.",
    }),
    startLabel: i18n._({ id: "experiments.startLabel", message: "Start", comment: "Label of the start date field." }),
    endLabel: i18n._({ id: "experiments.endLabel", message: "End", comment: "Label of the end date field." }),
    invalidForm: i18n._({
      id: "experiments.invalidForm",
      message: "Fix the marked fields before creating the experiment.",
      comment: "Error under the wizard.",
    }),
    resultsTitle: i18n._({
      id: "experiments.resultsTitle",
      message: "Experiment",
      comment: "Heading of the experiment page.",
    }),
    definition: i18n._({
      id: "experiments.definition",
      message: "Definition",
      comment: "Heading of the definition section.",
    }),
    results: i18n._({ id: "experiments.results", message: "Results", comment: "Heading of the results section." }),
    noResults: i18n._({
      id: "experiments.noResults",
      message: "No results yet.",
      comment: "Shown while the experiment has no results.",
    }),
    colVariant: i18n._({ id: "experiments.colVariant", message: "Variant", comment: "Column heading." }),
    colSubjects: i18n._({ id: "experiments.colSubjects", message: "Subjects", comment: "Column heading." }),
    colShare: i18n._({ id: "experiments.colShare", message: "Share", comment: "Column heading." }),
    colMean: i18n._({ id: "experiments.colMean", message: "Mean", comment: "Column heading." }),
    colDiff: i18n._({ id: "experiments.colDiff", message: "Difference", comment: "Column heading." }),
    colInterval: i18n._({ id: "experiments.colInterval", message: "Interval", comment: "Column heading." }),
    colP: i18n._({ id: "experiments.colP", message: "p-value", comment: "Column heading." }),
    colVerdict: i18n._({ id: "experiments.colVerdict", message: "Verdict", comment: "Column heading." }),
    improved: i18n._({ id: "experiments.improved", message: "Improved", comment: "Verdict of a comparison." }),
    worsened: i18n._({ id: "experiments.worsened", message: "Worsened", comment: "Verdict of a comparison." }),
    inconclusive: i18n._({
      id: "experiments.inconclusive",
      message: "Inconclusive",
      comment: "Verdict of a comparison.",
    }),
    guardrailTripped: i18n._({
      id: "experiments.guardrailTripped",
      message: "Guardrail tripped",
      comment: "Verdict: a guardrail regressed beyond its limit.",
    }),
    primaryMetric: i18n._({ id: "experiments.primaryMetric", message: "Primary metric", comment: "Heading." }),
    guardrails: i18n._({ id: "experiments.guardrails", message: "Guardrails", comment: "Heading." }),
    mismatch: i18n._({
      id: "experiments.mismatch",
      message: "Sample ratio mismatch: the observed split differs from the planned one. Do not trust the results.",
      comment: "Warning shown on a sample ratio mismatch.",
    }),
    decision: i18n._({ id: "experiments.decision", message: "Decision", comment: "Heading of the decision section." }),
    start: i18n._({
      id: "experiments.start",
      message: "Start experiment",
      comment: "Button that starts the experiment.",
    }),
    stop: i18n._({ id: "experiments.stop", message: "Stop experiment", comment: "Button that stops the experiment." }),
    ship: i18n._({ id: "experiments.ship", message: "Ship this variant", comment: "Button that ships a variant." }),
    shipLabel: i18n._({
      id: "experiments.shipLabel",
      message: "Release label",
      comment: "Label of the label field of a shipped variant.",
    }),
    shipped: i18n._({
      id: "experiments.shipped",
      message: "Shipped as a publish job.",
      comment: "Shown after a variant was shipped.",
    }),
    flagsTitle: i18n._({
      id: "experiments.flagsTitle",
      message: "Feature flags",
      comment: "Heading of the flags page.",
    }),
    flagsCaption: i18n._({
      id: "experiments.flagsCaption",
      message: "Flags of this environment: a typed value with a default and values per segment.",
      comment: "Caption of the flags page.",
    }),
    flagKey: i18n._({ id: "experiments.flagKey", message: "Flag key", comment: "Label of the flag key field." }),
    flagType: i18n._({ id: "experiments.flagType", message: "Type", comment: "Label of the flag type select." }),
    typeBool: i18n._({ id: "experiments.typeBool", message: "Boolean", comment: "Flag type." }),
    typeNumber: i18n._({ id: "experiments.typeNumber", message: "Number", comment: "Flag type." }),
    typeString: i18n._({ id: "experiments.typeString", message: "String", comment: "Flag type." }),
    typeJson: i18n._({ id: "experiments.typeJson", message: "JSON", comment: "Flag type." }),
    flagDescription: i18n._({
      id: "experiments.flagDescription",
      message: "Description",
      comment: "Label of the flag description field.",
    }),
    flagDefault: i18n._({
      id: "experiments.flagDefault",
      message: "Default value (JSON)",
      comment: "Label of the flag default field.",
    }),
    colDefault: i18n._({ id: "experiments.colDefault", message: "Default", comment: "Column heading." }),
    colType: i18n._({ id: "experiments.colType", message: "Type", comment: "Column heading." }),
    saveFlag: i18n._({ id: "experiments.saveFlag", message: "Save flag", comment: "Button that saves a flag." }),
    editFlag: i18n._({
      id: "experiments.editFlag",
      message: "Edit",
      comment: "Button that loads a flag into the form.",
    }),
    deleteFlag: i18n._({ id: "experiments.deleteFlag", message: "Delete", comment: "Button that deletes a flag." }),
    noFlags: i18n._({
      id: "experiments.noFlags",
      message: "No flags yet.",
      comment: "Shown when the environment has no flag.",
    }),
    badJson: i18n._({
      id: "experiments.badJson",
      message: "The default value must be valid JSON.",
      comment: "Error under the flag default field.",
    }),
    forKey: (key: string) =>
      i18n._({
        id: "experiments.forKey",
        message: "Experiment {key}",
        comment: "Heading of one experiment.",
        values: { key },
      }),
    removeNamed: (name: string) =>
      i18n._({
        id: "experiments.removeNamed",
        message: "Remove variant {name}",
        comment: "Accessible name of a remove button.",
        values: { name },
      }),
    editNamed: (key: string) =>
      i18n._({
        id: "experiments.editNamed",
        message: "Edit {key}",
        comment: "Accessible name of an edit button.",
        values: { key },
      }),
    deleteNamed: (key: string) =>
      i18n._({
        id: "experiments.deleteNamed",
        message: "Delete {key}",
        comment: "Accessible name of a delete button.",
        values: { key },
      }),
    shipNamed: (name: string) =>
      i18n._({
        id: "experiments.shipNamed",
        message: "Ship {name}",
        comment: "Accessible name of a ship button.",
        values: { name },
      }),
  };
}

export type ExperimentsMessages = ReturnType<typeof experimentsMessages>;

/** The names of the experiment states, keyed by the enum value. */
export function stateNames(m: ExperimentsMessages): Readonly<Record<number, string>> {
  return { 1: m.stateDraft, 2: m.stateRunning, 3: m.stateStopped, 4: m.stateConcluded };
}

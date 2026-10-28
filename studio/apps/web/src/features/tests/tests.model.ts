// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { z } from "zod";
import type { DocumentDraft, JsonValue } from "../editor/index.ts";

/** The directory of the app's scenario files, found through `tests` in `plux.yaml` (TST-001). */
export const SCENARIO_DIR = "tests";
const SCENARIO_FILE = /\.scenario\.json$/;
const TEST_ID = /^[a-z][a-zA-Z0-9_-]*$/;
const FILE_NAME = /^[a-z][a-z0-9-]{0,62}$/;
const ROUTE = /^[a-z][a-zA-Z0-9_-]*$/;

export const stepKinds = ["tap", "enterText", "waitFor", "scroll"] as const;
export type StepKind = (typeof stepKinds)[number];
export const expectKinds = ["visible", "notVisible", "textEquals", "navigatedTo"] as const;
export type ExpectKind = (typeof expectKinds)[number];

export interface DraftStep {
  readonly kind: StepKind;
  readonly testId: string;
  readonly text: string;
}

export interface DraftExpect {
  readonly kind: ExpectKind;
  readonly value: string;
  readonly text: string;
}

export interface Draft {
  /** The file the scenario goes into; an existing file gets the scenario appended. */
  readonly file: string;
  readonly name: string;
  readonly page: string;
  readonly steps: readonly DraftStep[];
  readonly expects: readonly DraftExpect[];
}

export const emptyDraft: Draft = {
  file: "",
  name: "",
  page: "",
  steps: [],
  expects: [{ kind: "visible", value: "", text: "" }],
};

export type DraftProblem = "file" | "name" | "page" | "step" | "expect";

/** What keeps a draft from being saved; the schema (`scenario.schema.json`) is the authority, this only mirrors it for early feedback. */
export function problems(d: Draft): readonly DraftProblem[] {
  const found: DraftProblem[] = [];
  if (!FILE_NAME.test(d.file)) {
    found.push("file");
  }
  if (d.name.trim() === "" || d.name.length > 200) {
    found.push("name");
  }
  if (!ROUTE.test(d.page)) {
    found.push("page");
  }
  if (d.steps.some((s) => !TEST_ID.test(s.testId) || (s.kind === "enterText" && s.text === ""))) {
    found.push("step");
  }
  const bad = (e: DraftExpect) =>
    e.kind === "navigatedTo"
      ? !ROUTE.test(e.value)
      : !TEST_ID.test(e.value) || (e.kind === "textEquals" && e.text === "");
  if (d.expects.length === 0 || d.expects.some(bad)) {
    found.push("expect");
  }
  return found;
}

export type Json = Record<string, unknown>;

function stepOf(s: DraftStep): Json {
  switch (s.kind) {
    case "tap":
      return { tap: s.testId };
    case "enterText":
      return { enterText: { testId: s.testId, text: s.text } };
    case "waitFor":
      return { waitFor: { testId: s.testId } };
    default:
      return { scroll: { testId: s.testId, dy: 300 } };
  }
}

function expectOf(e: DraftExpect): Json {
  return e.kind === "textEquals" ? { textEquals: { testId: e.value, text: e.text } } : { [e.kind]: e.value };
}

/** The scenario object of a draft, as the scenarios document holds it. */
export function scenarioOf(d: Draft): Json {
  return {
    name: d.name.trim(),
    page: d.page,
    ...(d.steps.length > 0 && { steps: d.steps.map(stepOf) }),
    expect: d.expects.map(expectOf),
  };
}

/** A new scenarios document holding the draft's scenario. */
export function buildDocument(d: Draft): JsonValue {
  return { schemaVersion: "1.0.0", kind: "scenarios", scenarios: [scenarioOf(d)] } as JsonValue;
}

/** The path of the scenarios file named `file`. */
export const pathOf = (file: string) => `${SCENARIO_DIR}/${file}.scenario.json`;

export const isScenarioPath = (path: string) => SCENARIO_FILE.test(path);

/** The file name (without directory and suffix) of a scenarios path. */
export const fileOf = (path: string) => path.slice(path.lastIndexOf("/") + 1).replace(SCENARIO_FILE, "");

export interface ScenarioRow {
  readonly path: string;
  readonly name: string;
  readonly target: string;
  readonly steps: number;
  readonly expectations: number;
}

const scenarioSchema = z.object({
  name: z.string(),
  page: z.string().optional(),
  flow: z.string().optional(),
  steps: z.array(z.unknown()).catch([]),
  expect: z.array(z.unknown()).catch([]),
});

/** The JSON of a stored document's bytes; undefined when it is not JSON. */
export function parseBytes(content: Uint8Array): unknown {
  try {
    return JSON.parse(new TextDecoder().decode(content));
  } catch {
    return undefined;
  }
}

/** The scenarios of one document's JSON; a malformed document has none. */
export function scenariosOf(path: string, value: unknown): readonly ScenarioRow[] {
  const parsed = z.object({ scenarios: z.array(scenarioSchema) }).safeParse(value);
  if (!parsed.success) {
    return [];
  }
  return parsed.data.scenarios.map((s) => ({
    path,
    name: s.name,
    target: s.page ?? s.flow ?? "",
    steps: s.steps.length,
    expectations: s.expect.length,
  }));
}

/** The rows of `rows` that match the `?q` text in their name, target or file. */
export function filterRows(rows: readonly ScenarioRow[], q: string | undefined): readonly ScenarioRow[] {
  const needle = (q ?? "").trim().toLowerCase();
  if (needle === "") {
    return rows;
  }
  return rows.filter((r) => [r.name, r.target, r.path].some((v) => v.toLowerCase().includes(needle)));
}

type Scenarios = { scenarios?: unknown };

/** The edit recipe that appends a scenario to a document draft. */
export function appendRecipe(scenario: Json): (draft: DocumentDraft) => void {
  return (draft) => {
    const doc = draft as Scenarios;
    doc.scenarios = [...(Array.isArray(doc.scenarios) ? doc.scenarios : []), scenario];
  };
}

/** The edit recipe that removes the scenario named `name`. */
export function removeRecipe(name: string): (draft: DocumentDraft) => void {
  return (draft) => {
    const doc = draft as Scenarios;
    if (Array.isArray(doc.scenarios)) {
      doc.scenarios = (doc.scenarios as { name?: unknown }[]).filter((s) => s.name !== name);
    }
  };
}

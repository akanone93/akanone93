// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { type AttributeCondition, parseAttributes } from "./experiments.schema.ts";

/** A segment document of the app's draft: `segments/<key>.segment.json` (REL-051, ABT-006). */
export interface SegmentDoc {
  readonly path: string;
  readonly revision: bigint;
  readonly id: string;
  readonly key: string;
  readonly name: string;
  readonly description: string;
  readonly rule: unknown;
}

/** What the form edits: the rule is one condition per line, all of which must hold. */
export interface SegmentForm {
  readonly key: string;
  readonly name: string;
  readonly description: string;
  readonly conditions: string;
  /** The document being edited, or undefined for a new segment. */
  readonly existing: SegmentDoc | undefined;
}

/** A segment key: a lower-kebab slug starting with a letter (SCH-002). */
export const segmentKeyPattern = /^[a-z][a-z0-9]*(-[a-z0-9]+)*$/;
const segmentFile = /^segments\/([a-z][a-z0-9]*(?:-[a-z0-9]+)*)\.segment\.json$/;

export const segmentPath = (key: string) => `segments/${key}.segment.json`;

/** The segment key of an app document path, or undefined when the path is not a segment's. */
export const segmentKeyOf = (path: string) => segmentFile.exec(path)?.[1];

/** A segment document read from the draft; anything that is not one is undefined. */
export function readSegment(path: string, revision: bigint, json: unknown): SegmentDoc | undefined {
  if (segmentKeyOf(path) === undefined || typeof json !== "object" || json === null) {
    return undefined;
  }
  const d = json as Record<string, unknown>;
  const text = (v: unknown) => (typeof v === "string" ? v : "");
  return {
    path,
    revision,
    id: text(d["id"]),
    key: text(d["key"]),
    name: text(d["name"]),
    description: text(d["description"]),
    rule: d["rule"],
  };
}

const isCondition = (r: unknown): r is AttributeCondition =>
  typeof r === "object" && r !== null && "attribute" in r && "operator" in r && "value" in r;

const conditionLine = (c: AttributeCondition) =>
  `${c.attribute} ${c.operator} ${Array.isArray(c.value) ? c.value.join(", ") : String(c.value)}`;

/**
 * The form text of a rule: one line per condition for a condition or an `all` of conditions; undefined for any other
 * shape (`any`, nested groups), which the form cannot show without changing its meaning.
 */
export function ruleText(rule: unknown): string | undefined {
  if (isCondition(rule)) {
    return conditionLine(rule);
  }
  const all = typeof rule === "object" && rule !== null ? (rule as { all?: unknown }).all : undefined;
  if (Array.isArray(all) && all.length > 0 && all.every(isCondition)) {
    return all.map(conditionLine).join("\n");
  }
  return undefined;
}

/** The rule of the form's conditions: the one condition, or all of them; undefined when the text is not valid. */
export function ruleOf(conditions: string): unknown {
  const parsed = parseAttributes(conditions);
  if (parsed === undefined || parsed.length === 0) {
    return undefined;
  }
  return parsed.length === 1 ? parsed[0] : { all: parsed };
}

/** The document a valid form saves; it keeps an edited segment's identifier. */
export function segmentDocument(form: SegmentForm, newId: () => string): Record<string, unknown> | undefined {
  const rule = ruleOf(form.conditions);
  if (rule === undefined || !segmentKeyPattern.test(form.key) || form.name.trim() === "") {
    return undefined;
  }
  return {
    schemaVersion: "1.0.0",
    kind: "segment",
    id: form.existing?.id || newId(),
    key: form.key,
    name: form.name.trim(),
    ...(form.description.trim() === "" ? {} : { description: form.description.trim() }),
    rule,
  };
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import {
  buildEnvelope,
  CLIPBOARD_MIME,
  freshNodes,
  hasMissing,
  missingDependencies,
  parseClipboard,
  type TargetHas,
} from "./envelope.ts";

const A = "01890000-0000-7000-8000-00000000000a";
const B = "01890000-0000-7000-8000-00000000000b";
const source = { app: "app1", plugin: "p1" };
const ok = async () => [] as readonly string[];
const nodes = () => [
  {
    id: A,
    type: "Column",
    children: [{ id: B, type: "Text", props: { translationKey: "greet", for: A, token: "x" } }],
    events: { onPressed: { steps: [{ action: "call", function: "fn.sum" }] } },
  },
];

describe("STU-067 STU-055 the clipboard envelope", () => {
  test("copy lists the referenced dependencies and never copies sensitive fields", () => {
    const env = buildEnvelope(source, nodes());
    expect(env.format).toBe(CLIPBOARD_MIME);
    expect(env.dependencies.functions).toEqual(["fn.sum"]);
    expect(env.dependencies.translations).toEqual(["greet"]);
    expect(JSON.stringify(env)).not.toContain('"token"');
  });

  test("a copy round trips through paste", async () => {
    const env = buildEnvelope(source, nodes());
    const r = await parseClipboard(JSON.stringify(env), { validate: ok });
    expect(r.kind).toBe("envelope");
  });

  test("text that is not an envelope is plain text only", async () => {
    expect(await parseClipboard("<b>hi</b>", { validate: ok })).toEqual({ kind: "text", text: "<b>hi</b>" });
    expect((await parseClipboard('{"a":1}', { validate: ok })).kind).toBe("text");
  });

  test("an oversized paste is refused unread", async () => {
    const r = await parseClipboard(JSON.stringify(buildEnvelope(source, nodes())), { validate: ok, maxBytes: 10 });
    expect(r).toEqual({ kind: "refused", reason: "too-large" });
  });

  test("an unknown version is refused", async () => {
    const env = { ...buildEnvelope(source, nodes()), version: 2 };
    expect(await parseClipboard(JSON.stringify(env), { validate: ok })).toEqual({
      kind: "refused",
      reason: "unknown-version",
    });
  });

  test("a malformed envelope is refused", async () => {
    const env = { ...buildEnvelope(source, nodes()), nodes: [{ id: 1 }] };
    expect(await parseClipboard(JSON.stringify(env), { validate: ok })).toEqual({
      kind: "refused",
      reason: "malformed",
    });
  });

  test("nodes the validator rejects are refused with its first message", async () => {
    const r = await parseClipboard(JSON.stringify(buildEnvelope(source, nodes())), {
      validate: async () => ["bad prop"],
    });
    expect(r).toEqual({ kind: "refused", reason: "invalid", detail: "bad prop" });
  });

  test("paste gives every node a new UUIDv7 and remaps internal references", () => {
    const env = buildEnvelope(source, nodes());
    let n = 0;
    const out = freshNodes(env, () => `01890000-0000-7000-8000-0000000001${String(n++).padStart(2, "0")}`) as [
      { id: string; children: [{ id: string; props: { for: string } }] },
    ];
    expect(out[0].id).not.toBe(A);
    expect(out[0].children[0].id).not.toBe(B);
    expect(out[0].children[0].props.for).toBe(out[0].id);
    expect(JSON.stringify(out)).not.toContain(A);
  });

  test("the report lists what the target lacks, and nothing inside the same plugin", () => {
    const env = buildEnvelope(source, nodes());
    const has: TargetHas = {
      functions: new Set(),
      translations: new Set(["greet"]),
      assets: new Set(),
      dataSources: new Set(),
    };
    const across = missingDependencies(env, { app: "app2", plugin: "p9" }, has);
    expect(across.functions).toEqual(["fn.sum"]);
    expect(across.translations).toEqual([]);
    expect(hasMissing(across)).toBe(true);
    expect(hasMissing(missingDependencies(env, source, has))).toBe(false);
  });
});

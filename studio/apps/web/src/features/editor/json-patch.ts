// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Patch } from "immer";

/** A JSON value as a document holds it. */
export type JsonValue = null | boolean | number | string | readonly JsonValue[] | { readonly [key: string]: JsonValue };

/** One RFC 6902 operation; Studio emits only add, remove, replace and test. */
export interface JsonPatchOp {
  readonly op: "add" | "remove" | "replace" | "test";
  readonly path: string;
  readonly value?: JsonValue;
}

/** Why a patch did not apply: a `test` failed (the base moved) or a path does not exist. */
export class PatchError extends Error {
  constructor(
    readonly reason: "test" | "path",
    readonly path: string,
  ) {
    super(`patch ${reason} failed at ${path}`);
    this.name = "PatchError";
  }
}

type Mutable = Record<string, unknown> | unknown[];

function escapeSegment(segment: string | number): string {
  return String(segment).replaceAll("~", "~0").replaceAll("/", "~1");
}

/** Builds an RFC 6901 JSON Pointer from path segments. */
export function pointer(segments: readonly (string | number)[]): string {
  return segments.map((s) => `/${escapeSegment(s)}`).join("");
}

function parsePointer(path: string): string[] {
  if (path === "") {
    return [];
  }
  if (!path.startsWith("/")) {
    throw new PatchError("path", path);
  }
  return path
    .slice(1)
    .split("/")
    .map((s) => s.replaceAll("~1", "/").replaceAll("~0", "~"));
}

function isContainer(value: unknown): value is Mutable {
  return typeof value === "object" && value !== null;
}

/** Structural equality of two JSON values; key order does not matter. */
export function jsonEqual(a: unknown, b: unknown): boolean {
  if (a === b) {
    return true;
  }
  if (Array.isArray(a) || Array.isArray(b)) {
    return Array.isArray(a) && Array.isArray(b) && a.length === b.length && a.every((v, i) => jsonEqual(v, b[i]));
  }
  if (!isContainer(a) || !isContainer(b)) {
    return false;
  }
  const ka = Object.keys(a);
  const kb = Object.keys(b);
  const bo = b as Record<string, unknown>;
  return (
    ka.length === kb.length &&
    ka.every((k) => Object.hasOwn(bo, k) && jsonEqual((a as Record<string, unknown>)[k], bo[k]))
  );
}

function clone<T>(value: T): T {
  return value === undefined ? value : (JSON.parse(JSON.stringify(value)) as T);
}

function get(doc: unknown, segments: readonly string[], path: string): unknown {
  let node = doc;
  for (const s of segments) {
    if (Array.isArray(node)) {
      const i = Number(s);
      if (!/^(0|[1-9]\d*)$/.test(s) || i >= node.length) {
        throw new PatchError("path", path);
      }
      node = node[i];
    } else if (isContainer(node) && Object.hasOwn(node, s)) {
      node = (node as Record<string, unknown>)[s];
    } else {
      throw new PatchError("path", path);
    }
  }
  return node;
}

function index(parent: unknown[], s: string, path: string, allowEnd: boolean): number {
  if (allowEnd && s === "-") {
    return parent.length;
  }
  const i = Number(s);
  if (!/^(0|[1-9]\d*)$/.test(s) || i > parent.length || (!allowEnd && i === parent.length)) {
    throw new PatchError("path", path);
  }
  return i;
}

function applyToArray(parent: unknown[], last: string, op: JsonPatchOp): void {
  const i = index(parent, last, op.path, op.op === "add");
  if (op.op === "add") {
    parent.splice(i, 0, clone(op.value));
  } else if (op.op === "remove") {
    parent.splice(i, 1);
  } else {
    parent[i] = clone(op.value);
  }
}

function applyToObject(obj: Record<string, unknown>, last: string, op: JsonPatchOp, strict: boolean): void {
  if (op.op !== "add" && !Object.hasOwn(obj, last)) {
    throw new PatchError("path", op.path);
  }
  if (strict && op.op === "add" && Object.hasOwn(obj, last) && !jsonEqual(obj[last], op.value)) {
    throw new PatchError("test", op.path);
  }
  if (op.op === "remove") {
    delete obj[last];
  } else {
    obj[last] = clone(op.value);
  }
}

/** Applies one operation to `root`, mutating it, and returns the (possibly new) root. */
function applyOne(root: unknown, op: JsonPatchOp, strict: boolean): unknown {
  const segments = parsePointer(op.path);
  if (op.op === "test") {
    if (!jsonEqual(get(root, segments, op.path), op.value)) {
      throw new PatchError("test", op.path);
    }
    return root;
  }
  if (segments.length === 0) {
    if (op.op === "remove") {
      throw new PatchError("path", op.path);
    }
    return clone(op.value);
  }
  const parent = get(root, segments.slice(0, -1), op.path);
  const last = segments[segments.length - 1] ?? "";
  if (Array.isArray(parent)) {
    applyToArray(parent, last, op);
  } else if (isContainer(parent)) {
    applyToObject(parent as Record<string, unknown>, last, op, strict);
  } else {
    throw new PatchError("path", op.path);
  }
  return root;
}

/**
 * Applies `ops` to a copy of `doc` (RFC 6902 subset) and returns the result; the input is never
 * changed. A failing `test` throws PatchError("test"), so a stale base never applies silently.
 * `strict` (used when replaying on a newer revision) also refuses an `add` that would overwrite a
 * different value someone else put at that key, which a JSON Patch `test` cannot express.
 */
export function applyPatch(
  doc: JsonValue,
  ops: readonly JsonPatchOp[],
  options: { readonly strict?: boolean } = {},
): JsonValue {
  let root: unknown = clone(doc);
  for (const op of ops) {
    root = applyOne(root, op, options.strict ?? false);
  }
  return root as JsonValue;
}

/**
 * Converts Immer patches made against `base` to JSON Patch, putting a `test` before every change
 * (SRV-030, decisions F2): replace and remove test the old value; an insert into an array tests the
 * array, since the indexes after it shift. An add of an object key cannot be guarded by a `test`
 * (JSON Patch cannot test absence): the server's `if_revision` guards it on save, and a replay uses
 * applyPatch's strict mode. Each test is taken from the document as
 * it stands at that point of the patch, so the result applies to `base` and to nothing that differs
 * where the edit reads or writes.
 */
export function toJsonPatch(base: JsonValue, patches: readonly Patch[]): JsonPatchOp[] {
  const out: JsonPatchOp[] = [];
  let working: JsonValue = base;
  for (const p of patches) {
    const path = pointer(p.path);
    const step: JsonPatchOp[] = [];
    if (p.op === "add") {
      const parentPath = pointer(p.path.slice(0, -1));
      const parent = get(working, parsePointer(parentPath), parentPath);
      if (Array.isArray(parent)) {
        step.push({ op: "test", path: parentPath, value: clone(parent as JsonValue) });
      }
      step.push({ op: "add", path, value: clone(p.value as JsonValue) });
    } else {
      step.push({ op: "test", path, value: clone(get(working, parsePointer(path), path) as JsonValue) });
      step.push(
        p.op === "remove" ? { op: "remove", path } : { op: "replace", path, value: clone(p.value as JsonValue) },
      );
    }
    working = applyPatch(working, step);
    out.push(...step);
  }
  return out;
}

/** The paths a patch changes (its non-test operations), for listing what was dropped. */
export function changedPaths(ops: readonly JsonPatchOp[]): string[] {
  return [...new Set(ops.filter((o) => o.op !== "test").map((o) => o.path))];
}

/** Pointers where two documents differ, outermost first, at most `max` of them (the D35 diff). */
export function diffPaths(a: unknown, b: unknown, max: number, at = ""): string[] {
  if (jsonEqual(a, b)) {
    return [];
  }
  const bothArrays = Array.isArray(a) && Array.isArray(b);
  const bothObjects = isContainer(a) && isContainer(b) && !Array.isArray(a) && !Array.isArray(b);
  if (!bothArrays && !bothObjects) {
    return [at === "" ? "/" : at];
  }
  const ao = a as Record<string, unknown>;
  const bo = b as Record<string, unknown>;
  const keys = [...new Set([...Object.keys(ao), ...Object.keys(bo)])].sort();
  const out: string[] = [];
  for (const k of keys) {
    if (out.length >= max) {
      break;
    }
    out.push(...diffPaths(ao[k], bo[k], max - out.length, `${at}/${escapeSegment(k)}`));
  }
  return out;
}

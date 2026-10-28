// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { applyPatches, type Patch } from "immer";
import type { CreateRecord, EditRecord, EditSession } from "./edit-session.ts";
import type { JsonPatchOp, JsonValue } from "./json-patch.ts";

/**
 * Undo and redo (F3, STU-069, ADR-0095). Only document changes are steps. There is one stack per
 * plugin and one for the app's own documents (stack key `""`). A step may span documents and
 * stacks (a cross-plugin change); it is undone atomically or not at all. Undo and redo go through
 * the pipeline's `edit`, `create` and `remove`, so each is an ordinary forward write the server
 * stores like any other.
 */

/** The patches that, applied to one document, undo (or redo) its part of a step. */
interface PatchPart {
  readonly stack: string;
  readonly path: string;
  readonly patches: readonly Patch[];
}

/** A whole document a step created or deleted: undoing (or redoing) it gives the document this content, or deletes it (null). */
interface WholePart {
  readonly stack: string;
  readonly path: string;
  readonly whole: JsonValue | null;
}

type Part = PatchPart | WholePart;

interface Step {
  /** Sent with every write of the step, so the rebuild after a reload regroups it. */
  readonly id: string;
  parts: Part[];
  readonly stacks: ReadonlySet<string>;
  readonly group: string | undefined;
  at: number;
  /** Selection when the step began, restored on undo; and when it ended, restored on redo. */
  readonly selectionBefore: unknown;
  selectionAfter: unknown;
}

interface Stack {
  undo: Step[];
  redo: Step[];
}

/** The outcome of an undo or redo: done, nothing to do, or blocked with the reason. */
export type UndoResult =
  | {
      readonly kind: "done";
      readonly stacks: readonly string[];
      readonly paths: readonly string[];
      /** Settles once the step's whole-document writes (creations, deletions) landed: true when all did. */
      readonly written: Promise<boolean>;
    }
  | { readonly kind: "empty" }
  | { readonly kind: "blocked"; readonly reason: UndoBlock; readonly stack: string };

/** Why a step cannot be applied: its stack is read-only or lost its lock, a later step of another
 * stack sits above it, a document is not open, or a document changed so its patches no longer apply. */
export type UndoBlock = "read-only" | "later-step" | "not-open" | "conflict";

export interface UndoManagerOptions {
  /** Steps kept per stack, from `studio.undoDepth`. */
  readonly depth: number;
  /** Edits of the same group closer than this are one step (a typing burst), F3 "~1 s pause". */
  readonly pauseMs?: number;
  readonly now?: () => number;
  /** New step IDs: opaque and random. */
  readonly newId?: () => string;
  /** Why a stack cannot be changed now (read-only, lock lost), or undefined when it can. */
  readonly blocked?: (stack: string) => "read-only" | undefined;
  /** The current selection, recorded with each step; UI state, never a step of its own. */
  readonly selection?: () => unknown;
  /** Restores the selection of the step undone or redone. */
  readonly restoreSelection?: (selection: unknown) => void;
}

/** One rebuilt entry from the server: the inverse of a write, newest first (ListEditorSessionEntries). */
export interface SessionEntry {
  readonly path: string;
  /** The write's step: a step's ID, `undo:<id>` or `redo:<id>`; empty for an untagged write. */
  readonly step: string;
  readonly inverse: readonly JsonPatchOp[];
}

/** Turns an RFC 6902 pointer into Immer path segments; digit-only segments index arrays. */
export function pointerSegments(pointer: string): (string | number)[] {
  if (pointer === "") {
    return [];
  }
  return pointer
    .slice(1)
    .split("/")
    .map((s) => s.replaceAll("~1", "/").replaceAll("~0", "~"))
    .map((s) => (/^(0|[1-9][0-9]*)$/.test(s) ? Number(s) : s));
}

/** Converts a JSON Patch from the server into Immer patches. */
export function toImmerPatches(ops: readonly JsonPatchOp[]): Patch[] {
  const out: Patch[] = [];
  for (const o of ops) {
    if (o.op === "add" || o.op === "replace" || o.op === "remove") {
      out.push({ op: o.op, path: pointerSegments(o.path), value: o.value });
    }
  }
  return out;
}

interface Run {
  readonly step: string;
  readonly parts: Part[];
}

/** Deletes a document once its pending saves have landed, so the delete never races its own save. */
async function removeSettled(session: EditSession, path: string): Promise<boolean> {
  session.flushAll();
  await new Promise<void>((resolve) => {
    const settled = () => {
      const doc = session.store.getState().docs[path];
      return doc === undefined || doc.waiting === 0 || doc.status === "conflict";
    };
    if (settled()) {
      resolve();
      return;
    }
    const stop = session.store.subscribe(() => {
      if (settled()) {
        stop();
        resolve();
      }
    });
  });
  return (await session.remove(path)).ok;
}

/**
 * What one part makes of a document in the trial run (`null`: absent), or why it cannot apply: a
 * creation needs the document absent, a deletion or a patch needs it open, and a patch must apply.
 */
function trialOf(part: Part, base: unknown): { readonly content: unknown } | "not-open" | "conflict" {
  if ("whole" in part && part.whole !== null) {
    return base === null ? { content: part.whole } : "conflict";
  }
  if (base === null || typeof base !== "object") {
    return "not-open";
  }
  if ("whole" in part) {
    return { content: null };
  }
  try {
    return { content: applyPatches(base as object, part.patches as Patch[]) };
  } catch {
    return "conflict";
  }
}

/** Groups entries (newest first) into runs oldest first: consecutive writes of one step are one run. */
function runsOf(stack: string, entries: readonly SessionEntry[]): Run[] {
  const runs: Run[] = [];
  for (const e of [...entries].reverse()) {
    const part: Part = { stack, path: e.path, patches: toImmerPatches(e.inverse) };
    const last = runs.at(-1);
    if (last !== undefined && e.step !== "" && last.step === e.step) {
      last.parts.push(part);
    } else {
      runs.push({ step: e.step, parts: [part] });
    }
  }
  return runs;
}

export class UndoManager {
  private readonly stacks = new Map<string, Stack>();
  private readonly sessions = new Map<string, EditSession>();
  private readonly now: () => number;
  private readonly pauseMs: number;
  private readonly newId: () => string;
  /** Set while undo or redo applies its own edits, which are not new steps. */
  private applying = false;
  /** Set while a cross-plugin change is made: its edits collect into this one step. */
  private open: Step | null = null;
  private readonly listeners = new Set<() => void>();

  constructor(private readonly options: UndoManagerOptions) {
    this.now = options.now ?? (() => Date.now());
    this.pauseMs = options.pauseMs ?? 1000;
    this.newId = options.newId ?? (() => globalThis.crypto.randomUUID());
  }

  /** Registers the pipeline of a stack (a plugin's EditorProvider, or the app's); returns the detach. */
  attach(stack: string, session: EditSession): () => void {
    this.sessions.set(stack, session);
    return () => {
      if (this.sessions.get(stack) === session) {
        this.sessions.delete(stack);
      }
    };
  }

  /** Re-renders on change, for enabling the commands. */
  subscribe(listener: () => void): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  canUndo(stack: string): boolean {
    return (this.stacks.get(stack)?.undo.length ?? 0) > 0 && this.options.blocked?.(stack) === undefined;
  }

  canRedo(stack: string): boolean {
    return (this.stacks.get(stack)?.redo.length ?? 0) > 0 && this.options.blocked?.(stack) === undefined;
  }

  /**
   * The step an edit is about to join (the pipeline's `stepFor`): the open cross-plugin step, the top
   * step while its group continues without a pause, or a new one.
   */
  stepFor(stack: string, group: string | undefined): string {
    if (this.open !== null) {
      return this.open.id;
    }
    const top = this.stacks.get(stack)?.undo.at(-1);
    if (this.continues(top, group)) {
      return (top as Step).id;
    }
    return this.newId();
  }

  private continues(top: Step | undefined, group: string | undefined): boolean {
    return (
      top !== undefined &&
      group !== undefined &&
      top.group === group &&
      top.stacks.size === 1 &&
      this.now() - top.at < this.pauseMs
    );
  }

  /**
   * Records an edit of the pipeline (its `onEdit`). A new edit clears the stack's redo. An edit
   * tagged with the step on top of another stack joins that step, so writes of one user action
   * across stacks (a key extraction) are one step, as the server regroups them after a reload.
   */
  record(stack: string, record: EditRecord): void {
    if (this.applying) {
      return;
    }
    this.add(stack, { stack, path: record.path, patches: record.inversePatches }, record.step, record.group);
  }

  /**
   * Records a document the pipeline created as part of a step (its `onCreated`): undoing the step
   * deletes the document again, redoing it creates it with the content it had when undone.
   */
  recordCreated(stack: string, record: CreateRecord): void {
    if (this.applying || record.step.startsWith("undo:") || record.step.startsWith("redo:")) {
      return;
    }
    this.add(stack, { stack, path: record.path, whole: null }, record.step, undefined);
  }

  private add(stack: string, part: Part, step: string | undefined, group: string | undefined): void {
    if (this.open !== null) {
      this.open.parts.push(part);
      (this.open.stacks as Set<string>).add(stack);
      return;
    }
    const s = this.stackOf(stack);
    s.redo = [];
    const top = s.undo.at(-1);
    const at = this.now();
    const shared = step === undefined || top?.id === step ? undefined : this.sharedTop(stack, step);
    if (top !== undefined && (step === undefined ? this.continues(top, group) : top.id === step)) {
      top.parts.push(part);
      top.at = at;
      top.selectionAfter = this.options.selection?.();
    } else if (shared !== undefined) {
      shared.parts.push(part);
      (shared.stacks as Set<string>).add(stack);
      shared.at = at;
      shared.selectionAfter = this.options.selection?.();
      this.push(s, shared);
    } else {
      const selection = this.options.selection?.();
      this.push(s, {
        id: step ?? this.newId(),
        parts: [part],
        stacks: new Set([stack]),
        group,
        at,
        selectionBefore: selection,
        selectionAfter: selection,
      });
    }
    this.changed();
  }

  /** The step on top of another stack with this ID, which an edit of the same step joins. */
  private sharedTop(stack: string, id: string): Step | undefined {
    for (const [key, other] of this.stacks) {
      const top = other.undo.at(-1);
      if (key !== stack && top?.id === id) {
        return top;
      }
    }
    return undefined;
  }

  /**
   * Makes every edit inside `change` one step, across documents and stacks (paste, wrap, move a
   * screen to another plugin, rename updating links).
   */
  step(change: () => void): void {
    const selection = this.options.selection?.();
    const step: Step = {
      id: this.newId(),
      parts: [],
      stacks: new Set(),
      group: undefined,
      at: this.now(),
      selectionBefore: selection,
      selectionAfter: selection,
    };
    const outer = this.open;
    this.open = outer ?? step;
    try {
      change();
    } finally {
      this.open = outer;
    }
    if (outer !== null || step.parts.length === 0) {
      return;
    }
    step.selectionAfter = this.options.selection?.();
    for (const key of step.stacks) {
      const s = this.stackOf(key);
      s.redo = [];
      this.push(s, step);
    }
    this.changed();
  }

  /** Ends the current group (a blur, the end of a drag): the next edit starts a new step. */
  seal(stack: string): void {
    const top = this.stacks.get(stack)?.undo.at(-1);
    if (top !== undefined) {
      top.at = Number.NEGATIVE_INFINITY;
    }
  }

  undo(stack: string): UndoResult {
    return this.move(stack, "undo");
  }

  redo(stack: string): UndoResult {
    return this.move(stack, "redo");
  }

  /**
   * Rebuilds a stack after a reload from the editor session's entries on the server, newest first,
   * replaying them in order: a step's writes regroup by their step ID, `undo:` and `redo:` writes move
   * it between the stacks, and a step whose ID another stack also holds becomes one cross-plugin
   * step again. A stack that already holds steps is left as it is.
   */
  rebuild(stack: string, entries: readonly SessionEntry[]): void {
    const s = this.stackOf(stack);
    if (s.undo.length > 0 || s.redo.length > 0) {
      return;
    }
    for (const run of runsOf(stack, entries)) {
      this.replay(s, stack, run);
    }
    this.regroup(stack);
    this.changed();
  }

  /** Replays one rebuilt run: an edit pushes a step, `undo:` moves it to redo, `redo:` moves it back. */
  private replay(s: Stack, stack: string, run: Run): void {
    const kind = run.step.startsWith("undo:") ? "undo" : run.step.startsWith("redo:") ? "redo" : "edit";
    const id = kind !== "edit" ? run.step.slice(5) : run.step === "" ? this.newId() : run.step;
    const step: Step = {
      id,
      parts: run.parts,
      stacks: new Set([stack]),
      group: undefined,
      at: Number.NEGATIVE_INFINITY,
      selectionBefore: undefined,
      selectionAfter: undefined,
    };
    if (kind === "undo") {
      if (s.undo.at(-1)?.id === id) {
        s.undo.pop();
      }
      s.redo.push(step);
      return;
    }
    if (kind === "redo" && s.redo.at(-1)?.id === id) {
      s.redo.pop();
    } else if (kind === "edit") {
      s.redo = [];
    }
    this.push(s, step);
  }

  /** Merges the steps of a rebuilt stack with the same steps of the other stacks. */
  private regroup(stack: string): void {
    const s = this.stackOf(stack);
    for (const list of [s.undo, s.redo] as const) {
      list.forEach((step, i) => {
        for (const [key, other] of this.stacks) {
          if (key === stack) {
            continue;
          }
          const shared = [...other.undo, ...other.redo].find((o) => o.id === step.id);
          if (shared !== undefined) {
            shared.parts.push(...step.parts);
            (shared.stacks as Set<string>).add(stack);
            list[i] = shared;
            return;
          }
        }
      });
    }
  }

  private move(stack: string, from: "undo" | "redo"): UndoResult {
    const to = from === "undo" ? "redo" : "undo";
    const step = this.stacks.get(stack)?.[from].at(-1);
    if (step === undefined) {
      return { kind: "empty" };
    }
    const reason = this.check(step, from);
    if (reason !== undefined) {
      return { kind: "blocked", ...reason };
    }
    const done: Part[] = [];
    const writes: Promise<boolean>[] = [];
    this.applying = true;
    try {
      // Undone in reverse order of the edits, so later edits of a document are reverted first.
      for (const part of [...step.parts].reverse()) {
        const inverse = this.apply(part, `${from}:${step.id}`, writes);
        if (inverse !== undefined) {
          done.push(inverse);
        }
      }
    } finally {
      this.applying = false;
    }
    for (const key of step.stacks) {
      const s = this.stackOf(key);
      s[from].pop();
    }
    const moved: Step = { ...step, parts: done, at: Number.NEGATIVE_INFINITY };
    for (const key of step.stacks) {
      this.stackOf(key)[to].push(moved);
    }
    this.options.restoreSelection?.(from === "undo" ? step.selectionBefore : step.selectionAfter);
    this.changed();
    return {
      kind: "done",
      stacks: [...step.stacks],
      paths: [...new Set(step.parts.map((p) => p.path))],
      written: Promise.all(writes).then((ok) => ok.every(Boolean)),
    };
  }

  /**
   * Applies one part of a step and returns its inverse. A whole-document part creates or deletes the
   * document; that write settles later and joins `writes`.
   */
  private apply(part: Part, tag: string, writes: Promise<boolean>[]): Part | undefined {
    const session = this.sessions.get(part.stack);
    if (session === undefined) {
      return undefined;
    }
    if ("whole" in part) {
      const before = session.store.getState().docs[part.path]?.content ?? null;
      writes.push(
        part.whole === null
          ? removeSettled(session, part.path)
          : session.create(part.path, part.whole, { step: tag }).then((w) => w.ok),
      );
      return { stack: part.stack, path: part.path, whole: before };
    }
    const record = session.edit(
      part.path,
      (draft) => {
        applyPatches(draft, part.patches as Patch[]);
      },
      { step: tag },
    );
    return record === undefined ? undefined : { stack: part.stack, path: part.path, patches: record.inversePatches };
  }

  /** Checks that every part of a step can be applied before any is: atomically or not at all. */
  private check(step: Step, from: "undo" | "redo"): { reason: UndoBlock; stack: string } | undefined {
    for (const key of step.stacks) {
      if (this.options.blocked?.(key) !== undefined) {
        return { reason: "read-only", stack: key };
      }
      if (this.stacks.get(key)?.[from].at(-1) !== step) {
        return { reason: "later-step", stack: key };
      }
    }
    // Trial run on copies: every document is open (or, to be created, absent) and every part still applies.
    const trial = new Map<string, unknown>();
    for (const part of [...step.parts].reverse()) {
      const key = `${part.stack}\u0000${part.path}`;
      const base = trial.has(key)
        ? trial.get(key)
        : (this.sessions.get(part.stack)?.store.getState().docs[part.path]?.content ?? null);
      const next = trialOf(part, base);
      if (typeof next === "string") {
        return { reason: next, stack: part.stack };
      }
      trial.set(key, next.content);
    }
    return undefined;
  }

  private push(s: Stack, step: Step): void {
    s.undo.push(step);
    if (s.undo.length > this.options.depth) {
      s.undo.splice(0, s.undo.length - this.options.depth);
    }
  }

  private stackOf(key: string): Stack {
    let s = this.stacks.get(key);
    if (s === undefined) {
      s = { undo: [], redo: [] };
      this.stacks.set(key, s);
    }
    return s;
  }

  private changed(): void {
    for (const l of this.listeners) {
      l();
    }
  }
}

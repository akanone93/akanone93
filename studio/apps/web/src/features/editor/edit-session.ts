// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { enablePatches, type Patch, produceWithPatches } from "immer";
import { createStore, type StoreApi } from "zustand/vanilla";
import { applyPatch, changedPaths, diffPaths, type JsonPatchOp, type JsonValue, toJsonPatch } from "./json-patch.ts";

/** A document as the server confirmed it. */
export interface ServerDocument {
  readonly path: string;
  readonly revision: bigint;
  readonly content: JsonValue;
}

/** How a failed save is handled: a revision conflict (F2), a transient outage (F4) or a rejection of its content. */
export type SaveFailure = "conflict" | "transient" | "rejected";

/** The server calls the pipeline makes; the provider binds them to DocumentService, tests stub them. */
export interface DocumentTransport {
  patch(
    path: string,
    ops: readonly JsonPatchOp[],
    ifRevision: bigint,
    options: { readonly keepalive: boolean },
  ): Promise<ServerDocument>;
  get(path: string): Promise<ServerDocument>;
  classify(error: unknown): SaveFailure;
}

/** Timers, injected so tests control time. */
export interface Scheduler {
  setTimeout(fn: () => void, ms: number): unknown;
  clearTimeout(handle: unknown): void;
}

export interface EditSessionOptions {
  readonly transport: DocumentTransport;
  /** Quiet time after the last edit before a save, from `studio.autosaveDebounceMs`. */
  readonly debounceMs: number;
  /** Automatic replays in a row after conflicts before asking, from `studio.autosaveReplays`. */
  readonly maxReplays: number;
  readonly scheduler?: Scheduler;
  readonly online?: () => boolean;
  /** A save was confirmed: update the server-confirmed data (the Query cache). */
  readonly onConfirmed?: (doc: ServerDocument) => void;
  /** The first edit of a document in this session; S2.3 acquires the lock here. */
  readonly onFirstEdit?: (path: string) => void;
  /** Pending edits now sit on a newer server revision; canvas and undo stack follow it. */
  readonly onRebased?: (doc: ServerDocument) => void;
  /** The server rejected a save for its content; those edits, and any built on them, were undone (F4). */
  readonly onRejected?: (path: string, error: unknown, undone: readonly string[]) => void;
  /** Every edit with its forward and inverse Immer patches, for the S2.4 undo stack. */
  readonly onEdit?: (record: EditRecord) => void;
}

/** A document as an edit recipe sees it: the mutable Immer draft of its root JSON object. */
export type DocumentDraft = Record<string, unknown>;

function isObject(value: JsonValue): value is { readonly [key: string]: JsonValue } {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

export interface EditRecord {
  readonly path: string;
  readonly patches: readonly Patch[];
  readonly inversePatches: readonly Patch[];
  readonly group: string | undefined;
}

export type DocStatus = "saved" | "pending" | "saving" | "waiting" | "conflict";

export interface DocState {
  readonly content: JsonValue;
  readonly revision: bigint;
  /** Edits not yet confirmed by the server, including those in flight. */
  readonly waiting: number;
  readonly status: DocStatus;
}

/** An unresolved conflict (D35): the user's version, the server's, and what keeping mine would drop. */
export interface Conflict {
  readonly path: string;
  readonly mine: JsonValue;
  readonly server: ServerDocument;
  readonly differences: readonly string[];
  readonly wouldDrop: readonly string[];
}

export interface EditState {
  readonly docs: Readonly<Record<string, DocState>>;
  readonly conflict: Conflict | null;
}

interface Unit {
  ops: JsonPatchOp[];
  readonly group: string | undefined;
}

interface Track {
  base: ServerDocument;
  local: JsonValue;
  queue: Unit[];
  inFlight: Unit[] | null;
  timer: unknown;
  replays: number;
  retries: number;
  paused: boolean;
  edited: boolean;
}

const diffLimit = 50;
// Backoff for an unreachable server (decisions F4): doubling from 1 s, at most every 30 s, with jitter.
const backoffStartMs = 1000;
const backoffMaxMs = 30_000;

const realScheduler: Scheduler = {
  setTimeout: (fn, ms) => globalThis.setTimeout(fn, ms),
  clearTimeout: (h) => globalThis.clearTimeout(h as ReturnType<typeof setTimeout>),
};

function fits(base: JsonValue, units: readonly Unit[]): { content: JsonValue; kept: Unit[]; dropped: Unit[] } {
  let content = base;
  const kept: Unit[] = [];
  const dropped: Unit[] = [];
  for (const u of units) {
    try {
      content = applyPatch(content, u.ops, { strict: true });
      kept.push(u);
    } catch {
      dropped.push(u);
    }
  }
  return { content, kept, dropped };
}

/**
 * The edit pipeline of one open plugin document set (SRV-030, decisions F2 and F4). Every edit is an
 * Immer recipe; its patches become JSON Patch guarded by `test` operations and are saved with the
 * revision they apply to. One save is in flight per document; edits made meanwhile queue and go
 * together with the confirmed revision. A conflict replays the pending edits on the server's version
 * while their tests hold, otherwise it pauses autosave and asks (D35). No edit is dropped silently.
 */
export class EditSession {
  readonly store: StoreApi<EditState>;
  private readonly tracks = new Map<string, Track>();
  private readonly scheduler: Scheduler;
  private readonly online: () => boolean;

  constructor(private readonly options: EditSessionOptions) {
    enablePatches();
    this.scheduler = options.scheduler ?? realScheduler;
    this.online = options.online ?? (() => globalThis.navigator?.onLine ?? true);
    this.store = createStore<EditState>(() => ({ docs: {}, conflict: null }));
  }

  /** Opens a document, or refreshes it from the server when nothing of it is pending. */
  open(doc: ServerDocument): void {
    const t = this.tracks.get(doc.path);
    if (t !== undefined && (t.queue.length > 0 || t.inFlight !== null || t.paused)) {
      return;
    }
    this.tracks.set(doc.path, {
      base: doc,
      local: doc.content,
      queue: [],
      inFlight: null,
      timer: t?.timer,
      replays: 0,
      retries: 0,
      paused: false,
      edited: t?.edited ?? false,
    });
    this.publish(doc.path);
  }

  /**
   * Edits a document with an Immer recipe and schedules its save. Returns the forward and inverse
   * patches (for undo), or undefined when the document is not open or the recipe changed nothing.
   * Edits sharing a `group` with the previous edit coalesce into one unit.
   */
  edit(
    path: string,
    recipe: (draft: DocumentDraft) => void,
    options: { readonly group?: string } = {},
  ): EditRecord | undefined {
    const t = this.tracks.get(path);
    if (t === undefined || !isObject(t.local)) {
      return undefined;
    }
    const [next, patches, inversePatches] = produceWithPatches<DocumentDraft>(t.local, recipe);
    if (patches.length === 0) {
      return undefined;
    }
    const ops = toJsonPatch(t.local, patches);
    t.local = next as JsonValue;
    const last = t.queue[t.queue.length - 1];
    if (options.group !== undefined && last !== undefined && last.group === options.group) {
      last.ops = [...last.ops, ...ops];
    } else {
      t.queue.push({ ops, group: options.group });
    }
    if (!t.edited) {
      t.edited = true;
      this.options.onFirstEdit?.(path);
    }
    const record = { path, patches, inversePatches, group: options.group };
    this.options.onEdit?.(record);
    this.schedule(path, t, this.options.debounceMs);
    this.publish(path);
    return record;
  }

  /** Sends every pending document now; `keepalive` lets the request outlive a hidden tab. */
  flushAll(keepalive = false): void {
    for (const path of this.tracks.keys()) {
      void this.flush(path, keepalive);
    }
  }

  /** Total edits not yet confirmed, for the "N changes waiting" chip (F4). */
  waiting(): number {
    let n = 0;
    for (const t of this.tracks.values()) {
      n += t.queue.length + (t.inFlight?.length ?? 0);
    }
    return n;
  }

  /** Flushes on tab hide (with keepalive) and on reconnect; returns the detach function. */
  attach(env: {
    readonly window: EventTarget;
    readonly document: EventTarget & { readonly visibilityState: string };
  }): () => void {
    const onVisibility = () => {
      if (env.document.visibilityState === "hidden") {
        this.flushAll(true);
      }
    };
    const onOnline = () => this.flushAll();
    env.document.addEventListener("visibilitychange", onVisibility);
    env.window.addEventListener("online", onOnline);
    return () => {
      env.document.removeEventListener("visibilitychange", onVisibility);
      env.window.removeEventListener("online", onOnline);
    };
  }

  /** D35 "keep mine": applies the edits that still fit on the server's version and resumes; returns what was dropped. */
  keepMine(): readonly string[] {
    const c = this.store.getState().conflict;
    const t = c === null ? undefined : this.tracks.get(c.path);
    if (c === null || t === undefined) {
      return [];
    }
    const { content, kept, dropped } = fits(c.server.content, t.queue);
    this.rebase(t, c.server, content, kept);
    this.store.setState({ conflict: null });
    void this.flush(c.path, false);
    return changedPaths(dropped.flatMap((u) => u.ops));
  }

  /** D35 "take the server's": discards the pending edits for the server's version. */
  takeServer(): void {
    const c = this.store.getState().conflict;
    const t = c === null ? undefined : this.tracks.get(c.path);
    if (c === null || t === undefined) {
      return;
    }
    this.rebase(t, c.server, c.server.content, []);
    this.store.setState({ conflict: null });
    this.publish(c.path);
  }

  /** Stops the timers; pending edits stay in memory (never on disk, F4). */
  dispose(): void {
    for (const t of this.tracks.values()) {
      this.clearTimer(t);
    }
  }

  private rebase(t: Track, server: ServerDocument, content: JsonValue, queue: Unit[]): void {
    t.base = server;
    t.local = content;
    t.queue = queue;
    t.inFlight = null;
    t.paused = false;
    t.replays = 0;
    this.options.onRebased?.(server);
    this.publish(server.path);
  }

  private schedule(path: string, t: Track, ms: number): void {
    this.clearTimer(t);
    t.timer = this.scheduler.setTimeout(() => {
      t.timer = undefined;
      void this.flush(path, false);
    }, ms);
  }

  private clearTimer(t: Track): void {
    if (t.timer !== undefined) {
      this.scheduler.clearTimeout(t.timer);
      t.timer = undefined;
    }
  }

  private async flush(path: string, keepalive: boolean): Promise<void> {
    const t = this.tracks.get(path);
    if (t === undefined || t.inFlight !== null || t.paused || t.queue.length === 0) {
      return;
    }
    if (!this.online()) {
      this.publish(path);
      return;
    }
    this.clearTimer(t);
    const units = t.queue;
    t.queue = [];
    t.inFlight = units;
    this.publish(path);
    try {
      const doc = await this.options.transport.patch(
        path,
        units.flatMap((u) => u.ops),
        t.base.revision,
        { keepalive },
      );
      t.base = doc;
      t.inFlight = null;
      t.replays = 0;
      t.retries = 0;
      this.options.onConfirmed?.(doc);
      this.publish(path);
      if (t.queue.length > 0 && t.timer === undefined) {
        await this.flush(path, keepalive);
      }
    } catch (error) {
      await this.failed(path, t, units, error);
    }
  }

  private async failed(path: string, t: Track, units: Unit[], error: unknown): Promise<void> {
    const kind = this.options.transport.classify(error);
    if (kind === "transient") {
      this.retryLater(path, t, units);
      return;
    }
    if (kind === "rejected") {
      t.inFlight = null;
      const { content, kept, dropped } = fits(t.base.content, t.queue);
      t.local = content;
      t.queue = kept;
      this.options.onRejected?.(path, error, changedPaths([...units, ...dropped].flatMap((u) => u.ops)));
      this.publish(path);
      return;
    }
    let server: ServerDocument;
    try {
      server = await this.options.transport.get(path);
    } catch {
      this.retryLater(path, t, units);
      return;
    }
    const all = [...units, ...t.queue];
    t.replays += 1;
    const { content, dropped } = fits(server.content, all);
    if (dropped.length === 0 && t.replays <= this.options.maxReplays) {
      const replays = t.replays;
      this.rebase(t, server, content, all);
      t.replays = replays;
      await this.flush(path, false);
      return;
    }
    t.queue = all;
    t.inFlight = null;
    t.paused = true;
    const wouldDrop = changedPaths(dropped.flatMap((u) => u.ops));
    this.store.setState({
      conflict: { path, mine: t.local, server, differences: diffPaths(t.local, server.content, diffLimit), wouldDrop },
    });
    this.publish(path);
  }

  /** Puts the unsent edits back in front and retries with backoff while online; `online` resends otherwise. */
  private retryLater(path: string, t: Track, units: Unit[]): void {
    t.queue = [...units, ...t.queue];
    t.inFlight = null;
    if (this.online()) {
      const ceiling = Math.min(backoffMaxMs, backoffStartMs * 2 ** t.retries);
      t.retries += 1;
      this.schedule(path, t, ceiling / 2 + Math.random() * (ceiling / 2));
    }
    this.publish(path);
  }

  private publish(path: string): void {
    const t = this.tracks.get(path);
    if (t === undefined) {
      return;
    }
    const waiting = t.queue.length + (t.inFlight?.length ?? 0);
    let status: DocStatus = "saved";
    if (t.paused) {
      status = "conflict";
    } else if (t.inFlight !== null) {
      status = "saving";
    } else if (waiting > 0) {
      status = this.online() ? "pending" : "waiting";
    }
    const doc: DocState = { content: t.local, revision: t.base.revision, waiting, status };
    this.store.setState((s) => ({ docs: { ...s.docs, [path]: doc } }));
  }
}

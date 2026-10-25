// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createStore, type StoreApi } from "zustand/vanilla";
import type { Scheduler } from "./edit-session.ts";

/**
 * Lock timing fixed by SRV-040 (the server's document.LockHeartbeat and LockTTL): a heartbeat
 * every 30 s, expiry two minutes after the last one. The lease lapses after two missed heartbeats (L-1).
 */
export const LOCK_HEARTBEAT_MS = 30_000;
export const LOCK_TTL_MS = 120_000;
export const LOCK_LEASE_LAPSE_MS = 2 * LOCK_HEARTBEAT_MS;

/** A plugin's lock as Studio shows it. */
export interface LockInfo {
  readonly holder: string;
  readonly session: string;
  readonly acquiredAt?: Date;
  readonly expiresAt?: Date;
  readonly lastEditAt?: Date;
  readonly takeoverAllowedAt?: Date;
  readonly requestedBy: readonly string[];
}

/** The PluginService lock calls; the provider binds them, tests stub them. */
export interface LockTransport {
  acquire(session: string, force: boolean): Promise<{ readonly lock: LockInfo; readonly preservedSnapshotId: string }>;
  renew(session: string): Promise<LockInfo>;
  release(session: string, options: { readonly keepalive: boolean }): Promise<void>;
  request(): Promise<LockInfo>;
  /** The current lock, or null when nobody holds it. */
  get(): Promise<LockInfo | null>;
  /** Outages are retried; anything else is the server refusing. */
  classify(error: unknown): "transient" | "refused";
}

/** A repeating tick; in the browser it runs in a worker, whose timers are not throttled in background tabs (F1.2). */
export interface Ticker {
  every(ms: number, fn: () => void): () => void;
}

/** Messages between the tabs of one person editing the same plugin (F1.3). */
export type TabMessage =
  | { readonly type: "held"; readonly session: string }
  | { readonly type: "released"; readonly session: string }
  | { readonly type: "query" };

/** A BroadcastChannel, narrowed; tests connect two controllers through a stub. */
export interface TabChannel {
  post(message: TabMessage): void;
  listen(fn: (message: TabMessage) => void): () => void;
}

/**
 * free: nobody holds it (or nobody we know of); acquiring: the first edit waits for AcquireLock;
 * held: this tab edits; other-tab: another tab of this person edits; held-by-other: read-only;
 * lost: this tab held it and no longer does (D37).
 */
export type LockStatus = "free" | "acquiring" | "held" | "other-tab" | "held-by-other" | "lost";

export interface LockState {
  readonly status: LockStatus;
  /** The lock as last seen: ours when held, the other holder's otherwise. */
  readonly lock: LockInfo | null;
  /** Why the lock was lost: taken over by someone, or expired after failed renewals. */
  readonly lost: "takeover" | "expired" | null;
  /** "Re-take and apply" found the lock taken. */
  readonly retakeFailed: boolean;
  /** "Take over" was refused: too early (L-1) or no plugin.lock.override. */
  readonly takeoverRefused: boolean;
  /** Users who asked this holder for the lock, not yet answered. */
  readonly requests: readonly string[];
  /** "Request edit" reached the holder. */
  readonly requested: boolean;
}

export interface LockControllerOptions {
  readonly transport: LockTransport;
  /** This tab's editing session. */
  readonly session: string;
  /** Minutes without edits before the lock is released, from `studio.lockIdleReleaseMinutes`. */
  readonly idleReleaseMs: number;
  /** From `studio.lockTakeoverIdleMinutes`, to mirror the server's takeover rule (L-1). */
  readonly takeoverIdleMs: number;
  readonly heartbeatMs?: number;
  readonly scheduler: Scheduler;
  readonly ticker: Ticker;
  readonly channel?: TabChannel;
  readonly now?: () => number;
  /** Flushes autosave before an idle release (F1.2). */
  readonly beforeRelease?: () => Promise<void>;
}

/** Rejects the saves of a tab that cannot hold the lock, or whose unsent changes were discarded. */
export class LockUnavailable extends Error {
  constructor() {
    super("the editing lock is held elsewhere");
    this.name = "LockUnavailable";
  }
}

/**
 * When another user may take a lock over (L-1, mirroring the server's document.Lock.TakeoverRule):
 * once the holder made no edit for the takeover idle time, or at once when the lease has lapsed.
 */
export function takeoverAllowedAt(lock: LockInfo, takeoverIdleMs: number): Date | undefined {
  if (lock.takeoverAllowedAt !== undefined) {
    return lock.takeoverAllowedAt;
  }
  if (lock.lastEditAt === undefined || lock.expiresAt === undefined) {
    return undefined;
  }
  const idle = lock.lastEditAt.getTime() + takeoverIdleMs;
  const lapsed = lock.expiresAt.getTime() - LOCK_TTL_MS + LOCK_LEASE_LAPSE_MS;
  return new Date(Math.min(idle, lapsed));
}

interface Waiter {
  resolve(session: string): void;
  reject(error: Error): void;
}

const initial: LockState = {
  status: "free",
  lock: null,
  lost: null,
  retakeFailed: false,
  takeoverRefused: false,
  requests: [],
  requested: false,
};

/**
 * The editing lock of one open plugin (F1, SRV-040–SRV-042): taken on the first edit, renewed by a
 * heartbeat, released when idle or when the tab closes, shared by one person's tabs.
 */
export class LockController {
  readonly store: StoreApi<LockState>;
  private readonly heartbeatMs: number;
  private readonly now: () => number;
  private waiters: Waiter[] = [];
  private acquiring: Promise<void> | null = null;
  private stopTicks: (() => void) | null = null;
  private idleTimer: unknown;
  private lastRenewed = 0;
  private dismissed = new Set<string>();
  private readonly unlisten: () => void;

  constructor(private readonly options: LockControllerOptions) {
    this.heartbeatMs = options.heartbeatMs ?? LOCK_HEARTBEAT_MS;
    this.now = options.now ?? Date.now;
    this.store = createStore<LockState>(() => initial);
    this.unlisten = options.channel?.listen((m) => this.onTab(m)) ?? (() => undefined);
    options.channel?.post({ type: "query" });
  }

  /** Reads who holds the lock, for the read-only banner of a plugin opened while someone edits it. */
  async refresh(): Promise<void> {
    const s = this.store.getState().status;
    if (s === "held" || s === "acquiring" || s === "lost" || s === "other-tab") {
      return;
    }
    try {
      const lock = await this.options.transport.get();
      this.setOther(lock);
    } catch {
      // A later heartbeat or edit reads it again.
    }
  }

  /** The first edit: takes the lock; the edit's save waits for it (F1.1). */
  ensure(): Promise<void> {
    const s = this.store.getState().status;
    if (s === "held" || s === "lost") {
      return Promise.resolve();
    }
    if (s === "other-tab") {
      return Promise.reject(new LockUnavailable());
    }
    this.acquiring ??= this.acquire(false).finally(() => {
      this.acquiring = null;
    });
    return this.acquiring;
  }

  /**
   * The session a save is sent under. It waits while the lock is being taken or was lost (the
   * unsent changes stay in memory, F1.4) and rejects when another tab or person holds it.
   */
  async whenHeld(): Promise<string> {
    const s = this.store.getState().status;
    if (s === "held") {
      this.touch();
      return this.options.session;
    }
    if (s === "free" || s === "held-by-other") {
      await this.ensure().catch(() => undefined);
      if (this.store.getState().status === "held") {
        this.touch();
        return this.options.session;
      }
      if (this.store.getState().status !== "lost") {
        throw new LockUnavailable();
      }
    }
    if (this.store.getState().status === "other-tab") {
      throw new LockUnavailable();
    }
    return new Promise<string>((resolve, reject) => {
      this.waiters.push({ resolve, reject });
    });
  }

  /** A save was refused under the lock: check it now rather than at the next heartbeat. */
  suspect(): void {
    if (this.store.getState().status === "held") {
      void this.beat();
    }
  }

  /** D37 "re-take and apply": only if the lock is free; the revision check is the server's (D35 on a conflict). */
  async retake(): Promise<void> {
    try {
      const { lock } = await this.options.transport.acquire(this.options.session, false);
      this.becomeHeld(lock);
    } catch {
      this.store.setState({ retakeFailed: true });
    }
  }

  /** D37 "discard": drops the unsent changes (the pipeline undoes them) and turns read-only. */
  async discard(): Promise<void> {
    this.rejectWaiters();
    this.store.setState({ ...initial });
    await this.refresh();
  }

  /** D36 "Take over", enabled only per L-1; the server enforces the same rule and the permission. */
  async takeOver(): Promise<string | undefined> {
    try {
      const { lock, preservedSnapshotId } = await this.options.transport.acquire(this.options.session, true);
      this.becomeHeld(lock);
      return preservedSnapshotId;
    } catch {
      this.store.setState({ takeoverRefused: true });
      await this.refresh();
      return undefined;
    }
  }

  /** D36 "Request edit": the holder is told on their next heartbeat (SRV-041). */
  async request(): Promise<void> {
    try {
      const lock = await this.options.transport.request();
      this.store.setState({ lock, requested: true });
    } catch {
      await this.refresh();
    }
  }

  /** Whether "Take over" is enabled now (L-1). */
  canTakeOver(): boolean {
    const { status, lock } = this.store.getState();
    if (status !== "held-by-other" || lock === null) {
      return false;
    }
    const at = takeoverAllowedAt(lock, this.options.takeoverIdleMs);
    return at !== undefined && this.now() >= at.getTime();
  }

  /** The holder keeps the lock despite the requests shown. */
  keep(): void {
    for (const r of this.store.getState().requests) {
      this.dismissed.add(r);
    }
    this.store.setState({ requests: [] });
  }

  /** Gives the lock up: on request, when idle, or when the tab closes (`keepalive`). */
  async release(keepalive = false): Promise<void> {
    if (this.store.getState().status !== "held") {
      return;
    }
    this.stop();
    this.store.setState({ ...initial });
    this.options.channel?.post({ type: "released", session: this.options.session });
    try {
      await this.options.transport.release(this.options.session, { keepalive });
    } catch {
      // The lock expires on its own two minutes after the last heartbeat (SRV-040).
    }
  }

  /** Releases on tab close; returns the detach function. */
  attach(target: EventTarget): () => void {
    const onHide = () => void this.release(true);
    target.addEventListener("pagehide", onHide);
    return () => target.removeEventListener("pagehide", onHide);
  }

  dispose(): void {
    this.stop();
    this.unlisten();
  }

  private async acquire(force: boolean): Promise<void> {
    this.store.setState({ status: "acquiring" });
    try {
      const { lock } = await this.options.transport.acquire(this.options.session, force);
      this.becomeHeld(lock);
    } catch (error) {
      if (this.options.transport.classify(error) === "transient") {
        this.store.setState({ status: "free" });
        this.rejectWaiters(error instanceof Error ? error : new LockUnavailable());
        throw error;
      }
      const lock = await this.options.transport.get().catch(() => null);
      this.setOther(lock);
      this.rejectWaiters();
      throw new LockUnavailable();
    }
  }

  private becomeHeld(lock: LockInfo): void {
    this.lastRenewed = this.now();
    this.store.setState({ ...initial, status: "held", lock });
    this.options.channel?.post({ type: "held", session: this.options.session });
    this.stop();
    this.stopTicks = this.options.ticker.every(this.heartbeatMs, () => void this.beat());
    this.touch();
    const waiters = this.waiters;
    this.waiters = [];
    for (const w of waiters) {
      w.resolve(this.options.session);
    }
  }

  private async beat(): Promise<void> {
    const state = this.store.getState();
    if (state.status === "held-by-other" || state.status === "free") {
      await this.refresh();
      return;
    }
    if (state.status !== "held") {
      return;
    }
    try {
      const lock = await this.options.transport.renew(this.options.session);
      if (this.store.getState().status !== "held") {
        return;
      }
      this.lastRenewed = this.now();
      const requests = lock.requestedBy.filter((r) => !this.dismissed.has(r));
      this.store.setState({ lock, requests });
    } catch (error) {
      if (this.options.transport.classify(error) === "transient") {
        if (this.now() - this.lastRenewed >= LOCK_TTL_MS) {
          this.lose("expired");
        }
        return;
      }
      const other = await this.options.transport.get().catch(() => null);
      this.lose(other !== null && other.session !== this.options.session ? "takeover" : "expired", other);
    }
  }

  private lose(reason: "takeover" | "expired", other: LockInfo | null = null): void {
    this.stop();
    this.store.setState({ ...initial, status: "lost", lost: reason, lock: other });
    this.options.channel?.post({ type: "released", session: this.options.session });
    // A ticker keeps reading the lock so D37 knows whether re-taking can work.
    this.stopTicks = this.options.ticker.every(this.heartbeatMs, () => {
      void this.options.transport
        .get()
        .then((lock) => {
          if (this.store.getState().status === "lost") {
            this.store.setState({ lock });
          }
        })
        .catch(() => undefined);
    });
  }

  private setOther(lock: LockInfo | null): void {
    if (lock === null) {
      this.store.setState({ ...initial });
    } else {
      this.store.setState({ status: "held-by-other", lock });
    }
    // Viewers re-read the lock every heartbeat, so "Take over" follows the holder's edits.
    if (this.stopTicks === null) {
      this.stopTicks = this.options.ticker.every(this.heartbeatMs, () => void this.beat());
    }
  }

  private touch(): void {
    this.options.scheduler.clearTimeout(this.idleTimer);
    this.idleTimer = this.options.scheduler.setTimeout(() => void this.idle(), this.options.idleReleaseMs);
  }

  private async idle(): Promise<void> {
    if (this.store.getState().status !== "held") {
      return;
    }
    await this.options.beforeRelease?.();
    await this.release(false);
  }

  private onTab(m: TabMessage): void {
    const { status } = this.store.getState();
    if (m.type === "query") {
      if (status === "held") {
        this.options.channel?.post({ type: "held", session: this.options.session });
      }
      return;
    }
    if (m.session === this.options.session) {
      return;
    }
    if (m.type === "held" && status !== "held") {
      this.store.setState({ ...initial, status: "other-tab" });
    } else if (m.type === "released" && status === "other-tab") {
      this.store.setState({ ...initial });
    }
  }

  private rejectWaiters(error: Error = new LockUnavailable()): void {
    const waiters = this.waiters;
    this.waiters = [];
    for (const w of waiters) {
      w.reject(error);
    }
  }

  private stop(): void {
    this.stopTicks?.();
    this.stopTicks = null;
    this.options.scheduler.clearTimeout(this.idleTimer);
  }
}

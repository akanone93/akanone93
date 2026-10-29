// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { limitDefault } from "@plux/schema";
import type { Scheduler } from "./edit-session.ts";
import {
  LOCK_HEARTBEAT_MS,
  LOCK_LEASE_LAPSE_MS,
  LOCK_TTL_MS,
  LockController,
  type LockInfo,
  type LockTransport,
  LockUnavailable,
  type TabChannel,
  type TabMessage,
  type Ticker,
  takeoverAllowedAt,
} from "./lock-controller.ts";

const idleReleaseMs = limitDefault("studio.lockIdleReleaseMinutes") * 60_000;
const takeoverIdleMs = limitDefault("studio.lockTakeoverIdleMinutes") * 60_000;

/** Fake time: timeouts and tickers fire as the clock advances. */
function fakeClock() {
  let now = 1_000_000;
  let next = 0;
  const timers = new Map<number, { at: number; fn: () => void; every?: number | undefined }>();
  const add = (fn: () => void, ms: number, every?: number) => {
    next += 1;
    timers.set(next, { at: now + ms, fn, every });
    return next;
  };
  const scheduler: Scheduler = {
    setTimeout: (fn, ms) => add(fn, ms),
    clearTimeout: (h) => timers.delete(h as number),
  };
  const ticker: Ticker = {
    every(ms, fn) {
      const id = add(fn, ms, ms);
      return () => timers.delete(id);
    },
  };
  return {
    scheduler,
    ticker,
    now: () => now,
    async advance(ms: number) {
      const end = now + ms;
      for (;;) {
        const due = [...timers.entries()].filter(([, t]) => t.at <= end).sort((a, b) => a[1].at - b[1].at)[0];
        if (due === undefined) {
          break;
        }
        const [id, t] = due;
        now = t.at;
        if (t.every === undefined) {
          timers.delete(id);
        } else {
          timers.set(id, { ...t, at: t.at + t.every });
        }
        t.fn();
        await settle();
      }
      now = end;
      await settle();
    },
  };
}

async function settle() {
  for (let i = 0; i < 10; i++) {
    await Promise.resolve();
  }
}

class Refused extends Error {}
class Down extends Error {}

/** A stub PluginService lock that applies the server's takeover rule (L-1). */
function stubServer(clock: { now: () => number }) {
  let held: { session: string; holder: string; lastEdit: number; heartbeat: number; requests: string[] } | null = null;
  let down = false;
  const calls: string[] = [];
  const info = (): LockInfo | null =>
    held === null
      ? null
      : {
          holder: held.holder,
          session: held.session,
          acquiredAt: new Date(held.heartbeat),
          expiresAt: new Date(held.heartbeat + LOCK_TTL_MS),
          lastEditAt: new Date(held.lastEdit),
          requestedBy: held.requests,
        };
  const live = () => held !== null && clock.now() < held.heartbeat + LOCK_TTL_MS;
  /** The server's rule: someone else's live lock is taken only by force, and only per L-1. */
  const refuses = (holder: string, force: boolean) => {
    const lock = info();
    if (!live() || lock === null || held?.holder === holder) {
      return false;
    }
    const at = takeoverAllowedAt(lock, takeoverIdleMs);
    return !force || at === undefined || clock.now() < at.getTime();
  };
  const as = (holder: string): LockTransport => ({
    async acquire(session, force) {
      calls.push(`acquire ${holder} ${force}`);
      if (down) {
        throw new Down();
      }
      if (refuses(holder, force)) {
        throw new Refused();
      }
      held = { session, holder, lastEdit: clock.now(), heartbeat: clock.now(), requests: [] };
      return { lock: info() as LockInfo, preservedSnapshotId: force ? "snap-1" : "" };
    },
    async renew(session) {
      calls.push(`renew ${holder}`);
      if (down) {
        throw new Down();
      }
      if (!live() || held === null || held.session !== session) {
        throw new Refused();
      }
      held.heartbeat = clock.now();
      return info() as LockInfo;
    },
    async release(session) {
      calls.push(`release ${holder}`);
      if (held?.session === session) {
        held = null;
      }
    },
    async request() {
      if (held === null) {
        throw new Refused();
      }
      held.requests = [...held.requests, holder];
      return info() as LockInfo;
    },
    async get() {
      return live() ? info() : null;
    },
    classify: (e) => (e instanceof Down ? "transient" : "refused"),
  });
  return {
    as,
    calls,
    edit() {
      if (held !== null) {
        held.lastEdit = clock.now();
      }
    },
    setDown(v: boolean) {
      down = v;
    },
  };
}

/** A BroadcastChannel between tabs of one browser. */
function channelHub() {
  const listeners = new Set<(m: TabMessage) => void>();
  return (): TabChannel => {
    let mine: ((m: TabMessage) => void) | undefined;
    return {
      post: (m) => {
        for (const l of listeners) {
          if (l !== mine) {
            l(m);
          }
        }
      },
      listen(fn) {
        mine = fn;
        listeners.add(fn);
        return () => listeners.delete(fn);
      },
    };
  };
}

function controller(
  clock: ReturnType<typeof fakeClock>,
  transport: LockTransport,
  session: string,
  extra: { channel?: TabChannel; beforeRelease?: () => Promise<void> } = {},
) {
  return new LockController({
    transport,
    session,
    idleReleaseMs,
    takeoverIdleMs,
    scheduler: clock.scheduler,
    ticker: clock.ticker,
    now: clock.now,
    ...extra,
  });
}

describe("LockController (F1, [SRV-040]–[SRV-042], [STU-075])", () => {
  test("[STU-075]: the first edit takes the lock, saves go under it, the heartbeat renews it", async () => {
    const clock = fakeClock();
    const server = stubServer(clock);
    const lock = controller(clock, server.as("ana"), "tab-1");
    expect(lock.store.getState().status).toBe("free");
    const save = lock.whenHeld();
    await lock.ensure();
    expect(await save).toBe("tab-1");
    expect(lock.store.getState().status).toBe("held");
    await clock.advance(LOCK_HEARTBEAT_MS * 3);
    expect(server.calls.filter((c) => c === "renew ana")).toHaveLength(3);
  });

  test("idle auto-release flushes autosave first and frees the lock", async () => {
    const clock = fakeClock();
    const server = stubServer(clock);
    const order: string[] = [];
    const lock = controller(clock, server.as("ana"), "tab-1", {
      beforeRelease: async () => {
        order.push("flush");
      },
    });
    await lock.ensure();
    await clock.advance(idleReleaseMs - 1000);
    expect(await lock.whenHeld()).toBe("tab-1");
    await clock.advance(idleReleaseMs - 1000);
    expect(lock.store.getState().status).toBe("held");
    await clock.advance(1000);
    expect(order).toEqual(["flush"]);
    expect(lock.store.getState().status).toBe("free");
    expect(server.calls).toContain("release ana");
  });

  test("a lock held by someone else turns the plugin read-only and undoes the edit", async () => {
    const clock = fakeClock();
    const server = stubServer(clock);
    await controller(clock, server.as("ben"), "b-1").ensure();
    const lock = controller(clock, server.as("ana"), "tab-1");
    await expect(lock.ensure()).rejects.toBeInstanceOf(LockUnavailable);
    expect(lock.store.getState().status).toBe("held-by-other");
    expect(lock.store.getState().lock?.holder).toBe("ben");
    await expect(lock.whenHeld()).rejects.toBeInstanceOf(LockUnavailable);
    await lock.request();
    expect(lock.store.getState().requested).toBe(true);
  });

  test("two tabs of one person share one lock and one heartbeat (F1.3)", async () => {
    const clock = fakeClock();
    const server = stubServer(clock);
    const hub = channelHub();
    const one = controller(clock, server.as("ana"), "tab-1", { channel: hub() });
    const two = controller(clock, server.as("ana"), "tab-2", { channel: hub() });
    await one.ensure();
    expect(two.store.getState().status).toBe("other-tab");
    await expect(two.whenHeld()).rejects.toBeInstanceOf(LockUnavailable);
    await clock.advance(LOCK_HEARTBEAT_MS);
    expect(server.calls.filter((c) => c.startsWith("renew"))).toHaveLength(1);
    // A tab opened later learns of the holder by asking.
    const three = controller(clock, server.as("ana"), "tab-3", { channel: hub() });
    expect(three.store.getState().status).toBe("other-tab");
    await one.release();
    expect(two.store.getState().status).toBe("free");
    expect(three.store.getState().status).toBe("free");
  });

  test("[SRV-041]: Take over is enabled only per L-1, mirroring the server", async () => {
    const clock = fakeClock();
    const server = stubServer(clock);
    const ben = controller(clock, server.as("ben"), "b-1");
    await ben.ensure();
    const ana = controller(clock, server.as("ana"), "tab-1");
    await ana.refresh();
    expect(ana.canTakeOver()).toBe(false);
    // Ben keeps editing: heartbeats alone keep the lease, the idle time counts from his last edit.
    await clock.advance(takeoverIdleMs - LOCK_HEARTBEAT_MS);
    server.edit();
    await clock.advance(LOCK_HEARTBEAT_MS * 2);
    expect(ana.canTakeOver()).toBe(false);
    expect(await ana.takeOver()).toBeUndefined();
    expect(ana.store.getState().takeoverRefused).toBe(true);
    await clock.advance(takeoverIdleMs - LOCK_HEARTBEAT_MS * 2 - 1);
    expect(ana.canTakeOver()).toBe(false);
    await clock.advance(1);
    expect(ana.canTakeOver()).toBe(true);
    expect(await ana.takeOver()).toBe("snap-1");
    expect(ana.store.getState().status).toBe("held");
    // Ben's next heartbeat finds the lock gone: D37, his unsent changes wait.
    await clock.advance(LOCK_HEARTBEAT_MS);
    expect(ben.store.getState().status).toBe("lost");
    expect(ben.store.getState().lost).toBe("takeover");
  });

  test("a lapsed lease allows takeover at once", () => {
    const lock: LockInfo = {
      holder: "ben",
      session: "b",
      lastEditAt: new Date(10_000),
      expiresAt: new Date(10_000 + LOCK_TTL_MS),
      requestedBy: [],
    };
    expect(takeoverAllowedAt(lock, takeoverIdleMs)?.getTime()).toBe(10_000 + LOCK_LEASE_LAPSE_MS);
    const server = { ...lock, takeoverAllowedAt: new Date(5) };
    expect(takeoverAllowedAt(server, takeoverIdleMs)?.getTime()).toBe(5);
  });

  test("lock lost: saves wait in memory; re-take and apply sends them, discard drops them (F1.4)", async () => {
    const clock = fakeClock();
    const server = stubServer(clock);
    const lock = controller(clock, server.as("ana"), "tab-1");
    await lock.ensure();
    server.setDown(true);
    await clock.advance(LOCK_TTL_MS);
    expect(lock.store.getState().status).toBe("lost");
    expect(lock.store.getState().lost).toBe("expired");
    let sent = "";
    void lock.whenHeld().then((s) => {
      sent = s;
    });
    await settle();
    expect(sent).toBe("");
    server.setDown(false);
    await lock.retake();
    await settle();
    expect(sent).toBe("tab-1");

    server.setDown(true);
    await clock.advance(LOCK_TTL_MS);
    expect(lock.store.getState().status).toBe("lost");
    server.setDown(false);
    const dropped = lock.whenHeld();
    await lock.discard();
    await expect(dropped).rejects.toBeInstanceOf(LockUnavailable);
  });

  test("the holder sees requests and may keep the lock or hand it over", async () => {
    const clock = fakeClock();
    const server = stubServer(clock);
    const ana = controller(clock, server.as("ana"), "tab-1");
    await ana.ensure();
    const ben = controller(clock, server.as("ben"), "b-1");
    await ben.refresh();
    await ben.request();
    await clock.advance(LOCK_HEARTBEAT_MS);
    expect(ana.store.getState().requests).toEqual(["ben"]);
    ana.keep();
    await clock.advance(LOCK_HEARTBEAT_MS);
    expect(ana.store.getState().requests).toEqual([]);
    await ana.release();
    expect(ana.store.getState().status).toBe("free");
    await ben.ensure();
    expect(ben.store.getState().status).toBe("held");
  });

  test("tab close releases with keepalive", async () => {
    const clock = fakeClock();
    const server = stubServer(clock);
    const lock = controller(clock, server.as("ana"), "tab-1");
    await lock.ensure();
    const target = new EventTarget();
    const detach = lock.attach(target);
    target.dispatchEvent(new Event("pagehide"));
    await settle();
    expect(server.calls).toContain("release ana");
    detach();
    lock.dispose();
  });
});

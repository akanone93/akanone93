// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { type Timestamp, timestampDate } from "@bufbuild/protobuf/wkt";
import { Code, ConnectError, createClient, type Transport } from "@connectrpc/connect";
import { keepaliveHeader, type Lock, PluginService } from "@plux/api";
import { classifySaveError } from "./document-transport.ts";
import type { LockInfo, LockTransport, TabChannel, TabMessage, Ticker } from "./lock-controller.ts";

function date(t: Timestamp | undefined): Date | undefined {
  return t === undefined ? undefined : timestampDate(t);
}

/** The lock as Studio shows it; the holder by display name, never an email address. */
export function toLockInfo(l: Lock): LockInfo {
  return {
    holder: l.holder?.display || l.holder?.id || "",
    session: l.session,
    acquiredAt: date(l.acquiredAt),
    expiresAt: date(l.expiresAt),
    lastEditAt: date(l.lastEditAt),
    takeoverAllowedAt: date(l.takeoverAllowedAt),
    requestedBy: l.requestedBy.map((a) => a.display || a.id),
  };
}

function required(l: Lock | undefined): LockInfo {
  if (l === undefined) {
    throw new ConnectError("the server returned no lock", Code.Internal);
  }
  return toLockInfo(l);
}

/** Binds the lock controller to PluginService for one plugin (SRV-040–SRV-042). */
export function lockTransport(
  transport: Transport,
  ids: { readonly appId: string; readonly pluginId: string },
): LockTransport {
  const client = createClient(PluginService, transport);
  return {
    async acquire(session, force) {
      const res = await client.acquireLock({ ...ids, session, force });
      return { lock: required(res.lock), preservedSnapshotId: res.preservedSnapshotId };
    },
    async renew(session) {
      return required((await client.renewLock({ ...ids, session })).lock);
    },
    async release(session, { keepalive }) {
      await client.releaseLock({ ...ids, session }, keepalive ? { headers: { [keepaliveHeader]: "1" } } : {});
    },
    async request() {
      return required((await client.requestLock(ids)).lock);
    },
    async get() {
      const l = (await client.getLock(ids)).lock;
      return l === undefined || l.session === "" ? null : toLockInfo(l);
    },
    classify: (error) => (classifySaveError(error) === "transient" ? "transient" : "refused"),
  };
}

/** One BroadcastChannel per plugin, so one person's tabs share one lock (F1.3). */
export function tabChannel(appId: string, pluginId: string): TabChannel | undefined {
  if (typeof BroadcastChannel === "undefined") {
    return undefined;
  }
  const channel = new BroadcastChannel(`plux-lock:${appId}/${pluginId}`);
  return {
    post: (m) => channel.postMessage(m),
    listen(fn) {
      const on = (e: MessageEvent<TabMessage>) => fn(e.data);
      channel.addEventListener("message", on);
      return () => {
        channel.removeEventListener("message", on);
        channel.close();
      };
    },
  };
}

/** A worker-like target: the heartbeat worker, or a stub. */
export interface TickWorker {
  postMessage(message: { readonly every: number } | { readonly stop: true }): void;
  addEventListener(type: "message", fn: () => void): void;
  terminate(): void;
}

/** Ticks from a worker, whose timers background tabs do not throttle (F1.2); timers without one. */
export function workerTicker(make: (() => TickWorker) | undefined): Ticker {
  return {
    every(ms, fn) {
      if (make === undefined) {
        const handle = setInterval(fn, ms);
        return () => clearInterval(handle);
      }
      const worker = make();
      worker.addEventListener("message", fn);
      worker.postMessage({ every: ms });
      return () => worker.terminate();
    },
  };
}

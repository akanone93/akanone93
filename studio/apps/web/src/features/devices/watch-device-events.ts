// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { type Client, Code, ConnectError } from "@connectrpc/connect";
import type { DevEvent, DevSessionService } from "@plux/api";
import { backoffMs, type StreamState } from "../shell/studio-events.ts";

export interface WatchOptions {
  readonly client: Client<typeof DevSessionService>;
  readonly sessionId: string;
  readonly deviceIds: readonly string[];
  readonly signal: AbortSignal;
  readonly onState: (state: StreamState) => void;
  readonly onEvent: (event: DevEvent) => void;
  /** The server numbers the session's events from 1 again (it lost its counter): what was shown is dropped. */
  readonly onReset?: () => void;
  readonly sleep?: (ms: number) => Promise<void>;
  /**
   * How long a connection may stay silent before its first response, in milliseconds. The server sends a heartbeat
   * as soon as the stream opens, so a connection that says nothing for this long is held back on the way.
   */
  readonly stallMs?: number;
}

const defaultSleep = (ms: number) => new Promise<void>((resolve) => setTimeout(resolve, ms));

/** The silence after which a connection counts as held back (the server's first heartbeat comes at once). */
export const STALL_MS = 6000;

/** How many held-back connections switch the watch to long polls. */
export const STALLS_BEFORE_POLLING = 2;

/** The window of one long poll, in milliseconds: the server ends the stream after it (DEV-021). */
export const POLL_WINDOW_MS = 2000;

/**
 * Whether the server will never accept the watch: the person may not see it, it does not exist, or the session ended
 * (PLX-9200, which a watch of an ended session is answered with).
 */
function isFinal(error: unknown): boolean {
  return (
    error instanceof ConnectError &&
    (error.code === Code.Unauthenticated ||
      error.code === Code.PermissionDenied ||
      error.code === Code.NotFound ||
      error.code === Code.FailedPrecondition)
  );
}

interface Progress {
  after: bigint;
  attempt: number;
  /** Connections in a row that said nothing before the stall time. */
  stalls: number;
  /** Long polls in place of one stream: a stream was held back on the way more than once. */
  polling: boolean;
  /** Whether the watch was live once: a later wait is a reconnect. */
  wasLive: boolean;
}

class ReadFailed extends Error {
  constructor(
    readonly after: bigint,
    readonly live: boolean,
    readonly stalled: boolean,
    readonly reason: unknown,
  ) {
    super("stream failed");
  }
}

/**
 * Reads one connection to its end; returns the last sequence seen and whether any response arrived. A connection
 * with no response within the stall time is aborted and reported as stalled; the caller's abort ends it as usual.
 */
async function readOnce(
  o: WatchOptions,
  p: Progress,
  report: (state: StreamState) => void,
): Promise<{ after: bigint; live: boolean }> {
  const attempt = new AbortController();
  const stop = () => attempt.abort();
  o.signal.addEventListener("abort", stop);
  const window = p.polling ? POLL_WINDOW_MS : 0;
  let live = false;
  let stalled = false;
  const watchdog = setTimeout(
    () => {
      if (!live) {
        stalled = true;
        attempt.abort();
      }
    },
    (o.stallMs ?? STALL_MS) + window,
  );
  const request = { sessionId: o.sessionId, deviceIds: [...o.deviceIds], afterSequence: p.after, waitMs: window };
  let last = p.after;
  try {
    for await (const res of o.client.watchEvents(request, { signal: attempt.signal })) {
      // An empty response is the server's heartbeat: the stream is open.
      live = true;
      report("live");
      if (res.event !== undefined) {
        // A drop carries the sequence it stands for; any other event at or below the last one means the server
        // numbers from 1 again, and resumes from the start (DEV-021).
        if (res.event.sequence <= last && last > 0n && res.event.payload.case !== "drop") {
          o.onReset?.();
        }
        last = res.event.sequence;
        o.onEvent(res.event);
      }
    }
  } catch (error) {
    throw new ReadFailed(last, live, stalled && !o.signal.aborted, error);
  } finally {
    clearTimeout(watchdog);
    o.signal.removeEventListener("abort", stop);
  }
  return { after: last, live };
}

/** Whether a failure ends the watch: the caller stopped it, or the server will never accept it. */
function ends(o: WatchOptions, error: unknown): boolean {
  return !(error instanceof ReadFailed) || o.signal.aborted || (!error.stalled && isFinal(error.reason));
}

/**
 * One connection: true when the watch goes on with another one, false when it is over. A stream the server ends
 * cleanly is opened again: a server restarts or drains its streams on a deploy, and the console must stay live
 * (DEV-021). A long poll ends after its window and the next one starts at once. Only a refusal the server will repeat
 * ends the watch.
 */
async function cycle(
  o: WatchOptions,
  p: Progress,
  sleep: (ms: number) => Promise<void>,
  report: (state: StreamState) => void,
): Promise<boolean> {
  if (!p.polling) {
    report(p.wasLive || p.attempt > 0 || p.stalls > 0 ? "reconnecting" : "connecting");
  }
  try {
    const read = await readOnce(o, p, report);
    if (o.signal.aborted) {
      return false;
    }
    p.after = read.after;
    p.stalls = 0;
    p.wasLive ||= read.live;
    if (p.polling && read.live) {
      p.attempt = 0;
      return true;
    }
    p.attempt = (read.live ? 0 : p.attempt) + 1;
    report("reconnecting");
    await sleep(backoffMs(p.attempt - 1));
    return true;
  } catch (error) {
    if (ends(o, error)) {
      report("ended");
      return false;
    }
    const failed = error as ReadFailed;
    p.after = failed.after;
    p.wasLive ||= failed.live;
    if (failed.stalled) {
      p.stalls += 1;
      p.polling ||= p.stalls >= STALLS_BEFORE_POLLING;
      report("reconnecting");
      return true;
    }
    p.attempt = (failed.live ? 0 : p.attempt) + 1;
    report("reconnecting");
    await sleep(backoffMs(p.attempt - 1));
    return true;
  }
}

/**
 * Streams the dev session's events, resuming after the last sequence and retrying with the shared backoff. A
 * connection held back on the way (no heartbeat within the stall time) is tried again, and after two such the watch
 * reads the session in long polls of a short window instead. Once the caller aborts, the watch reports no state: a
 * new watch reports its own.
 */
export async function watchDeviceEvents(o: WatchOptions): Promise<void> {
  const progress: Progress = { after: 0n, attempt: 0, stalls: 0, polling: false, wasLive: false };
  const sleep = o.sleep ?? defaultSleep;
  const report = (state: StreamState) => {
    if (!o.signal.aborted) {
      o.onState(state);
    }
  };
  while (!o.signal.aborted && (await cycle(o, progress, sleep, report))) {
    // each cycle resumes after the last sequence it saw
  }
}

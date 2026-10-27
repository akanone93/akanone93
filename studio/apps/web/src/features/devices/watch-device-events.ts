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
  readonly sleep?: (ms: number) => Promise<void>;
}

const defaultSleep = (ms: number) => new Promise<void>((resolve) => setTimeout(resolve, ms));

function isFinal(error: unknown): boolean {
  return (
    error instanceof ConnectError &&
    (error.code === Code.Unauthenticated || error.code === Code.PermissionDenied || error.code === Code.NotFound)
  );
}

/** Reads one connection to its end; returns the last sequence seen and whether any event arrived. */
async function readOnce(o: WatchOptions, after: bigint): Promise<{ after: bigint; live: boolean }> {
  const request = { sessionId: o.sessionId, deviceIds: [...o.deviceIds], afterSequence: after };
  let last = after;
  let live = false;
  try {
    for await (const res of o.client.watchEvents(request, { signal: o.signal })) {
      live = true;
      o.onState("live");
      if (res.event !== undefined) {
        last = res.event.sequence;
        o.onEvent(res.event);
      }
    }
  } catch (error) {
    throw new ReadFailed(last, live, error);
  }
  return { after: last, live };
}

class ReadFailed extends Error {
  constructor(
    readonly after: bigint,
    readonly live: boolean,
    readonly reason: unknown,
  ) {
    super("stream failed");
  }
}

/** Whether a failure ends the watch: the caller stopped it, or the server will never accept it. */
function ends(o: WatchOptions, error: unknown): boolean {
  return !(error instanceof ReadFailed) || o.signal.aborted || isFinal(error.reason);
}

interface Progress {
  after: bigint;
  attempt: number;
}

/** One connection: true when the watch goes on with another one, false when it is over. */
async function cycle(o: WatchOptions, progress: Progress, sleep: (ms: number) => Promise<void>): Promise<boolean> {
  o.onState(progress.attempt === 0 ? "connecting" : "reconnecting");
  try {
    await readOnce(o, progress.after);
    o.onState("ended");
    return false;
  } catch (error) {
    if (ends(o, error)) {
      o.onState(o.signal.aborted ? "connecting" : "ended");
      return false;
    }
    const failed = error as ReadFailed;
    progress.after = failed.after;
    progress.attempt = (failed.live ? 0 : progress.attempt) + 1;
    await sleep(backoffMs(progress.attempt - 1));
    return true;
  }
}

/** Streams the dev session's events, resuming after the last sequence and retrying with the shared backoff. */
export async function watchDeviceEvents(o: WatchOptions): Promise<void> {
  const progress: Progress = { after: 0n, attempt: 0 };
  const sleep = o.sleep ?? defaultSleep;
  while (!o.signal.aborted && (await cycle(o, progress, sleep))) {
    // each cycle resumes after the last sequence it saw
  }
}

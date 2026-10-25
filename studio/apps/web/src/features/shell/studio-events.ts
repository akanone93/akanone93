// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Code, ConnectError, createClient, type Transport } from "@connectrpc/connect";
import { StudioEventsService, type StudioEventsServiceWatchResponse } from "@plux/api";

/** Where the stream is: `ended` means the server closed it for good, i.e. the session was revoked (D6). */
export type StreamState = "connecting" | "live" | "reconnecting" | "ended";

export interface WatchOptions {
  readonly transport: Transport;
  readonly signal: AbortSignal;
  readonly onState: (state: StreamState) => void;
  readonly onInbox: (unread: bigint) => void;
  readonly onReset: () => void;
  /** Waits before a retry; tests pass a no-op. */
  readonly sleep?: (ms: number) => Promise<void>;
}

const baseDelayMs = 500;
const maxDelayMs = 30_000;

/** Exponential backoff with a ceiling. */
export function backoffMs(attempt: number): number {
  return Math.min(maxDelayMs, baseDelayMs * 2 ** attempt);
}

const defaultSleep = (ms: number) => new Promise<void>((resolve) => setTimeout(resolve, ms));

function dispatch(event: StudioEventsServiceWatchResponse, options: WatchOptions): void {
  if (event.event.case === "inbox") {
    options.onInbox(event.event.value.unread);
  } else if (event.event.case === "reset") {
    options.onReset();
  }
}

function isRevocation(error: unknown): boolean {
  return error instanceof ConnectError && (error.code === Code.Unauthenticated || error.code === Code.PermissionDenied);
}

/**
 * Watches the events stream, resuming from the last cursor and retrying with backoff on errors.
 * A stream that completes without error, or fails as unauthenticated, ends the watch (session revoked).
 */
export async function watchStudioEvents(options: WatchOptions): Promise<void> {
  const client = createClient(StudioEventsService, options.transport);
  const sleep = options.sleep ?? defaultSleep;
  let cursor = "";
  let attempt = 0;
  options.onState("connecting");
  while (!options.signal.aborted) {
    try {
      for await (const event of client.watch({ cursor }, { signal: options.signal })) {
        attempt = 0;
        cursor = event.cursor === "" ? cursor : event.cursor;
        options.onState("live");
        dispatch(event, options);
      }
      options.onState("ended");
      return;
    } catch (error) {
      if (options.signal.aborted) {
        return;
      }
      if (isRevocation(error)) {
        options.onState("ended");
        return;
      }
      options.onState("reconnecting");
      await sleep(backoffMs(attempt));
      attempt += 1;
    }
  }
}

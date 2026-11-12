// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { create } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";
import { type DevEvent, DevEventSchema } from "@plux/api";
import type { StreamState } from "../shell/studio-events.ts";
import { POLL_WINDOW_MS, watchDeviceEvents } from "./watch-device-events.ts";

const log = (sequence: bigint, message: string): DevEvent =>
  create(DevEventSchema, { sequence, deviceId: "d1", payload: { case: "log", value: { message } } });

/** A connection the fake server opens: what it streams, then how it ends (cleanly, or with an error). */
type Connection = { readonly events: readonly (DevEvent | undefined)[]; readonly end?: Error };

/** A client whose watch plays one scripted connection per call and records where each resumed. */
function scripted(connections: Connection[]) {
  const resumedAfter: bigint[] = [];
  const client = {
    async *watchEvents(req: { afterSequence: bigint }) {
      resumedAfter.push(req.afterSequence);
      const c = connections.shift();
      if (c === undefined) {
        throw new ConnectError("the session ended", Code.FailedPrecondition);
      }
      for (const event of c.events) {
        yield event === undefined ? {} : { event };
      }
      if (c.end !== undefined) {
        throw c.end;
      }
    },
  };
  return { client, resumedAfter };
}

async function run(connections: Connection[]) {
  const { client, resumedAfter } = scripted(connections);
  const states: StreamState[] = [];
  const events: string[] = [];
  let resets = 0;
  await watchDeviceEvents({
    // biome-ignore lint/suspicious/noExplicitAny: the fake has the one method the watch calls
    client: client as any,
    sessionId: "s1",
    deviceIds: [],
    signal: new AbortController().signal,
    onState: (s) => states.push(s),
    onEvent: (e) => events.push(e.payload.case === "log" ? e.payload.value.message : String(e.payload.case)),
    onReset: () => resets++,
    sleep: async () => undefined,
  });
  return { states, events, resets, resumedAfter };
}

describe("the dev session's event stream (DEV-020, DEV-021)", () => {
  test("a stream the server ends cleanly, as on a restart, is opened again after the last event", async () => {
    const r = await run([{ events: [undefined, log(1n, "one"), log(2n, "two")] }, { events: [log(3n, "three")] }]);
    expect(r.events).toEqual(["one", "two", "three"]);
    expect(r.resumedAfter).toEqual([0n, 2n, 3n]);
    expect(r.states).toContain("reconnecting");
    expect(r.states.at(-1)).toBe("ended");
  });

  test("a stream that fails is opened again, and a heartbeat alone makes it live", async () => {
    const r = await run([
      { events: [undefined], end: new ConnectError("network", Code.Unavailable) },
      { events: [log(1n, "one")] },
    ]);
    expect(r.states.slice(0, 2)).toEqual(["connecting", "live"]);
    expect(r.events).toEqual(["one"]);
  });

  test("a session that ended ends the watch: it is not tried for ever", async () => {
    const r = await run([{ events: [], end: new ConnectError("the session ended", Code.FailedPrecondition) }]);
    expect(r.resumedAfter).toEqual([0n]);
    expect(r.states.at(-1)).toBe("ended");
  });

  test("events numbered from 1 again (the server lost its counter) reset what is shown", async () => {
    const r = await run([{ events: [log(1n, "one"), log(2n, "two")] }, { events: [log(1n, "again")] }]);
    expect(r.events).toEqual(["one", "two", "again"]);
    expect(r.resets).toBe(1);
  });

  test("a connection held back on the way is tried again, and after two the watch long-polls", async () => {
    const waits: number[] = [];
    let call = 0;
    const client = {
      async *watchEvents(req: { waitMs: number }, opts: { signal: AbortSignal }) {
        waits.push(req.waitMs);
        call += 1;
        if (call <= 2) {
          // Says nothing until the watch gives up on it.
          await new Promise<void>((resolve) => opts.signal.addEventListener("abort", () => resolve()));
          throw new ConnectError("aborted", Code.Canceled);
        }
        if (call === 3) {
          yield {};
          yield { event: log(1n, "in a window") };
          return;
        }
        throw new ConnectError("the session ended", Code.FailedPrecondition);
      },
    };
    const states: StreamState[] = [];
    const events: string[] = [];
    await watchDeviceEvents({
      // biome-ignore lint/suspicious/noExplicitAny: the fake has the one method the watch calls
      client: client as any,
      sessionId: "s1",
      deviceIds: [],
      signal: new AbortController().signal,
      onState: (s) => states.push(s),
      onEvent: (e) => events.push(e.payload.case === "log" ? e.payload.value.message : ""),
      sleep: async () => undefined,
      stallMs: 5,
    });
    expect(waits).toEqual([0, 0, POLL_WINDOW_MS, POLL_WINDOW_MS]);
    expect(events).toEqual(["in a window"]);
    expect(states.slice(0, 3)).toEqual(["connecting", "reconnecting", "reconnecting"]);
    expect(states).toContain("live");
  });

  test("a watch the caller stopped reports nothing more: the next watch reports its own state", async () => {
    const stop = new AbortController();
    const client = {
      async *watchEvents(_req: unknown, opts: { signal: AbortSignal }) {
        yield {};
        await new Promise<void>((resolve) => opts.signal.addEventListener("abort", () => resolve()));
        throw new ConnectError("aborted", Code.Canceled);
      },
    };
    const states: StreamState[] = [];
    const done = watchDeviceEvents({
      // biome-ignore lint/suspicious/noExplicitAny: the fake has the one method the watch calls
      client: client as any,
      sessionId: "s1",
      deviceIds: [],
      signal: stop.signal,
      onState: (s) => states.push(s),
      onEvent: () => undefined,
      sleep: async () => undefined,
    });
    await new Promise((resolve) => setTimeout(resolve, 5));
    stop.abort();
    await done;
    expect(states).toEqual(["connecting", "live"]);
  });
});

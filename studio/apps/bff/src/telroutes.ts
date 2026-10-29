// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Context, Hono } from "hono";
import type { BffConfig } from "./config.ts";
import { cookieValue } from "./http.ts";
import { type RateLimiter, sha256Hex } from "./limiter.ts";
import { type BrowserEvent, maxTelemetryBytes, parseBatch } from "./telemetry.ts";

export type TelemetryDeps = {
  config: BffConfig;
  limiter: RateLimiter;
  ip: (req: Request) => string;
  log: (line: Record<string, unknown>) => void;
  random: () => number;
};

const traceparentPattern = /^[0-9a-f]{2}-[0-9a-f]{32}-[0-9a-f]{16}-[0-9a-f]{2}$/;

/** Reads and validates the body; a number is the status to answer with. */
async function readBatch(c: Context): Promise<BrowserEvent[] | number> {
  if (Number(c.req.header("content-length") ?? "0") > maxTelemetryBytes) {
    return 413;
  }
  const raw = await c.req.arrayBuffer();
  if (raw.byteLength > maxTelemetryBytes) {
    return 413;
  }
  try {
    return parseBatch(JSON.parse(new TextDecoder().decode(raw))) ?? 400;
  } catch {
    return 400;
  }
}

/** POST /telemetry (OBS-001); returns the count of events dropped by the per-session limit. */
export function registerTelemetry(app: Hono<{ Variables: { nonce: string } }>, deps: TelemetryDeps): () => number {
  const { config, limiter, ip, log, random } = deps;
  let dropped = 0;
  const forward = async (events: BrowserEvent[], key: string, extra: Record<string, string>) => {
    let accepted = 0;
    let over = 0;
    for (const e of events) {
      if (random() >= config.telemetrySampleRate) {
        continue;
      }
      if (await limiter.allow(key)) {
        accepted += 1;
        log({ msg: "browser.event", ...e, ...extra });
      } else {
        over += 1;
      }
    }
    return { accepted, over };
  };
  app.post("/telemetry", async (c) => {
    if (!config.telemetryEnabled) {
      return c.body(null, 204);
    }
    const events = await readBatch(c);
    if (typeof events === "number") {
      return c.body(null, events as 400 | 413);
    }
    const session = cookieValue(c.req.raw, "__Host-plux_session");
    const key = `t:${session === "" ? ip(c.req.raw) : await sha256Hex(session)}`;
    const trace = c.req.header("traceparent") ?? "";
    const extra = traceparentPattern.test(trace) ? { traceparent: trace } : {};
    const { accepted, over } = await forward(events, key, extra);
    dropped += over;
    return c.json({ accepted, dropped: over }, 202);
  });
  return () => dropped;
}

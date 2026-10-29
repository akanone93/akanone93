// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { readFileSync } from "node:fs";
import { join, relative } from "node:path";
import { Hono } from "hono";
import { serveStatic } from "hono/bun";
import { registerAuthRoutes } from "./authroutes.ts";
import { brotliStatic } from "./brotli.ts";
import { makeClientIp } from "./clientip.ts";
import type { BffConfig } from "./config.ts";
import { cookieValue, text } from "./http.ts";
import { MemoryRateLimiter, type RateLimiter, StreamCounter, sha256Hex } from "./limiter.ts";
import { proxyRequest } from "./proxy.ts";
import { injectNonce, newNonce, securityHeaders } from "./security.ts";
import { stdoutLine } from "./telemetry.ts";
import { registerTelemetry } from "./telroutes.ts";
import { isOlder, upstreamVersion } from "./version.ts";

export type AppOptions = {
  /** Directory holding the built web application. */
  staticDir: string;
  config: BffConfig;
  /** Client address of a request; the server supplies it. */
  clientIp?: (req: Request) => string;
  limiter?: RateLimiter;
  /** Limiter for telemetry events, counted apart from requests at their own registry limit. */
  telemetryLimiter?: RateLimiter;
  readyTimeoutMs?: number;
  /** Version of Studio reported by /version. */
  studioVersion?: string;
  /** Sink for telemetry lines; defaults to stdout. */
  logLine?: (line: Record<string, unknown>) => void;
  /** Random source for sampling. */
  random?: () => number;
  /** Key for the sign-in state cookie; required and identical on every replica. */
  stateKey: Uint8Array;
};

export type BffApp = {
  app: Hono<{ Variables: { nonce: string } }>;
  /** Number of CSP violation reports received. */
  cspReports: () => number;
  /** Telemetry events dropped by the per-session limit. */
  telemetryDropped: () => number;
  openStreams: () => number;
  /** Stops accepting new requests; /readyz answers 503 afterwards. */
  startDrain: () => void;
};

const maxReportBytes = 16384;
function isStream(req: Request): boolean {
  const ct = req.headers.get("content-type") ?? "";
  const accept = req.headers.get("accept") ?? "";
  return (
    ct.startsWith("application/connect+") ||
    accept.includes("text/event-stream") ||
    accept.includes("application/connect+")
  );
}

/** Builds the BFF: security headers, the API proxy, limits, health and the static web application. */
export function createBff(options: AppOptions): BffApp {
  const { config } = options;
  const app = new Hono<{ Variables: { nonce: string } }>();
  const root = relative(process.cwd(), options.staticDir) || ".";
  const https = config.publicBaseUrl.startsWith("https:");
  const limiter = options.limiter ?? new MemoryRateLimiter(config.requestsPerMinute);
  const reportLimiter = new MemoryRateLimiter(60);
  const streams = new StreamCounter(config.streamsPerSession, config.maxStreamConnections);
  const ip = makeClientIp(config.trustedProxies, options.clientIp ?? (() => "unknown"));
  const stateKey = options.stateKey;
  const telemetryLimiter = options.telemetryLimiter ?? new MemoryRateLimiter(config.telemetryEventsPerMinute);
  let reports = 0;
  let draining = false;

  app.use("*", async (c, next) => {
    const nonce = newNonce();
    c.set("nonce", nonce);
    await next();
    for (const [k, v] of Object.entries(securityHeaders(nonce, { https }))) {
      c.res.headers.set(k, v);
    }
  });

  app.get("/livez", (c) => c.text("ok", 200, text));
  app.get("/readyz", async (c) => {
    if (draining) {
      return c.text("draining", 503, text);
    }
    try {
      const r = await fetch(new URL("/readyz", config.apiUpstream), {
        signal: AbortSignal.timeout(options.readyTimeoutMs ?? 2000),
      });
      await r.body?.cancel();
      if (!r.ok) {
        return c.text("upstream not ready", 503, text);
      }
      const v = await upstreamVersion(config.apiUpstream, options.readyTimeoutMs ?? 2000);
      if (v !== undefined && isOlder(v, config.minServer)) {
        return c.text("upstream too old", 503, text);
      }
      return c.text("ready", 200, text);
    } catch {
      return c.text("upstream unreachable", 503, text);
    }
  });

  app.get("/version", (c) =>
    c.json(
      {
        studio: options.studioVersion ?? "0.0.0",
        minServer: config.minServer,
        ...(config.docsUrl === "" ? {} : { docs: config.docsUrl }),
      },
      200,
      { "Cache-Control": "no-store" },
    ),
  );

  registerAuthRoutes(app, { upstream: config.apiUpstream, https, stateKey, limiter, ip });
  const telemetryDropped = registerTelemetry(app, {
    config,
    limiter: telemetryLimiter,
    ip,
    log: options.logLine ?? stdoutLine,
    random: options.random ?? Math.random,
  });

  app.post("/csp-report", async (c) => {
    if (!reportLimiter.allow(ip(c.req.raw))) {
      return c.body(null, 429, { "Retry-After": "60" });
    }
    const len = Number(c.req.header("content-length") ?? "0");
    if (len > maxReportBytes) {
      return c.body(null, 413);
    }
    const body = await c.req.arrayBuffer();
    if (body.byteLength > maxReportBytes) {
      return c.body(null, 413);
    }
    reports += 1;
    return c.body(null, 204);
  });

  app.all("/api/*", async (c) => {
    if (draining) {
      return c.text("draining", 503, { ...text, "Retry-After": "5" });
    }
    const req = c.req.raw;
    const session = cookieValue(req, "__Host-plux_session");
    const key = session === "" ? `ip:${ip(req)}` : `s:${await sha256Hex(session)}`;
    if (!(await limiter.allow(key)) || !(await limiter.allow(`ip:${ip(req)}`))) {
      return c.text("rate limit exceeded", 429, { ...text, "Retry-After": "60" });
    }
    let release = () => {};
    if (isStream(req)) {
      const got = streams.acquire(key);
      if (got === undefined) {
        return c.text("too many open streams", 429, { ...text, "Retry-After": "5" });
      }
      release = got;
    }
    const url = new URL(req.url);
    return proxyRequest(req, config.apiUpstream, url.pathname.slice("/api".length) + url.search, release);
  });

  app.get("/", async (c) => serveIndex(c.get("nonce")));
  app.get("/index.html", async (c) => serveIndex(c.get("nonce")));
  function serveIndex(nonce: string): Response {
    try {
      const html = readFileSync(join(options.staticDir, "index.html"), "utf8");
      return new Response(injectNonce(html, nonce), {
        headers: { "Content-Type": "text/html; charset=UTF-8", "Cache-Control": "no-store" },
      });
    } catch {
      return new Response("not found", { status: 404, headers: text });
    }
  }
  app.use("/assets/*", async (c, next) => {
    await next();
    if (c.res.status === 200) {
      c.res.headers.set("Cache-Control", "public, max-age=31536000, immutable");
    }
  });
  app.use("/*", brotliStatic(options.staticDir));
  app.use("/*", serveStatic({ root }));
  return {
    app,
    cspReports: () => reports,
    telemetryDropped,
    openStreams: () => streams.open,
    startDrain: () => {
      draining = true;
    },
  };
}

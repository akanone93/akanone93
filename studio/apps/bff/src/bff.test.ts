// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterAll, beforeAll, describe, expect, test } from "bun:test";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createBff } from "./app.ts";
import { type BffConfig, ConfigError, decodeStateSecret, loadConfig } from "./config.ts";
import { MemoryRateLimiter, StreamCounter } from "./limiter.ts";
import { filterRequestHeaders, filterResponseHeaders } from "./proxy.ts";

let dir = "";
const SECRET = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY";
const KEY = new TextEncoder().encode("0123456789abcdef0123456789abcdef");
let upstream: ReturnType<typeof Bun.serve>;
let lastHeaders: Headers = new Headers();
let upstreamAborted = false;
let ready = true;

beforeAll(() => {
  dir = mkdtempSync(join(tmpdir(), "plux-bff-"));
  writeFileSync(join(dir, "index.html"), '<html><script type="module" src="/a.js"></script><style>x{}</style></html>');
  writeFileSync(join(dir, "hello.txt"), "hello");
  upstream = Bun.serve({
    port: 0,
    fetch(req) {
      const url = new URL(req.url);
      lastHeaders = req.headers;
      if (url.pathname === "/readyz") {
        return new Response(ready ? "ok" : "no", { status: ready ? 200 : 503 });
      }
      if (url.pathname === "/stream") {
        const enc = new TextEncoder();
        let i = 0;
        const body = new ReadableStream<Uint8Array>({
          async pull(c) {
            await Bun.sleep(30);
            c.enqueue(enc.encode(`chunk${i}\n`));
            i += 1;
            if (i === 3) {
              c.close();
            }
          },
        });
        return new Response(body, { headers: { "Content-Type": "text/plain", "X-Secret": "leak" } });
      }
      if (url.pathname === "/hang") {
        req.signal.addEventListener("abort", () => {
          upstreamAborted = true;
        });
        return new Response(
          new ReadableStream<Uint8Array>({
            start(c) {
              c.enqueue(new TextEncoder().encode("x"));
            },
          }),
          { headers: { "Content-Type": "text/plain" } },
        );
      }
      const h = new Headers({
        "Content-Type": "application/json",
        "X-Internal": "1",
        "Connect-Content-Encoding": "identity",
      });
      h.append("Set-Cookie", "__Host-plux_session=abc; HttpOnly; Secure; Path=/");
      h.append("Set-Cookie", "other=1");
      return new Response("{}", { headers: h });
    },
  });
});

afterAll(() => {
  upstream.stop(true);
  rmSync(dir, { recursive: true, force: true });
});

function cfg(over: Partial<BffConfig> = {}): BffConfig {
  return {
    publicBaseUrl: "https://studio.example.com",
    apiUpstream: `http://127.0.0.1:${upstream.port}`,
    host: "127.0.0.1",
    port: 0,
    drainTimeoutMs: 30000,
    telemetryEnabled: true,
    telemetrySampleRate: 1,
    maxStreamConnections: 1000,
    requestsPerMinute: 1000,
    streamsPerSession: 5,
    telemetryEventsPerMinute: 60,
    minServer: "0.0",
    docsUrl: "",
    valkeyUrl: "",
    trustedProxies: [],
    stateSecret: SECRET,
    ...over,
  };
}

const mk = (over: Partial<BffConfig> = {}) => createBff({ staticDir: dir, config: cfg(over), stateKey: KEY });

describe("[SEC-101] configuration", () => {
  const base = {
    PLUX_BFF_STATE_SECRET: SECRET,
    PLUX_BFF_PUBLIC_BASE_URL: "https://s.example.com",
    PLUX_BFF_API_UPSTREAM: "http://api:8080",
  };
  test("the state secret is required and must be 32 bytes of base64 (PLUX_BFF_STATE_SECRET)", () => {
    const { PLUX_BFF_STATE_SECRET: _omit, ...without } = base;
    expect(() => loadConfig(without)).toThrow(ConfigError);
    expect(() => loadConfig(without)).toThrow("stateSecret is required");
    expect(() => loadConfig({ ...base, PLUX_BFF_STATE_SECRET: "c2hvcnQ" })).toThrow("32 bytes");
    expect(() => loadConfig({ ...base, PLUX_BFF_STATE_SECRET: "not base64!!" })).toThrow(ConfigError);
    expect(decodeStateSecret(SECRET)?.length).toBe(32);
  });
  test("defaults come from the registry and the drain is 30 s", () => {
    const { config, warnings } = loadConfig(base);
    expect(config.drainTimeoutMs).toBe(30000);
    expect(config.requestsPerMinute).toBeGreaterThan(0);
    expect(config.port).toBe(8081);
    expect(warnings).toEqual([]);
  });
  test("a JSON file is read and the environment wins", () => {
    const { config } = loadConfig(
      { ...base, PLUX_BFF_PORT: "9000" },
      JSON.stringify({ port: 1, drainTimeout: "2m", telemetryEnabled: false }),
    );
    expect(config.port).toBe(9000);
    expect(config.drainTimeoutMs).toBe(120000);
    expect(config.telemetryEnabled).toBe(false);
  });
  test("invalid values fail fast with a clear message", () => {
    expect(() => loadConfig({})).toThrow(ConfigError);
    expect(() => loadConfig({ ...base, PLUX_BFF_PUBLIC_BASE_URL: "nope" })).toThrow("not a valid URL");
    expect(() => loadConfig({ ...base, PLUX_BFF_PUBLIC_BASE_URL: "ftp://x" })).toThrow("http(s)");
    expect(() => loadConfig({ ...base, PLUX_BFF_API_UPSTREAM: undefined })).toThrow("required");
    expect(() => loadConfig({ ...base, PLUX_BFF_DRAIN_TIMEOUT: "soon" })).toThrow("drainTimeout");
    expect(() => loadConfig({ ...base, PLUX_BFF_PORT: "0" })).toThrow("port");
    expect(() => loadConfig({ ...base, PLUX_BFF_PORT: "70000" })).toThrow("65535");
    expect(() => loadConfig({ ...base, PLUX_BFF_TELEMETRY_SAMPLE_RATE: "2" })).toThrow("telemetrySampleRate");
    expect(() => loadConfig(base, "[1]")).toThrow("JSON object");
    expect(() => loadConfig(base, "{")).toThrow("JSON object");
  });
  test("unsafe values warn and credentials are stripped", () => {
    const a = loadConfig({ ...base, PLUX_BFF_PUBLIC_BASE_URL: "http://studio.example.com" });
    expect(a.warnings[0]).toContain("UNSAFE");
    const b = loadConfig({ ...base, PLUX_BFF_API_UPSTREAM: "http://u:p@api:8080" });
    expect(b.warnings[0]).toContain("credentials");
    expect(b.config.apiUpstream).toBe("http://api:8080");
    expect(loadConfig({ ...base, PLUX_BFF_PUBLIC_BASE_URL: "http://localhost:5173" }).warnings).toEqual([]);
  });
});

describe("[SEC-101] [STU-002] security headers", () => {
  test("every response carries the headers and a fresh nonce in index.html", async () => {
    const { app } = mk();
    const r1 = await app.request("/");
    const r2 = await app.request("/index.html");
    const csp1 = r1.headers.get("content-security-policy") ?? "";
    const csp2 = r2.headers.get("content-security-policy") ?? "";
    const n1 = /'nonce-([^']+)'/.exec(csp1)?.[1] ?? "";
    const n2 = /'nonce-([^']+)'/.exec(csp2)?.[1] ?? "";
    expect(n1).not.toBe("");
    expect(n1).not.toBe(n2);
    const html = await r1.text();
    expect(html).toContain(`<script nonce="${n1}" type="module"`);
    expect(html).toContain(`<style nonce="${n1}">`);
    expect(r1.headers.get("cache-control")).toBe("no-store");
    for (const d of [
      "'strict-dynamic'",
      "object-src 'none'",
      "base-uri 'none'",
      "frame-ancestors 'none'",
      "require-trusted-types-for 'script'",
      "trusted-types plux-worker 'allow-duplicates'",
      "connect-src 'self'",
    ]) {
      expect(csp1).toContain(d);
    }
    expect(csp1).not.toContain("'unsafe-eval'");
    // Only style attributes may be inline (Monaco, ADR-0014 Revision 2026-10-26); elements stay nonce-only.
    const directives = csp1.split("; ");
    expect(directives).toContain(`style-src 'self' 'nonce-${n1}'`);
    expect(directives).toContain("style-src-attr 'unsafe-inline'");
    expect(directives.filter((d) => d.includes("'unsafe-inline'"))).toEqual(["style-src-attr 'unsafe-inline'"]);
    for (const path of ["/livez", "/missing", "/hello.txt"]) {
      const h = (await app.request(path)).headers;
      expect(h.get("strict-transport-security")).toContain("max-age");
      expect(h.get("x-content-type-options")).toBe("nosniff");
      expect(h.get("referrer-policy")).toBe("no-referrer");
      expect(h.get("cross-origin-opener-policy")).toBe("same-origin");
      expect(h.get("cross-origin-resource-policy")).toBe("same-origin");
      expect(h.get("cross-origin-embedder-policy")).toBe("require-corp");
      expect(h.get("permissions-policy")).toContain("camera=()");
    }
  });
  test("the CSP is exactly the documented policy, on every kind of response [SEC-109]", async () => {
    const { app } = mk();
    const responses = [
      await app.request("/"),
      await app.request("/hello.txt"),
      await app.request("/assets/none.js"),
      await app.request("/api/plux.v1.X/Y", { method: "POST", body: "{}" }),
      await app.request("/csp-report", { method: "POST", body: "{}" }),
      await app.request("/auth/oidc/state", { method: "POST", body: "{}" }),
      await app.request("/version"),
    ];
    for (const r of responses) {
      const csp = r.headers.get("content-security-policy") ?? "";
      const nonce = /'nonce-([A-Za-z0-9+/]{22}==)'/.exec(csp)?.[1] ?? "";
      expect(nonce).not.toBe("");
      // Any change to the policy is a change to a security control: it is made here and in
      // studio-development.md §11.2 and docs/security/asvs-studio.md together.
      expect(csp.split("; ")).toEqual([
        "default-src 'none'",
        `script-src 'nonce-${nonce}' 'strict-dynamic' 'wasm-unsafe-eval'`,
        `style-src 'self' 'nonce-${nonce}'`,
        "style-src-attr 'unsafe-inline'",
        "img-src 'self' data: blob:",
        "font-src 'self'",
        "connect-src 'self'",
        "worker-src 'self'",
        "manifest-src 'self'",
        "object-src 'none'",
        "base-uri 'none'",
        "form-action 'self'",
        "frame-ancestors 'none'",
        "require-trusted-types-for 'script'",
        "trusted-types plux-worker 'allow-duplicates'",
        "report-uri /csp-report",
      ]);
      for (const unsafe of ["'unsafe-eval'", "'unsafe-hashes'", "*", "http:", "https:", "'default'"]) {
        expect(csp.split(/[ ;]+/)).not.toContain(unsafe);
      }
    }
  });
  test("no HSTS over plain http; a missing index.html is 404", async () => {
    const res = await createBff({
      staticDir: join(dir, "none"),
      config: cfg({ publicBaseUrl: "http://localhost:1" }),
      stateKey: KEY,
    }).app.request("/");
    expect(res.status).toBe(404);
    expect(res.headers.get("strict-transport-security")).toBeNull();
  });
  test("an unknown asset is not marked immutable", async () => {
    const res = await mk().app.request("/assets/none.js");
    expect(res.headers.get("cache-control") ?? "").not.toContain("immutable");
  });
  test("CSP reports are counted, bounded and never echoed", async () => {
    const b = mk();
    const ok = await b.app.request("/csp-report", { method: "POST", body: '{"secret":"x"}' });
    expect(ok.status).toBe(204);
    expect(await ok.text()).toBe("");
    expect(b.cspReports()).toBe(1);
    const big = await b.app.request("/csp-report", { method: "POST", body: "x".repeat(20000) });
    expect(big.status).toBe(413);
    expect(b.cspReports()).toBe(1);
    let last = 0;
    for (let i = 0; i < 70; i++) {
      last = (await b.app.request("/csp-report", { method: "POST", body: "{}" })).status;
    }
    expect(last).toBe(429);
  });
});

describe("[STU-002] [SRV-007] API proxy", () => {
  test("forwards only allowed headers both ways", async () => {
    const res = await mk().app.request("/api/plux.v1.X/Y?a=1", {
      method: "POST",
      body: "{}",
      headers: {
        Cookie: "__Host-plux_session=abc",
        "X-CSRF-Token": "t",
        "X-Plux-Organization": "o",
        "Content-Type": "application/json",
        "Connect-Protocol-Version": "1",
        Authorization: "Bearer nope",
        "X-Evil": "1",
      },
    });
    expect(lastHeaders.get("cookie")).toBe("__Host-plux_session=abc");
    expect(lastHeaders.get("x-csrf-token")).toBe("t");
    expect(lastHeaders.get("x-plux-organization")).toBe("o");
    expect(lastHeaders.get("connect-protocol-version")).toBe("1");
    expect(lastHeaders.get("authorization")).toBeNull();
    expect(lastHeaders.get("x-evil")).toBeNull();
    expect(res.headers.get("x-internal")).toBeNull();
    expect(res.headers.get("connect-content-encoding")).toBe("identity");
    expect(res.headers.getSetCookie()).toEqual(["__Host-plux_session=abc; HttpOnly; Secure; Path=/"]);
  });
  test("header filters are exact", () => {
    expect(
      [
        ...filterRequestHeaders(
          new Headers({ "Idempotency-Key": "k", Traceparent: "t", Accept: "a", Host: "h" }),
        ).keys(),
      ].sort(),
    ).toEqual(["accept", "idempotency-key", "traceparent"]);
    expect(filterResponseHeaders(new Headers({ "Grpc-Status": "0", Server: "x" })).get("server")).toBeNull();
  });
  test("a chunked upstream response is streamed chunk by chunk", async () => {
    const res = await mk().app.request("/api/stream", { headers: { Accept: "text/event-stream" } });
    expect(res.headers.get("x-secret")).toBeNull();
    const reader = res.body?.getReader();
    if (reader === undefined) {
      throw new Error("no body");
    }
    const t0 = Date.now();
    const first = await reader.read();
    expect(new TextDecoder().decode(first.value)).toBe("chunk0\n");
    expect(Date.now() - t0).toBeLessThan(500);
    let rest = "";
    for (let r = await reader.read(); !r.done; r = await reader.read()) {
      rest += new TextDecoder().decode(r.value);
    }
    expect(rest).toBe("chunk1\nchunk2\n");
  });
  test("cancellation reaches the upstream and frees the stream slot", async () => {
    upstreamAborted = false;
    const b = mk();
    const res = await b.app.request("/api/hang", {
      headers: { Accept: "text/event-stream", Cookie: "__Host-plux_session=z" },
    });
    expect(b.openStreams()).toBe(1);
    await res.body?.cancel();
    for (let i = 0; i < 40 && !upstreamAborted; i++) {
      await Bun.sleep(25);
    }
    expect(upstreamAborted).toBe(true);
    expect(b.openStreams()).toBe(0);
  });
  test("an unreachable upstream answers 502", async () => {
    const res = await mk({ apiUpstream: "http://127.0.0.1:1" }).app.request("/api/x");
    expect(res.status).toBe(502);
  });
  test("an upstream with no body passes status through", async () => {
    const s = Bun.serve({ port: 0, fetch: () => new Response(null, { status: 204 }) });
    const res = await mk({ apiUpstream: `http://127.0.0.1:${s.port}` }).app.request("/api/x");
    expect(res.status).toBe(204);
    s.stop(true);
  });
});

describe("[SEC-101] limits", () => {
  test("requests over the per-minute limit answer 429 with Retry-After", async () => {
    const b = mk({ requestsPerMinute: 2 });
    const h = { Cookie: "__Host-plux_session=s1" };
    expect((await b.app.request("/api/x", { headers: h })).status).toBe(200);
    expect((await b.app.request("/api/x", { headers: h })).status).toBe(200);
    const r = await b.app.request("/api/x", { headers: h });
    expect(r.status).toBe(429);
    expect(r.headers.get("retry-after")).toBe("60");
  });
  test("streams per session are capped", async () => {
    const b = mk({ streamsPerSession: 1 });
    const h = { Accept: "text/event-stream", Cookie: "__Host-plux_session=s2" };
    const first = await b.app.request("/api/hang", { headers: h });
    const second = await b.app.request("/api/hang", { headers: h });
    expect(second.status).toBe(429);
    expect(second.headers.get("retry-after")).toBe("5");
    await first.body?.cancel();
  });
  test("limiter and counter units", () => {
    let t = 0;
    const l = new MemoryRateLimiter(1, () => t);
    expect(l.allow("a")).toBe(true);
    expect(l.allow("a")).toBe(false);
    t = 61000;
    expect(l.allow("a")).toBe(true);
    const c = new StreamCounter(1, 2);
    const rel = c.acquire("k");
    expect(c.acquire("k")).toBeUndefined();
    rel?.();
    rel?.();
    expect(c.open).toBe(0);
    const x = new StreamCounter(5, 1);
    expect(x.acquire("a")).toBeDefined();
    expect(x.acquire("b")).toBeUndefined();
  });
});

describe("[SRV-007] health and drain", () => {
  test("/livez is ok; /readyz reflects the upstream", async () => {
    const { app } = mk();
    expect((await app.request("/livez")).status).toBe(200);
    ready = true;
    expect((await app.request("/readyz")).status).toBe(200);
    ready = false;
    expect((await app.request("/readyz")).status).toBe(503);
    ready = true;
    const down = await mk({ apiUpstream: "http://127.0.0.1:1" }).app.request("/readyz");
    expect(down.status).toBe(503);
  });
  test("draining refuses new API calls and reports not ready", async () => {
    const b = mk();
    b.startDrain();
    expect((await b.app.request("/api/x")).status).toBe(503);
    expect((await b.app.request("/readyz")).status).toBe(503);
    expect((await b.app.request("/livez")).status).toBe(200);
  });
});

describe("session cookie forwarding [SEC-101]", () => {
  test("only __Host-plux_session reaches the upstream", () => {
    const out = filterRequestHeaders(new Headers({ cookie: "theme=dark; __Host-plux_session=s1; other=x" }));
    expect(out.get("cookie")).toBe("__Host-plux_session=s1");
  });

  test("a request without the session cookie sends no cookie", () => {
    const out = filterRequestHeaders(new Headers({ cookie: "theme=dark" }));
    expect(out.has("cookie")).toBe(false);
  });
});

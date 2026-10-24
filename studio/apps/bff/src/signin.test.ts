// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterAll, beforeAll, describe, expect, test } from "bun:test";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { brotliCompressSync } from "node:zlib";
import { createBff } from "./app.ts";
import { signState } from "./auth.ts";
import { loadConfig } from "./config.ts";
import { MemoryRateLimiter } from "./limiter.ts";
import { precompressForTest } from "./testutil.ts";
import { ValkeyRateLimiter } from "./valkey.ts";
import { isOlder, majorMinor } from "./version.ts";

const key = new TextEncoder().encode("0123456789abcdef0123456789abcdef");
let dir = "";
let upstream: ReturnType<typeof Bun.serve>;
let upstreamVersionValue: string | undefined = "1.4.0";
let calls = 0;

beforeAll(() => {
  dir = mkdtempSync(join(tmpdir(), "plux-bff2-"));
  mkdirSync(join(dir, "assets"));
  writeFileSync(join(dir, "assets", "a.js"), "console.log(1);");
  writeFileSync(join(dir, "assets", "a.js.br"), brotliCompressSync(Buffer.from("console.log(1);")));
  writeFileSync(join(dir, "plain.txt"), "plain");
  upstream = Bun.serve({
    port: 0,
    fetch(req) {
      const url = new URL(req.url);
      if (url.pathname === "/readyz") {
        return new Response("ok");
      }
      if (url.pathname === "/version") {
        return upstreamVersionValue === undefined
          ? new Response("no", { status: 404 })
          : Response.json({ version: upstreamVersionValue });
      }
      if (url.pathname.endsWith("/CompleteOidcLogin") || url.pathname.endsWith("/CompleteSamlLogin")) {
        calls += 1;
        const h = new Headers({ "Content-Type": "application/proto" });
        h.append("Set-Cookie", "__Host-plux_session=abc; Path=/; HttpOnly; Secure");
        h.append("Set-Cookie", "tracker=1; Path=/");
        return new Response(new Uint8Array(0), { headers: h });
      }
      return new Response(JSON.stringify({ code: "unauthenticated", message: "x" }), {
        status: 401,
        headers: { "Content-Type": "application/json" },
      });
    },
  });
});

afterAll(() => {
  upstream.stop(true);
  rmSync(dir, { recursive: true, force: true });
});

function build(extra: Record<string, string> = {}, opts: Partial<Parameters<typeof createBff>[0]> = {}) {
  const { config } = loadConfig({
    PLUX_BFF_PUBLIC_BASE_URL: "https://studio.example.com",
    PLUX_BFF_API_UPSTREAM: `http://127.0.0.1:${upstream.port}`,
    ...extra,
  });
  const lines: Record<string, unknown>[] = [];
  const bff = createBff({
    staticDir: dir,
    config,
    stateKey: key,
    studioVersion: "0.0.1",
    logLine: (l) => lines.push(l),
    clientIp: () => "10.0.0.9",
    ...opts,
  });
  return { bff, lines };
}

describe("STU-002 SEC-101 sign-in callbacks", () => {
  test("OIDC callback with a valid state passes only the session cookie and redirects", async () => {
    const { bff } = build();
    const cookie = await signState(key, "st1");
    const res = await bff.app.request("/auth/oidc/callback?code=c1&state=st1", {
      headers: { cookie: `__Host-plux_oidc=${cookie}` },
    });
    expect(res.status).toBe(303);
    expect(res.headers.get("location")).toBe("/");
    const set = res.headers.getSetCookie();
    expect(set.some((c) => c.startsWith("__Host-plux_session=abc"))).toBe(true);
    expect(set.some((c) => c.includes("tracker"))).toBe(false);
    expect(set.some((c) => c.startsWith("__Host-plux_oidc=;"))).toBe(true);
  });

  test("OIDC callback rejects a wrong, missing, forged or expired state without calling the server", async () => {
    const { bff } = build();
    const before = calls;
    const good = await signState(key, "st1");
    const expired = await signState(key, "st1", Date.now() - 3600_000);
    const cases: [string, string][] = [
      ["/auth/oidc/callback?code=c&state=other", `__Host-plux_oidc=${good}`],
      ["/auth/oidc/callback?code=c&state=st1", ""],
      ["/auth/oidc/callback?code=c&state=st1", `__Host-plux_oidc=${good}x`],
      ["/auth/oidc/callback?code=c&state=st1", `__Host-plux_oidc=${expired}`],
      ["/auth/oidc/callback?state=st1", `__Host-plux_oidc=${good}`],
    ];
    for (const [url, cookie] of cases) {
      const res = await bff.app.request(url, { headers: cookie === "" ? {} : { cookie } });
      expect(res.status).toBe(400);
      expect(await res.text()).not.toContain("st1");
    }
    expect(calls).toBe(before);
  });

  test("the state endpoint sets a signed HttpOnly __Host- cookie", async () => {
    const { bff } = build();
    const res = await bff.app.request("/auth/oidc/state", { method: "POST", body: JSON.stringify({ state: "s9" }) });
    expect(res.status).toBe(204);
    const c = res.headers.get("set-cookie") ?? "";
    expect(c).toContain("__Host-plux_oidc=");
    expect(c).toContain("HttpOnly");
    expect(c).toContain("Secure");
    const bad = await bff.app.request("/auth/oidc/state", { method: "POST", body: "{}" });
    expect(bad.status).toBe(400);
  });

  test("OIDC callback reports a server rejection as a plain error page", async () => {
    const { bff } = build();
    const cookie = await signState(key, "st2");
    // The mock rejects every other path; point the BFF at a server that does.
    const { config } = loadConfig({
      PLUX_BFF_PUBLIC_BASE_URL: "https://studio.example.com",
      PLUX_BFF_API_UPSTREAM: "http://127.0.0.1:1",
    });
    const dead = createBff({ staticDir: dir, config, stateKey: key });
    const res = await dead.app.request("/auth/oidc/callback?code=c&state=st2", {
      headers: { cookie: `__Host-plux_oidc=${cookie}` },
    });
    expect(res.status).toBe(400);
    expect(res.headers.get("content-security-policy")).toContain("default-src 'none'");
    expect(bff).toBeDefined();
  });

  test("SAML ACS passes the session cookie through and enforces the body limit", async () => {
    const { bff } = build();
    const ok = await bff.app.request("/auth/saml/acs?provider=p1", {
      method: "POST",
      headers: { "content-type": "application/x-www-form-urlencoded" },
      body: new URLSearchParams({ SAMLResponse: "PHNhbWw+", RelayState: "r" }).toString(),
    });
    expect(ok.status).toBe(303);
    expect(ok.headers.getSetCookie().some((c) => c.startsWith("__Host-plux_session=abc"))).toBe(true);
    expect(ok.headers.getSetCookie().some((c) => c.includes("tracker"))).toBe(false);
    const big = await bff.app.request("/auth/saml/acs", {
      method: "POST",
      body: `SAMLResponse=${"A".repeat(300000)}`,
    });
    expect(big.status).toBe(413);
    const none = await bff.app.request("/auth/saml/acs", { method: "POST", body: "RelayState=x" });
    expect(none.status).toBe(400);
  });

  test("sign-in endpoints are rate limited", async () => {
    const { bff } = build({}, { limiter: new MemoryRateLimiter(2) });
    const codes: number[] = [];
    for (let i = 0; i < 4; i++) {
      codes.push((await bff.app.request("/auth/oidc/callback?code=c&state=s")).status);
    }
    expect(codes.at(-1)).toBe(429);
  });
});

describe("OBS-001 telemetry endpoint", () => {
  const post = (bff: ReturnType<typeof build>["bff"], body: unknown, headers: Record<string, string> = {}) =>
    bff.app.request("/telemetry", {
      method: "POST",
      body: typeof body === "string" ? body : JSON.stringify(body),
      headers,
    });

  test("accepts a valid batch, logs names only and forwards traceparent", async () => {
    const { bff, lines } = build();
    const tp = "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01";
    const res = await post(
      bff,
      {
        events: [
          { name: "page.load", kind: "timing", durationMs: 120, route: "/apps" },
          { name: "js.error", kind: "error", code: "type_error" },
        ],
      },
      { traceparent: tp, cookie: "__Host-plux_session=secret" },
    );
    expect(res.status).toBe(202);
    expect(lines).toHaveLength(2);
    expect((lines[0] as { traceparent?: string } | undefined)?.traceparent).toBe(tp);
    expect(JSON.stringify(lines)).not.toContain("secret");
  });

  test("rejects events that do not match the schema", async () => {
    const { bff, lines } = build();
    for (const bad of [
      "not json",
      {},
      { events: [] },
      { events: [{ name: "Bad Name", kind: "page" }] },
      { events: [{ name: "x", kind: "other" }] },
      { events: [{ name: "x", kind: "error", stack: "at foo" }] },
      { events: [{ name: "x", kind: "error", code: "has space" }] },
      { events: [{ name: "x", kind: "timing", durationMs: -1 }] },
      { events: [{ name: "x", kind: "page", route: "http://evil" }] },
      { events: Array.from({ length: 51 }, () => ({ name: "x", kind: "page" })) },
    ]) {
      expect((await post(bff, bad)).status).toBe(400);
    }
    expect((await bff.app.request("/telemetry", { method: "POST", body: "x".repeat(70000) })).status).toBe(413);
    expect(lines).toHaveLength(0);
  });

  test("samples by configuration and can be disabled", async () => {
    const { bff, lines } = build({ PLUX_BFF_TELEMETRY_SAMPLE_RATE: "0.5" }, { random: () => 0.9 });
    await post(bff, { events: [{ name: "x", kind: "page" }] });
    expect(lines).toHaveLength(0);
    const off = build({ PLUX_BFF_TELEMETRY_ENABLED: "false" });
    expect((await post(off.bff, { events: [{ name: "x", kind: "page" }] })).status).toBe(204);
    expect(off.lines).toHaveLength(0);
  });

  test("drops and counts events beyond the per-session limit", async () => {
    const { bff, lines } = build({}, { limiter: new MemoryRateLimiter(3) });
    const res = await post(bff, { events: Array.from({ length: 5 }, () => ({ name: "x", kind: "page" })) });
    expect(await res.json()).toEqual({ accepted: 3, dropped: 2 });
    expect(lines).toHaveLength(3);
    expect(bff.telemetryDropped()).toBe(2);
  });
});

describe("STU-002 version reporting and readiness", () => {
  test("GET /version reports Studio and the oldest supported server", async () => {
    const { bff } = build({ PLUX_BFF_MIN_SERVER: "1.2" });
    const res = await bff.app.request("/version");
    expect(await res.json()).toEqual({ studio: "0.0.1", minServer: "1.2" });
  });

  test("readyz fails when the upstream is older than minServer", async () => {
    const { bff } = build({ PLUX_BFF_MIN_SERVER: "1.4" });
    upstreamVersionValue = "1.4.2";
    expect((await bff.app.request("/readyz")).status).toBe(200);
    upstreamVersionValue = "1.3.9";
    expect((await bff.app.request("/readyz")).status).toBe(503);
    upstreamVersionValue = undefined;
    expect((await bff.app.request("/readyz")).status).toBe(200);
    upstreamVersionValue = "1.4.0";
  });

  test("version comparison", () => {
    expect(majorMinor("v2.1.0-rc1")).toEqual([2, 1]);
    expect(majorMinor("dev")).toBeUndefined();
    expect(isOlder("1.9.0", "2.0")).toBe(true);
    expect(isOlder("dev", "2.0")).toBe(false);
  });
});

describe("STU-002 Brotli static files", () => {
  test("serves the precompressed sibling when the client accepts br", async () => {
    const { bff } = build();
    const res = await bff.app.request("/assets/a.js", { headers: { "accept-encoding": "gzip, br" } });
    expect(res.headers.get("content-encoding")).toBe("br");
    expect(res.headers.get("vary")).toContain("Accept-Encoding");
    expect(res.headers.get("content-type")).toContain("javascript");
    const plain = await bff.app.request("/assets/a.js", { headers: { "accept-encoding": "gzip" } });
    expect(plain.headers.get("content-encoding")).toBeNull();
    expect(await plain.text()).toBe("console.log(1);");
    const none = await bff.app.request("/plain.txt", { headers: { "accept-encoding": "br" } });
    expect(none.headers.get("content-encoding")).toBeNull();
    expect(precompressForTest(dir)).toBeGreaterThan(0);
  });
});

describe("SEC-101 client address and shared limits", () => {
  test("X-Forwarded-For is ignored unless the peer is a trusted proxy", async () => {
    const seen: string[] = [];
    const limiter = {
      allow: (k: string) => {
        seen.push(k);
        return true;
      },
    };
    const plain = build({}, { limiter });
    await plain.bff.app.request("/api/x", { headers: { "x-forwarded-for": "6.6.6.6" } });
    expect(seen.every((k) => k === "ip:10.0.0.9")).toBe(true);
    seen.length = 0;
    const proxied = build({ PLUX_BFF_TRUSTED_PROXIES: "10.0.0.0/24" }, { limiter });
    await proxied.bff.app.request("/api/x", { headers: { "x-forwarded-for": "6.6.6.6, 10.0.0.3" } });
    expect(seen.every((k) => k === "ip:6.6.6.6")).toBe(true);
  });

  test("Valkey limiter counts in a shared store and falls back locally on error", async () => {
    const counts = new Map<string, number>();
    const store = {
      incr: async (k: string) => {
        const n = (counts.get(k) ?? 0) + 1;
        counts.set(k, n);
        return n;
      },
      expire: async () => 1,
    };
    const l = new ValkeyRateLimiter(store, 2);
    expect([await l.allow("a"), await l.allow("a"), await l.allow("a")]).toEqual([true, true, false]);
    const broken = new ValkeyRateLimiter(
      { incr: async () => Promise.reject(new Error("down")), expire: async () => 0 },
      1,
    );
    expect([await broken.allow("a"), await broken.allow("a")]).toEqual([true, false]);
  });
});

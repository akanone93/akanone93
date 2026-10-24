// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Hono } from "hono";
import {
  authMessages,
  clearStateCookie,
  createLoginClient,
  maxAcsBytes,
  readState,
  signState,
  stateCookie,
  stateCookieHeader,
} from "./auth.ts";
import { cookieValue, text } from "./http.ts";
import type { RateLimiter } from "./limiter.ts";

export type AuthDeps = {
  upstream: string;
  https: boolean;
  stateKey: Uint8Array;
  limiter: RateLimiter;
  ip: (req: Request) => string;
};

/** The sign-in pages the server lacks: OIDC callback and SAML assertion consumer (L-13). */
export function registerAuthRoutes(app: Hono<{ Variables: { nonce: string } }>, deps: AuthDeps): void {
  const { https, stateKey, limiter, ip } = deps;
  const login = createLoginClient(deps.upstream);
  const limited = async (req: Request, prefix: string): Promise<boolean> =>
    !(await limiter.allow(`${prefix}:${ip(req)}`));
  const fail = (status: 400 | 413 | 429 | 502, msg: string) =>
    new Response(msg, { status, headers: { ...text, "Cache-Control": "no-store" } });
  const done = (cookies: string[]) => {
    const h = new Headers({ Location: "/", "Cache-Control": "no-store" });
    for (const c of cookies) {
      h.append("Set-Cookie", c);
    }
    h.append("Set-Cookie", clearStateCookie(https));
    return new Response(null, { status: 303, headers: h });
  };

  app.post("/auth/oidc/state", async (c) => {
    if (await limited(c.req.raw, "auth")) {
      return c.body(null, 429, { "Retry-After": "60" });
    }
    // Only Studio's own page may set the state it later checks: refusing cross-site requests
    // (and non-JSON bodies, which a cross-site form could send without a preflight) stops a
    // login CSRF that would plant an attacker's state (SEC-101).
    if (!sameOrigin(c.req.raw) || !(c.req.header("content-type") ?? "").startsWith("application/json")) {
      return fail(400, authMessages.badRequest);
    }
    const body = (await c.req.json().catch(() => ({}))) as { state?: unknown };
    if (typeof body.state !== "string" || body.state === "" || body.state.length > 512) {
      return fail(400, authMessages.badRequest);
    }
    return c.body(null, 204, { "Set-Cookie": stateCookieHeader(await signState(stateKey, body.state), https) });
  });

  app.get("/auth/oidc/callback", async (c) => {
    if (await limited(c.req.raw, "auth")) {
      return fail(429, authMessages.failed);
    }
    const url = new URL(c.req.url);
    const state = url.searchParams.get("state") ?? "";
    const code = url.searchParams.get("code") ?? "";
    const expected = await readState(stateKey, cookieValue(c.req.raw, stateCookie));
    if (state === "" || code === "" || expected === undefined || expected !== state) {
      return fail(400, authMessages.failed);
    }
    const r = await login.oidc(code, state);
    return r.ok ? done(r.cookies) : fail(400, authMessages.failed);
  });

  app.post("/auth/saml/acs", async (c) => {
    if (await limited(c.req.raw, "auth")) {
      return fail(429, authMessages.failed);
    }
    if (Number(c.req.header("content-length") ?? "0") > maxAcsBytes) {
      return fail(413, authMessages.tooLarge);
    }
    const raw = await c.req.arrayBuffer();
    if (raw.byteLength > maxAcsBytes) {
      return fail(413, authMessages.tooLarge);
    }
    const form = new URLSearchParams(new TextDecoder().decode(raw));
    const response = form.get("SAMLResponse") ?? "";
    if (response === "") {
      return fail(400, authMessages.badRequest);
    }
    const provider = new URL(c.req.url).searchParams.get("provider") ?? "";
    const r = await login.saml(provider, response, form.get("RelayState") ?? "");
    return r.ok ? done(r.cookies) : fail(400, authMessages.failed);
  });
}

/** A request from Studio's own origin: Fetch Metadata when the browser sends it, else Origin. */
export function sameOrigin(req: Request): boolean {
  const site = req.headers.get("sec-fetch-site");
  if (site !== null) {
    return site === "same-origin";
  }
  const origin = req.headers.get("origin");
  return origin !== null && origin === new URL(req.url).origin;
}

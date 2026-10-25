// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { create } from "@bufbuild/protobuf";
import { createClient, type Transport } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import {
  CompleteOidcLoginRequestSchema,
  CompleteSamlLoginRequestSchema,
  FederationService,
  IdentityService,
} from "@plux/api";

export const stateCookie = "__Host-plux_oidc";
const sessionCookie = "__Host-plux_session=";
export const stateTtlSeconds = 600;
export const maxAcsBytes = 262144;

/** Fixed messages; nothing from the request is reflected (SEC-101). */
export const authMessages = {
  failed: "Sign-in failed. Return to Plux Studio and try again.",
  tooLarge: "The sign-in response was too large.",
  badRequest: "The sign-in request was not valid.",
} as const;

async function hmac(key: Uint8Array, data: string): Promise<string> {
  const k = await crypto.subtle.importKey(
    "raw",
    key as Uint8Array<ArrayBuffer>,
    { name: "HMAC", hash: "SHA-256" },
    false,
    ["sign"],
  );
  const sig = await crypto.subtle.sign("HMAC", k, new TextEncoder().encode(data));
  return Buffer.from(sig).toString("base64url");
}

function equal(a: string, b: string): boolean {
  if (a.length !== b.length) {
    return false;
  }
  let d = 0;
  for (let i = 0; i < a.length; i++) {
    d |= a.charCodeAt(i) ^ b.charCodeAt(i);
  }
  return d === 0;
}

/** Signs the OIDC state into a short-lived cookie value: state.expiry.mac. */
export async function signState(key: Uint8Array, state: string, nowMs: number = Date.now()): Promise<string> {
  const body = `${Buffer.from(state).toString("base64url")}.${Math.floor(nowMs / 1000) + stateTtlSeconds}`;
  return `${body}.${await hmac(key, body)}`;
}

/** Returns the state a cookie value carries, or undefined when forged, expired or malformed. */
export async function readState(
  key: Uint8Array,
  value: string,
  nowMs: number = Date.now(),
): Promise<string | undefined> {
  const parts = value.split(".");
  if (parts.length !== 3) {
    return undefined;
  }
  const [s, exp, mac] = parts as [string, string, string];
  if (!equal(mac, await hmac(key, `${s}.${exp}`)) || Number(exp) * 1000 < nowMs) {
    return undefined;
  }
  return Buffer.from(s, "base64url").toString("utf8");
}

export function stateCookieHeader(value: string, https: boolean): string {
  return `${stateCookie}=${value}; Max-Age=${stateTtlSeconds}; Path=/; HttpOnly; SameSite=Lax${https ? "; Secure" : ""}`;
}

export function clearStateCookie(https: boolean): string {
  return `${stateCookie}=; Max-Age=0; Path=/; HttpOnly; SameSite=Lax${https ? "; Secure" : ""}`;
}

export type LoginResult = { ok: true; cookies: string[] } | { ok: false };

/**
 * One sign-in completion with its own transport: the session cookie is read from
 * this call's own HTTP response, never from state shared with a concurrent
 * completion (which could hand one person's session to another).
 */
async function completeLogin(upstream: string, call: (transport: Transport) => Promise<unknown>): Promise<LoginResult> {
  let cookies: string[] = [];
  const transport = createConnectTransport({
    baseUrl: upstream,
    useBinaryFormat: true,
    fetch: (async (input: string | URL | Request, init?: RequestInit) => {
      const res = await fetch(input, init);
      cookies = res.headers.getSetCookie().filter((c) => c.startsWith(sessionCookie));
      return res;
    }) as typeof fetch,
  });
  try {
    await call(transport);
  } catch {
    return { ok: false };
  }
  return cookies.length > 0 ? { ok: true, cookies } : { ok: false };
}

/** Calls the server's sign-in completion RPCs; the server sets the session cookie. */
export function createLoginClient(upstream: string) {
  return {
    oidc(code: string, state: string): Promise<LoginResult> {
      return completeLogin(upstream, (t) =>
        createClient(IdentityService, t).completeOidcLogin(create(CompleteOidcLoginRequestSchema, { code, state })),
      );
    },
    saml(providerId: string, samlResponse: string, relayState: string): Promise<LoginResult> {
      return completeLogin(upstream, (t) =>
        createClient(FederationService, t).completeSamlLogin(
          create(CompleteSamlLoginRequestSchema, { providerId, samlResponse, relayState }),
        ),
      );
    },
  };
}

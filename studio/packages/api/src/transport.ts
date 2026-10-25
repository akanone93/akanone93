// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Interceptor } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";

/** Header names fixed by backend/internal/api/authn.go. */
export const CSRF_HEADER = "X-CSRF-Token";
export const ORGANIZATION_HEADER = "X-Plux-Organization";

export interface StudioTransportOptions {
  baseUrl: string;
  /** The session's CSRF token, read from memory on every call (SEC-101). */
  csrfToken: () => string | undefined;
  /** The active organisation, read on every call. */
  organization: () => string | undefined;
}

/** Sets the CSRF and organisation headers when a value is present. */
export function headerInterceptor(options: Pick<StudioTransportOptions, "csrfToken" | "organization">): Interceptor {
  return (next) => (req) => {
    const csrf = options.csrfToken();
    if (csrf) {
      req.header.set(CSRF_HEADER, csrf);
    }
    const organization = options.organization();
    if (organization) {
      req.header.set(ORGANIZATION_HEADER, organization);
    }
    return next(req);
  };
}

/**
 * Binary Connect transport to the BFF on Studio's own origin. The session cookie is
 * sent same-origin only; no token is stored here (SEC-101).
 */
export function createStudioTransport(options: StudioTransportOptions) {
  return createConnectTransport({
    baseUrl: options.baseUrl,
    useBinaryFormat: true,
    fetch: ((input, init) => globalThis.fetch(input, keepaliveInit(init))) as typeof fetch,
    interceptors: [headerInterceptor(options)],
  });
}

/**
 * A call that must outlive the page (the autosave flush on tab hide) asks for `keepalive` through
 * this header, since Connect call options do not reach fetch's init; the header is removed again.
 */
export const keepaliveHeader = "plux-keepalive";

function keepaliveInit(init: RequestInit | undefined): RequestInit {
  const headers = new Headers(init?.headers);
  const keepalive = headers.has(keepaliveHeader);
  headers.delete(keepaliveHeader);
  return { ...init, headers, credentials: "same-origin", ...(keepalive ? { keepalive: true } : {}) };
}

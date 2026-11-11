// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

const OBJECTS = "/v1/objects/";

/**
 * Where Studio reads a stored object, such as an asset's file, a font or a screen thumbnail. The server hands out
 * object URLs under its own public address (`/v1/objects/…`, or `objectStorage.cdnBaseURL` in front of it), which is
 * another origin than Studio's, and Studio's CSP (`img-src`, `connect-src 'self'`) and COEP refuse it. The BFF
 * proxies `/api/*` to the server, so the same content-addressed object is read same-origin at `/api/v1/objects/…`.
 * A URL of another shape (a signed object-store URL, a CDN path of its own) is returned unchanged.
 */
export function objectUrl(url: string): string {
  let parsed: URL;
  try {
    parsed = new URL(url, "http://studio.invalid");
  } catch {
    return url;
  }
  if (!parsed.pathname.startsWith(OBJECTS) || parsed.pathname.includes("..")) {
    return url;
  }
  return `/api${parsed.pathname}${parsed.search}`;
}

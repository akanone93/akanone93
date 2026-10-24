// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

export type SecurityOptions = { https: boolean };

/** Studio registers no Trusted Types policy of its own; `default` is never allowed. */
export const trustedTypePolicies: readonly string[] = [];

export function newNonce(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  return btoa(String.fromCharCode(...bytes));
}

export function contentSecurityPolicy(nonce: string): string {
  const tt = trustedTypePolicies.length === 0 ? "'none'" : trustedTypePolicies.join(" ");
  return [
    "default-src 'none'",
    `script-src 'nonce-${nonce}' 'strict-dynamic' 'wasm-unsafe-eval'`,
    `style-src 'self' 'nonce-${nonce}'`,
    "img-src 'self' data: blob:",
    "font-src 'self'",
    "connect-src 'self'",
    "worker-src 'self' blob:",
    "manifest-src 'self'",
    "object-src 'none'",
    "base-uri 'none'",
    "form-action 'self'",
    "frame-ancestors 'none'",
    "require-trusted-types-for 'script'",
    `trusted-types ${tt}`,
    "report-uri /csp-report",
  ].join("; ");
}

export function securityHeaders(nonce: string, opts: SecurityOptions): Record<string, string> {
  const h: Record<string, string> = {
    "Content-Security-Policy": contentSecurityPolicy(nonce),
    "X-Content-Type-Options": "nosniff",
    "Referrer-Policy": "no-referrer",
    "Cross-Origin-Opener-Policy": "same-origin",
    "Cross-Origin-Resource-Policy": "same-origin",
    "Permissions-Policy": "camera=(), microphone=(), geolocation=(), payment=(), usb=(), interest-cohort=()",
  };
  if (opts.https) {
    h["Strict-Transport-Security"] = "max-age=63072000; includeSubDomains";
  }
  return h;
}

/** Adds the nonce to every script and inline style tag of an HTML document. */
export function injectNonce(html: string, nonce: string): string {
  return html.replace(
    /<(script|style)(?![^>]*\snonce=)([\s>])/gi,
    (_m, tag: string, tail: string) => `<${tag} nonce="${nonce}"${tail}`,
  );
}

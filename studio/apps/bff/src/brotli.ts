// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { existsSync } from "node:fs";
import { join, normalize } from "node:path";
import type { MiddlewareHandler } from "hono";

function brotliSibling(staticDir: string, pathname: string): string | undefined {
  const rel = normalize(decodeURIComponent(pathname)).replace(/^\/+/, "");
  if (rel === "" || rel.startsWith("..") || rel.endsWith("/") || !existsSync(join(staticDir, `${rel}.br`))) {
    return undefined;
  }
  return rel;
}

/** Serves a precompressed .br sibling when the client accepts Brotli; always varies on Accept-Encoding. */
export function brotliStatic(staticDir: string): MiddlewareHandler {
  return async (c, next) => {
    const wantsBr = /\bbr\b/.test(c.req.header("accept-encoding") ?? "");
    const rel =
      wantsBr && (c.req.method === "GET" || c.req.method === "HEAD")
        ? brotliSibling(staticDir, new URL(c.req.url).pathname)
        : undefined;
    if (rel !== undefined) {
      return new Response(Bun.file(join(staticDir, `${rel}.br`)).stream(), {
        headers: {
          "Content-Type": Bun.file(join(staticDir, rel)).type,
          "Content-Encoding": "br",
          Vary: "Accept-Encoding",
          ...(rel.startsWith("assets/") ? { "Cache-Control": "public, max-age=31536000, immutable" } : {}),
        },
      });
    }
    await next();
    c.res.headers.append("Vary", "Accept-Encoding");
    return undefined;
  };
}

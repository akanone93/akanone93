// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { relative } from "node:path";
import { Hono } from "hono";
import { serveStatic } from "hono/bun";

export type AppOptions = {
  /** Directory holding the built web application. */
  staticDir: string;
};

/** Builds the BFF: the liveness probe and the static web application. */
export function createApp(options: AppOptions): Hono {
  const app = new Hono();
  // serveStatic resolves its root against the working directory.
  const root = relative(process.cwd(), options.staticDir) || ".";

  app.get("/livez", (c) => c.text("ok", 200, { "Content-Type": "text/plain; charset=UTF-8" }));
  app.use("/*", serveStatic({ root }));
  return app;
}

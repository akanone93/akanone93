// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { readFileSync } from "node:fs";
import { lingui } from "@lingui/vite-plugin";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig, type Plugin } from "vite";

const pkg = JSON.parse(readFileSync(new URL("./package.json", import.meta.url), "utf-8")) as { version: string };

const sonnerPinned = "2.0.8";
const insertCssHead = "function __insertCSS(code) {";

/**
 * sonner injects its stylesheet through a nonce-less <style> element, which the strict CSP blocks
 * (rule book §11.2). Its CSS ships through app.css instead (`sonner/dist/styles.css`), and this
 * transform turns the runtime injection into a no-op. Pinned to one sonner version: any other
 * version, or a changed code shape, fails the build so an upgrade cannot silently regress.
 */
function sonnerNoStyleInjection(): Plugin {
  return {
    name: "plux:sonner-no-style-injection",
    enforce: "pre",
    transform(code, id) {
      if (!/[\\/]sonner[\\/]dist[\\/]index\.mjs$/.test(id)) {
        return null;
      }
      const version = (
        JSON.parse(readFileSync(id.replace(/dist[\\/]index\.mjs$/, "package.json"), "utf-8")) as { version: string }
      ).version;
      if (version !== sonnerPinned || code.split(insertCssHead).length !== 2) {
        throw new Error(
          `sonner ${version}: expected exactly one "${insertCssHead}" in sonner ${sonnerPinned}; review the CSP transform in vite.config.ts and the sonner styles import in app.css`,
        );
      }
      return { code: code.replace(insertCssHead, `${insertCssHead}\n  return;`), map: null };
    },
  };
}

export default defineConfig({
  plugins: [sonnerNoStyleInjection(), lingui(), react(), tailwindcss()],
  build: {
    target: "es2023",
    sourcemap: true,
    manifest: true,
    outDir: "dist",
    emptyOutDir: true,
  },
  worker: { format: "es" },
  define: { __STUDIO_VERSION__: JSON.stringify(pkg.version) },
  server: {
    host: "127.0.0.1",
    // In development the BFF runs beside Vite (`make studio-dev`): its routes are forwarded to it, so
    // the dev app goes through the same session, CSRF and proxy rules as production (STU-002). The
    // Host header is kept, so the BFF's same-origin checks see Vite's origin.
    proxy: Object.fromEntries(
      ["/api", "/auth", "/version", "/telemetry", "/csp-report"].map((path) => [
        path,
        { target: process.env["PLUX_STUDIO_DEV_BFF"] ?? "http://127.0.0.1:8081", changeOrigin: false },
      ]),
    ),
  },
});

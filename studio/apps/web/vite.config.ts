// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { readFileSync } from "node:fs";
import { lingui } from "@lingui/vite-plugin";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

const pkg = JSON.parse(readFileSync(new URL("./package.json", import.meta.url), "utf-8")) as { version: string };

export default defineConfig({
  plugins: [lingui(), react(), tailwindcss()],
  build: {
    target: "es2023",
    sourcemap: true,
    manifest: true,
    outDir: "dist",
    emptyOutDir: true,
  },
  define: { __STUDIO_VERSION__: JSON.stringify(pkg.version) },
  server: { host: "127.0.0.1" },
});

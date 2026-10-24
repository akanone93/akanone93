// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { lingui } from "@lingui/vite-plugin";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [lingui(), react(), tailwindcss()],
  build: {
    target: "es2023",
    sourcemap: true,
    manifest: true,
    outDir: "dist",
    emptyOutDir: true,
  },
  server: { host: "127.0.0.1" },
});

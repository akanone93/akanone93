// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [react()],
  build: {
    target: "es2023",
    sourcemap: true,
    manifest: true,
    outDir: "dist",
    emptyOutDir: true,
  },
  server: { host: "127.0.0.1" },
});

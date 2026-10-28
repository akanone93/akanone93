// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { defineConfig, devices } from "@playwright/test";

// The ports can move (PLUX_E2E_PORT_BASE) when another run on the machine holds the defaults.
const base = Number(process.env["PLUX_E2E_PORT_BASE"] ?? 4173);
const previewPort = base;
const devPort = base + 1;
const bffPort = base + 2;

export default defineConfig({
  testDir: ".",
  fullyParallel: true,
  forbidOnly: true,
  reporter: [["list"]],
  projects: [
    {
      name: "preview",
      testMatch: "smoke.spec.ts",
      use: { ...devices["Desktop Chrome"], baseURL: `http://127.0.0.1:${previewPort}` },
    },
    {
      // The catalogue page is compiled only by the dev server (STU-011).
      name: "dev",
      testMatch: ["catalogue.spec.ts", "shell.spec.ts", "s3-flow.spec.ts"],
      use: {
        ...devices["Desktop Chrome"],
        baseURL: `http://127.0.0.1:${devPort}`,
        // The canvas renders with CanvasKit on WebGL; SwiftShader gives machines without a GPU the same pipeline (ADR-0013 M-12).
        launchOptions: {
          args: ["--use-gl=angle", "--use-angle=swiftshader", "--enable-unsafe-swiftshader", "--ignore-gpu-blocklist"],
        },
      },
    },
    {
      // The built app behind the BFF and its strict CSP.
      name: "csp",
      testMatch: ["csp.spec.ts", "json-editor.spec.ts", "action-graph.spec.ts", "rust-editor.spec.ts"],
      use: { ...devices["Desktop Chrome"], baseURL: `http://127.0.0.1:${bffPort}` },
    },
  ],
  webServer: [
    {
      command: "bun apps/bff/src/main.ts",
      cwd: "..",
      env: {
        PLUX_BFF_API_UPSTREAM: "http://127.0.0.1:9",
        PLUX_BFF_STATE_SECRET: "ZTJlLXRlc3Qtc3RhdGUtc2VjcmV0LW5vdC1mb3ItcHJvZHVjdGlvbg",
        PLUX_BFF_PORT: String(bffPort),
        PLUX_BFF_PUBLIC_BASE_URL: `http://127.0.0.1:${bffPort}`,
      },
      url: `http://127.0.0.1:${bffPort}/livez`,
      reuseExistingServer: false,
      timeout: 60_000,
    },
    {
      // The app is built first (`make studio-e2e`); the preview serves that build.
      command: `bun run --filter @plux/web preview --host 127.0.0.1 --port ${previewPort} --strictPort`,
      url: `http://127.0.0.1:${previewPort}`,
      reuseExistingServer: false,
      timeout: 60_000,
    },
    {
      command: `bun run --filter @plux/web dev --host 127.0.0.1 --port ${devPort} --strictPort`,
      url: `http://127.0.0.1:${devPort}`,
      reuseExistingServer: false,
      timeout: 60_000,
    },
  ],
});

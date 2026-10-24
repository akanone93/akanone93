// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { defineConfig, devices } from "@playwright/test";

const previewPort = 4173;
const devPort = 4174;

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
      testMatch: "catalogue.spec.ts",
      use: { ...devices["Desktop Chrome"], baseURL: `http://127.0.0.1:${devPort}` },
    },
  ],
  webServer: [
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

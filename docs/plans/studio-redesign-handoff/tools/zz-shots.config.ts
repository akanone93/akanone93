import { defineConfig, devices } from "@playwright/test";
export default defineConfig({ testDir: ".", testMatch: "zz-shots.spec.ts", reporter: [["list"]], timeout: 120_000,
  use: { ...devices["Desktop Chrome"], viewport: { width: 1440, height: 900 }, baseURL: "http://127.0.0.1:8081" } });

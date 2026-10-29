// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// The design gate of the UI language (U18.6, decision O-4): the Impeccable detector scans the built
// app and the served catalogue page in each theme, so the element rules (text size, contrast, glow)
// run on the rendered DOM. Any finding fails the gate; the report is kept in design-gate-report.txt.
// The project config is studio/.impeccable/config.json. Three rules are off there because they flag
// the approved Porcelain design, not a defect (maintainer, 2026-10-26): `undersized-ui-text` (the 10px
// `type.tag`, U7), `dark-glow` (`shadow-primary`, U7) and `pulsing-dot` (the live-device dot, U10).
// The detector has no value-scoped ignore for them on a URL scan, so they are ignored by rule; the
// decision is listed under Open decisions in docs/WORKLOG.md to be reviewed, not forgotten.
// Monaco's bundled chunk (`monaco-view-*.js`) is skipped in the regex scan of dist: it is vendor
// code (an `<img>` template of its Markdown renderer), not Studio's markup.

import { chmodSync, mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { chromium } from "@playwright/test";

const port = 4176;
const themes = ["light", "dark", "contrast"] as const;

// Root has no Chromium sandbox; a wrapper adds the flag the way CI containers do.
let browser = chromium.executablePath();
if (process.getuid?.() === 0) {
  const wrapper = join(mkdtempSync(join(tmpdir(), "design-gate-")), "chrome");
  writeFileSync(wrapper, `#!/bin/sh\nexec "${browser}" --no-sandbox "$@"\n`);
  chmodSync(wrapper, 0o755);
  browser = wrapper;
}

const dev = Bun.spawn(
  ["bun", "run", "--filter", "@plux/web", "dev", "--host", "127.0.0.1", "--port", String(port), "--strictPort"],
  { stdout: "ignore", stderr: "ignore" },
);

async function ready(): Promise<void> {
  for (let i = 0; i < 120; i++) {
    try {
      if ((await fetch(`http://127.0.0.1:${port}/`)).ok) {
        return;
      }
    } catch {
      await Bun.sleep(500);
    }
  }
  throw new Error("the Vite dev server did not start");
}

const targets = ["apps/web/dist", ...themes.map((t) => `http://127.0.0.1:${port}/__catalogue?theme=${t}`)];
let failed = false;
const report: string[] = [];
try {
  await ready();
  for (const target of targets) {
    const run = Bun.spawn(["bunx", "impeccable", "detect", target], {
      env: { ...process.env, IMPECCABLE_BROWSER: browser },
      stdout: "pipe",
      stderr: "pipe",
    });
    const [out, err] = await Promise.all([new Response(run.stdout).text(), new Response(run.stderr).text()]);
    const code = await run.exited;
    report.push(`== ${target} (exit ${code})`, out, err);
    failed ||= code !== 0;
  }
} finally {
  dev.kill();
}
await Bun.write("design-gate-report.txt", `${report.join("\n")}\n`);
process.stdout.write(failed ? "design gate: findings, see studio/design-gate-report.txt\n" : "design gate: clean\n");
process.exit(failed ? 1 : 0);

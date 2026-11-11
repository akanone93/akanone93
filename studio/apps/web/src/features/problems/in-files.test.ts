// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import type { Problem } from "@plux/validator";
import { inFiles } from "./use-validation.ts";

const problem = (file: string): Problem => ({ code: "PLX-1606", severity: "warning", file, path: "", message: "" });

describe("[STU-062] the plugin canvas's problems", () => {
  test("the plugin's own files and the app's own documents; not another plugin's", () => {
    const plugin = [{ path: "plugins/main/pages/home.page.json", content: new Uint8Array() }];
    const shown = inFiles(
      [
        problem("plugins/main/pages/home.page.json"),
        problem("translations/de.json"),
        problem("translations/keys.json"),
        problem("app.json"),
        problem("plugins/other/pages/x.page.json"),
      ],
      plugin,
    ).map((p) => p.file);
    expect(shown).toEqual([
      "plugins/main/pages/home.page.json",
      "translations/de.json",
      "translations/keys.json",
      "app.json",
    ]);
  });
});

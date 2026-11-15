// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import type { Problem } from "@plux/validator";
import { helpFor, withoutPlugin, withPlugin } from "./problem-help.ts";

const problem = (code: string, file: string, message: string): Problem => ({
  code,
  severity: "error",
  file,
  path: "",
  message,
});
const any = () => true;
const none = () => false;

describe("[STU-062] each finding says what it means and offers its fix", () => {
  test("a missing asset file is uploaded again under its name", () => {
    const h = helpFor(
      problem("PLX-1020", "assets/index.json", "asset file assets/images/app-icon.png is missing"),
      none,
    );
    expect(h.why).toBe("assetMissing");
    expect(h.fixes).toEqual([{ kind: "uploadAsset", file: "images/app-icon.png" }]);
  });

  test("app.json's plugins list is corrected either way", () => {
    expect(
      helpFor(
        problem(
          "PLX-1020",
          "plugins/launch/plugin.json",
          'plugin directory "launch" is not listed in app.json plugins',
        ),
        none,
      ).fixes,
    ).toEqual([{ kind: "listPlugin", plugin: "launch" }]);
    expect(
      helpFor(problem("PLX-1020", "app.json", 'plugin "main" has no directory plugins/main/'), none).fixes,
    ).toEqual([{ kind: "unlistPlugin", plugin: "main" }]);
    expect(withPlugin(["main"], "launch")).toEqual(["main", "launch"]);
    expect(withPlugin(["main"], "main")).toEqual(["main"]);
    expect(withoutPlugin(["main", "launch"], "main")).toEqual(["launch"]);
  });

  test("any finding in a file the JSON editor opens can be opened there; others say why only", () => {
    const route = helpFor(
      problem("PLX-1103", "plugins/a/pages/home.page.json", 'route name "home" is already used'),
      any,
    );
    expect(route.why).toBe("duplicateRoute");
    expect(route.fixes).toEqual([{ kind: "openJson", file: "plugins/a/pages/home.page.json" }]);
    expect(helpFor(problem("PLX-9999", "", "something"), any)).toEqual({ why: undefined, fixes: [] });
  });
});

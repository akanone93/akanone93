// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { create } from "@bufbuild/protobuf";
import { DiagnosticSchema, Severity } from "@plux/api";
import { createStudioI18n } from "@plux/i18n";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import type { LoadedDoc } from "../safety/index.ts";
import {
  type AppProblem,
  AppProblems,
  draftsRevision,
  groupByPlugin,
  type ProblemTarget,
  problemOf,
  problemSearch,
  targetOf,
} from "./app-problems.tsx";

afterEach(cleanup);

const plugins = [
  { id: "p-launch", key: "launch", name: "Launch" },
  { id: "p-main", key: "main", name: "Main" },
];
const home: LoadedDoc = {
  pluginId: "p-main",
  path: "plugins/main/pages/home.page.json",
  kind: "page",
  revision: 2,
  json: {
    id: "pg-home",
    kind: "page",
    root: { id: "w-root", type: "Scaffold", children: [{ id: "w-t", type: "Text" }] },
  },
};
const problem = (file: string, path: string, severity: AppProblem["severity"] = "error"): AppProblem => ({
  code: "PLX-1103",
  severity,
  file,
  path,
  message: `a problem in ${file}`,
});

describe("the app's problems on the app canvas (STU-030, build log entry 258)", () => {
  test("a diagnostic of the server's check keeps its code, severity, place and message", () => {
    const d = create(DiagnosticSchema, {
      code: "PLX-1102",
      severity: Severity.WARNING,
      location: { file: "app.json", path: "/entryRoute" },
      message: "no such key",
    });
    expect(problemOf(d)).toEqual({
      code: "PLX-1102",
      severity: "warning",
      file: "app.json",
      path: "/entryRoute",
      message: "no such key",
    });
  });

  test("problems group by plugin, the app's own files first, the plugins in the canvas's order", () => {
    const groups = groupByPlugin(
      [
        problem("plugins/main/pages/home.page.json", "/route"),
        problem("app.json", ""),
        problem("plugins/launch/plugin.json", ""),
      ],
      plugins,
    );
    expect(groups.map((g) => g.plugin?.key ?? "app")).toEqual(["app", "launch", "main"]);
    expect(groups[2]?.files.map(([file]) => file)).toEqual(["plugins/main/pages/home.page.json"]);
  });

  test("a problem opens its plugin at the screen and the widget its pointer reaches", () => {
    const main = plugins[1];
    expect(targetOf(problem(home.path, "/root/children/0/props/text"), main, [home])).toEqual({
      pluginId: "p-main",
      focus: "pg-home",
      node: "w-t",
    });
    expect(targetOf(problem("plugins/main/plugin.json", ""), main, [home])).toEqual({ pluginId: "p-main" });
    expect(targetOf(problem("app.json", ""), undefined, [home])).toBeUndefined();
    expect(problemSearch({ pluginId: "p-main", focus: "pg-home" })).toEqual({ focus: "pg-home" });
  });

  test("the drafts read again unchanged keep their key; an edit, a new or a deleted document change it", () => {
    const a = { pluginId: "p-main", path: "pages/home.page.json", revision: 2n };
    const b = { pluginId: "", path: "app.json", revision: 7n };
    expect(draftsRevision(undefined)).toBeUndefined();
    expect(draftsRevision([a, b])).toBe(draftsRevision([{ ...b }, { ...a }]));
    expect(draftsRevision([a, { ...b, revision: 8n }])).not.toBe(draftsRevision([a, b]));
    expect(draftsRevision([a])).not.toBe(draftsRevision([a, b]));
    expect(draftsRevision([a, { ...a, pluginId: "p-other" }])).not.toBe(draftsRevision([a, a]));
  });

  test("the list says where each problem is and opens it; notes are folded", async () => {
    const opened: ProblemTarget[] = [];
    const studio = createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false });
    render(
      <I18nProvider studio={studio}>
        <AppProblems
          state="done"
          problems={[problem(home.path, "/root/children/0"), problem("app.json", "", "info")]}
          plugins={plugins}
          docs={[home]}
          onOpen={(t) => opened.push(t)}
        />
      </I18nProvider>,
    );
    expect(await screen.findByText(`a problem in ${home.path}`)).toBeTruthy();
    expect(screen.getByRole("region", { name: "Main" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Open" }));
    expect(opened).toEqual([{ pluginId: "p-main", focus: "pg-home", node: "w-t" }]);
    expect(screen.getByText("1 note")).toBeTruthy();
  });
});

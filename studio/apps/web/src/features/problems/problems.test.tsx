// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import type { Problem } from "@plux/validator";
import { cleanup, screen } from "@testing-library/react";
import { orgA, renderApp } from "../../test-harness.tsx";
import { countProblems, groupByFile } from "./project-files.ts";
import type { ValidatorClient } from "./validator-client.ts";

afterEach(cleanup);

const problems: Problem[] = [
  {
    code: "PLX-0100",
    severity: "error",
    file: "plugins/pl/pages/home.page.json",
    path: "/root",
    message: "Unknown widget",
  },
  {
    code: "PLX-0200",
    severity: "warning",
    file: "plugins/pl/pages/home.page.json",
    path: "/root/1",
    message: "Low contrast",
  },
  { code: "PLX-0300", severity: "info", file: "", path: "", message: "Project note" },
];
const documents = [
  { kind: "plugin", path: "plugins/pl/plugin.json", json: { id: "pl", kind: "plugin", pages: [] } },
  { kind: "page", path: "plugins/pl/pages/home.page.json", json: { id: "home", kind: "page" } },
];
const path = `/o/${orgA}/apps/app1/plugins/pl`;

describe("S2.1 validation against the whole project", () => {
  test("loads app.json with the plugin's files and shows no problems for a valid project", async () => {
    const validator = (): ValidatorClient => ({
      // Like the real validator: a plugin without its app.json is a layout problem.
      load: async (files) => {
        const paths = files.map((f) => f.path);
        return paths.includes("app.json")
          ? [{ code: "PLX-0900", severity: "error", file: "plugins/other/plugin.json", path: "", message: "Elsewhere" }]
          : [
              {
                code: "PLX-0001",
                severity: "error",
                file: "plugins/pl/plugin.json",
                path: "",
                message: "Not listed in app.json",
              },
            ];
      },
      validatePage: async () => [],
      dispose: () => undefined,
    });
    const appDocuments = [
      { kind: "app", path: "app.json", json: { id: "app1", kind: "app", plugins: ["pl", "other"] } },
      ...documents,
    ];
    renderApp(path, { documents, appDocuments, validator });
    expect(await screen.findByText("No problems found.")).toBeTruthy();
  });

  test("loads every plugin of the app, so a cross-plugin reference is not a problem", async () => {
    const other = [{ kind: "plugin", path: "plugins/other/plugin.json", json: { id: "other", kind: "plugin" } }];
    const validator = (): ValidatorClient => ({
      load: async (files) =>
        files.some((f) => f.path === "plugins/other/plugin.json")
          ? []
          : [
              {
                code: "PLX-0002",
                severity: "error",
                file: "plugins/pl/plugin.json",
                path: "",
                message: "Unknown plugin other",
              },
            ],
      validatePage: async () => [],
      dispose: () => undefined,
    });
    renderApp(path, {
      documents,
      appDocuments: [{ kind: "app", path: "app.json", json: { id: "app1", kind: "app" } }],
      pluginDocuments: { pl: documents, other },
      validator,
    });
    expect(await screen.findByText("No problems found.")).toBeTruthy();
    expect(screen.queryByText("Not listed in app.json")).toBeNull();
    expect(screen.queryByText("Elsewhere")).toBeNull();
  });
});

describe("S2.1 the Problems tab and counts (F9)", () => {
  test("lists the validator's findings grouped by file and counts them in the action bar", async () => {
    const loaded: string[] = [];
    const validator = (): ValidatorClient => ({
      load: async (files) => {
        loaded.push(...files.map((f) => f.path));
        return problems;
      },
      validatePage: async () => [],
      dispose: () => undefined,
    });
    renderApp(path, { documents, validator });
    expect(await screen.findByText("Unknown widget")).toBeTruthy();
    expect(screen.getByText("Low contrast")).toBeTruthy();
    expect(screen.getByRole("tab", { name: "Problems" })).toBeTruthy();
    expect(screen.getByLabelText("1 error")).toBeTruthy();
    expect(screen.getByLabelText("1 warning")).toBeTruthy();
    expect(screen.getByLabelText("plugins/pl/pages/home.page.json")).toBeTruthy();
    expect(loaded).toEqual(["plugins/pl/plugin.json", "plugins/pl/pages/home.page.json"]);
  });

  test("says so when nothing is wrong, and when the validator fails", async () => {
    renderApp(path, { documents });
    expect(await screen.findByText("No problems found.")).toBeTruthy();
    cleanup();
    renderApp(path, {
      documents,
      validator: () => ({
        load: () => Promise.reject(new Error("x")),
        validatePage: async () => [],
        dispose: () => undefined,
      }),
    });
    expect(await screen.findByText("The validator could not run.")).toBeTruthy();
  });

  test("counts by severity and groups project-level problems first", () => {
    expect(countProblems(problems)).toEqual({ error: 1, warning: 1, info: 1 });
    expect(groupByFile(problems).map(([f]) => f)).toEqual(["", "plugins/pl/pages/home.page.json"]);
  });
});

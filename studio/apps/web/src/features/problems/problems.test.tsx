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
const documents = [{ kind: "plugin", path: "plugins/pl/plugin.json", json: { id: "pl", kind: "plugin", pages: [] } }];
const path = `/o/${orgA}/apps/app1/plugins/pl`;

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
    expect(loaded).toEqual(["plugins/pl/plugin.json"]);
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

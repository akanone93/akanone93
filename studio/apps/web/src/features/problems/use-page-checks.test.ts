// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, mock, test } from "bun:test";
import type { Problem } from "@plux/validator";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import { mergedProblems, usePageChecks } from "./use-page-checks.ts";
import type { Validation } from "./use-validation.ts";

afterEach(cleanup);

const p = (file: string, code: string, severity: Problem["severity"] = "error"): Problem => ({
  code,
  severity,
  file,
  path: "/root",
  message: code,
});

describe("PC-34 every page checked in full [SCH-040] [ANI-008]", () => {
  test("a checked page's findings replace its file's load findings; others stay; each problem once", () => {
    const loaded = [p("a.json", "PLX-1"), p("b.json", "PLX-2"), p("plugin.json", "PLX-3")];
    const pages = new Map([
      ["a.json", [p("a.json", "PLX-7", "warning"), p("g.json", "PLX-8")]],
      ["b.json", [p("b.json", "PLX-2"), p("g.json", "PLX-8")]],
    ]);
    expect(mergedProblems(loaded, pages).map((x) => x.code)).toEqual(["PLX-3", "PLX-7", "PLX-8", "PLX-2"]);
  });

  test("validates each page after the load, and only the page an edit changed afterwards", async () => {
    const validatePage = mock(async (file: string, data: string) => [
      p(file, data.includes("bad") ? "PLX-9" : "PLX-0"),
    ]);
    const validation: Validation = {
      state: "done",
      problems: [p("plugin.json", "PLX-3")],
      validatePage,
      language: undefined,
      tool: undefined,
    };
    type Doc = { readonly path: string; readonly kind: string; readonly parsed: unknown };
    const pageA: Doc = { path: "a.json", kind: "page", parsed: { id: "a" } };
    const pageB: Doc = { path: "b.json", kind: "page", parsed: { id: "b" } };
    const plugin: Doc = { path: "plugin.json", kind: "plugin", parsed: {} };
    const docs = [pageA, pageB, plugin];
    const { result, rerender } = renderHook(({ d }) => usePageChecks(validation, d), { initialProps: { d: docs } });
    await waitFor(() => expect(result.current.state === "done" && result.current.problems.length).toBe(3));
    expect(validatePage).toHaveBeenCalledTimes(2);
    rerender({ d: [{ ...pageA, parsed: { id: "a", bad: true } }, pageB, plugin] });
    await waitFor(() =>
      expect(result.current.state === "done" && result.current.problems.some((x) => x.code === "PLX-9")).toBe(true),
    );
    expect(validatePage).toHaveBeenCalledTimes(3);
  });

  test("a validation that is not done is passed through", () => {
    const { result } = renderHook(() => usePageChecks({ state: "running" }, []));
    expect(result.current.state).toBe("running");
  });
});

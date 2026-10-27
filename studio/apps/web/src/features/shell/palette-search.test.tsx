// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { createStudioI18n } from "@plux/i18n";
import { act, cleanup, fireEvent, render, renderHook, screen } from "@testing-library/react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { type Command, CommandsProvider, createRegistry } from "../../lib/commands/index.ts";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import { CommandPalette } from "./command-palette.tsx";
import {
  type OrgHit,
  type OrgSearchFn,
  parseScope,
  recentsFirst,
  searchDebounceMs,
  useOrgSearch,
} from "./palette-search.ts";

afterEach(cleanup);

const sleep = (ms: number) => act(() => new Promise<void>((r) => setTimeout(r, ms)));
const hit = (name: string): OrgHit => ({ kind: "app", id: name, name });

describe("STU-013 F15 palette scope and recents", () => {
  test("prefixes narrow the scope", () => {
    expect(parseScope("> publish")).toEqual({ scope: "commands", text: "publish" });
    expect(parseScope("#home")).toEqual({ scope: "screens", text: "home" });
    expect(parseScope(" shop ")).toEqual({ scope: "all", text: "shop" });
  });

  test("recents come first, the rest keep their order", () => {
    expect(recentsFirst(["a", "b", "c"], (x) => x, ["c", "b"])).toEqual(["c", "b", "a"]);
  });
});

describe("STU-013 F15 organisation search", () => {
  test("debounces: one request for a burst of typing, after the delay", async () => {
    const calls: string[] = [];
    const search: OrgSearchFn = async (q) => {
      calls.push(q);
      return [hit(q)];
    };
    const { result, rerender } = renderHook(({ t }) => useOrgSearch(search, "all", t), { initialProps: { t: "s" } });
    rerender({ t: "sh" });
    rerender({ t: "sho" });
    await sleep(searchDebounceMs / 2);
    expect(calls).toEqual([]);
    await sleep(searchDebounceMs);
    expect(calls).toEqual(["sho"]);
    expect(result.current.hits.map((h) => h.name)).toEqual(["sho"]);
  });

  test("a stale response never overwrites a newer query", async () => {
    const release = new Map<string, () => void>();
    const search: OrgSearchFn = (q) =>
      new Promise((resolve) => {
        release.set(q, () => resolve([hit(q)]));
      });
    const { result, rerender } = renderHook(({ t }) => useOrgSearch(search, "all", t), { initialProps: { t: "old" } });
    await sleep(searchDebounceMs + 20);
    rerender({ t: "new" });
    await sleep(searchDebounceMs + 20);
    await act(async () => release.get("new")?.());
    await act(async () => release.get("old")?.());
    expect(result.current.hits.map((h) => h.name)).toEqual(["new"]);
  });

  test("commands scope and empty text never call the server", async () => {
    let n = 0;
    const search: OrgSearchFn = async () => {
      n += 1;
      return [];
    };
    renderHook(() => useOrgSearch(search, "commands", "x"));
    renderHook(() => useOrgSearch(search, "all", ""));
    await sleep(searchDebounceMs + 20);
    expect(n).toBe(0);
  });

  test("the palette shows server hits below the local results", async () => {
    const cmd: Command = {
      id: "shop.cmd",
      title: "Shop settings",
      group: "navigate",
      permission: null,
      run: () => undefined,
    };
    const registry = createRegistry([cmd]);
    const studio = createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false });
    const search: OrgSearchFn = async () => [hit("Shop app")];
    render(
      <I18nProvider studio={studio}>
        <CommandsProvider value={{ registry, states: registry.stateFor([]) }}>
          <CommandPalette open onOpenChange={() => undefined} search={search} />
        </CommandsProvider>
      </I18nProvider>,
    );
    const box = await screen.findByRole("combobox");
    fireEvent.change(box, { target: { value: "shop" } });
    await sleep(searchDebounceMs + 50);
    const options = await screen.findAllByRole("option");
    expect(options.map((o) => o.textContent)).toEqual(["Shop settings", "Shop appapp"]);
  });
});

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, mock, test } from "bun:test";
import { createStudioI18n } from "@plux/i18n";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import { listMessages } from "./list-panel.messages.ts";
import { addStateRecipe, ListPanel, ListPreviewContext, stateTemplate } from "./list-panel.tsx";

afterEach(cleanup);

describe("PC-24 the list panel [WGT-012]", () => {
  test("it shows the items' binding, picks the state drawn and adds a state view from a template", async () => {
    const show = mock((_id: string, _s: string | undefined) => undefined);
    const addState = mock((_s: string, _v: object) => undefined);
    let n = 0;
    render(
      <I18nProvider studio={createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false })}>
        <ListPreviewContext.Provider value={{ stateOf: () => undefined, show }}>
          <ListPanel
            id="list"
            node={{
              id: "list",
              type: "ListView",
              props: { items: { $expr: "data.orders.value" } },
              slots: { empty: {} },
            }}
            canEdit
            drawn={() => "[3]"}
            addState={addState}
            newId={() => `n${++n}`}
          />
        </ListPreviewContext.Provider>
      </I18nProvider>,
    );
    expect(await screen.findByText(/data\.orders\.value/)).toBeTruthy();
    const chips = screen.getAllByRole("radio");
    expect(chips).toHaveLength(6);
    expect(chips[0]?.getAttribute("aria-checked")).toBe("true");
    fireEvent.click(chips[3] as HTMLElement);
    expect(show).toHaveBeenLastCalledWith("list", "loading");
    const adds = screen
      .getAllByRole("button")
      .filter((b) => b.className === "list-state" && !b.hasAttribute("data-designed"));
    expect(adds).toHaveLength(2);
    fireEvent.click(adds[1] as HTMLElement);
    expect(addState.mock.calls[0]?.[0]).toBe("error");
    expect(show).toHaveBeenLastCalledWith("list", "error");
  });

  test("a state template has fresh ids and the recipe puts it in the list's slot", () => {
    const m = listMessages({ _: (d: { message: string }) => d.message } as never);
    let n = 0;
    const view = stateTemplate("error", m, () => `x${++n}`);
    expect(JSON.stringify(view)).toContain("TextButton");
    expect(JSON.stringify(view)).not.toContain("events");
    expect(JSON.stringify(stateTemplate("error", m, () => "y", "orders"))).toContain(
      '"steps":[{"id":"retry","action":"refreshData","input":{"source":"orders"}}]',
    );
    const root = { id: "list", type: "ListView", slots: { item: { id: "i", type: "Text" } } };
    addStateRecipe("list", "error", view)(root);
    expect(Object.keys(root.slots)).toEqual(["item", "error"]);
  });
});

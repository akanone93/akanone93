// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, mock, test } from "bun:test";
import { createStudioI18n } from "@plux/i18n";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import { BranchPreviewContext, ConditionPanel } from "./condition-panel.tsx";

afterEach(cleanup);

describe("PC-25 the conditions panel [SCH-023]", () => {
  test("an If shows its condition and picks a branch; a visibility condition is written as an expression", async () => {
    const pick = mock((_id: string, _p: string | undefined) => undefined);
    const setVisible = mock((_: unknown) => undefined);
    render(
      <I18nProvider studio={createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false })}>
        <BranchPreviewContext.Provider value={{ pickOf: () => undefined, pick }}>
          <ConditionPanel
            id="if"
            node={{ id: "if", type: "If", props: { condition: { $expr: "page.count > 0" } } }}
            canEdit
            decide={() => true}
            setVisible={setVisible}
          />
        </BranchPreviewContext.Provider>
      </I18nProvider>,
    );
    expect(await screen.findByText("page.count > 0")).toBeTruthy();
    const chips = screen.getAllByRole("radio");
    expect(chips).toHaveLength(3);
    fireEvent.click(chips[2] as HTMLElement);
    expect(pick).toHaveBeenLastCalledWith("if", "else");
    const field = screen.getByRole("textbox", { name: "Visible when" });
    fireEvent.change(field, { target: { value: "page.items.isNotEmpty" } });
    fireEvent.blur(field);
    expect(setVisible).toHaveBeenLastCalledWith({ $expr: "page.items.isNotEmpty" });
  });
});

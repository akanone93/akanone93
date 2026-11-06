// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, mock, test } from "bun:test";
import { createStudioI18n } from "@plux/i18n";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import { LaneDialog } from "./lane-dialog.tsx";

afterEach(cleanup);

describe("PC-28 a lane's parameters and transition [NAV-001] [NAV-010]", () => {
  test("each declared parameter is a field; a missing required one is named; a typed value is written", async () => {
    const onParam = mock((_n: string, _v: unknown) => undefined);
    const onTransition = mock((_t: string) => undefined);
    render(
      <I18nProvider studio={createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false })}>
        <LaneDialog
          title="Orders → Order"
          site={{ nodeId: "tile", link: { event: "onTap", step: 0, route: "order" } }}
          routes={[]}
          canEdit
          onRedirect={() => undefined}
          onRemove={() => undefined}
          onClose={() => undefined}
          target={{
            declared: [
              { name: "orderId", type: "string", required: true },
              { name: "tab", type: "int", required: false },
            ],
            passed: { tab: 2 },
            transition: "platform",
            onParam,
            onTransition,
          }}
        />
      </I18nProvider>,
    );
    expect(await screen.findByRole("alert")).toBeTruthy();
    expect(screen.getByRole("alert").textContent).toContain("orderId");
    const field = screen.getByRole("textbox", { name: "orderId" });
    fireEvent.change(field, { target: { value: "item.id" } });
    fireEvent.blur(field);
    expect(onParam).toHaveBeenLastCalledWith("orderId", { $expr: "item.id" });
    expect((screen.getByRole("textbox", { name: "tab" }) as HTMLInputElement).value).toBe("2");
  });
});

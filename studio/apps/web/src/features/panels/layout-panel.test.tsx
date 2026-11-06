// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, mock, test } from "bun:test";
import { createStudioI18n } from "@plux/i18n";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import { ConstraintsEditor, LayoutPanel, type LayoutRecipes } from "./layout-panel.tsx";

afterEach(cleanup);

const renderI18n = (node: ReactNode) =>
  render(
    <I18nProvider studio={createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false })}>
      {node}
    </I18nProvider>,
  );

const recipe = () => () => undefined;
const recipes = (): LayoutRecipes & Record<string, ReturnType<typeof mock>> => ({
  flip: mock(recipe),
  sizing: mock(recipe),
  pin: mock(recipe),
  flex: mock(recipe),
});

function panel(root: object, id: string, node: object, extra: object = {}) {
  const applyProps = mock((_: Record<string, unknown>) => undefined);
  const structure = mock((_: (root: unknown) => void) => undefined);
  const r = recipes();
  const onBp = mock((_: string) => undefined);
  const onClearOverrides = mock((_: string) => undefined);
  renderI18n(
    <LayoutPanel
      node={node as Record<string, unknown>}
      root={root}
      id={id}
      bp="compact"
      canEdit
      applyProps={applyProps}
      structure={structure}
      recipes={r}
      layoutNames={["mainAxisAlignment", "spacing"]}
      onBp={onBp}
      onClearOverrides={onClearOverrides}
      {...extra}
    />,
  );
  return { applyProps, structure, r, onBp, onClearOverrides };
}

describe("PC-22 the layout panel [WGT-010]", () => {
  test("a row's auto layout flips, aligns from the pad, stretches and resets", async () => {
    const node = { id: "row", type: "Row", props: { spacing: 8 }, children: [] };
    const { applyProps, structure, r } = panel(node, "row", node);
    fireEvent.click(await screen.findByRole("radio", { name: /Column/ }));
    expect(r.flip).toHaveBeenCalledTimes(1);
    expect(structure).toHaveBeenCalledTimes(1);
    const cells = screen.getAllByRole("button", { pressed: false });
    fireEvent.click(cells[cells.length - 1] as HTMLElement);
    expect(applyProps).toHaveBeenLastCalledWith({ mainAxisAlignment: "end", crossAxisAlignment: "end" });
    fireEvent.click(screen.getByRole("switch", { name: "Stretch children" }));
    expect(applyProps).toHaveBeenLastCalledWith({ crossAxisAlignment: "stretch" });
    fireEvent.click(screen.getByRole("radio", { name: "Hug" }));
    expect(applyProps).toHaveBeenLastCalledWith({ mainAxisSize: "min" });
    fireEvent.click(screen.getByRole("button", { name: /Reset layout/ }));
    expect(applyProps).toHaveBeenLastCalledWith({ spacing: undefined });
  });

  test("a widget in a row is sized hug, fill or fixed; a widget in a stack is pinned", async () => {
    const child = { id: "a", type: "Text", props: { data: "a" } };
    const { r } = panel({ id: "row", type: "Row", children: [child] }, "a", child);
    fireEvent.click((await screen.findAllByRole("radio", { name: "Fill" }))[0] as HTMLElement);
    expect(r.sizing).toHaveBeenLastCalledWith("horizontal", "fill", 120);
    cleanup();
    const inStack = { id: "u", type: "Text" };
    const s = panel({ id: "s", type: "Stack", children: [inStack] }, "u", inStack);
    expect(await screen.findByText(/Not pinned/)).toBeTruthy();
    const pins = screen.getAllByRole("button", { pressed: false });
    fireEvent.click(pins[0] as HTMLElement);
    expect(s.r.pin).toHaveBeenLastCalledWith("top", 0);
  });

  test("the size classes a widget overrides switch the inspector to them or clear them", async () => {
    const node = { id: "n", type: "Text", responsive: { medium: { data: "m" } } };
    const { onBp, onClearOverrides } = panel(node, "n", node);
    const buttons = await screen.findAllByRole("button");
    fireEvent.click(buttons[0] as HTMLElement);
    expect(onBp).toHaveBeenLastCalledWith("medium");
    fireEvent.click(buttons[1] as HTMLElement);
    expect(onClearOverrides).toHaveBeenLastCalledWith("medium");
  });

  test("constraints edit least and most width and height; clearing the last one clears them", async () => {
    const onChange = mock((_: unknown) => undefined);
    renderI18n(
      <ConstraintsEditor
        name="constraints"
        value={{ maxWidth: 320 }}
        readOnly={false}
        onChange={onChange}
        onEnd={() => undefined}
      />,
    );
    fireEvent.change(await screen.findByRole("spinbutton", { name: "Least Height" }), { target: { value: "48" } });
    expect(onChange).toHaveBeenLastCalledWith({ maxWidth: 320, minHeight: 48 });
    fireEvent.change(screen.getByRole("spinbutton", { name: "Most Width" }), { target: { value: "" } });
    expect(onChange).toHaveBeenLastCalledWith(undefined);
  });
});

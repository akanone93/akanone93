// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, mock, test } from "bun:test";
import { createStudioI18n } from "@plux/i18n";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import { MotionPanel } from "./motion-panel.tsx";

afterEach(cleanup);

function renderPanel(animation: Record<string, unknown>, set: (key: string, value: unknown) => void) {
  const studio = createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false });
  return render(
    <I18nProvider studio={studio}>
      <MotionPanel id="c" node={{ id: "c", type: "Card", animation }} canEdit heroCount={0} set={set} />
    </I18nProvider>,
  );
}

/** Picks an option of a Select, as a person does. */
async function choose(label: string, option: string): Promise<void> {
  fireEvent.click(await screen.findByRole("combobox", { name: label }));
  const item = await screen.findByRole("option", { name: option });
  await new Promise((resolve) => setTimeout(resolve, 300));
  fireEvent.pointerDown(item);
  fireEvent.mouseDown(item);
  fireEvent.pointerUp(item);
  fireEvent.mouseUp(item);
  fireEvent.click(item);
}

describe("[ANI-001] an animation's curve in the motion panel (build log row 48)", () => {
  test("a named curve can become a cubic Bézier, starting from Material's standard easing", async () => {
    const set = mock((_key: string, _value: unknown) => undefined);
    renderPanel({ durationMs: 200, curve: "easeIn" }, set);
    await choose("Curve", "Cubic Bézier");
    await waitFor(() => expect(set).toHaveBeenCalledWith("curve", [0.2, 0, 0, 1]));
  });

  test("a cubic Bézier's control points are edited one by one; x stays from 0 to 1", async () => {
    const set = mock((_key: string, _value: unknown) => undefined);
    renderPanel({ durationMs: 200, cubic: [0.2, 0, 0, 1] }, set);
    const x1 = await screen.findByRole("spinbutton", { name: "Control point x1" });
    fireEvent.change(x1, { target: { value: "1.5" } });
    fireEvent.blur(x1);
    expect(set).toHaveBeenLastCalledWith("curve", [1, 0, 0, 1]);
    const y2 = screen.getByRole("spinbutton", { name: "Control point y2" });
    fireEvent.change(y2, { target: { value: "1.2" } });
    fireEvent.blur(y2);
    expect(set).toHaveBeenLastCalledWith("curve", [0.2, 0, 0, 1.2]);
  });
});

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { createStudioI18n } from "@plux/i18n";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import { GhostLeaveDialog } from "./ghost-dialog.tsx";
import type { GhostTarget } from "./ghosts.ts";

afterEach(cleanup);

const ghost: GhostTarget = {
  id: "ghost:s1",
  kind: "screen",
  title: "Loan form",
  route: "/loan",
  pluginId: "loans",
  screenId: "s1",
  links: 2,
};

describe("D21 leave and open ghost screen [STU-053]", () => {
  const show = (
    g: GhostTarget | null,
    onCancel: () => void = () => undefined,
    onOpen: (g: GhostTarget) => void = () => undefined,
  ) =>
    render(
      <I18nProvider studio={createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false })}>
        <GhostLeaveDialog ghost={g} onCancel={onCancel} onOpen={onOpen} />
      </I18nProvider>,
    );

  test("names the screen and opens it only after confirmation", async () => {
    const opened: GhostTarget[] = [];
    show(ghost, undefined, (g) => opened.push(g));
    expect(await screen.findByText(/Loan form/)).toBeTruthy();
    expect(opened).toEqual([]);
    fireEvent.click(screen.getByRole("button", { name: "Open screen" }));
    expect(opened).toEqual([ghost]);
  });

  test("staying closes it without opening", async () => {
    let cancelled = 0;
    const opened: GhostTarget[] = [];
    show(
      ghost,
      () => cancelled++,
      (g) => opened.push(g),
    );
    fireEvent.click(await screen.findByRole("button", { name: "Stay here" }));
    expect(cancelled).toBe(1);
    expect(opened).toEqual([]);
  });

  test("shows nothing without a ghost", async () => {
    show(null);
    await Promise.resolve();
    expect(screen.queryByRole("alertdialog")).toBeNull();
  });
});

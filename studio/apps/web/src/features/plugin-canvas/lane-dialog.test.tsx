// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { createStudioI18n } from "@plux/i18n";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import { LaneDialog } from "./lane-dialog.tsx";
import { navigateSite, redirectRecipe, removeLinkRecipe } from "./links.ts";

afterEach(cleanup);

const renderI18n = (node: ReactNode) =>
  render(
    <I18nProvider studio={createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false })}>
      {node}
    </I18nProvider>,
  );

const root = () => ({
  id: "root",
  children: [
    {
      id: "b1",
      events: { onPressed: { steps: [{ action: "navigate", id: "s1", input: { route: "detail" } }] } },
    },
    {
      id: "b2",
      events: { onPressed: { steps: [{ action: "navigate", id: "s2", input: { route: "tall" } }] } },
    },
  ],
});

describe("[STU-046] lane fixes find the step behind an arrow", () => {
  test("the n-th navigate step in document order resolves to its widget, event and position", () => {
    expect(navigateSite(root(), 1)).toEqual({ nodeId: "b2", link: { event: "onPressed", step: 0, route: "tall" } });
    expect(navigateSite(root(), 2)).toBeUndefined();
  });

  test("a navigate step outside a widget event handler cannot be fixed", () => {
    expect(navigateSite({ id: "r", slots: { x: { action: "navigate", input: { route: "a" } } } }, 0)).toBeUndefined();
  });

  test("the redirect and remove recipes edit the resolved step", () => {
    const r = root();
    const site = navigateSite(r, 0);
    if (site === undefined) {
      throw new Error("no site");
    }
    redirectRecipe(site.nodeId, site.link, "tall")(r);
    expect(navigateSite(r, 0)?.link.route).toBe("tall");
    removeLinkRecipe(site.nodeId, { ...site.link, route: "tall" })(r);
    expect(navigateSite(r, 1)).toBeUndefined();
  });
});

describe("[STU-046] the lane dialog", () => {
  const routes = [
    { route: "detail", title: "Detail", own: true },
    { route: "tall", title: "Tall", own: true },
  ];
  const site = { nodeId: "b1", link: { event: "onPressed", step: 0, route: "detail" } };

  test("remove runs the removal", async () => {
    let removed = 0;
    renderI18n(
      <LaneDialog
        title="A → B"
        site={site}
        routes={routes}
        canEdit
        onRedirect={() => undefined}
        onRemove={() => removed++}
        onClose={() => undefined}
      />,
    );
    fireEvent.click(await screen.findByRole("button", { name: "Remove link" }));
    expect(removed).toBe(1);
  });

  test("redirect offers the other screens and passes the chosen route", async () => {
    const chosen: string[] = [];
    renderI18n(
      <LaneDialog
        title="A → B"
        site={site}
        routes={routes}
        canEdit
        onRedirect={(r) => chosen.push(r)}
        onRemove={() => undefined}
        onClose={() => undefined}
      />,
    );
    fireEvent.click(await screen.findByRole("button", { name: "Redirect to another screen" }));
    fireEvent.click(await screen.findByRole("combobox", { name: "New target screen" }));
    expect(screen.queryByRole("option", { name: "Detail" })).toBeNull();
    fireEvent.click(await screen.findByRole("option", { name: "Tall" }));
    fireEvent.click(screen.getByRole("button", { name: "Redirect" }));
    expect(chosen).toEqual(["tall"]);
  });

  test("without the lock both fixes are disabled with the reason", async () => {
    renderI18n(
      <LaneDialog
        title="A → B"
        site={site}
        routes={routes}
        canEdit={false}
        onRedirect={() => undefined}
        onRemove={() => undefined}
        onClose={() => undefined}
      />,
    );
    expect(await screen.findByText("Take the editing lock to change this link.")).toBeDefined();
    expect(screen.getByRole("button", { name: "Remove link" }).getAttribute("aria-disabled")).toBe("true");
  });
});

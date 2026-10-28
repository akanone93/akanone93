// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { createStudioI18n } from "@plux/i18n";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import { ActionsTab } from "../panels/actions-tab.tsx";
import type { ApplyMode } from "./links.ts";
import { eventsOf, linksOf } from "./links.ts";
import { TriggerPicker } from "./trigger-picker.tsx";

afterEach(cleanup);

function withI18n(children: React.ReactNode) {
  const studio = createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false });
  return <I18nProvider studio={studio}>{children}</I18nProvider>;
}

const bound = {
  id: "w",
  type: "ElevatedButton",
  events: { onPressed: { steps: [{ action: "showToast", id: "t" }] } },
};

describe("trigger picker (E7, [STU-047])", () => {
  test("offers exactly the events the widget type declares", async () => {
    render(
      withI18n(
        <TriggerPicker
          open
          node={{ id: "w", type: "ElevatedButton" }}
          events={eventsOf("ElevatedButton")}
          target="Detail"
          onConfirm={() => {}}
          onCancel={() => {}}
        />,
      ),
    );
    const radios = await screen.findAllByRole("radio");
    expect(radios.map((r) => (r as HTMLInputElement).value)).toEqual([...eventsOf("ElevatedButton")]);
  });

  test("confirming reports the chosen event", async () => {
    const got: [string, ApplyMode][] = [];
    render(
      withI18n(
        <TriggerPicker
          open
          node={{ id: "w", type: "ElevatedButton" }}
          events={["onPressed", "onLongPress"]}
          target="Detail"
          onConfirm={(e, m) => got.push([e, m])}
          onCancel={() => {}}
        />,
      ),
    );
    fireEvent.click(await screen.findByRole("radio", { name: "onLongPress" }));
    fireEvent.click(screen.getByRole("button", { name: "Create link" }));
    expect(got).toEqual([["onLongPress", "replace"]]);
  });

  test("an event that already has steps asks replace or add after", async () => {
    const got: [string, ApplyMode][] = [];
    render(
      withI18n(
        <TriggerPicker
          open
          node={bound}
          events={["onPressed"]}
          target="Detail"
          onConfirm={(e, m) => got.push([e, m])}
          onCancel={() => {}}
        />,
      ),
    );
    fireEvent.click(await screen.findByRole("radio", { name: "Add after" }));
    fireEvent.click(screen.getByRole("button", { name: "Create link" }));
    expect(got).toEqual([["onPressed", "append"]]);
  });

  test("cancel discards", async () => {
    let cancelled = 0;
    render(
      withI18n(
        <TriggerPicker
          open
          node={bound}
          events={["onPressed"]}
          target="Detail"
          onConfirm={() => {}}
          onCancel={() => cancelled++}
        />,
      ),
    );
    fireEvent.click(await screen.findByRole("button", { name: "Cancel" }));
    expect(cancelled).toBe(1);
  });
});

describe("Actions tab navigate path (F6)", () => {
  const routes = [
    { route: "home", title: "Home", own: true },
    { route: "pay", title: "Pay", own: false },
  ];

  test("adds a navigate step to the chosen event with the picked target", async () => {
    const root = { id: "w", type: "ElevatedButton" };
    render(
      withI18n(<ActionsTab nodeId="w" node={root} routes={routes} canEdit apply={(r) => r(root)} newId={() => "n1"} />),
    );
    fireEvent.click(await screen.findByRole("button", { name: "Add link" }));
    expect(linksOf(root)).toEqual([{ event: eventsOf("ElevatedButton")[0] as string, step: 0, route: "home" }]);
  });

  test("a link to a missing screen offers redirect and remove", async () => {
    const root = {
      id: "w",
      type: "ElevatedButton",
      events: { onPressed: { steps: [{ action: "navigate", id: "s", input: { route: "gone" } }] } },
    };
    render(withI18n(<ActionsTab nodeId="w" node={root} routes={routes} canEdit apply={(r) => r(root)} />));
    expect(await screen.findByText("No screen has the route gone.")).toBeDefined();
    fireEvent.click(screen.getByRole("button", { name: "Redirect to the chosen screen" }));
    expect(linksOf(root)[0]?.route).toBe("home");
  });

  test("is read-only when editing is blocked", async () => {
    const root = { id: "w", type: "ElevatedButton" };
    let applied = 0;
    render(withI18n(<ActionsTab nodeId="w" node={root} routes={routes} canEdit={false} apply={() => applied++} />));
    fireEvent.click(await screen.findByRole("button", { name: "Add link" }));
    expect(applied).toBe(0);
  });
});

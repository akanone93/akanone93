// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { createStudioI18n } from "@plux/i18n";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import { BlockedDeletePanel, RenameWarning, TypeToConfirmDialog } from "./safety-dialogs.tsx";
import type { Use } from "./where-used.ts";

afterEach(cleanup);

function renderI18n(node: ReactNode) {
  const studio = createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false });
  return render(<I18nProvider studio={studio}>{node}</I18nProvider>);
}

const link: Use = {
  kind: "navigates",
  pluginId: "C",
  path: "plugins/cards/pages/cards-home.page.json",
  fromId: "p-cards",
  fromTitle: "Cards home",
  pointer: "/root/events/onPressed/steps/0/input/route",
  step: "/root/events/onPressed/steps/0",
  target: { kind: "page", id: "p-offers" },
};

describe("[STU-074] type-to-confirm for production", () => {
  test("the action stays disabled until the exact name is typed", async () => {
    let confirmed = 0;
    renderI18n(
      <TypeToConfirmDialog
        open
        title="Disable loans in production"
        description="Users lose access at the next sync."
        name="loans"
        action="Disable plugin"
        onConfirm={() => confirmed++}
        onOpenChange={() => undefined}
      />,
    );
    const field = await screen.findByLabelText("Type loans to confirm");
    const action = screen.getByRole("button", { name: "Disable plugin" });
    expect(action.getAttribute("aria-disabled")).toBe("true");
    fireEvent.change(field, { target: { value: "Loans" } });
    expect(await screen.findByText("The text does not match yet")).toBeDefined();
    fireEvent.click(action);
    expect(confirmed).toBe(0);
    fireEvent.change(field, { target: { value: "loans" } });
    expect(action.getAttribute("aria-disabled")).not.toBe("true");
    fireEvent.click(action);
    expect(confirmed).toBe(1);
  });
});

describe("[STU-070] [STU-071] [STU-076] blocked delete panel and rename warning", () => {
  test("lists each use with navigation and offers the fixes that apply", async () => {
    const visited: string[] = [];
    let redirected = 0;
    renderI18n(
      <BlockedDeletePanel
        open
        check={{ entry: true, incoming: [link], outgoing: [] }}
        onNavigate={(u) => visited.push(u.fromId)}
        onRedirect={() => redirected++}
        onRemoveOutgoing={() => undefined}
        onOpenChange={() => undefined}
      />,
    );
    expect(await screen.findByText("It is an entry point. Choose another entry first.")).toBeDefined();
    expect(screen.getByText("1 use or link into it")).toBeDefined();
    expect(screen.queryByRole("button", { name: "Remove outgoing links" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Go to Cards home" }));
    fireEvent.click(screen.getByRole("button", { name: "Redirect all incoming links to…" }));
    expect(visited).toEqual(["p-cards"]);
    expect(redirected).toBe(1);
  });

  test("a route change warns with the deep link and generated API", async () => {
    renderI18n(
      <RenameWarning
        effects={[
          { kind: "deepLink", before: "/loan-offers", after: "/offers" },
          { kind: "generatedRoute", before: "loanOffers", after: "offers" },
        ]}
      />,
    );
    expect(await screen.findByRole("alert")).toBeDefined();
    expect(screen.getByText("Deep link")).toBeDefined();
    expect(screen.getByText("loanOffers → offers")).toBeDefined();
  });
});

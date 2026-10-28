// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, mock, test } from "bun:test";
import { createStudioI18n } from "@plux/i18n";
import { createMemoryHistory, createRootRoute, createRouter, RouterProvider } from "@tanstack/react-router";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { createSession } from "../../lib/session.ts";
import { AppContextProvider } from "../../lib/use-app.tsx";
import { createUpstream, loadSourceCatalogue } from "../../test-harness.tsx";
import {
  JustificationDialog,
  SignedOutDialog,
  StepUpDialog,
  TypeConfirmDialog,
  UnsavedChangesDialog,
} from "./index.ts";

afterEach(cleanup);

function renderWith(node: React.ReactNode) {
  const studio = createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false });
  const session = createSession("/api", createUpstream({}));
  const root = createRootRoute({ component: () => node });
  const router = createRouter({ routeTree: root, history: createMemoryHistory({ initialEntries: ["/"] }) });
  return render(
    <I18nProvider studio={studio}>
      <AppContextProvider value={{ session, studio }}>
        <RouterProvider router={router} />
      </AppContextProvider>
    </I18nProvider>,
  );
}

describe("STU-074 D4 type-to-confirm", () => {
  test("the button is disabled until the text matches exactly", async () => {
    const onConfirm = mock(() => undefined);
    renderWith(
      <TypeConfirmDialog
        open
        onOpenChange={() => undefined}
        phrase="production"
        description="Roll back"
        onConfirm={onConfirm}
      />,
    );
    const input = await screen.findByLabelText("Type production to confirm");
    const button = screen.getByRole("button", { name: "Confirm" });
    expect(button.getAttribute("aria-disabled")).toBe("true");
    fireEvent.change(input, { target: { value: "Production" } });
    expect(button.getAttribute("aria-disabled")).toBe("true");
    fireEvent.click(button);
    expect(onConfirm).not.toHaveBeenCalled();
    fireEvent.change(input, { target: { value: "production" } });
    expect(button.getAttribute("aria-disabled")).not.toBe("true");
    fireEvent.click(button);
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });
});

describe("GOV-024 D5 justification", () => {
  test("submit needs a non-empty trimmed justification", async () => {
    const onSubmit = mock((_text: string) => undefined);
    renderWith(<JustificationDialog open onOpenChange={() => undefined} description="Why" onSubmit={onSubmit} />);
    const field = await screen.findByLabelText("Justification");
    const button = screen.getByRole("button", { name: "Confirm" }) as HTMLButtonElement;
    fireEvent.change(field, { target: { value: "   " } });
    expect(button.getAttribute("aria-disabled")).toBe("true");
    fireEvent.change(field, { target: { value: "long enough text" } });
    expect(button.getAttribute("aria-disabled")).not.toBe("true");
    fireEvent.click(button);
    expect(onSubmit).toHaveBeenCalledWith("long enough text");
  });
});

describe("SEC-103 D3 step-up", () => {
  test("asks the authenticator with the given options and hands the assertion on", async () => {
    const credential = { id: "c1" } as Credential;
    const get = mock(async (_o?: CredentialRequestOptions) => credential);
    Object.defineProperty(navigator, "credentials", { value: { get }, configurable: true });
    const options = { publicKey: { challenge: new Uint8Array([1]) } };
    const onAssertion = mock((_c: Credential) => undefined);
    renderWith(<StepUpDialog open onOpenChange={() => undefined} options={options} onAssertion={onAssertion} />);
    fireEvent.click(await screen.findByRole("button", { name: "Use security key or passkey" }));
    await waitFor(() => expect(onAssertion).toHaveBeenCalledWith(credential));
    expect(get).toHaveBeenCalledWith(options);
  });

  test("a failed check shows an alert and hands nothing on", async () => {
    Object.defineProperty(navigator, "credentials", {
      value: { get: async () => Promise.reject(new Error("no")) },
      configurable: true,
    });
    const onAssertion = mock((_c: Credential) => undefined);
    renderWith(<StepUpDialog open onOpenChange={() => undefined} options={{}} onAssertion={onAssertion} />);
    fireEvent.click(await screen.findByRole("button", { name: "Use security key or passkey" }));
    await screen.findByRole("alert");
    expect(onAssertion).not.toHaveBeenCalled();
  });
});

describe("F4 D6 signed out", () => {
  test("the only action signs in again", async () => {
    const onSignIn = mock(() => undefined);
    renderWith(<SignedOutDialog open onSignIn={onSignIn} />);
    fireEvent.click(await screen.findByRole("button", { name: "Sign in again" }));
    expect(onSignIn).toHaveBeenCalledTimes(1);
  });
});

describe("F4 D7 unsaved changes", () => {
  test("renders closed while nothing is being left", async () => {
    renderWith(<UnsavedChangesDialog dirty />);
    await waitFor(() => expect(screen.queryByRole("alertdialog") === null).toBe(true));
  });
});

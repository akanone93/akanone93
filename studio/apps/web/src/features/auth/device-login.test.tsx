// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { orgA, orgB, renderApp, type Upstream } from "../../test-harness.tsx";
import { cleanUserCode } from "./device-login-page.tsx";

afterEach(cleanup);

const identity = (refuse?: NonNullable<Upstream["identity"]>["refuse"]): NonNullable<Upstream["identity"]> => ({
  factors: [],
  calls: [],
  ...(refuse === undefined ? {} : { refuse }),
});

describe("[CLI-002] approving plux login in Studio", () => {
  test("the code from the link is cleaned and bounded, never trusted as it comes", () => {
    expect(cleanUserCode("bcdf-ghjk")).toBe("BCDF-GHJK");
    expect(cleanUserCode('BCDF-GHJK"><script>')).toBe("BCDF-GHJK");
    expect(cleanUserCode("bcdf ghjk 1234")).toBe("BCDFGHJK");
    expect(cleanUserCode(undefined)).toBe("");
  });

  test("nothing is approved on arrival; approval needs the person's own confirmation and names the organisation", async () => {
    const id = identity();
    renderApp("/device?user_code=bcdf-ghjk", { identity: id });
    const code = await screen.findByLabelText("Code from your terminal");
    expect((code as HTMLInputElement).value).toBe("BCDF-GHJK");
    const approve = screen.getByRole("button", { name: "Approve" });
    expect(approve).toHaveProperty("disabled", true);
    expect(id.calls).toEqual([]);
    screen.getByText(/Only approve a code you started yourself/);
    fireEvent.click(screen.getByRole("checkbox", { name: /I started this sign-in myself/ }));
    await waitFor(() => expect(approve).toHaveProperty("disabled", false));
    fireEvent.click(approve);
    await screen.findByText(/Approved. Return to your terminal/);
    expect(id.calls).toEqual([`approve:BCDF-GHJK:${orgA}`]);
    expect(orgB).not.toBe(orgA);
  });

  test("changing the code clears the confirmation", async () => {
    renderApp("/device?user_code=bcdf-ghjk", { identity: identity() });
    const box = await screen.findByRole("checkbox", { name: /I started this sign-in myself/ });
    fireEvent.click(box);
    await waitFor(() => expect(screen.getByRole("button", { name: "Approve" })).toHaveProperty("disabled", false));
    fireEvent.change(screen.getByLabelText("Code from your terminal"), { target: { value: "bcdf-ghjl" } });
    await waitFor(() => expect(screen.getByRole("button", { name: "Approve" })).toHaveProperty("disabled", true));
  });

  test("a session without a second factor is told how to get one [SEC-100]", async () => {
    const id = identity({ approve: "PLX-8011" });
    renderApp("/device?user_code=BCDFGHJK", { identity: id });
    fireEvent.click(await screen.findByRole("checkbox", { name: /I started this sign-in myself/ }));
    fireEvent.click(screen.getByRole("button", { name: "Approve" }));
    await screen.findByText(/Approving needs a security key in this session/);
  });

  test("a code that is not waiting says so; deny needs no confirmation", async () => {
    const id = identity({ deny: "PLX-8031" });
    renderApp("/device?user_code=BCDF-GHJK", { identity: id });
    fireEvent.click(await screen.findByRole("button", { name: "Deny" }));
    await screen.findByText(/No sign-in is waiting for this code/);
    expect(id.calls).toEqual(["deny:BCDF-GHJK"]);
  });

  test("a signed-out visitor signs in first and comes back with the code", async () => {
    const r = renderApp("/device?user_code=BCDF-GHJK", { signedIn: false });
    await waitFor(() => expect(r.router.state.location.pathname).toBe("/signin"));
    expect(String(r.router.state.location.search.next ?? "")).toContain("/device?user_code=BCDF-GHJK");
  });
});

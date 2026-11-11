// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { Code, ConnectError } from "@connectrpc/connect";
import { reasonOf, refusalOf } from "./refusal.tsx";

describe("[SRV-006] a refusal's own words reach the person", () => {
  test("the server's code and message are kept; anything else says nothing", () => {
    const refused = new ConnectError("PLX-8052: the project to render is incomplete", Code.FailedPrecondition);
    expect(reasonOf(refused)).toBe("PLX-8052: the project to render is incomplete");
    expect(reasonOf(new ConnectError("network down", Code.Unavailable))).toBe("");
    expect(reasonOf(new Error("PLX-1234: not a server refusal"))).toBe("");
  });

  test("a step-up that is missing or too old asks for the passkey [SEC-103]", () => {
    expect(refusalOf(new ConnectError("PLX-6200: needs a step-up", Code.PermissionDenied))).toBe("step-up");
    expect(refusalOf(new ConnectError("PLX-6201: the step-up is too old", Code.PermissionDenied))).toBe("step-up");
  });
});

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { create } from "@bufbuild/protobuf";
import { DevJoinState, JoinRequestEventSchema } from "@plux/api";
import { promptedJoins, reconcileJoins, withJoinEvent } from "./join-prompt.tsx";

const ev = (requestId: string, state: DevJoinState) =>
  create(JoinRequestEventSchema, { requestId, state, phoneName: "Pixel 8" });

describe("the join prompts and what Studio reads of the open requests (DEV-002, ADR-0104)", () => {
  test("a listed request shows even when its event never arrived", () => {
    const next = reconcileJoins(new Map(), [ev("r1", DevJoinState.WAITING)], new Set(), new Set());
    expect(promptedJoins(next).map((e) => e.requestId)).toEqual(["r1"]);
  });

  test("a request an event brought stays until a listing that named it no longer does", () => {
    const shown = withJoinEvent(new Map(), ev("r1", DevJoinState.WAITING));
    expect(reconcileJoins(shown, [], new Set(), new Set()).has("r1")).toBe(true);
    expect(reconcileJoins(shown, [], new Set(), new Set(["r1"])).has("r1")).toBe(false);
  });

  test("a dismissed request is not shown again by a listing", () => {
    const next = reconcileJoins(new Map(), [ev("r1", DevJoinState.FAILED)], new Set(["r1"]), new Set());
    expect(next.size).toBe(0);
  });
});

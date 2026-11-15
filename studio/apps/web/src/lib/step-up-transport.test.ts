// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { Code, ConnectError, type Transport } from "@connectrpc/connect";
import { needsStepUp, sharedBroker, withStepUp } from "./step-up-transport.ts";

/** A transport whose unary calls answer from a list of outcomes, in order. */
function scripted(outcomes: readonly (Error | "ok")[]): { transport: Transport; calls: () => number } {
  let n = 0;
  const transport = {
    unary: () => {
      const outcome = outcomes[Math.min(n, outcomes.length - 1)];
      n += 1;
      return outcome === "ok" ? Promise.resolve({ message: {} }) : Promise.reject(outcome);
    },
    stream: () => Promise.reject(new Error("no streams")),
  } as unknown as Transport;
  return { transport, calls: () => n };
}

const stepUp = new ConnectError("PLX-6200: this action needs a step-up with a security key", Code.PermissionDenied);
const tooOld = new ConnectError("PLX-6201: the last step-up is more than 5 minutes old", Code.PermissionDenied);
const denied = new ConnectError("PLX-8030: teams.manage is required", Code.PermissionDenied);

const call = (t: Transport, name = "CreateTeam") =>
  // biome-ignore lint/suspicious/noExplicitAny: the wrapper forwards whatever Connect passes.
  t.unary({ name } as any, undefined, undefined, undefined, {} as any, undefined);

describe("a call that needs a step-up asks once and is sent again [SEC-103]", () => {
  test("PLX-6200 and PLX-6201 need a step-up; other refusals do not", () => {
    expect(needsStepUp(stepUp)).toBe(true);
    expect(needsStepUp(tooOld)).toBe(true);
    expect(needsStepUp(denied)).toBe(false);
    expect(needsStepUp(new Error("PLX-6200"))).toBe(false);
  });

  test("confirmed: the refused call is sent again and its answer returned", async () => {
    const inner = scripted([stepUp, "ok"]);
    let asked = 0;
    const t = withStepUp(inner.transport, () => async () => {
      asked += 1;
      return true;
    });
    await call(t);
    expect(asked).toBe(1);
    expect(inner.calls()).toBe(2);
  });

  test("cancelled, or no one to ask: the refusal comes back unchanged", async () => {
    const cancelled = withStepUp(scripted([stepUp]).transport, () => async () => false);
    await expect(call(cancelled)).rejects.toBe(stepUp);
    const nobody = withStepUp(scripted([stepUp]).transport, () => undefined);
    await expect(call(nobody)).rejects.toBe(stepUp);
  });

  test("another refusal is never retried, and a second step-up refusal is returned", async () => {
    let asked = 0;
    const ask = () => async () => {
      asked += 1;
      return true;
    };
    await expect(call(withStepUp(scripted([denied]).transport, ask))).rejects.toBe(denied);
    expect(asked).toBe(0);
    const twice = scripted([stepUp, tooOld]);
    await expect(call(withStepUp(twice.transport, ask))).rejects.toBe(tooOld);
    expect(twice.calls()).toBe(2);
  });

  test("a refused read is left to its page, which offers the confirmation", async () => {
    let asked = 0;
    const t = withStepUp(scripted([stepUp]).transport, () => async () => {
      asked += 1;
      return true;
    });
    await expect(call(t, "ListSessions")).rejects.toBe(stepUp);
    expect(asked).toBe(0);
  });

  test("calls refused together share one ceremony", async () => {
    let runs = 0;
    let release: (ok: boolean) => void = () => undefined;
    const broker = sharedBroker(() => {
      runs += 1;
      return new Promise<boolean>((resolve) => {
        release = resolve;
      });
    });
    const a = broker();
    const b = broker();
    release(true);
    expect(await Promise.all([a, b])).toEqual([true, true]);
    expect(runs).toBe(1);
    void broker();
    expect(runs).toBe(2);
  });
});

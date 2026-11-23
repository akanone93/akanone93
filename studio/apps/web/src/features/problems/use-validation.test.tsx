// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import type { Problem, ProjectFile } from "@plux/validator";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { useValidation } from "./use-validation.ts";
import type { ValidatorClient, ValidatorFactory } from "./validator-client.ts";

afterEach(cleanup);

const encoder = new TextEncoder();
const doc = (path: string, text: string) => ({ path, content: encoder.encode(text) });
const NO_ASSETS: readonly string[] = [];

/** A validator in memory: counts its workers and loads; a load waits until the test lets it finish. */
function fakeValidators(fail = false) {
  const made: { loads: ProjectFile[][]; disposed: boolean }[] = [];
  const finish: (() => void)[] = [];
  const factory: ValidatorFactory = () => {
    const me = { loads: [] as ProjectFile[][], disposed: false };
    made.push(me);
    const client: ValidatorClient = {
      load: (files) => {
        me.loads.push([...files]);
        return new Promise<readonly Problem[]>((resolve, reject) => {
          finish.push(() => (fail ? reject(new Error("the worker died")) : resolve([])));
        });
      },
      validatePage: async () => [],
      dispose: () => {
        me.disposed = true;
      },
    };
    return client;
  };
  const release = () =>
    act(() => {
      for (const f of finish.splice(0)) {
        f();
      }
    });
  return { factory, made, release };
}

describe("the validator across refetches (build log entry 252)", () => {
  test("the same documents again load nothing; a check stays usable while changed ones load", async () => {
    const v = fakeValidators();
    const app = [doc("app.json", "{}")];
    const plugin = [doc("plugins/a/plugin.json", '{"key":"a"}')];
    const { result, rerender } = renderHook(
      ({ a, p }: { a: typeof app; p: typeof plugin }) => useValidation(v.factory, a, p, NO_ASSETS),
      { initialProps: { a: app, p: plugin } },
    );
    await waitFor(() => expect(v.made[0]?.loads.length).toBe(1));
    await v.release();
    await waitFor(() => expect(result.current.state).toBe("done"));

    // A refetch when the window regains focus: new arrays, the same content.
    rerender({ a: [doc("app.json", "{}")], p: [doc("plugins/a/plugin.json", '{"key":"a"}')] });
    expect(v.made.length).toBe(1);
    expect(v.made[0]?.loads.length).toBe(1);
    expect(result.current.state).toBe("done");

    // An edit: loaded into the same worker, and the last check stays usable meanwhile.
    rerender({ a: app, p: [doc("plugins/a/plugin.json", '{"key":"b"}')] });
    await waitFor(() => expect(v.made[0]?.loads.length).toBe(2));
    expect(v.made.length).toBe(1);
    expect(result.current.state).toBe("done");
    expect(result.current.state === "done" && typeof result.current.validatePage).toBe("function");
    await v.release();
    expect(v.made[0]?.disposed).toBe(false);
  });

  test("a load that fails starts a new worker once for the same files, then says it failed", async () => {
    const v = fakeValidators(true);
    const { result } = renderHook(() =>
      useValidation(v.factory, [doc("app.json", "{}")], [doc("plugins/a/plugin.json", "{}")], NO_ASSETS),
    );
    await waitFor(() => expect(v.made[0]?.loads.length).toBe(1));
    await v.release();
    await waitFor(() => expect(v.made[1]?.loads.length).toBe(1));
    expect(v.made[0]?.disposed).toBe(true);
    await v.release();
    await waitFor(() => expect(result.current.state).toBe("failed"));
    expect(v.made.length).toBe(2);
  });
});

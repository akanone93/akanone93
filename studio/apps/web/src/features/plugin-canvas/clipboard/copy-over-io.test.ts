// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { sourceReader } from "./copy-over-io.ts";

const enc = (v: unknown) => new TextEncoder().encode(JSON.stringify(v));

describe("STU-067 the source reader uses the user's own access", () => {
  test("finds a document by id and a translation by key", async () => {
    const calls: string[] = [];
    const client = {
      listDocuments: async (r: { pluginId: string }) => {
        calls.push(r.pluginId);
        return {
          documents:
            r.pluginId === "p"
              ? [{ content: enc({ id: "d1" }) }, { content: enc({ id: "t", messages: { hi: "Hi" } }) }]
              : [],
        };
      },
    };
    const read = sourceReader(client as never, { app: "a", plugin: "p" });
    expect(await read("dataSources", "d1")).toEqual({ content: { id: "d1" } });
    expect(await read("translations", "hi")).toEqual({ content: { id: "t", messages: { hi: "Hi" } } });
    expect(await read("dataSources", "none")).toBeUndefined();
    expect(calls).toEqual(["p", ""]);
  });

  test("a source the server refuses reads as nothing found", async () => {
    const client = {
      listDocuments: async () => {
        throw new Error("permission denied");
      },
    };
    expect(await sourceReader(client as never, { app: "a", plugin: "p" })("assets", "x")).toBeUndefined();
  });
});

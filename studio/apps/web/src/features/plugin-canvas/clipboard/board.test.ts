// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { type Board, CLIPBOARD_WEB_TYPE, readEnvelopeText, writeEnvelope } from "./board.ts";

class FakeItem {
  readonly types: string[];
  constructor(readonly data: Record<string, Blob>) {
    this.types = Object.keys(data);
  }
  async getType(type: string): Promise<Blob> {
    const blob = this.data[type];
    if (blob === undefined) {
      throw new Error("no such type");
    }
    return blob;
  }
}

const g = globalThis as unknown as { ClipboardItem?: unknown };
const original = g.ClipboardItem;
afterEach(() => {
  g.ClipboardItem = original;
});

const fake = () => {
  const state = { items: [] as FakeItem[], text: "" };
  const board: Board = {
    writeText: async (t) => {
      state.text = t;
    },
    readText: async () => state.text,
    write: async (items) => {
      state.items = items as unknown as FakeItem[];
    },
    read: async () => state.items as unknown as ClipboardItems,
  };
  return { state, board };
};

describe("STU-067 the clipboard format", () => {
  test("writes a ClipboardItem under the web custom type plus the text/plain fallback", async () => {
    g.ClipboardItem = FakeItem;
    const { state, board } = fake();
    await writeEnvelope(board, '{"a":1}');
    expect(state.items[0]?.types).toEqual([CLIPBOARD_WEB_TYPE, "text/plain"]);
    expect(CLIPBOARD_WEB_TYPE).toBe("web application/x-plux+json");
    expect(await readEnvelopeText(board)).toBe('{"a":1}');
  });

  test("read prefers the custom type over the text", async () => {
    const { state, board } = fake();
    state.items = [
      new FakeItem({
        "text/plain": new Blob(["plain"]),
        [CLIPBOARD_WEB_TYPE]: new Blob(["custom"]),
      }),
    ];
    expect(await readEnvelopeText(board)).toBe("custom");
  });

  test("read falls back to text/plain, then to readText", async () => {
    const { state, board } = fake();
    state.items = [new FakeItem({ "text/plain": new Blob(["plain"]) })];
    expect(await readEnvelopeText(board)).toBe("plain");
    state.items = [];
    state.text = "via readText";
    expect(await readEnvelopeText(board)).toBe("via readText");
  });

  test("without ClipboardItem it writes text, and a refused custom type falls back too", async () => {
    g.ClipboardItem = undefined;
    const a = fake();
    await writeEnvelope(a.board, "x");
    expect(a.state.text).toBe("x");
    expect(a.state.items).toEqual([]);

    g.ClipboardItem = FakeItem;
    const b = fake();
    const refusing: Board = {
      ...b.board,
      write: async () => {
        throw new Error("NotAllowedError");
      },
    };
    await writeEnvelope(refusing, "y");
    expect(b.state.text).toBe("y");
  });
});

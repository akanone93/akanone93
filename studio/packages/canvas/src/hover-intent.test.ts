// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { HOVER_ENTER_MS, HOVER_LEAVE_MS, HoverIntent, type IntentTimers } from "./hover-intent.ts";

/** A fake clock: timers run when `advance` passes their time. */
function clock() {
  let at = 0;
  let next = 1;
  const due = new Map<number, { at: number; run: () => void }>();
  const timers: IntentTimers = {
    set: (run, ms) => {
      due.set(next, { at: at + ms, run });
      return next++;
    },
    clear: (handle) => void due.delete(handle),
  };
  const advance = (ms: number) => {
    at += ms;
    for (const [handle, t] of [...due].sort((a, b) => a[1].at - b[1].at)) {
      if (t.at <= at) {
        due.delete(handle);
        t.run();
      }
    }
  };
  return { timers, advance };
}

describe("[STU-044] hover intent of the arrows (#206)", () => {
  test("an arrow lights up only after the pointer rests on it, so a sweep across arrows lights none", () => {
    const c = clock();
    const shown: (string | null)[] = [];
    const intent = new HoverIntent<string>((v) => shown.push(v), c.timers);
    for (const id of ["a", "b", "c", "d"]) {
      intent.hover(id);
      c.advance(HOVER_ENTER_MS / 4);
    }
    intent.hover(null);
    c.advance(HOVER_LEAVE_MS * 2);
    expect(shown).toEqual([]);
    intent.hover("e");
    c.advance(HOVER_ENTER_MS);
    expect(shown).toEqual(["e"]);
  });

  test("a lit arrow survives a short gap and goes out after the leave delay", () => {
    const c = clock();
    const shown: (string | null)[] = [];
    const intent = new HoverIntent<string>((v) => shown.push(v), c.timers);
    intent.hover("a");
    c.advance(HOVER_ENTER_MS);
    intent.hover(null);
    c.advance(HOVER_LEAVE_MS / 2);
    intent.hover("a");
    c.advance(HOVER_LEAVE_MS * 2);
    expect(shown).toEqual(["a"]);
    intent.hover(null);
    c.advance(HOVER_LEAVE_MS);
    expect(shown).toEqual(["a", null]);
  });

  test("a click or keyboard focus acts at once and drops a waiting hover", () => {
    const c = clock();
    const shown: (string | null)[] = [];
    const intent = new HoverIntent<string>((v) => shown.push(v), c.timers);
    intent.hover("a");
    intent.now("b");
    c.advance(HOVER_ENTER_MS * 2);
    expect(shown).toEqual(["b"]);
    intent.dispose();
  });
});

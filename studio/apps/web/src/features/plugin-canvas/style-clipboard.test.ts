// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, mock, test } from "bun:test";
import { applyLive, emptyLive } from "./live.ts";
import type { Node } from "./plugin-model.ts";
import { withStates } from "./state-preview.ts";
import { copyStyle, pasteStyleRecipe, styleCommands, styleFor } from "./style-clipboard.ts";

const style = {
  backgroundColor: { value: { $token: "color.tertiary" } },
  shape: { value: { kind: "stadium" } },
};

describe("PC-21 style copy and paste [STU-062]", () => {
  test("copying keeps a widget's appearance props, deep-copied; layout and content stay behind", () => {
    const node = { type: "FilledButton", props: { style, autofocus: true } };
    const copied = copyStyle({ type: "Container", props: { color: "#FF0000FF", padding: { all: 8 }, width: 40 } });
    expect(copied).toEqual({ from: "Container", props: { color: "#FF0000FF" } });
    const button = copyStyle(node);
    expect(button?.props).toEqual({ style });
    expect(button?.props["style"]).not.toBe(style);
    expect(copyStyle({ type: "SizedBox", props: { width: 4 } })).toBeUndefined();
  });

  test("pasting replaces the target's style with the props its widget declares", () => {
    const copied = { from: "Container", props: { color: "#00FF00FF", decoration: { color: "#000000FF" } } };
    expect(styleFor("FilledButton", copied)).toEqual({});
    const root = {
      id: "r",
      type: "Column",
      children: [{ id: "c", type: "Container", props: { color: "#FF0000FF", width: 10, foregroundDecoration: {} } }],
    };
    pasteStyleRecipe("c", copied)(root);
    expect(root.children[0]?.props as unknown).toEqual({
      width: 10,
      color: "#00FF00FF",
      decoration: { color: "#000000FF" },
    });
  });

  test("the commands copy the first selected widget and paste on every one that takes it, in one undo step", () => {
    const nodes: Record<string, { type: string; props: Record<string, unknown> }> = {
      a: { type: "FilledButton", props: { style } },
      b: { type: "OutlinedButton", props: {} },
      t: { type: "Text", props: { data: "x" } },
    };
    let held: ReturnType<typeof copyStyle>;
    let selected = ["a"];
    const edit = mock((_doc: string, _recipe: (d: unknown) => void, _o?: { group?: string }) => undefined);
    const note = mock((_: string) => undefined);
    const [copy, paste] = styleCommands({
      selected: () => selected,
      nodeOf: (id) => nodes[id],
      docOf: () => "page",
      edit,
      held: () => held,
      hold: (s) => {
        held = s;
      },
      note,
      text: { copy: "Copy", paste: "Paste", copied: (t) => `copied ${t}`, pasted: (n) => `pasted ${n}`, none: "none" },
    });
    expect(copy?.shortcut).toBe("mod+alt+c");
    copy?.run();
    expect(note).toHaveBeenLastCalledWith("copied FilledButton");
    selected = ["b", "t"];
    paste?.run();
    expect(edit).toHaveBeenCalledTimes(1);
    expect(note).toHaveBeenLastCalledWith("pasted 1");
    selected = ["t"];
    paste?.run();
    expect(note).toHaveBeenLastCalledWith("none");
  });

  test("a previewed state is drawn on the selection only, and only for a state other than Default", () => {
    const root = { id: "r", type: "Center", slots: { child: { id: "b", type: "FilledButton" } } } as unknown as Node;
    expect(withStates(emptyLive, ["b"], "value")).toBe(emptyLive);
    const shown = applyLive(root, withStates(emptyLive, ["b"], "pressed")) as unknown as {
      slots: { child: Record<string, unknown> };
    };
    expect(shown.slots.child["x-studio"]).toEqual({ state: "pressed" });
    expect(applyLive(root, emptyLive)).toBe(root);
  });
});

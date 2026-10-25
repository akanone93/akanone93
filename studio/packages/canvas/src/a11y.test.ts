// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { box, link, text } from "../test-support/canvaskit.ts";
import { type A11yDocumentLike, type A11yElementLike, AccessibleTree } from "./a11y.ts";
import { CanvasHost, type WorkerLike } from "./host.ts";
import type { FrameSpec } from "./protocol.ts";

class FakeElement implements A11yElementLike {
  readonly style = { position: "", left: "", top: "", width: "", height: "", transform: "", transformOrigin: "" };
  readonly attributes = new Map<string, string>();
  readonly children: FakeElement[] = [];
  parent: FakeElement | null = null;
  setAttribute(name: string, value: string): void {
    this.attributes.set(name, value);
  }
  removeAttribute(name: string): void {
    this.attributes.delete(name);
  }
  appendChild(child: A11yElementLike): void {
    const node = child as FakeElement;
    node.remove();
    node.parent = this;
    this.children.push(node);
  }
  remove(): void {
    if (this.parent) {
      this.parent.children.splice(this.parent.children.indexOf(this), 1);
      this.parent = null;
    }
  }
}

class FakeDocument implements A11yDocumentLike {
  created = 0;
  createElement(): FakeElement {
    this.created++;
    return new FakeElement();
  }
}

function frame(id: string, x: number, y: number): FrameSpec {
  return {
    kind: "frame",
    id,
    name: `Screen ${id}`,
    x,
    y,
    w: 390,
    h: 844,
    radius: 40,
    fill: 0xffffffff,
    shadow: true,
    z: 0,
  };
}

function tree(): { tree: AccessibleTree; root: FakeElement; document: FakeDocument } {
  const document = new FakeDocument();
  const root = new FakeElement();
  return { tree: new AccessibleTree(document, root), root, document };
}

function el(tree: AccessibleTree, id: string): FakeElement {
  return tree.element(id) as FakeElement;
}

describe("accessible tree (ADR-0013)", () => {
  test("mirrors frames and items with role, name and bounds, items inside their frame", () => {
    const { tree: t, root } = tree();
    t.apply([
      { op: "add", item: frame("home", 100, 50) },
      { op: "add", item: { ...box("button", 120, 80), frame: "home", label: "Sign in" } },
      { op: "add", item: { ...text("title", 110, 60, "Welcome"), frame: "home" } },
      { op: "add", item: link("l", 0, 0, 10, 10) },
    ]);
    expect(root.attributes.get("role")).toBe("application");
    expect(root.children.map((c) => c.attributes.get("data-id"))).toEqual(["home", "l"]);
    const home = el(t, "home");
    expect(home.attributes.get("role")).toBe("group");
    expect(home.attributes.get("aria-label")).toBe("Screen home");
    expect(home.style.left).toBe("100px");
    expect(home.style.width).toBe("390px");
    expect(home.children.map((c) => c.attributes.get("data-id"))).toEqual(["button", "title"]);
    const button = el(t, "button");
    expect(button.attributes.get("role")).toBe("img");
    expect(button.attributes.get("aria-label")).toBe("Sign in");
    expect([button.style.left, button.style.top, button.style.width, button.style.height]).toEqual([
      "20px",
      "30px",
      "100px",
      "50px",
    ]);
    expect(el(t, "title").attributes.get("role")).toBe("paragraph");
    expect(el(t, "l").attributes.has("aria-label")).toBe(false);
  });

  test("updates incrementally: one element per change, children follow their frame", () => {
    const { tree: t, document } = tree();
    t.apply([
      { op: "add", item: frame("f", 0, 0) },
      { op: "add", item: { ...box("b", 10, 10), frame: "f" } },
    ]);
    const created = document.created;
    t.apply([{ op: "move", id: "b", dx: 5, dy: 0 }]);
    expect(el(t, "b").style.left).toBe("15px");
    t.apply([{ op: "move", id: "f", dx: 100, dy: 0 }]);
    // The scene does not move children with a frame: relative to it, the box moved left.
    expect(el(t, "b").style.left).toBe("-85px");
    t.apply([{ op: "replace", item: { ...box("b", 0, 0), frame: "f", label: "Renamed" } }]);
    expect(el(t, "b").attributes.get("aria-label")).toBe("Renamed");
    expect(document.created).toBe(created);
  });

  test("parks items whose frame is missing at the root and adopts them when it arrives", () => {
    const { tree: t, root } = tree();
    t.apply([{ op: "add", item: { ...box("b", 10, 10), frame: "late" } }]);
    expect(el(t, "b").parent).toBe(root);
    t.apply([{ op: "add", item: frame("late", 0, 0) }]);
    expect(el(t, "b").parent).toBe(el(t, "late"));
    t.apply([{ op: "remove", id: "late" }]);
    expect(el(t, "b").parent).toBe(root);
    expect(el(t, "b").style.left).toBe("10px");
    t.apply([{ op: "clear" }]);
    expect(t.size).toBe(0);
    expect(root.children).toHaveLength(0);
  });

  test("follows the view with one transform on the root", () => {
    const { tree: t, root } = tree();
    t.setView(100, 50, 2);
    expect(root.style.transform).toBe("scale(2) translate(-100px, -50px)");
  });

  test("the host feeds the tree from its patches and the view", () => {
    const { tree: t, root } = tree();
    const worker: WorkerLike = { onmessage: null, postMessage: () => undefined, terminate: () => undefined };
    const element = { width: 0, height: 0, transferControlToOffscreen: () => ({ width: 0, height: 0 }) };
    const host = new CanvasHost(
      element,
      { width: 300, height: 200, dpr: 1 },
      { renderer: "raster", tileSize: 256, background: 0, fonts: [], fontFamily: "Roboto", measureFrames: false },
      {
        createWorker: () => worker,
        mainThread: () => {
          throw new Error("unused");
        },
      },
      null,
      { tree: t },
    );
    host.patch([{ op: "add", item: frame("f", 0, 0) }]);
    expect(t.size).toBe(1);
    host.pan(-20, 0);
    expect(root.style.transform).toBe("scale(1) translate(-20px, 0px)");
  });
});

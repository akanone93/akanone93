// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The accessible-tree feed (ADR-0013): a parallel DOM tree that mirrors the
 * canvas's frames and items for screen readers, with a role, a name and the
 * bounds each occupies on screen. It is updated incrementally from the patch
 * stream the scene receives: an add creates one element, a replace or move
 * updates one (and a frame's children), a remove deletes one. Items sit
 * inside the element of their frame; positions are in world units inside a
 * root whose single transform follows the view, so a pan or zoom is one
 * style write however large the scene.
 */

import type { ItemSpec, SceneOp } from "./protocol.ts";
import { moveSpec, TEXT_HEIGHT_FACTOR } from "./scene.ts";

/** The style properties the tree sets. */
export interface A11yStyleLike {
  position: string;
  left: string;
  top: string;
  width: string;
  height: string;
  transform: string;
  transformOrigin: string;
}

/** The DOM element calls the tree uses; an `HTMLElement` in the browser. */
export interface A11yElementLike {
  readonly style: A11yStyleLike;
  setAttribute(name: string, value: string): void;
  removeAttribute(name: string): void;
  appendChild(child: A11yElementLike): unknown;
  remove(): void;
}

/** Creates elements; `document` in the browser. */
export interface A11yDocumentLike {
  createElement(tag: "div"): A11yElementLike;
}

/** The ARIA role of an item kind. */
export const ROLES: Readonly<Record<ItemSpec["kind"], string>> = {
  frame: "group",
  rect: "img",
  text: "paragraph",
  link: "img",
};

interface Node {
  spec: ItemSpec;
  readonly element: A11yElementLike;
  /** The frame this node is placed in, or null at the root. */
  parent: string | null;
}

function frameOf(spec: ItemSpec): string | null {
  return spec.kind === "rect" || spec.kind === "text" ? (spec.frame ?? null) : null;
}

function nameOf(spec: ItemSpec): string {
  switch (spec.kind) {
    case "frame":
      return spec.name;
    case "text":
      return spec.text;
    case "rect":
      return spec.label ?? "";
    default:
      return "";
  }
}

/** The world box of `spec`: x, y, width, height. */
function boxOf(spec: ItemSpec): { x: number; y: number; w: number; h: number } {
  switch (spec.kind) {
    case "link": {
      const xs = [spec.from.x, spec.to.x, spec.c1?.x ?? spec.from.x, spec.c2?.x ?? spec.to.x];
      const ys = [spec.from.y, spec.to.y, spec.c1?.y ?? spec.from.y, spec.c2?.y ?? spec.to.y];
      const x = Math.min(...xs);
      const y = Math.min(...ys);
      return { x, y, w: Math.max(...xs) - x, h: Math.max(...ys) - y };
    }
    case "text":
      return { x: spec.x, y: spec.y, w: spec.w, h: spec.size * TEXT_HEIGHT_FACTOR };
    default:
      return { x: spec.x, y: spec.y, w: spec.w, h: spec.h };
  }
}

/** The DOM mirror of the scene for assistive technology. */
export class AccessibleTree {
  private readonly nodes = new Map<string, Node>();
  /** Items whose frame is not in the tree yet, by frame id. */
  private readonly waiting = new Map<string, Set<string>>();

  /** Mirrors the scene under `root`, which the host places over the canvas. */
  constructor(
    private readonly document: A11yDocumentLike,
    readonly root: A11yElementLike,
  ) {
    root.setAttribute("role", "application");
    const style = root.style;
    style.position = "absolute";
    style.left = "0";
    style.top = "0";
    style.transformOrigin = "0 0";
  }

  /** The number of mirrored items. */
  get size(): number {
    return this.nodes.size;
  }

  /** The element mirroring item `id`, if any. */
  element(id: string): A11yElementLike | undefined {
    return this.nodes.get(id)?.element;
  }

  /** Follows the view: world top-left (`x`, `y`) at `zoom`. */
  setView(x: number, y: number, zoom: number): void {
    this.root.style.transform = `scale(${zoom}) translate(${-x}px, ${-y}px)`;
  }

  /** Applies scene patches in order, exactly as the scene does. */
  apply(ops: readonly SceneOp[]): void {
    for (const op of ops) {
      switch (op.op) {
        case "add":
        case "replace":
          this.put(op.item);
          break;
        case "move": {
          const node = this.nodes.get(op.id);
          if (node) {
            this.put(moveSpec(node.spec, op.dx, op.dy));
          }
          break;
        }
        case "remove":
          this.remove(op.id);
          break;
        case "clear":
          for (const id of [...this.nodes.keys()]) {
            this.remove(id);
          }
          break;
      }
    }
  }

  private put(spec: ItemSpec): void {
    let node = this.nodes.get(spec.id);
    if (!node) {
      const element = this.document.createElement("div");
      element.style.position = "absolute";
      element.setAttribute("data-id", spec.id);
      node = { spec, element, parent: null };
      this.nodes.set(spec.id, node);
    }
    node.spec = spec;
    const element = node.element;
    element.setAttribute("role", ROLES[spec.kind]);
    const name = nameOf(spec);
    if (name) {
      element.setAttribute("aria-label", name);
    } else {
      element.removeAttribute("aria-label");
    }
    this.place(node);
    if (spec.kind === "frame") {
      this.adoptWaiting(spec.id);
      for (const child of this.nodes.values()) {
        if (child.parent === spec.id) {
          this.position(child);
        }
      }
    }
  }

  /** Puts `node` into its frame's element, or the root while the frame is missing. */
  private place(node: Node): void {
    const frame = frameOf(node.spec);
    const owner = frame === null ? undefined : this.nodes.get(frame);
    this.unwait(node.spec.id);
    if (frame !== null && !owner) {
      let set = this.waiting.get(frame);
      if (!set) {
        set = new Set();
        this.waiting.set(frame, set);
      }
      set.add(node.spec.id);
    }
    const parent = owner ? frame : null;
    (owner ? owner.element : this.root).appendChild(node.element);
    node.parent = parent;
    this.position(node);
  }

  private unwait(id: string): void {
    for (const [frame, set] of this.waiting) {
      if (set.delete(id) && set.size === 0) {
        this.waiting.delete(frame);
      }
    }
  }

  private adoptWaiting(frame: string): void {
    const set = this.waiting.get(frame);
    if (!set) {
      return;
    }
    this.waiting.delete(frame);
    for (const id of set) {
      const node = this.nodes.get(id);
      if (node) {
        this.place(node);
      }
    }
  }

  private position(node: Node): void {
    const box = boxOf(node.spec);
    const owner = node.parent === null ? undefined : this.nodes.get(node.parent);
    const origin = owner ? boxOf(owner.spec) : { x: 0, y: 0 };
    const style = node.element.style;
    style.left = `${box.x - origin.x}px`;
    style.top = `${box.y - origin.y}px`;
    style.width = `${box.w}px`;
    style.height = `${box.h}px`;
  }

  private remove(id: string): void {
    const node = this.nodes.get(id);
    if (!node) {
      return;
    }
    this.nodes.delete(id);
    this.unwait(id);
    if (node.spec.kind === "frame") {
      // Children outlive their frame in the scene; they move to the root until it returns.
      for (const child of this.nodes.values()) {
        if (child.parent === id) {
          this.place(child);
        }
      }
    }
    node.element.remove();
  }
}

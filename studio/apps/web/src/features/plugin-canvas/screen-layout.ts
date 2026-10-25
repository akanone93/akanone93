// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import {
  Alignment,
  Axis,
  CrossAxisAlignment,
  EdgeInsets,
  globalRects,
  MainAxisSize,
  PipelineOwner,
  type RenderBox,
  RenderFlex,
  RenderPadding,
  RenderPositionedBox,
  RenderTestBox,
  RenderView,
  Size,
  TextDirection,
} from "@plux/layout";
import type { Node } from "./plugin-model.ts";

/** The device frame every screen is drawn in, in logical pixels. */
export const DEVICE = { width: 390, height: 844 } as const;
const PAGE_PADDING = 16;
const APP_BAR_HEIGHT = 56;
const CONTROL_HEIGHT = 48;
const TEXT_SIZE = 16;
const LINE_HEIGHT = 1.4;
const GLYPH_WIDTH = 0.55;
/** Tall enough for any screen; the content is measured, not clipped to it. */
const MEASURE_HEIGHT = 100_000;

/** One laid-out box of a screen, relative to the screen's top-left. */
export interface Placed {
  readonly id: string;
  readonly kind: "box" | "text";
  readonly text?: string;
  readonly x: number;
  readonly y: number;
  readonly w: number;
  readonly h: number;
}

export interface ScreenLayout {
  readonly boxes: readonly Placed[];
  /** Height of the content; beyond DEVICE.height the screen overflows (STU-056). */
  readonly contentHeight: number;
}

function textBox(id: string, text: string, available: number): RenderBox {
  const natural = text.length * TEXT_SIZE * GLYPH_WIDTH;
  const lines = Math.max(1, Math.ceil(natural / available));
  const box = new RenderTestBox({ size: new Size(Math.min(natural, available), lines * TEXT_SIZE * LINE_HEIGHT) });
  box.debugId = id;
  return box;
}

function fixed(id: string, w: number, h: number): RenderBox {
  const box = new RenderTestBox({ size: new Size(w, h) });
  box.debugId = id;
  return box;
}

/** A control with at most one text child: the control is drawn, its label inside it. */
function control(node: Node, only: Node | undefined, available: number, texts: Map<string, string>): RenderBox {
  const title = only === undefined ? undefined : firstLabel(only);
  if (title === undefined) {
    return fixed(node.id, Math.min(available, 120), CONTROL_HEIGHT);
  }
  texts.set(node.id, title);
  return fixed(node.id, Math.min(available, title.length * TEXT_SIZE * GLYPH_WIDTH + 32), CONTROL_HEIGHT);
}

const SKIPPED_SLOTS = new Set(["appBar"]);

function build(node: Node, available: number, texts: Map<string, string>): RenderBox {
  const label = node.type === "Text" && typeof node.props?.data === "string" ? node.props.data : undefined;
  if (label !== undefined) {
    texts.set(node.id, label);
    return textBox(node.id, label, available);
  }
  const nested = [
    ...(node.children ?? []),
    ...Object.entries(node.slots ?? {})
      .filter(([name]) => !SKIPPED_SLOTS.has(name))
      .map(([, child]) => child),
  ];
  const isStack = (node.children?.length ?? 0) > 0 || nested.length > 1;
  if (!isStack) {
    const only = nested[0];
    return only !== undefined && node.events === undefined
      ? build(only, available, texts)
      : control(node, only, available, texts);
  }
  const horizontal = node.type === "Row";
  return Object.assign(
    new RenderFlex({
      direction: horizontal ? Axis.horizontal : Axis.vertical,
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.start,
      textDirection: TextDirection.ltr,
      spacing: 8,
      children: nested.map((c) => build(c, horizontal ? available / Math.max(1, nested.length) : available, texts)),
    }),
    { debugId: node.id },
  );
}

function firstLabel(node: Node): string | undefined {
  if (node.type === "Text" && typeof node.props?.data === "string") {
    return node.props.data;
  }
  for (const child of [...(node.children ?? []), ...Object.values(node.slots ?? {})]) {
    const t = firstLabel(child);
    if (t !== undefined) {
      return t;
    }
  }
  return undefined;
}

/**
 * Lays one screen out with the Flutter-faithful layout (STU-005) at the device width. Text is
 * measured by an estimate until the CanvasKit paragraph engine is wired into the web app.
 */
export function layoutScreen(root: Node | undefined, title: string): ScreenLayout {
  const texts = new Map<string, string>();
  const rootId = "root";
  const bar = root?.slots?.["appBar"];
  const barBox = fixed("appBar", DEVICE.width, APP_BAR_HEIGHT);
  if (bar !== undefined) {
    texts.set("appBar", title);
  }
  const content = Object.assign(
    new RenderFlex({
      direction: Axis.vertical,
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.start,
      textDirection: TextDirection.ltr,
      children: [
        ...(bar === undefined ? [] : [barBox]),
        new RenderPadding({
          padding: EdgeInsets.all(PAGE_PADDING),
          child: root === undefined ? fixed("empty", 1, 0) : build(root, DEVICE.width - 2 * PAGE_PADDING, texts),
        }),
      ],
    }),
    { debugId: rootId },
  );
  const view = new RenderView({
    size: new Size(DEVICE.width, MEASURE_HEIGHT),
    child: new RenderPositionedBox({ alignment: Alignment.topLeft, child: content }),
  });
  const owner = new PipelineOwner();
  owner.rootNode = view;
  view.prepareInitialFrame();
  owner.flushLayout();
  const rects = globalRects(view);
  const boxes: Placed[] = [];
  for (const [id, r] of rects) {
    if (id === rootId || id === "empty") {
      continue;
    }
    const text = texts.get(id);
    boxes.push({
      id,
      kind: text !== undefined && root !== undefined && isTextNode(id, root) ? "text" : "box",
      ...(text === undefined ? {} : { text }),
      x: r.x,
      y: r.y,
      w: r.w,
      h: r.h,
    });
  }
  const contentHeight = rects.get(rootId)?.h ?? 0;
  return { boxes, contentHeight };
}

function isTextNode(id: string, root: Node): boolean {
  if (root.id === id) {
    return root.type === "Text";
  }
  for (const child of [...(root.children ?? []), ...Object.values(root.slots ?? {})]) {
    if (isTextNode(id, child)) {
      return true;
    }
  }
  return false;
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Global rectangles of a laid-out render tree, keyed by Plux node ID. The
 * layout conformance suite (STU-005) compares exactly these against goldens
 * captured from Flutter on a device.
 */

import { Offset } from "./flutter/geometry.ts";
import { Affine, isPaintTransformer } from "./flutter/matrix.ts";
import type { RenderObject } from "./flutter/object.ts";
import { RenderParagraph } from "./flutter/paragraph.ts";
import { RenderBox, TextBaseline } from "./flutter/render-box.ts";
import { providesChildPaintOffset, RenderSliver } from "./flutter/sliver.ts";

/** A rectangle in logical pixels relative to the root's origin. */
export interface NodeRect {
  readonly x: number;
  readonly y: number;
  readonly w: number;
  readonly h: number;
}

/** A layout golden: the rect of every identified node of one scene under one configuration. */
export interface LayoutGolden {
  /** The Flutter framework revision the golden was captured with. */
  flutterRevision: string;
  /** The scene's name. */
  scene: string;
  /** The frame's logical size and its device pixel ratio. */
  frame: { width: number; height: number; devicePixelRatio: number };
  /** The BCP 47 locale the scene was rendered in. */
  locale: string;
  /** The text scale factor. */
  textScale: number;
  /** `[x, y, width, height]` per Plux node ID, in logical pixels. */
  rects: Record<string, [number, number, number, number]>;
  /** Alphabetic baseline distance from the top of each Text node, in logical pixels (absent without Text). */
  baselines?: Record<string, number>;
}

/**
 * Walks the tree from `root` and returns the global rect of every laid-out
 * box or sliver with a [RenderObject.debugId]. A box's position is its corners carried
 * through the [BoxParentData] offsets (a sliver parent's child paint offsets) and paint
 * transforms from `root` down to it; a sliver's rect is its paint area. Throws if two boxes share
 * an ID, since a golden could not tell them apart.
 */
export function globalRects(root: RenderObject): Map<string, NodeRect> {
  const rects = new Map<string, NodeRect>();
  const visit = (node: RenderObject, toGlobal: Affine): void => {
    if (node.debugId !== null && node instanceof RenderSliver && node.geometry !== null) {
      // A sliver has no size: its rect is its paint area, as Flutter's golden writer measures it.
      if (rects.has(node.debugId)) {
        throw new Error(`duplicate debugId "${node.debugId}" in the render tree`);
      }
      const bounds = node.paintBounds;
      const origin = toGlobal.apply(Offset.zero);
      rects.set(node.debugId, { x: origin.dx, y: origin.dy, w: bounds.width, h: bounds.height });
    }
    if (node.debugId !== null && node instanceof RenderBox && node.hasSize) {
      if (rects.has(node.debugId)) {
        throw new Error(`duplicate debugId "${node.debugId}" in the render tree`);
      }
      // Like WidgetTester.getRect: the box's top-left and bottom-right corners, transformed.
      const topLeft = toGlobal.apply(Offset.zero);
      const bottomRight = toGlobal.apply(new Offset(node.size.width, node.size.height));
      rects.set(node.debugId, {
        x: Math.min(topLeft.dx, bottomRight.dx),
        y: Math.min(topLeft.dy, bottomRight.dy),
        w: Math.abs(bottomRight.dx - topLeft.dx),
        h: Math.abs(bottomRight.dy - topLeft.dy),
      });
    }
    node.visitChildren((child) => {
      // A paint transform replaces the child's offset (RenderObject.applyPaintTransform); a
      // sliver parent places its children by their paint offset.
      let local: Affine;
      if (isPaintTransformer(node)) {
        local = node.paintTransformOf(child);
      } else {
        const offset = providesChildPaintOffset(node) ? node.childPaintOffset(child) : RenderBox.offsetOf(child);
        local = Affine.translation(offset.dx, offset.dy);
      }
      visit(child, toGlobal.multiply(local));
    });
  };
  visit(root, Affine.identity);
  return rects;
}

/** [globalRects] in the golden's `rects` shape, with keys sorted for stable output. */
export function goldenRects(root: RenderObject): Record<string, [number, number, number, number]> {
  const rects = globalRects(root);
  const result: Record<string, [number, number, number, number]> = {};
  for (const id of [...rects.keys()].sort()) {
    const rect = rects.get(id);
    if (rect !== undefined) {
      result[id] = [rect.x, rect.y, rect.w, rect.h];
    }
  }
  return result;
}

/** The alphabetic baseline distance from the top of every identified paragraph, keys sorted. */
export function goldenBaselines(root: RenderObject): Record<string, number> {
  const found = new Map<string, number>();
  const visit = (node: RenderObject): void => {
    if (node.debugId !== null && node instanceof RenderParagraph && node.hasSize) {
      found.set(node.debugId, node.getDistanceToBaseline(TextBaseline.alphabetic) ?? Number.NaN);
    }
    node.visitChildren(visit);
  };
  visit(root);
  return Object.fromEntries([...found.keys()].sort().map((id) => [id, found.get(id) ?? Number.NaN]));
}

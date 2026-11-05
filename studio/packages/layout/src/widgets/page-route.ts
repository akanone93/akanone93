// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * A dialog or bottom-sheet page on the canvas (PC-13, SCH-022), as the runtime presents it: the page that opens it
 * underneath, a scrim over it, and the page's root on Material 3's surface — `DialogRoute` with `Dialog(child: root)`
 * (inside the safe area, inset 40 by 24, centred, at least 280 wide, a 28 corner card), or a scroll-controlled
 * `ModalBottomSheetRoute` (at the bottom, at most 640 wide, its top corners rounded by 28). The canvas's preview builds
 * the route node (`ROUTE_TYPE`) around the page; the document holds only the page.
 */

import { BoxConstraints } from "../flutter/constraints.ts";
import { ContainerRenderBox } from "../flutter/container.ts";
import { Offset, type Size } from "../flutter/geometry.ts";
import { RenderConstrainedBox } from "../flutter/proxy-box.ts";
import type { RenderBox } from "../flutter/render-box.ts";
import { setChildOffset } from "../flutter/shifted-box.ts";
import type { ViewPadding } from "../scene/scene.ts";
import type { WidgetContext, WidgetNode } from "./screen.ts";

/** The type of the design-time node the canvas wraps a dialog or sheet page in; no document holds one. */
export const ROUTE_TYPE = "__PluxRoute";

/** The kinds of page a route node presents. */
export type RouteKind = "dialog" | "bottomSheet";

/** `Dialog`'s `_defaultInsetPadding` and `BoxConstraints(minWidth: 280)`; `BottomSheet`'s 640 maximum width. */
const INSET_H = 40;
const INSET_V = 24;
const DIALOG_MIN_WIDTH = 280;
const SHEET_MAX_WIDTH = 640;

const NONE: ViewPadding = { top: 0, right: 0, bottom: 0, left: 0 };

/** The scene id of the part a page's route paints its surface in: the dialog's card (`#dialog`) or the sheet. */
export const surfacePart = (page: string, kind: RouteKind): string =>
  `${page}#${kind === "dialog" ? "dialog" : "sheet"}`;
/** The scene id of the part a page's route paints its scrim in. */
export const scrimPart = (page: string): string => `${page}#scrim`;

/** The route node's kind, opener and page. */
export function routeParts(
  node: WidgetNode,
): { kind: RouteKind; below: WidgetNode | undefined; page: WidgetNode | undefined } | undefined {
  if (node.type !== ROUTE_TYPE) {
    return undefined;
  }
  const kind =
    (node.props as Record<string, unknown> | undefined)?.["kind"] === "bottomSheet" ? "bottomSheet" : "dialog";
  const slot = (name: string) => {
    const v = node.slots?.[name];
    return v === undefined || Array.isArray(v) ? undefined : (v as WidgetNode);
  };
  return { kind, below: slot("below"), page: slot("page") };
}

/** Lays the opener, the scrim and the page's surface out at the device's size, the page where its route puts it. */
class RenderRoute extends ContainerRenderBox<RenderBox> {
  constructor(
    private readonly kind: RouteKind,
    private readonly below: RenderBox | null,
    private readonly scrim: RenderBox,
    private readonly surface: RenderBox,
    private readonly page: RenderBox | null,
    private readonly safe: ViewPadding,
  ) {
    super();
    for (const child of [below, scrim, surface, page]) {
      if (child !== null) {
        this.add(child);
      }
    }
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    return constraints.biggest;
  }

  protected override performLayout(): void {
    const size = this.constraints.biggest;
    this.size = size;
    const tight = BoxConstraints.tight(size);
    if (this.below !== null) {
      this.below.layout(tight, { parentUsesSize: true });
      setChildOffset(this.below, Offset.zero);
    }
    this.scrim.layout(tight, { parentUsesSize: true });
    setChildOffset(this.scrim, Offset.zero);
    const box = this.kind === "dialog" ? this.dialog(size) : this.sheet(size);
    this.surface.layout(BoxConstraints.tightFor({ width: box.w, height: box.h }), { parentUsesSize: true });
    setChildOffset(this.surface, new Offset(box.x, box.y));
  }

  /** `DialogRoute` (in a `SafeArea`) around `Dialog`: inset, centred, at least 280 wide. */
  private dialog(size: Size): { x: number; y: number; w: number; h: number } {
    const s = this.safe;
    const left = s.left + INSET_H;
    const top = s.top + INSET_V;
    const maxW = Math.max(0, size.width - left - s.right - INSET_H);
    const maxH = Math.max(0, size.height - top - s.bottom - INSET_V);
    let w = Math.min(DIALOG_MIN_WIDTH, maxW);
    let h = 0;
    if (this.page !== null) {
      this.page.layout(
        new BoxConstraints({ minWidth: Math.min(DIALOG_MIN_WIDTH, maxW), maxWidth: maxW, maxHeight: maxH }),
        {
          parentUsesSize: true,
        },
      );
      w = this.page.size.width;
      h = this.page.size.height;
    }
    const x = left + (maxW - w) / 2;
    const y = top + (maxH - h) / 2;
    if (this.page !== null) {
      setChildOffset(this.page, new Offset(x, y));
    }
    return { x, y, w, h };
  }

  /** A scroll-controlled modal sheet: as wide as allowed up to 640, centred, at the bottom, as high as its content. */
  private sheet(size: Size): { x: number; y: number; w: number; h: number } {
    const w = Math.min(size.width, SHEET_MAX_WIDTH);
    let h = 0;
    if (this.page !== null) {
      this.page.layout(new BoxConstraints({ minWidth: w, maxWidth: w, maxHeight: size.height }), {
        parentUsesSize: true,
      });
      h = this.page.size.height;
    }
    const x = (size.width - w) / 2;
    const y = size.height - h;
    if (this.page !== null) {
      setChildOffset(this.page, new Offset(x, y));
    }
    return { x, y, w, h };
  }
}

/** Builds with `padding` as the view padding where the context supports it. */
function within<T>(ctx: WidgetContext, padding: ViewPadding, build: () => T): T {
  return ctx.withViewPadding === undefined ? build() : ctx.withViewPadding(padding, build);
}

/**
 * Builds a route node: the opener as the device shows it, the scrim and the surface as parts of the page (its painter
 * paints them, `#scrim` and `#surface`), and the page inside. A dialog's content has no view padding (the route's
 * `SafeArea` took it); a sheet's has none at the top.
 */
export function buildPageRoute(node: WidgetNode, ctx: WidgetContext): RenderBox {
  const parts = routeParts(node);
  const pad = ctx.viewPadding ?? NONE;
  const pageId = parts?.page?.id ?? node.id;
  const below = parts?.below === undefined ? null : ctx.build(parts.below);
  const page =
    parts?.page === undefined
      ? null
      : within(ctx, parts.kind === "dialog" ? NONE : { ...pad, top: 0 }, () => ctx.build(parts.page as WidgetNode));
  const part = (id: string) => {
    const box = new RenderConstrainedBox({ additionalConstraints: new BoxConstraints() });
    box.debugId = id;
    return box;
  };
  const kind = parts?.kind ?? "dialog";
  const route = new RenderRoute(kind, below, part(scrimPart(pageId)), part(surfacePart(pageId, kind)), page, pad);
  return new RenderConstrainedBox({
    additionalConstraints: BoxConstraints.tightFor({
      width: ctx.available(),
      height: ctx.deviceHeight ?? ctx.theme.viewportHeight,
    }),
    child: route,
  });
}

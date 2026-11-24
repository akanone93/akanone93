// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * A page's `Scaffold` on a painted screen (PC-12): laid out by the ported `RenderScaffold` at the device's size, so its
 * parts stand where the device puts them — the app bar at the top under the status bar, the navigation bar and the
 * persistent footer at the bottom over the home indicator, the floating action button at its location, the bottom
 * sheet above the bottom bar — and, when the canvas shows it open, the drawer over a scrim at its edge. The body keeps
 * the whole of a scroll view's content, and the page grows to hold it: the scaffold is laid out as high as the device
 * and its scroll content together, so the bottom bars stand below the content, never over it (STU-056, ADR-0111).
 */

import { BoxConstraints } from "../flutter/constraints.ts";
import { ContainerRenderBox } from "../flutter/container.ts";
import { Axis, CrossAxisAlignment, MainAxisAlignment, MainAxisSize, RenderFlex } from "../flutter/flex.ts";
import { EdgeInsets, Offset, Size, TextDirection } from "../flutter/geometry.ts";
import { type FabLocation, RenderScaffold, type ScaffoldSlots } from "../flutter/material/scaffold.ts";
import { RenderConstrainedBox } from "../flutter/proxy-box.ts";
import type { RenderBox } from "../flutter/render-box.ts";
import { RenderPadding, RenderShiftedBox, setChildOffset } from "../flutter/shifted-box.ts";
import type { ViewPadding } from "../scene/scene.ts";
import type { WidgetContext, WidgetNode } from "./screen.ts";

/** The editor-only object of a node, where the canvas's preview marks a scaffold's drawer as shown open. */
const STUDIO_KEY = "x-studio";

/** Scroll views: a body of one of these shows its whole content, past the frame. */
export const SCROLLING: ReadonlySet<string> = new Set([
  "SingleChildScrollView",
  "ListView",
  "CustomScrollView",
  "GridView",
  "NestedScrollView",
  "ReorderableListView",
]);

const NONE: ViewPadding = { top: 0, right: 0, bottom: 0, left: 0 };

/**
 * Wrappers of one child that hand the page's height on to it: a scroll view under a chain of them — `SafeArea` ›
 * `SingleChildScrollView` is the common page — is still the page's own scroll view (ADR-0111).
 */
const PASSES_HEIGHT: ReadonlySet<string> = new Set([
  "SafeArea",
  "Padding",
  "FormScope",
  "RefreshIndicator",
  "Scrollbar",
]);

/** How many wrappers deep a page's own scroll view is looked for. */
const WRAPPER_DEPTH = 6;

/**
 * Whether `node` is a page's own scroll view on a design screen: a scroll view, or one wrapped only in wrappers that
 * pass the height on. Its whole content is drawn and the page grows to hold it (STU-056, ADR-0111).
 */
export function ownScrollView(node: WidgetNode | undefined): boolean {
  let at = node;
  for (let depth = 0; at !== undefined && depth <= WRAPPER_DEPTH; depth += 1) {
    const type = at.type ?? "";
    if (SCROLLING.has(type)) {
      return true;
    }
    if (!PASSES_HEIGHT.has(type) || (at.children?.length ?? 0) > 0) {
      return false;
    }
    at = one(at, "child");
  }
  return false;
}

/** The drawer a scaffold is shown with on the canvas: `drawer`, `endDrawer`, or none. */
export function shownDrawer(node: WidgetNode): "drawer" | "endDrawer" | undefined {
  const meta = (node as unknown as Record<string, unknown>)[STUDIO_KEY];
  const open = typeof meta === "object" && meta !== null ? (meta as Record<string, unknown>)["open"] : undefined;
  return open === "drawer" || open === "endDrawer" ? open : undefined;
}

/** How far a live screen's scroll view is scrolled and how far it can be (PC-15). */
export interface ScrollExtent {
  readonly node: string;
  max: number;
}

/** The design-time scroll offset the preview marks a scroll view with (`x-studio.scroll`), 0 when none. */
export function scrollOf(node: WidgetNode): number {
  const meta = (node as unknown as Record<string, unknown>)[STUDIO_KEY];
  const v = typeof meta === "object" && meta !== null ? (meta as Record<string, unknown>)["scroll"] : undefined;
  return typeof v === "number" && Number.isFinite(v) && v > 0 ? v : 0;
}

/**
 * A box as big as its parent allows; its child as wide, and as high as allowed unless it scrolls. On a live screen
 * (PC-15) a scroll view's content is shifted up by its offset, at most to its end, inside a viewport its painter clips
 * to (`#viewport`); its extent is reported.
 */
export class RenderPageBody extends RenderShiftedBox {
  constructor(
    child: RenderBox,
    private readonly scrolls: boolean,
    private readonly live?: { readonly offset: number; readonly extent: ScrollExtent },
  ) {
    super(child);
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    return constraints.biggest;
  }

  /** How much higher than its box a design screen's scroll content is: what the page grows by to hold it; 0 live. */
  get overflow(): number {
    const child = this.child;
    return this.scrolls && this.live === undefined && child !== null
      ? Math.max(0, child.size.height - this.size.height)
      : 0;
  }

  protected override performLayout(): void {
    this.size = this.constraints.biggest;
    const child = this.child;
    if (child !== null) {
      const width = this.size.width;
      child.layout(
        this.scrolls
          ? BoxConstraints.tightFor({ width })
          : new BoxConstraints({ minWidth: width, maxWidth: width, maxHeight: this.size.height }),
        { parentUsesSize: true },
      );
      const max = Math.max(0, child.size.height - this.size.height);
      if (this.live !== undefined) {
        this.live.extent.max = max;
      }
      setChildOffset(child, new Offset(0, -Math.min(this.live?.offset ?? 0, max)));
    }
  }
}

/**
 * A scroll view's body on a live screen: shifted by its offset in a viewport named `<node>#viewport`, which its
 * painter clips its content to; off a live screen, the plain body that shows its whole content.
 */
export function pageBody(child: RenderBox, node: WidgetNode, ctx: WidgetContext): RenderPageBody {
  const live = ctx.live;
  if (live === undefined || !SCROLLING.has(node.type ?? "")) {
    // A live screen scrolls only a body that is itself a scroll view; a design screen draws a wrapped one whole too.
    const scrolls = live === undefined && ownScrollView(node);
    const body = new RenderPageBody(child, scrolls);
    // The screen's own scroll view: drawn whole, the page grown to hold it, not faded in its box.
    if (scrolls) {
      body.debugId = `${node.id}#page`;
    }
    return body;
  }
  const extent: ScrollExtent = { node: node.id, max: 0 };
  live.push(extent);
  const body = new RenderPageBody(child, true, { offset: scrollOf(node), extent });
  body.debugId = `${node.id}#viewport`;
  return body;
}

/** Lays its child out at any height and takes the child's height: an app bar's `Align` shrinks to its toolbar. */
class RenderOwnHeight extends RenderShiftedBox {
  protected override computeDryLayout(constraints: BoxConstraints): Size {
    return constraints.constrain(
      this.child?.getDryLayout(
        new BoxConstraints({ minWidth: constraints.minWidth, maxWidth: constraints.maxWidth }),
      ) ?? constraints.smallest,
    );
  }

  protected override performLayout(): void {
    const c = this.constraints;
    const child = this.child;
    if (child === null) {
      this.size = c.smallest;
      return;
    }
    child.layout(new BoxConstraints({ minWidth: c.minWidth, maxWidth: c.maxWidth }), { parentUsesSize: true });
    this.size = c.constrain(child.size);
    setChildOffset(child, Offset.zero);
  }
}

/**
 * A page as high as the device, or higher by as much as its body's scroll content needs (STU-056, ADR-0111): laid out
 * at the device's height first, then once more grown by the body's overflow. The second pass lays the scroll content
 * out at the constraints it already had, so it is not laid out again. Under a route (a dialog or sheet page over it)
 * the constraints keep the page at the device's height.
 */
class RenderGrowingPage extends RenderShiftedBox {
  constructor(
    child: RenderBox,
    private readonly width: number,
    private readonly height: number,
    private readonly body: RenderPageBody | null,
  ) {
    super(child);
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    return constraints.constrain(new Size(this.width, this.height));
  }

  protected override performLayout(): void {
    const c = this.constraints;
    const child = this.child;
    if (child === null) {
      this.size = c.constrain(new Size(this.width, this.height));
      return;
    }
    const at = (height: number) =>
      child.layout(BoxConstraints.tightFor({ width: this.width, height }).enforce(c), { parentUsesSize: true });
    at(this.height);
    const grow = this.body?.overflow ?? 0;
    if (grow > 0.5) {
      at(this.height + Math.ceil(grow));
    }
    this.size = child.size;
    setChildOffset(child, Offset.zero);
  }
}

/** The scaffold with its drawer open over a scrim: the drawer at its edge, as high as the screen. */
class RenderDrawerLayer extends ContainerRenderBox<RenderBox> {
  constructor(
    private readonly scaffold: RenderBox,
    private readonly scrim: RenderBox,
    private readonly drawer: RenderBox,
    private readonly atEnd: boolean,
  ) {
    super();
    for (const child of [scaffold, scrim, drawer]) {
      this.add(child);
    }
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    return constraints.biggest;
  }

  protected override performLayout(): void {
    const size = this.constraints.biggest;
    this.size = size;
    const tight = BoxConstraints.tight(size);
    this.scaffold.layout(tight, { parentUsesSize: true });
    setChildOffset(this.scaffold, Offset.zero);
    this.scrim.layout(tight, { parentUsesSize: true });
    setChildOffset(this.scrim, Offset.zero);
    this.drawer.layout(new BoxConstraints({ maxWidth: size.width, minHeight: size.height, maxHeight: size.height }), {
      parentUsesSize: true,
    });
    setChildOffset(this.drawer, new Offset(this.atEnd ? size.width - this.drawer.size.width : 0, 0));
  }
}

function one(node: WidgetNode, name: string): WidgetNode | undefined {
  const slot = node.slots?.[name];
  return slot === undefined || Array.isArray(slot) ? undefined : (slot as WidgetNode);
}

function many(node: WidgetNode, name: string): WidgetNode[] {
  const slot = node.slots?.[name] as WidgetNode | WidgetNode[] | undefined;
  if (slot === undefined) {
    return [];
  }
  return Array.isArray(slot) ? slot : [slot];
}

/** Builds with `padding` as the view padding (`MediaQuery.padding`) where the context supports it. */
function within<T>(ctx: WidgetContext, padding: ViewPadding, build: () => T): T {
  return ctx.withViewPadding === undefined ? build() : ctx.withViewPadding(padding, build);
}

/**
 * `Scaffold._buildPersistentFooter`'s content: the buttons at the end of a row, 8 apart, inside 8 of padding, over the
 * home indicator.
 */
function footerOf(buttons: readonly RenderBox[], pad: ViewPadding, direction: TextDirection): RenderBox {
  return new RenderPadding({
    padding: EdgeInsets.only({ left: pad.left + 8, top: 8, right: pad.right + 8, bottom: pad.bottom + 8 }),
    child: new RenderFlex({
      direction: Axis.horizontal,
      mainAxisAlignment: MainAxisAlignment.end,
      mainAxisSize: MainAxisSize.max,
      crossAxisAlignment: CrossAxisAlignment.center,
      spacing: 8,
      textDirection: direction,
      children: [...buttons],
    }),
  });
}

/** The bottom of the scaffold: the persistent footer above the navigation bar, as `_ScaffoldLayout` stacks them. */
function bottomOf(nav: RenderBox | null, footer: RenderBox | null, direction: TextDirection): RenderBox | null {
  if (footer === null) {
    return nav;
  }
  return new RenderFlex({
    direction: Axis.vertical,
    mainAxisSize: MainAxisSize.min,
    crossAxisAlignment: CrossAxisAlignment.stretch,
    textDirection: direction,
    children: nav === null ? [footer] : [footer, nav],
  });
}

/** The scrim of a drawer shown open: a part of the drawer, painted by it (`#scrim`). */
function scrimOf(drawer: WidgetNode): RenderBox {
  const scrim = new RenderConstrainedBox({ additionalConstraints: new BoxConstraints() });
  scrim.debugId = `${drawer.id}#scrim`;
  return scrim;
}

/**
 * Builds a page's `Scaffold` at the device's size (`ctx.deviceHeight`), grown to hold its body's scroll content, its
 * parts in place. The app bar takes the whole view padding; the body has none at the top below an app bar and none at
 * the bottom above a bottom bar, as Flutter's `Scaffold` removes them.
 */
export function buildPageScaffold(
  node: WidgetNode,
  ctx: WidgetContext,
  name: (id: string, box: RenderBox) => RenderBox,
): RenderBox {
  const pad = ctx.viewPadding ?? NONE;
  const direction = ctx.textDirection ?? TextDirection.ltr;
  const parts = scaffoldParts(node, ctx, pad, direction);
  const scaffold = new RenderScaffold({
    ...parts,
    fabLocation: fabLocationOf(node),
    textDirection: direction,
    minViewPaddingBottom: pad.bottom,
  });
  const height = ctx.deviceHeight ?? ctx.theme.viewportHeight;
  const body = parts.body instanceof RenderPageBody ? parts.body : null;
  const sized = (child: RenderBox) => new RenderGrowingPage(child, ctx.available(), height, body);
  const open = shownDrawer(node);
  const drawerNode = open === undefined ? undefined : one(node, open);
  if (open === undefined || drawerNode === undefined) {
    return name(node.id, sized(scaffold));
  }
  const rtl = direction === TextDirection.rtl;
  const atEnd = open === "endDrawer" ? !rtl : rtl;
  const drawer = within(ctx, pad, () => ctx.build(drawerNode));
  return name(node.id, sized(new RenderDrawerLayer(scaffold, scrimOf(drawerNode), drawer, atEnd)));
}

/** The scaffold's parts built with the view padding each one gets. */
function scaffoldParts(
  node: WidgetNode,
  ctx: WidgetContext,
  pad: ViewPadding,
  direction: TextDirection,
): ScaffoldSlots {
  const barNode = one(node, "appBar");
  const bodyNode = one(node, "body");
  const navNode = one(node, "bottomNavigationBar");
  const footerNodes = many(node, "persistentFooterButtons");
  const fabNode = one(node, "floatingActionButton");
  const sheetNode = one(node, "bottomSheet");
  // A snackbar the canvas shows for the selected widget (PC-27): a design-time slot, never in a document.
  const snackNode = one(node, "snackBar");
  const bottomBar = navNode !== undefined || footerNodes.length > 0;
  const bodyPad: ViewPadding = { ...pad, top: barNode === undefined ? pad.top : 0, bottom: bottomBar ? 0 : pad.bottom };
  const build = (n: WidgetNode | undefined, padding: ViewPadding): RenderBox | null =>
    n === undefined ? null : within(ctx, padding, () => ctx.build(n));
  const footer =
    footerNodes.length === 0
      ? null
      : footerOf(
          within(ctx, NONE, () => footerNodes.map((b) => ctx.build(b))),
          { ...pad, bottom: navNode === undefined ? pad.bottom : 0 },
          direction,
        );
  const bar = build(barNode, pad);
  const body = build(bodyNode, bodyPad);
  const sheet = build(sheetNode, bodyPad);
  const snack = build(snackNode, { ...bodyPad, top: 0 });
  return {
    appBar: bar === null ? null : new RenderOwnHeight(bar),
    body: body === null || bodyNode === undefined ? null : pageBody(body, bodyNode, ctx),
    bottomNavigationBar: bottomOf(build(navNode, pad), footer, direction),
    bottomSheet: sheet === null ? null : new RenderOwnHeight(sheet),
    floatingActionButton: build(fabNode, bodyPad),
    ...(snack === null ? {} : { snackBar: new RenderOwnHeight(snack) }),
  };
}

const FAB_LOCATIONS: ReadonlySet<string> = new Set([
  "startTop",
  "centerTop",
  "endTop",
  "startFloat",
  "centerFloat",
  "endFloat",
  "startDocked",
  "centerDocked",
  "endDocked",
]);

/** The scaffold's `floatingActionButtonLocation`; a mini location stands where its full-size one does. */
function fabLocationOf(node: WidgetNode): FabLocation {
  const raw = (node.props as Record<string, unknown> | undefined)?.["floatingActionButtonLocation"];
  if (typeof raw !== "string") {
    return "endFloat";
  }
  const plain = raw.startsWith("mini") ? `${raw.charAt(4).toLowerCase()}${raw.slice(5)}` : raw;
  return FAB_LOCATIONS.has(plain) ? (plain as FabLocation) : "endFloat";
}

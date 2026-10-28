// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Sliver constraint and geometry math, viewports and the sliver lists (STU-068, STU-005).
// The conformance suite compares whole scenes with Flutter's goldens; these tests pin the arithmetic.

import { describe, expect, test } from "bun:test";
import { BoxConstraints } from "./flutter/constraints.ts";
import { Axis } from "./flutter/flex.ts";
import { EdgeInsets, Offset, Size, TextDirection } from "./flutter/geometry.ts";
import { PipelineOwner } from "./flutter/object.ts";
import { RenderConstrainedBox } from "./flutter/proxy-box.ts";
import type { RenderBox } from "./flutter/render-box.ts";
import {
  AxisDirection,
  applyGrowthDirectionToAxisDirection,
  applyGrowthDirectionToScrollDirection,
  axisDirectionIsReversed,
  axisDirectionToAxis,
  FixedViewportOffset,
  flipAxis,
  flipAxisDirection,
  GrowthDirection,
  PageViewportOffset,
  type RenderSliver,
  RenderSliverToBoxAdapter,
  ScrollDirection,
  SliverConstraints,
  SliverGeometry,
  textDirectionToAxisDirection,
} from "./flutter/sliver.ts";
import {
  RenderSliverGrid,
  SliverGridDelegateWithFixedCrossAxisCount,
  SliverGridDelegateWithMaxCrossAxisExtent,
  SliverGridRegularTileLayout,
} from "./flutter/sliver-grid.ts";
import {
  PrebuiltChildManager,
  RenderSliverFixedExtentList,
  RenderSliverList,
  type RenderSliverMultiBoxAdaptor,
} from "./flutter/sliver-list.ts";
import { RenderSliverPadding } from "./flutter/sliver-padding.ts";
import {
  RenderSliverPinnedPersistentHeader,
  RenderSliverScrollingPersistentHeader,
} from "./flutter/sliver-persistent-header.ts";
import { RenderView } from "./flutter/view.ts";
import { RenderShrinkWrappingViewport, RenderViewport, ScrollCacheExtent } from "./flutter/viewport.ts";
import { RenderTestBox } from "./test-box.ts";

function constraints(overrides: Partial<ConstructorParameters<typeof SliverConstraints>[0]> = {}): SliverConstraints {
  return new SliverConstraints({
    axisDirection: AxisDirection.down,
    growthDirection: GrowthDirection.forward,
    userScrollDirection: ScrollDirection.idle,
    scrollOffset: 0,
    precedingScrollExtent: 0,
    overlap: 0,
    remainingPaintExtent: 400,
    crossAxisExtent: 300,
    crossAxisDirection: AxisDirection.right,
    viewportMainAxisExtent: 400,
    remainingCacheExtent: 650,
    cacheOrigin: 0,
    ...overrides,
  });
}

function tile(width: number, height: number): RenderBox {
  return new RenderTestBox({ size: new Size(width, height) });
}

/** Lays `viewport` out filling a 300 x 400 frame and returns the view. */
function layOut(viewport: RenderViewport | RenderShrinkWrappingViewport, size = new Size(300, 400)): RenderView {
  const owner = new PipelineOwner();
  const view = new RenderView({ size, child: viewport });
  owner.rootNode = view;
  view.prepareInitialFrame();
  owner.flushLayout();
  return view;
}

function liveIndices(sliver: RenderSliverMultiBoxAdaptor): number[] {
  const result: number[] = [];
  sliver.visitChildren((child) => {
    result.push(sliver.indexOf(child as RenderBox));
  });
  return result;
}

describe("[STU-068] axis directions", () => {
  test("axis, reversal and flipping", () => {
    expect(axisDirectionToAxis(AxisDirection.up)).toBe(Axis.vertical);
    expect(axisDirectionToAxis(AxisDirection.left)).toBe(Axis.horizontal);
    expect(axisDirectionIsReversed(AxisDirection.up)).toBe(true);
    expect(axisDirectionIsReversed(AxisDirection.right)).toBe(false);
    expect(flipAxisDirection(AxisDirection.left)).toBe(AxisDirection.right);
    expect(flipAxisDirection(AxisDirection.down)).toBe(AxisDirection.up);
    expect(flipAxis(Axis.vertical)).toBe(Axis.horizontal);
    expect(textDirectionToAxisDirection(TextDirection.rtl)).toBe(AxisDirection.left);
    expect(textDirectionToAxisDirection(TextDirection.ltr)).toBe(AxisDirection.right);
  });

  test("growth direction flips the axis and scroll direction", () => {
    expect(applyGrowthDirectionToAxisDirection(AxisDirection.down, GrowthDirection.reverse)).toBe(AxisDirection.up);
    expect(applyGrowthDirectionToAxisDirection(AxisDirection.down, GrowthDirection.forward)).toBe(AxisDirection.down);
    expect(applyGrowthDirectionToScrollDirection(ScrollDirection.forward, GrowthDirection.reverse)).toBe(
      ScrollDirection.reverse,
    );
    expect(applyGrowthDirectionToScrollDirection(ScrollDirection.reverse, GrowthDirection.reverse)).toBe(
      ScrollDirection.forward,
    );
    expect(applyGrowthDirectionToScrollDirection(ScrollDirection.idle, GrowthDirection.reverse)).toBe(
      ScrollDirection.idle,
    );
    expect(applyGrowthDirectionToScrollDirection(ScrollDirection.forward, GrowthDirection.forward)).toBe(
      ScrollDirection.forward,
    );
  });
});

describe("[STU-068] SliverConstraints", () => {
  test("asBoxConstraints is tight across and ranges along the scroll axis", () => {
    const vertical = constraints().asBoxConstraints({ minExtent: 20, maxExtent: 90 });
    expect([vertical.minWidth, vertical.maxWidth, vertical.minHeight, vertical.maxHeight]).toEqual([300, 300, 20, 90]);
    const horizontal = constraints({
      axisDirection: AxisDirection.left,
      crossAxisDirection: AxisDirection.down,
    }).asBoxConstraints({ crossAxisExtent: 120 });
    expect([horizontal.minWidth, horizontal.maxWidth, horizontal.minHeight, horizontal.maxHeight]).toEqual([
      0,
      Number.POSITIVE_INFINITY,
      120,
      120,
    ]);
  });

  test("normalizedGrowthDirection flips for reversed axes; isNormalized and equals", () => {
    expect(constraints().normalizedGrowthDirection).toBe(GrowthDirection.forward);
    expect(constraints({ axisDirection: AxisDirection.up }).normalizedGrowthDirection).toBe(GrowthDirection.reverse);
    expect(
      constraints({ axisDirection: AxisDirection.up, growthDirection: GrowthDirection.reverse })
        .normalizedGrowthDirection,
    ).toBe(GrowthDirection.forward);
    expect(constraints().isNormalized).toBe(true);
    expect(constraints({ scrollOffset: -1 }).isNormalized).toBe(false);
    expect(constraints({ crossAxisDirection: AxisDirection.up }).isNormalized).toBe(false);
    expect(constraints().isTight).toBe(false);
    expect(constraints().equals(constraints())).toBe(true);
    expect(constraints().equals(constraints({ overlap: 1 }))).toBe(false);
    expect(constraints().equals(BoxConstraints.tight(new Size(1, 1)))).toBe(false);
    expect(constraints().copyWith({ scrollOffset: 5 }).scrollOffset).toBe(5);
  });
});

describe("[STU-068] SliverGeometry", () => {
  test("layout, hit test and cache extents default from the paint extent", () => {
    const geometry = new SliverGeometry({ paintExtent: 80, scrollExtent: 200 });
    expect([geometry.layoutExtent, geometry.hitTestExtent, geometry.cacheExtent, geometry.visible]).toEqual([
      80,
      80,
      80,
      true,
    ]);
    expect(new SliverGeometry({ paintExtent: 80, layoutExtent: 30 }).cacheExtent).toBe(30);
    expect(SliverGeometry.zero.visible).toBe(false);
    expect(new SliverGeometry({ paintExtent: 5, visible: false }).visible).toBe(false);
    expect(new SliverGeometry({ scrollOffsetCorrection: -4 }).scrollOffsetCorrection).toBe(-4);
  });
});

describe("[STU-068] paint and cache offsets", () => {
  const sliver = new RenderSliverToBoxAdapter();
  const scrolled = constraints({
    scrollOffset: 100,
    remainingPaintExtent: 400,
    remainingCacheExtent: 650,
    cacheOrigin: -60,
  });

  test("calculatePaintOffset counts the part of the range inside the viewport", () => {
    expect(sliver.calculatePaintOffset(scrolled, { from: 0, to: 80 })).toBe(0);
    expect(sliver.calculatePaintOffset(scrolled, { from: 50, to: 160 })).toBe(60);
    expect(sliver.calculatePaintOffset(scrolled, { from: 0, to: 1000 })).toBe(400);
  });

  test("calculateCacheOffset counts the part of the range inside the cache area", () => {
    expect(sliver.calculateCacheOffset(scrolled, { from: 0, to: 40 })).toBe(0);
    expect(sliver.calculateCacheOffset(scrolled, { from: 0, to: 160 })).toBe(120);
    expect(sliver.calculateCacheOffset(scrolled, { from: 0, to: 5000 })).toBe(650);
  });
});

describe("[STU-068] viewport offsets", () => {
  test("a fixed offset stays and is corrected by the amount asked", () => {
    const offset = new FixedViewportOffset(40, ScrollDirection.forward);
    expect(offset.applyViewportDimension(400)).toBe(true);
    expect(offset.applyContentDimensions(0, 100)).toBe(true);
    offset.correctBy(-15);
    expect([offset.pixels, offset.userScrollDirection]).toEqual([25, ScrollDirection.forward]);
  });

  test("a page offset starts on its initial page and asks for another pass when the viewport is first sized", () => {
    const offset = new PageViewportOffset(2, 0.8);
    expect(offset.applyViewportDimension(400)).toBe(false);
    expect(offset.pixels).toBeCloseTo(2 * 400 * 0.8);
    expect(offset.applyViewportDimension(400)).toBe(true);
    expect(offset.getPageFromPixels(offset.pixels, 400)).toBe(2);
    expect(offset.applyViewportDimension(500)).toBe(false);
    expect(offset.pixels).toBeCloseTo(2 * 500 * 0.8);
    expect(offset.applyContentDimensions(0, 10)).toBe(true);
    offset.correctBy(5);
    expect(offset.pixels).toBeCloseTo(2 * 500 * 0.8 + 5);
  });

  test("a page fraction above one adds the initial page offset", () => {
    const offset = new PageViewportOffset(1, 1.5);
    offset.applyViewportDimension(200);
    expect(offset.pixels).toBe(1 * 200 * 1.5 + 200 * 0.25);
  });
});

describe("[STU-068] viewports", () => {
  test("a viewport fills its constraints and reports the scrollable range", () => {
    const list = new RenderSliverList({
      childManager: new PrebuiltChildManager([
        tile(10, 100),
        tile(10, 100),
        tile(10, 100),
        tile(10, 100),
        tile(10, 100),
      ]),
    });
    const viewport = new RenderViewport({
      crossAxisDirection: AxisDirection.right,
      offset: new FixedViewportOffset(60),
      children: [list],
    });
    layOut(viewport);
    expect(viewport.size).toEqual(new Size(300, 400));
    expect(viewport.scrollExtents).toEqual({ min: 0, max: 500 });
    expect(list.geometry?.scrollExtent).toBe(500);
    expect(list.geometry?.paintExtent).toBe(400);
    expect(viewport.paintOffsetOf(list)).toEqual(new Offset(0, 0));
    expect(viewport.hasVisualOverflow).toBe(true);
  });

  test("a reversed axis places the sliver from the bottom", () => {
    const box = new RenderSliverToBoxAdapter({ child: tile(10, 100) });
    const viewport = new RenderViewport({
      axisDirection: AxisDirection.up,
      crossAxisDirection: AxisDirection.right,
      offset: new FixedViewportOffset(0),
      children: [box],
    });
    layOut(viewport);
    expect(viewport.paintOffsetOf(box)).toEqual(new Offset(0, 300));
    expect(box.childPaintOffset(box.child as RenderBox)).toEqual(new Offset(0, 0));
  });

  test("an empty viewport has no extent to scroll", () => {
    const viewport = new RenderViewport({
      crossAxisDirection: AxisDirection.right,
      offset: new FixedViewportOffset(0),
    });
    layOut(viewport);
    expect(viewport.scrollExtents).toEqual({ min: 0, max: 0 });
    expect(viewport.indexOfFirstChild).toBe(0);
  });

  test("a shrink-wrapping viewport is as long as its content, up to the constraints", () => {
    const sliver = new RenderSliverToBoxAdapter({ child: tile(10, 120) });
    const viewport = new RenderShrinkWrappingViewport({
      crossAxisDirection: AxisDirection.right,
      offset: new FixedViewportOffset(0),
      children: [sliver],
    });
    const owner = new PipelineOwner();
    const loose = new RenderView({ size: new Size(300, 400), child: viewport });
    owner.rootNode = loose;
    loose.prepareInitialFrame();
    // The view gives tight constraints, so shrink-wrapping needs a loose parent: lay the viewport out directly.
    viewport.layout(new BoxConstraints({ maxWidth: 300, maxHeight: 400 }), { parentUsesSize: true });
    expect(viewport.size).toEqual(new Size(300, 120));
    expect(viewport.indexOfFirstChild).toBe(0);
    expect(viewport.paintOffsetOf(sliver)).toEqual(new Offset(0, 0));
  });

  test("the cache extent comes in pixels or in viewports", () => {
    expect(ScrollCacheExtent.pixels(250).calculateCacheOffset(400)).toBe(250);
    expect(ScrollCacheExtent.viewport(1.5).calculateCacheOffset(400)).toBe(600);
  });
});

describe("[STU-068] sliver lists", () => {
  function listViewport(sliver: RenderSliver, offset: number, cache = 0): RenderViewport {
    return new RenderViewport({
      crossAxisDirection: AxisDirection.right,
      offset: new FixedViewportOffset(offset),
      scrollCacheExtent: ScrollCacheExtent.pixels(cache),
      children: [sliver],
    });
  }

  test("a list keeps the children in or near the viewport, and extrapolates the rest of its extent", () => {
    const tiles = Array.from({ length: 20 }, () => tile(10, 50));
    const list = new RenderSliverList({ childManager: new PrebuiltChildManager(tiles) });
    layOut(listViewport(list, 275, 100));
    expect(liveIndices(list)).toEqual([3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15]);
    expect(list.geometry?.scrollExtent).toBe(1000);
  });

  test("a fixed extent list computes child indices from the scroll offset", () => {
    const tiles = Array.from({ length: 30 }, () => tile(10, 10));
    const list = new RenderSliverFixedExtentList({ childManager: new PrebuiltChildManager(tiles), itemExtent: 40 });
    layOut(listViewport(list, 130));
    expect(liveIndices(list)).toEqual([3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13]);
    expect(list.getMinChildIndexForScrollOffset(130)).toBe(3);
    expect(list.getMinChildIndexForScrollOffset(120)).toBe(3);
    expect(list.getMaxChildIndexForScrollOffset(530)).toBe(13);
    expect(list.getMaxChildIndexForScrollOffset(520)).toBe(12);
    expect(list.indexToLayoutOffset(7)).toBe(280);
    expect(list.computeMaxScrollOffset()).toBe(1200);
    expect(list.geometry?.scrollExtent).toBe(1200);
  });

  test("a list shorter than the viewport lays out every child and no more", () => {
    const list = new RenderSliverList({ childManager: new PrebuiltChildManager([tile(10, 50), tile(10, 70)]) });
    layOut(listViewport(list, 0, 250));
    expect(liveIndices(list)).toEqual([0, 1]);
    expect(list.geometry?.scrollExtent).toBe(120);
    expect(list.geometry?.paintExtent).toBe(120);
  });

  test("padding adds its insets to the scroll extent and offsets the child", () => {
    const list = new RenderSliverFixedExtentList({
      childManager: new PrebuiltChildManager([tile(10, 10), tile(10, 10)]),
      itemExtent: 30,
    });
    const padding = new RenderSliverPadding({ padding: EdgeInsets.fromLTRB(5, 10, 15, 20), child: list });
    layOut(listViewport(padding, 0));
    expect(padding.geometry?.scrollExtent).toBe(90);
    expect(padding.childPaintOffset(list)).toEqual(new Offset(5, 10));
    expect(list.constraints.crossAxisExtent).toBe(280);
  });
});

describe("[STU-068] sliver grids", () => {
  test("a regular tile layout places tiles by stride and mirrors the cross axis when reversed", () => {
    const init = {
      crossAxisCount: 3,
      mainAxisStride: 50,
      crossAxisStride: 40,
      childMainAxisExtent: 40,
      childCrossAxisExtent: 30,
    };
    const layout = new SliverGridRegularTileLayout({ ...init, reverseCrossAxis: false });
    const mirrored = new SliverGridRegularTileLayout({ ...init, reverseCrossAxis: true });
    expect(layout.getMinChildIndexForScrollOffset(120)).toBe(6);
    expect(layout.getMaxChildIndexForScrollOffset(120)).toBe(8);
    expect(layout.getMaxChildIndexForScrollOffset(0)).toBe(0);
    const tileFive = layout.getGeometryForChildIndex(5);
    expect([tileFive.scrollOffset, tileFive.crossAxisOffset, tileFive.trailingScrollOffset]).toEqual([50, 80, 90]);
    expect(mirrored.getGeometryForChildIndex(5).crossAxisOffset).toBe(0);
    expect(mirrored.getGeometryForChildIndex(3).crossAxisOffset).toBe(80);
    expect(layout.computeMaxScrollOffset(0)).toBe(0);
    expect(layout.computeMaxScrollOffset(7)).toBe(50 * 3 - 10);
    expect(
      new SliverGridRegularTileLayout({
        ...init,
        mainAxisStride: 0,
        reverseCrossAxis: false,
      }).getMaxChildIndexForScrollOffset(10),
    ).toBe(0);
  });

  test("the stock delegates divide the cross axis", () => {
    const fixed = new SliverGridDelegateWithFixedCrossAxisCount(3, { crossAxisSpacing: 15, childAspectRatio: 2 });
    const layout = fixed.getLayout(constraints({ crossAxisExtent: 300 })) as SliverGridRegularTileLayout;
    expect([layout.childCrossAxisExtent, layout.childMainAxisExtent, layout.crossAxisStride]).toEqual([90, 45, 105]);
    const max = new SliverGridDelegateWithMaxCrossAxisExtent(100, { crossAxisSpacing: 10, mainAxisExtent: 70 });
    const maxLayout = max.getLayout(
      constraints({ crossAxisExtent: 300, crossAxisDirection: AxisDirection.left }),
    ) as SliverGridRegularTileLayout;
    expect(maxLayout.crossAxisCount).toBe(3);
    expect(maxLayout.childMainAxisExtent).toBe(70);
    expect(maxLayout.reverseCrossAxis).toBe(true);
    const narrow = max.getLayout(constraints({ crossAxisExtent: 20 })) as SliverGridRegularTileLayout;
    expect(narrow.crossAxisCount).toBe(1);
  });

  test("a grid lays tiles out across rows", () => {
    const tiles = Array.from({ length: 10 }, () => tile(10, 10));
    const grid = new RenderSliverGrid({
      childManager: new PrebuiltChildManager(tiles),
      gridDelegate: new SliverGridDelegateWithFixedCrossAxisCount(2, { mainAxisExtent: 60 }),
    });
    layOut(
      new RenderViewport({
        crossAxisDirection: AxisDirection.right,
        offset: new FixedViewportOffset(130),
        scrollCacheExtent: ScrollCacheExtent.pixels(0),
        children: [grid],
      }),
    );
    expect(liveIndices(grid)).toEqual([4, 5, 6, 7, 8, 9]);
    expect(grid.geometry?.scrollExtent).toBe(300);
    const fifth = tiles[5] as RenderBox;
    expect(fifth.size).toEqual(new Size(150, 60));
    expect(grid.childPaintOffset(fifth)).toEqual(new Offset(150, 120 - 130));
  });
});

describe("[STU-068] persistent headers", () => {
  function header(kind: "scrolling" | "pinned", offset: number) {
    const expanding = new RenderConstrainedBox({
      additionalConstraints: BoxConstraints.tightFor({
        width: Number.POSITIVE_INFINITY,
        height: Number.POSITIVE_INFINITY,
      }),
    });
    const options = { minExtent: 60, maxExtent: 150, child: expanding };
    const sliver =
      kind === "pinned"
        ? new RenderSliverPinnedPersistentHeader(options)
        : new RenderSliverScrollingPersistentHeader(options);
    const after = new RenderSliverToBoxAdapter({ child: tile(10, 500) });
    layOut(
      new RenderViewport({
        crossAxisDirection: AxisDirection.right,
        offset: new FixedViewportOffset(offset),
        children: [sliver, after],
      }),
    );
    return { sliver, after };
  }

  test("a scrolling header shrinks and leaves with the content", () => {
    const { sliver } = header("scrolling", 100);
    expect(sliver.geometry?.paintExtent).toBe(50);
    expect(sliver.geometry?.scrollExtent).toBe(150);
    expect(header("scrolling", 200).sliver.geometry?.paintExtent).toBe(0);
  });

  test("a pinned header keeps its minimum extent at the leading edge", () => {
    const { sliver, after } = header("pinned", 100);
    expect(sliver.geometry?.paintExtent).toBe(60);
    expect(sliver.geometry?.layoutExtent).toBe(50);
    expect(sliver.geometry?.maxScrollObstructionExtent).toBe(60);
    expect(after.constraints.overlap).toBe(10);
    const collapsed = header("pinned", 300).sliver;
    expect(collapsed.geometry?.paintExtent).toBe(60);
    expect(collapsed.geometry?.layoutExtent).toBe(0);
  });
});

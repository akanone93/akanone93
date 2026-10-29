// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { BoxConstraints } from "./flutter/constraints.ts";
import { ContainerRenderBox } from "./flutter/container.ts";
import { EdgeInsets, Offset, Size } from "./flutter/geometry.ts";
import { PipelineOwner } from "./flutter/object.ts";
import { RenderConstrainedBox, RenderProxyBox } from "./flutter/proxy-box.ts";
import { BaselineOffset, BoxParentData, RenderBox, TextBaseline } from "./flutter/render-box.ts";
import { RenderPadding, RenderPositionedBox } from "./flutter/shifted-box.ts";
import { RenderView } from "./flutter/view.ts";
import { globalRects } from "./node-rects.ts";
import { RenderTestBox } from "./test-box.ts";

/** Builds an attached tree under an 800x600 view and lays it out once. */
function mount(child: RenderBox): { owner: PipelineOwner; view: RenderView; updates: () => number } {
  let updates = 0;
  const owner = new PipelineOwner({
    onNeedVisualUpdate: () => {
      updates += 1;
    },
  });
  const view = new RenderView({ size: new Size(800, 600), child });
  owner.rootNode = view;
  view.prepareInitialFrame();
  owner.flushLayout();
  return { owner, view, updates: () => updates };
}

describe("[STU-005] relayout boundaries", () => {
  test("the boundary rule and propagation of markNeedsLayout to the nearest boundary", () => {
    const leaf = new RenderTestBox({ size: new Size(50, 50) });
    const proxy = new RenderProxyBox(leaf);
    const align = new RenderPositionedBox({ child: proxy });
    const { owner, view, updates } = mount(align);

    expect(view.isRelayoutBoundary).toBe(true);
    expect(align.isRelayoutBoundary).toBe(true); // tight constraints from the view
    expect(proxy.isRelayoutBoundary).toBe(false); // loose, parent uses size
    expect(leaf.isRelayoutBoundary).toBe(false);
    expect(leaf.layoutCount).toBe(1);
    expect(RenderBox.offsetOf(proxy).equals(new Offset(375, 275))).toBe(true);

    leaf.preferredSize = new Size(60, 60);
    expect(leaf.needsLayout && proxy.needsLayout && align.needsLayout).toBe(true);
    expect(view.needsLayout).toBe(false);
    expect(owner.nodesNeedingLayout).toEqual([align]);
    expect(updates()).toBe(1);

    owner.flushLayout();
    expect(leaf.layoutCount).toBe(2);
    expect(owner.nodesNeedingLayout).toEqual([]);
    expect(RenderBox.offsetOf(proxy).equals(new Offset(370, 270))).toBe(true);

    // A second markNeedsLayout on a dirty node is a no-op.
    leaf.markNeedsLayout();
    leaf.markNeedsLayout();
    expect(owner.nodesNeedingLayout).toEqual([align]);
    owner.flushLayout();
  });

  test("a child under tight constraints is its own boundary", () => {
    const leaf = new RenderTestBox({ size: new Size(10, 10) });
    const sized = new RenderConstrainedBox({
      additionalConstraints: BoxConstraints.tightFor({ width: 100, height: 100 }),
      child: leaf,
    });
    const align = new RenderPositionedBox({ child: sized });
    const { owner } = mount(align);
    expect(leaf.isRelayoutBoundary).toBe(true);
    expect(leaf.size.equals(new Size(100, 100))).toBe(true);
    leaf.preferredSize = new Size(20, 20);
    expect(sized.needsLayout).toBe(false);
    expect(owner.nodesNeedingLayout).toEqual([leaf]);
    owner.flushLayout();
    expect(leaf.layoutCount).toBe(2);
  });

  test("sizedByParent and parentUsesSize: false make boundaries", () => {
    const sizedLeaf = new RenderTestBox({ size: new Size(10, 10), sizedByParent: true });
    sizedLeaf.layout(BoxConstraints.loose(new Size(30, 40)), { parentUsesSize: true });
    expect(sizedLeaf.isRelayoutBoundary).toBe(true);
    expect(sizedLeaf.size.equals(new Size(10, 10))).toBe(true);
    const proxy = new RenderProxyBox();
    const leaf = new RenderTestBox({ size: new Size(10, 10) });
    proxy.child = leaf;
    leaf.layout(BoxConstraints.loose(new Size(30, 40)));
    expect(leaf.isRelayoutBoundary).toBe(true);
    sizedLeaf.markNeedsLayoutForSizedByParentChange();
    expect(sizedLeaf.needsLayout).toBe(true);
  });

  test("a clean node given equal constraints skips layout", () => {
    const leaf = new RenderTestBox({ size: new Size(10, 10) });
    leaf.layout(BoxConstraints.loose(new Size(30, 40)));
    leaf.layout(BoxConstraints.loose(new Size(30, 40)));
    expect(leaf.layoutCount).toBe(1);
    leaf.layout(BoxConstraints.loose(new Size(5, 5)));
    expect(leaf.layoutCount).toBe(2);
    expect(leaf.size.equals(new Size(5, 5))).toBe(true);
  });

  test("the owner lays out shallower boundaries first and skips nodes already laid out", () => {
    const deep = new RenderTestBox({ size: new Size(10, 10) });
    const sized = new RenderConstrainedBox({
      additionalConstraints: BoxConstraints.tightFor({ width: 20, height: 20 }),
      child: deep,
    });
    const align = new RenderPositionedBox({ child: sized });
    const { owner } = mount(align);
    deep.markNeedsLayout();
    sized.additionalConstraints = BoxConstraints.tightFor({ width: 30, height: 30 });
    expect(owner.nodesNeedingLayout).toEqual([deep, align]);
    owner.flushLayout();
    expect(deep.layoutCount).toBe(2);
    expect(deep.size.equals(new Size(30, 30))).toBe(true);
  });

  test("detaching and reattaching reschedules a dirty boundary", () => {
    const leaf = new RenderTestBox({ size: new Size(10, 10) });
    const { owner, view } = mount(new RenderPositionedBox({ child: leaf }));
    owner.rootNode = view;
    owner.rootNode = null;
    expect(view.attached).toBe(false);
    expect(leaf.owner).toBeNull();
    // Without an owner, dirtiness climbs past boundaries to the root.
    leaf.preferredSize = new Size(40, 40);
    expect(view.needsLayout).toBe(true);
    owner.rootNode = view;
    // Every dirty boundary is rescheduled as it reattaches, as in Flutter.
    expect(owner.nodesNeedingLayout.length).toBe(2);
    expect(owner.nodesNeedingLayout[0]).toBe(view);
    owner.flushLayout();
    expect(leaf.size.equals(new Size(40, 40))).toBe(true);
    expect(leaf.depth).toBe(2);
  });

  test("dropping a child clears its boundary state and dirties the parent", () => {
    const leaf = new RenderTestBox({ size: new Size(10, 10) });
    const proxy = new RenderProxyBox(leaf);
    const { owner, view } = mount(new RenderPositionedBox({ child: proxy }));
    proxy.child = null;
    expect(leaf.parent).toBeNull();
    expect(leaf.parentData).toBeNull();
    expect(leaf.isRelayoutBoundary).toBeNull();
    expect(leaf.attached).toBe(false);
    owner.flushLayout();
    expect(proxy.size.equals(Size.zero)).toBe(true);
    view.configurationSize = new Size(800, 600);
    view.configurationSize = new Size(400, 300);
    owner.flushLayout();
    expect(view.size.equals(new Size(400, 300))).toBe(true);
    expect(view.getDryLayout(new BoxConstraints()).equals(new Size(400, 300))).toBe(true);
  });

  test("misuse throws a descriptive error", () => {
    const leaf = new RenderTestBox({ size: new Size(10, 10) });
    expect(leaf.hasSize).toBe(false);
    expect(leaf.hasConstraints).toBe(false);
    expect(() => leaf.size).toThrow("has no size");
    expect(() => leaf.constraints).toThrow("has not been laid out");
    expect(() => leaf.scheduleInitialLayout()).toThrow("PipelineOwner");
    const fake = { isTight: true, isNormalized: true, equals: () => false };
    expect(() => leaf.layout(fake)).toThrow("without BoxConstraints");
    expect(leaf.hasConstraints).toBe(true);
  });
});

describe("[STU-005] intrinsic and dry-layout caches", () => {
  test("intrinsics are cached until markNeedsLayout, which then dirties the parent", () => {
    const leaf = new RenderTestBox({ size: new Size(50, 40) });
    const sized = new RenderConstrainedBox({
      additionalConstraints: BoxConstraints.tightFor({ width: 100, height: 100 }),
      child: new RenderPadding({ padding: EdgeInsets.all(10), child: leaf }),
    });
    const { owner } = mount(new RenderPositionedBox({ child: sized }));
    const padding = sized.child;
    if (!(padding instanceof RenderPadding)) {
      throw new Error("expected padding");
    }
    expect(padding.isRelayoutBoundary).toBe(true);
    expect(padding.getMinIntrinsicWidth(100)).toBe(70);
    expect(padding.getMinIntrinsicWidth(100)).toBe(70);
    expect(leaf.intrinsicComputeCount).toBe(1);

    // The padding is a boundary, but its cached intrinsics were read by its
    // parent's query, so the dirtiness crosses the boundary.
    leaf.markNeedsLayout();
    expect(padding.needsLayout).toBe(true);
    expect(sized.needsLayout).toBe(true);
    owner.flushLayout();
    expect(padding.getMinIntrinsicWidth(100)).toBe(70);
    expect(leaf.intrinsicComputeCount).toBe(2);
  });

  test("dry layout is cached per constraints until markNeedsLayout", () => {
    const leaf = new RenderTestBox({ size: new Size(50, 40) });
    const loose = BoxConstraints.loose(new Size(100, 100));
    expect(leaf.getDryLayout(loose).equals(new Size(50, 40))).toBe(true);
    expect(leaf.getDryLayout(new BoxConstraints({ maxWidth: 100, maxHeight: 100 }))).toBe(leaf.getDryLayout(loose));
    expect(leaf.dryLayoutComputeCount).toBe(1);
    leaf.getDryLayout(BoxConstraints.tight(new Size(1, 1)));
    expect(leaf.dryLayoutComputeCount).toBe(2);
    leaf.layout(loose);
    leaf.markNeedsLayout();
    leaf.getDryLayout(loose);
    expect(leaf.dryLayoutComputeCount).toBe(3);
  });

  test("a box with no dry layout or intrinsics reports zero", () => {
    class Bare extends RenderBox {}
    const bare = new Bare();
    expect(bare.getDryLayout(new BoxConstraints()).equals(Size.zero)).toBe(true);
    expect(bare.getMinIntrinsicWidth(1)).toBe(0);
    expect(bare.getMaxIntrinsicWidth(1)).toBe(0);
    expect(bare.getMinIntrinsicHeight(1)).toBe(0);
    expect(bare.getMaxIntrinsicHeight(1)).toBe(0);
    expect(bare.getDryBaseline(new BoxConstraints(), TextBaseline.alphabetic)).toBeNull();
  });
});

describe("[STU-005] BaselineOffset", () => {
  test("arithmetic keeps no-baseline", () => {
    expect(BaselineOffset.noBaseline.plus(3).offset).toBeNull();
    expect(new BaselineOffset(2).plus(3).offset).toBe(5);
    expect(BaselineOffset.noBaseline.minOf(new BaselineOffset(4)).offset).toBe(4);
    expect(new BaselineOffset(4).minOf(BaselineOffset.noBaseline).offset).toBe(4);
    expect(new BaselineOffset(4).minOf(new BaselineOffset(1)).offset).toBe(1);
    expect(new BaselineOffset(1).minOf(new BaselineOffset(4)).offset).toBe(1);
  });
});

/** A minimal vertical list, exercising the container seam. */
class RenderVerticalList extends ContainerRenderBox {
  protected override performLayout(): void {
    let y = 0;
    let width = 0;
    for (const child of this.getChildrenAsList()) {
      child.layout(this.constraints.loosen(), { parentUsesSize: true });
      this.childParentData(child).offset = new Offset(0, y);
      y += child.size.height;
      width = Math.max(width, child.size.width);
    }
    this.size = this.constraints.constrain(new Size(width, y));
  }

  protected override computeDistanceToActualBaseline(baseline: TextBaseline): number | null {
    return this.defaultComputeDistanceToFirstActualBaseline(baseline);
  }

  highest(baseline: TextBaseline): number | null {
    return this.defaultComputeDistanceToHighestActualBaseline(baseline);
  }
}

describe("[STU-005] container children", () => {
  test("insert, move, remove and baselines over a child list", () => {
    const a = new RenderTestBox({ size: new Size(10, 10) });
    const b = new RenderTestBox({ size: new Size(20, 20), baseline: 5 });
    const c = new RenderTestBox({ size: new Size(30, 30), baseline: 1 });
    const list = new RenderVerticalList();
    list.addAll([a, c]);
    list.insert(b, a);
    expect(list.getChildrenAsList()).toEqual([a, b, c]);
    expect(list.childCount).toBe(3);
    expect(list.firstChild).toBe(a);
    expect(list.lastChild).toBe(c);
    expect(list.childAfter(a)).toBe(b);
    expect(list.childBefore(a)).toBeNull();
    list.debugId = "list";
    a.debugId = "a";
    b.debugId = "b";
    c.debugId = "c";
    const { owner } = mount(new RenderPositionedBox({ child: list }));
    expect(globalRects(list).get("c")).toEqual({ x: 0, y: 30, w: 30, h: 30 });
    expect(list.getDistanceToBaseline(TextBaseline.alphabetic)).toBe(15);
    expect(list.highest(TextBaseline.alphabetic)).toBe(15);

    list.move(c);
    list.move(c);
    expect(list.getChildrenAsList()).toEqual([c, a, b]);
    list.move(a, b);
    expect(list.getChildrenAsList()).toEqual([c, b, a]);
    owner.flushLayout();
    expect(list.getDistanceToBaseline(TextBaseline.alphabetic)).toBe(1);
    expect(list.highest(TextBaseline.alphabetic)).toBe(1);

    list.remove(b);
    expect(list.getChildrenAsList()).toEqual([c, a]);
    list.remove(a);
    list.insert(a);
    expect(list.getChildrenAsList()).toEqual([a, c]);
    expect(() => list.childAfter(b)).toThrow("is not a child");
    owner.rootNode = null;
    expect(c.attached).toBe(false);
    list.removeAll();
    expect(list.childCount).toBe(0);
    expect(list.firstChild).toBeNull();
    expect(a.parent).toBeNull();
    const empty = new RenderVerticalList();
    empty.layout(new BoxConstraints());
    expect(empty.getDistanceToBaseline(TextBaseline.alphabetic, { onlyReal: true })).toBeNull();
    expect(empty.highest(TextBaseline.alphabetic)).toBeNull();
  });

  test("an existing container parent data is kept", () => {
    const list = new RenderVerticalList();
    const child = new RenderTestBox({ size: Size.zero });
    list.add(child);
    const data = child.parentData;
    list.remove(child);
    child.parentData = data;
    list.add(child);
    expect(child.parentData).toBe(data);
    list.remove(child);
    const box = new RenderProxyBox();
    const data2 = new BoxParentData();
    child.parentData = data2;
    box.child = child;
    expect(child.parentData).toBe(data2);
  });
});

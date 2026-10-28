// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { BoxConstraints } from "./flutter/constraints.ts";
import {
  Alignment,
  AlignmentDirectional,
  EdgeInsets,
  EdgeInsetsDirectional,
  Offset,
  Size,
  TextDirection,
} from "./flutter/geometry.ts";
import { PipelineOwner } from "./flutter/object.ts";
import { DecorationPosition, RenderConstrainedBox, RenderDecoratedBox, RenderProxyBox } from "./flutter/proxy-box.ts";
import { type RenderBox, TextBaseline } from "./flutter/render-box.ts";
import { RenderPadding, RenderPositionedBox } from "./flutter/shifted-box.ts";
import { RenderView } from "./flutter/view.ts";
import { globalRects, goldenRects, type LayoutGolden } from "./node-rects.ts";
import { RenderTestBox } from "./test-box.ts";

const inf = Number.POSITIVE_INFINITY;
const alphabetic = TextBaseline.alphabetic;

function withId<T extends RenderBox>(id: string, box: T): T {
  box.debugId = id;
  return box;
}

function frame(child: RenderBox, size = new Size(800, 600)): RenderView {
  const owner = new PipelineOwner();
  const view = withId("frame", new RenderView({ size, child }));
  owner.rootNode = view;
  view.prepareInitialFrame();
  owner.flushLayout();
  return view;
}

/** Center > Padding(start 10, top 20, end 30) > SizedBox(100x50), as Flutter lays it out. */
function paddedScene(direction: TextDirection): RenderView {
  return frame(
    withId(
      "center",
      new RenderPositionedBox({
        child: withId(
          "padding",
          new RenderPadding({
            padding: EdgeInsetsDirectional.only({ start: 10, top: 20, end: 30 }),
            textDirection: direction,
            child: withId(
              "box",
              new RenderConstrainedBox({
                additionalConstraints: BoxConstraints.tightFor({ width: 100, height: 50 }),
              }),
            ),
          }),
        ),
      }),
    ),
  );
}

describe("[STU-005] padding, align and sized box layouts", () => {
  test("LTR rects", () => {
    expect(goldenRects(paddedScene(TextDirection.ltr))).toEqual({
      box: [340, 285, 100, 50],
      center: [0, 0, 800, 600],
      frame: [0, 0, 800, 600],
      padding: [330, 265, 140, 70],
    });
  });

  test("RTL rects mirror the directional padding", () => {
    const rects = goldenRects(paddedScene(TextDirection.rtl));
    expect(rects["box"]).toEqual([360, 285, 100, 50]);
    expect(rects["padding"]).toEqual([330, 265, 140, 70]);
  });

  test("golden keys are sorted and the golden type holds them", () => {
    const view = paddedScene(TextDirection.ltr);
    const golden: LayoutGolden = {
      flutterRevision: "6a19cca56475dbfba1478ee68d7bd0c2ef891da1",
      scene: "padded",
      frame: { width: 800, height: 600, devicePixelRatio: 2 },
      locale: "en-US",
      textScale: 1,
      rects: goldenRects(view),
    };
    expect(Object.keys(golden.rects)).toEqual(["box", "center", "frame", "padding"]);
  });

  test("duplicate node IDs are rejected", () => {
    const view = frame(withId("x", new RenderProxyBox(withId("x", new RenderTestBox({ size: Size.zero })))));
    expect(() => globalRects(view)).toThrow("duplicate debugId");
  });

  test("directional alignment resolves against the text direction", () => {
    const leaf = new RenderTestBox({ size: new Size(50, 40) });
    const align = new RenderPositionedBox({
      alignment: AlignmentDirectional.topStart,
      textDirection: TextDirection.rtl,
      child: leaf,
    });
    align.layout(BoxConstraints.tight(new Size(200, 100)));
    expect(leaf.parentData).not.toBeNull();
    expect(globalRects(withId("a", align)).size).toBe(1);
    expect(offsetOf(leaf)).toEqual(new Offset(150, 0));
    align.textDirection = TextDirection.rtl;
    align.textDirection = TextDirection.ltr;
    align.layout(BoxConstraints.tight(new Size(200, 100)));
    expect(offsetOf(leaf)).toEqual(new Offset(0, 0));
    align.alignment = AlignmentDirectional.topStart;
    align.alignment = Alignment.bottomRight;
    expect(align.alignment).toBe(Alignment.bottomRight);
    expect(align.textDirection).toBe(TextDirection.ltr);
    align.layout(BoxConstraints.tight(new Size(200, 100)));
    expect(offsetOf(leaf)).toEqual(new Offset(150, 60));
  });

  test("padding without a child, with Infinity, and when changed", () => {
    const padding = new RenderPadding({ padding: EdgeInsets.fromLTRB(1, 2, 3, 4) });
    padding.layout(new BoxConstraints());
    expect(padding.size).toEqual(new Size(4, 6));
    expect(padding.getMinIntrinsicWidth(10)).toBe(4);
    expect(padding.getMaxIntrinsicWidth(10)).toBe(4);
    expect(padding.getMinIntrinsicHeight(10)).toBe(6);
    expect(padding.getMaxIntrinsicHeight(10)).toBe(6);
    expect(padding.getDryLayout(BoxConstraints.tight(new Size(9, 9)))).toEqual(new Size(9, 9));
    expect(padding.getDryBaseline(new BoxConstraints(), alphabetic)).toBeNull();
    expect(padding.getDistanceToBaseline(alphabetic)).toBe(6);
    const same = padding.padding;
    padding.padding = same;
    padding.padding = EdgeInsets.all(5);
    padding.textDirection = null;
    padding.textDirection = TextDirection.ltr;
    expect(padding.textDirection).toBe(TextDirection.ltr);
    padding.child = new RenderTestBox({ size: new Size(50, 40) });
    padding.layout(new BoxConstraints({ maxWidth: 30 }));
    expect(padding.size).toEqual(new Size(30, 50));
    expect(padding.child?.size).toEqual(new Size(20, 40));
    expect(padding.getMinIntrinsicWidth(inf)).toBe(60);
    expect(padding.getMaxIntrinsicWidth(4)).toBe(60);
    expect(padding.getMinIntrinsicHeight(100)).toBe(50);
    expect(padding.getMaxIntrinsicHeight(100)).toBe(50);
    expect(padding.getDryLayout(new BoxConstraints({ maxWidth: 30 }))).toEqual(padding.size);
  });
});

function offsetOf(box: RenderBox): Offset {
  const data = box.parentData;
  return data !== null && "offset" in data && data.offset instanceof Offset ? data.offset : Offset.zero;
}

describe("[STU-005] Align widthFactor and heightFactor", () => {
  const child = () => new RenderTestBox({ size: new Size(50, 40) });

  test("factors under loose constraints", () => {
    const leaf = child();
    const align = new RenderPositionedBox({ widthFactor: 2, heightFactor: 0.5, child: leaf });
    const loose = BoxConstraints.loose(new Size(300, 300));
    align.layout(loose);
    expect(align.size).toEqual(new Size(100, 20));
    expect(offsetOf(leaf)).toEqual(new Offset(25, -10));
    expect(align.getDryLayout(loose)).toEqual(align.size);
    expect(align.getMinIntrinsicWidth(inf)).toBe(100);
    expect(align.getMaxIntrinsicWidth(inf)).toBe(100);
    expect(align.getMinIntrinsicHeight(inf)).toBe(20);
    expect(align.getMaxIntrinsicHeight(inf)).toBe(20);
    align.widthFactor = 2;
    align.widthFactor = null;
    align.heightFactor = 0.5;
    align.heightFactor = null;
    expect([align.widthFactor, align.heightFactor]).toEqual([null, null]);
    align.layout(loose);
    expect(align.size).toEqual(new Size(300, 300));
    expect(offsetOf(leaf)).toEqual(new Offset(125, 130));
  });

  test("without factors: shrink-wraps unbounded axes, expands bounded ones", () => {
    const leaf = child();
    const align = new RenderPositionedBox({ child: leaf });
    align.layout(new BoxConstraints());
    expect(align.size).toEqual(new Size(50, 40));
    align.layout(new BoxConstraints({ maxHeight: 200 }));
    expect(align.size).toEqual(new Size(50, 200));
    expect(offsetOf(leaf)).toEqual(new Offset(0, 80));
    expect(align.getDryLayout(new BoxConstraints({ maxHeight: 200 }))).toEqual(align.size);
  });

  test("without a child", () => {
    const align = new RenderPositionedBox();
    align.layout(new BoxConstraints());
    expect(align.size).toEqual(Size.zero);
    align.layout(BoxConstraints.loose(new Size(300, 300)));
    expect(align.size).toEqual(new Size(300, 300));
    expect(align.getDryLayout(new BoxConstraints({ maxWidth: 10 }))).toEqual(new Size(10, 0));
    expect(align.getMinIntrinsicWidth(0)).toBe(0);
    expect(align.getDryBaseline(new BoxConstraints(), alphabetic)).toBeNull();
    expect(align.getDistanceToBaseline(alphabetic, { onlyReal: true })).toBeNull();
  });
});

describe("[STU-005] constrained, sized and decorated boxes", () => {
  test("SizedBox and ConstrainedBox layouts", () => {
    const leaf = new RenderTestBox({ size: new Size(50, 40) });
    const box = new RenderConstrainedBox({
      additionalConstraints: new BoxConstraints({ minWidth: 80, maxHeight: 30 }),
      child: leaf,
    });
    box.layout(BoxConstraints.loose(new Size(200, 200)));
    expect(box.size).toEqual(new Size(80, 30));
    expect(box.getDryLayout(BoxConstraints.loose(new Size(200, 200)))).toEqual(box.size);
    box.additionalConstraints = new BoxConstraints({ minWidth: 80, maxHeight: 30 });
    expect(box.needsLayout).toBe(false);
    box.additionalConstraints = BoxConstraints.tightFor({ width: 500 });
    box.layout(BoxConstraints.loose(new Size(200, 200)));
    expect(box.size).toEqual(new Size(200, 40));
    const empty = new RenderConstrainedBox({
      additionalConstraints: BoxConstraints.tightFor({ width: 10 }),
    });
    empty.layout(BoxConstraints.loose(new Size(100, 100)));
    expect(empty.size).toEqual(new Size(10, 0));
    expect(empty.getDryLayout(new BoxConstraints())).toEqual(new Size(10, 0));
    expect(empty.getDryBaseline(new BoxConstraints(), alphabetic)).toBeNull();
  });

  test("ConstrainedBox intrinsics", () => {
    const leaf = () => new RenderTestBox({ size: new Size(50, 40) });
    const tight = new RenderConstrainedBox({
      additionalConstraints: BoxConstraints.tightFor({ width: 30, height: 20 }),
      child: leaf(),
    });
    expect(tight.getMinIntrinsicWidth(inf)).toBe(30);
    expect(tight.getMaxIntrinsicWidth(inf)).toBe(30);
    expect(tight.getMinIntrinsicHeight(inf)).toBe(20);
    expect(tight.getMaxIntrinsicHeight(inf)).toBe(20);
    const min = new RenderConstrainedBox({
      additionalConstraints: new BoxConstraints({ minWidth: 80, maxHeight: 30 }),
      child: leaf(),
    });
    expect(min.getMinIntrinsicWidth(inf)).toBe(80);
    expect(min.getMaxIntrinsicWidth(inf)).toBe(80);
    expect(min.getMinIntrinsicHeight(inf)).toBe(30);
    expect(min.getMaxIntrinsicHeight(inf)).toBe(30);
    const expand = new RenderConstrainedBox({
      additionalConstraints: BoxConstraints.expand(),
      child: leaf(),
    });
    expect(expand.getMinIntrinsicWidth(inf)).toBe(50);
    expect(expand.getMaxIntrinsicWidth(inf)).toBe(50);
    expect(expand.getMinIntrinsicHeight(inf)).toBe(40);
    expect(expand.getMaxIntrinsicHeight(inf)).toBe(40);
  });

  test("proxy and decorated boxes take the child's size", () => {
    const decorated = new RenderDecoratedBox({
      decoration: { color: "#fff" },
      child: new RenderTestBox({ size: new Size(50, 40) }),
    });
    expect(decorated.position).toBe(DecorationPosition.background);
    decorated.layout(BoxConstraints.loose(new Size(100, 100)));
    expect(decorated.size).toEqual(new Size(50, 40));
    expect(decorated.getMinIntrinsicWidth(inf)).toBe(50);
    expect(decorated.getMaxIntrinsicWidth(inf)).toBe(50);
    expect(decorated.getMinIntrinsicHeight(inf)).toBe(40);
    expect(decorated.getMaxIntrinsicHeight(inf)).toBe(40);
    expect(decorated.getDryLayout(new BoxConstraints({ maxWidth: 10 }))).toEqual(new Size(10, 40));
    const empty = new RenderProxyBox();
    empty.layout(new BoxConstraints({ minWidth: 5, minHeight: 6 }));
    expect(empty.size).toEqual(new Size(5, 6));
    expect(empty.getDryLayout(new BoxConstraints({ minWidth: 1 }))).toEqual(new Size(1, 0));
    expect(empty.getMinIntrinsicWidth(0)).toBe(0);
    expect(empty.getMaxIntrinsicWidth(0)).toBe(0);
    expect(empty.getMinIntrinsicHeight(0)).toBe(0);
    expect(empty.getMaxIntrinsicHeight(0)).toBe(0);
    expect(empty.getDryBaseline(new BoxConstraints(), alphabetic)).toBeNull();
    expect(empty.getDistanceToBaseline(alphabetic)).toBe(6);
  });
});

describe("[STU-005] baselines through proxy and shifted boxes", () => {
  const leaf = () => new RenderTestBox({ size: new Size(50, 40), baseline: 15 });

  test("padding adds its top inset; align adds the child's offset", () => {
    const padding = new RenderPadding({ padding: EdgeInsets.only({ top: 10 }), child: leaf() });
    const proxy = new RenderProxyBox(padding);
    const align = new RenderPositionedBox({ child: proxy });
    const loose = BoxConstraints.loose(new Size(200, 200));
    align.layout(BoxConstraints.tight(new Size(200, 200)));
    expect(padding.getDistanceToBaseline(alphabetic)).toBe(25);
    expect(proxy.getDistanceToBaseline(alphabetic)).toBe(25);
    // The padded child is 50x50, centered at (75, 75).
    expect(align.getDistanceToBaseline(TextBaseline.alphabetic)).toBe(100);
    expect(align.getDryBaseline(loose, alphabetic)).toBe(100);
    expect(proxy.getDryBaseline(loose, alphabetic)).toBe(25);
    expect(padding.getDryBaseline(loose, TextBaseline.ideographic)).toBe(25);
    const sized = new RenderConstrainedBox({
      additionalConstraints: BoxConstraints.tightFor({ width: 10 }),
      child: leaf(),
    });
    expect(sized.getDryBaseline(loose, alphabetic)).toBe(15);
  });

  test("boxes without a real baseline", () => {
    const plain = new RenderTestBox({ size: new Size(50, 40) });
    const padding = new RenderPadding({ padding: EdgeInsets.all(5), child: plain });
    const align = new RenderPositionedBox({ child: padding });
    align.layout(BoxConstraints.loose(new Size(100, 100)));
    expect(plain.getDistanceToBaseline(alphabetic)).toBe(40);
    expect(plain.getDistanceToBaseline(alphabetic, { onlyReal: true })).toBeNull();
    expect(padding.getDistanceToBaseline(alphabetic, { onlyReal: true })).toBeNull();
    expect(padding.getDryBaseline(new BoxConstraints(), alphabetic)).toBeNull();
    expect(align.getDryBaseline(new BoxConstraints(), alphabetic)).toBeNull();
  });
});

// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/material/input_decorator.dart, text_field.dart
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * `InputDecorator` (Material 3 defaults) as `TextField` builds it: the render
 * object places the input line, the prefix and suffix icons and the helper or
 * error text inside the container the border draws. The label and the hint
 * never change the container's size, only the room the floating label takes
 * above the input of a non-outlined field (`floatingLabelHeight`), so they are
 * not boxes here.
 */

import { BoxConstraints } from "../constraints.ts";
import { ContainerRenderBox } from "../container.ts";
import { Alignment, EdgeInsets, EdgeInsetsDirectional, Offset, Size, TextDirection } from "../geometry.ts";
import { RenderConstrainedBox } from "../proxy-box.ts";
import { type BoxParentData, type RenderBox, TextBaseline } from "../render-box.ts";
import { RenderPositionedBox, RenderShiftedBox, setChildOffset } from "../shifted-box.ts";
import { kMinInteractiveDimension, type VisualDensity } from "./theme.ts";

/** `_kSubtextCounterPadding` is unused without a counter; the gap below the container is `subtextGap`. */
const subtextGap = 4;
/** `_kSubtextCounterPadding`: the gap between the helper text and the counter. */
const subtextCounterPadding = 16;
const prefixToInputGap = 4;
const inputToSuffixGap = 4;
/** `_kInputExtraPadding`. */
const inputExtraPadding = 4;
/** `OutlineInputBorder.gapPadding`. */
const outlineGapPadding = 4;

export interface InputDecoratorOptions {
  /** The editable line: tight to the input width, one text line high. */
  input: RenderBox;
  prefixIcon: RenderBox | null;
  suffixIcon: RenderBox | null;
  /** `InputDecoration.icon`: outside the container, at the start, with 16 after it. */
  icon?: RenderBox | null;
  /** `prefixText` and `suffixText`, drawn inside the container beside the input. */
  prefix?: RenderBox | null;
  suffix?: RenderBox | null;
  /** `hintText`: laid out over the input and sizing it only while the field is empty. */
  hint?: RenderBox | null;
  /** Whether the field has no text. */
  isEmpty?: boolean;
  /** `counterText`, at the end of the subtext line. */
  counter?: RenderBox | null;
  /** The helper or error text. */
  subtext: RenderBox | null;
  /** `OutlineInputBorder` rather than `UnderlineInputBorder`. */
  outline: boolean;
  filled: boolean;
  isDense: boolean;
  hasLabel: boolean;
  /** Whether the label floats (the field has text); a floating label moves into the outline's gap. */
  floating: boolean;
  /** `textScaler.scale(4 + 0.75 * labelFontSize)`. */
  floatingLabelHeight: number;
  density: VisualDensity;
  textDirection: TextDirection;
}

/** `_RenderDecoration`'s inputs reduced to the numbers layout reads. */
interface Metrics {
  start: number;
  top: number;
  end: number;
  bottom: number;
  inputGap: number;
  floatingLabelHeight: number;
}

function metricsOf(o: InputDecoratorOptions): Metrics {
  const dense = o.isDense;
  let padding: readonly [number, number, number, number];
  let floatingLabelHeight = 0;
  if (!o.outline) {
    floatingLabelHeight = o.floatingLabelHeight;
    if (o.filled) {
      padding = dense ? [12, 4, 12, 4] : [12, 8, 12, 8];
    } else {
      padding = dense ? [0, 4, 0, 4] : [0, 8, 0, 8];
    }
  } else {
    padding = dense ? [12, 16, 12, 8] : [12, 20, 12, 12];
  }
  const inputGap = o.outline ? outlineGapPadding : o.filled ? inputExtraPadding : 0;
  return {
    start: padding[0],
    top: padding[1],
    end: padding[2],
    bottom: padding[3],
    inputGap,
    floatingLabelHeight,
  };
}

interface Subtext {
  ascent: number;
  bottomHeight: number;
  subtextHeight: number;
}

/** `_RenderDecoration`. */
class RenderDecoration extends ContainerRenderBox<RenderBox> {
  private readonly m: Metrics;
  private readonly helperError: RenderBox;

  constructor(private readonly o: InputDecoratorOptions) {
    super();
    this.m = metricsOf(o);
    this.helperError =
      o.subtext ??
      new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ width: 0, height: 0 }) });
    for (const child of [
      o.icon,
      o.input,
      o.hint,
      o.prefix,
      o.suffix,
      o.counter,
      o.prefixIcon,
      o.suffixIcon,
      this.helperError,
    ]) {
      if (child !== null && child !== undefined) {
        this.add(child);
      }
    }
  }

  private get densityDy(): number {
    return this.o.density.baseSizeAdjustmentY;
  }

  /** `TextAlignVertical.y`: top for an underline field, centre for an outline. */
  private get alignY(): number {
    return this.o.outline ? 0 : -1;
  }

  protected override computeMinIntrinsicWidth(height: number): number {
    return this.intrinsicWidth((b) => b.getMinIntrinsicWidth(height));
  }

  protected override computeMaxIntrinsicWidth(height: number): number {
    return this.intrinsicWidth((b) => b.getMaxIntrinsicWidth(height));
  }

  private intrinsicWidth(of: (b: RenderBox) => number): number {
    const { prefixIcon, suffixIcon, input, icon, prefix, suffix, hint } = this.o;
    const content = this.o.isEmpty && hint ? Math.max(of(input), of(hint)) : of(input);
    return (
      (icon ? of(icon) : 0) +
      (prefixIcon !== null ? prefixToInputGap : this.m.start + this.m.inputGap) +
      (prefixIcon === null ? 0 : of(prefixIcon)) +
      (prefix ? of(prefix) : 0) +
      content +
      (suffix ? of(suffix) : 0) +
      (suffixIcon === null ? 0 : of(suffixIcon)) +
      (suffixIcon !== null ? inputToSuffixGap : this.m.end + this.m.inputGap)
    );
  }

  protected override computeMinIntrinsicHeight(width: number): number {
    const { prefixIcon, suffixIcon, input, hasLabel } = this.o;
    const m = this.m;
    const prefixIconHeight = prefixIcon?.getMinIntrinsicHeight(width) ?? 0;
    const suffixIconHeight = suffixIcon?.getMinIntrinsicHeight(width) ?? 0;
    const contentWidth = Math.max(width - m.start - m.end - m.inputGap * 2, 0);
    const subtextHeight = Math.max(this.helperError.getMinIntrinsicHeight(contentWidth), 0);
    const inputHeight = input.getMinIntrinsicHeight(contentWidth);
    const contentHeight = m.top + (hasLabel ? m.floatingLabelHeight : 0) + inputHeight + m.bottom + this.densityDy;
    const containerHeight = Math.max(contentHeight, prefixIconHeight, suffixIconHeight);
    const minContainerHeight = this.o.isDense ? 0 : kMinInteractiveDimension;
    return Math.max(containerHeight, minContainerHeight) + (subtextHeight > 0 ? subtextHeight + subtextGap : 0);
  }

  protected override computeMaxIntrinsicHeight(width: number): number {
    return this.getMinIntrinsicHeight(width);
  }

  protected override computeDistanceToActualBaseline(baseline: TextBaseline): number | null {
    const input = this.o.input;
    return (
      (input.parentData as BoxParentData).offset.dy + (input.getDistanceToActualBaseline(baseline) ?? input.size.height)
    );
  }

  /** `_layout`; with `place` the children are laid out, otherwise their dry layout is read. */
  // biome-ignore lint/complexity/noExcessiveCognitiveComplexity: a line-for-line port of _RenderDecoration._layout, shared by layout and dry layout
  private layoutAll(constraints: BoxConstraints, place: boolean) {
    const m = this.m;
    const o = this.o;
    const layoutChild = (box: RenderBox, c: BoxConstraints): Size => {
      if (place) {
        box.layout(c, { parentUsesSize: true });
        return box.size;
      }
      return box.getDryLayout(c);
    };
    const baselineOf = (box: RenderBox, c: BoxConstraints): number => {
      const b = place
        ? box.getDistanceToBaseline(TextBaseline.alphabetic)
        : box.getDryBaseline(c, TextBaseline.alphabetic);
      return b ?? (place ? box.size.height : box.getDryLayout(c).height);
    };
    const boxConstraints = constraints.loosen();
    const iconWidth = o.icon ? layoutChild(o.icon, boxConstraints).width : 0;
    const containerConstraints = boxConstraints.deflate(EdgeInsets.only({ left: iconWidth }));
    const contentConstraints = containerConstraints.deflate(
      EdgeInsetsDirectional.only({ start: m.start + m.inputGap, end: m.end + m.inputGap }),
    );
    // _computeSubtextSizes.
    const counterSize = o.counter ? layoutChild(o.counter, contentConstraints) : Size.zero;
    const counterAscent = o.counter ? baselineOf(o.counter, contentConstraints) : 0;
    const counterPadding = o.counter ? subtextCounterPadding : 0;
    const helperConstraints = contentConstraints.deflate(EdgeInsets.only({ left: counterSize.width + counterPadding }));
    const helperHeight = layoutChild(this.helperError, helperConstraints).height;
    const subtext: Subtext | null =
      helperHeight === 0 && counterSize.height === 0
        ? null
        : {
            ascent: Math.max(counterAscent, baselineOf(this.helperError, helperConstraints)) + subtextGap,
            bottomHeight: Math.max(counterAscent, helperHeight) + subtextGap,
            subtextHeight: Math.max(counterSize.height, helperHeight) + subtextGap,
          };
    const prefixIconSize = o.prefixIcon === null ? Size.zero : layoutChild(o.prefixIcon, containerConstraints);
    const suffixIconSize = o.suffixIcon === null ? Size.zero : layoutChild(o.suffixIcon, containerConstraints);
    const prefixSize = o.prefix ? layoutChild(o.prefix, contentConstraints) : Size.zero;
    const suffixSize = o.suffix ? layoutChild(o.suffix, contentConstraints) : Size.zero;
    const accessoryStart =
      iconWidth +
      prefixSize.width +
      (o.prefixIcon === null ? m.start + m.inputGap : prefixIconSize.width + prefixToInputGap);
    const accessoryEnd =
      suffixSize.width + (o.suffixIcon === null ? m.end + m.inputGap : suffixIconSize.width + inputToSuffixGap);
    const inputWidth = Math.max(0, constraints.maxWidth - (accessoryStart + accessoryEnd));
    const bottomHeight = subtext?.bottomHeight ?? 0;
    const topHeight = o.hasLabel && !o.outline ? m.floatingLabelHeight : 0;
    const inputConstraints = boxConstraints
      .deflate(EdgeInsets.only({ top: m.top + m.bottom + topHeight + bottomHeight + this.densityDy }))
      .tighten({ width: inputWidth });
    const inputSize = layoutChild(o.input, inputConstraints);
    const hintConstraints = boxConstraints.tighten({ width: inputWidth });
    const hintSize = o.hint ? layoutChild(o.hint, hintConstraints) : Size.zero;
    const inputBaseline = baselineOf(o.input, inputConstraints);
    const hintBaseline = o.hint ? baselineOf(o.hint, hintConstraints) : 0;
    const inputHeight = Math.max(o.isEmpty ? hintSize.height : 0, inputSize.height);
    const inputInternalBaseline = Math.max(inputBaseline, hintBaseline);
    const prefixBaseline = o.prefix ? baselineOf(o.prefix, contentConstraints) : 0;
    const suffixBaseline = o.suffix ? baselineOf(o.suffix, contentConstraints) : 0;
    const fixAboveInput = Math.max(0, Math.max(prefixBaseline, suffixBaseline) - inputInternalBaseline);
    const fixBelowBaseline = Math.max(prefixSize.height - prefixBaseline, suffixSize.height - suffixBaseline);
    const fixBelowInput = Math.max(0, fixBelowBaseline - (inputHeight - inputInternalBaseline));
    const fixIconHeight = Math.max(prefixIconSize.height, suffixIconSize.height);
    const contentHeight = Math.max(
      fixIconHeight,
      topHeight + m.top + fixAboveInput + inputHeight + fixBelowInput + m.bottom + this.densityDy,
    );
    const minContainerHeight = o.isDense ? inputHeight : kMinInteractiveDimension;
    const maxContainerHeight = Math.max(0, boxConstraints.maxHeight - bottomHeight);
    const containerHeight = Math.min(Math.max(contentHeight, minContainerHeight), maxContainerHeight);
    const interactiveAdjustment = minContainerHeight > contentHeight ? (minContainerHeight - contentHeight) / 2 : 0;
    const overflow = Math.max(0, contentHeight - maxContainerHeight);
    const alignFactor = (this.alignY + 1) / 2;
    const baselineAdjustment = fixAboveInput - overflow * (1 - alignFactor);
    const topInputBaseline =
      m.top + topHeight + inputInternalBaseline + baselineAdjustment + interactiveAdjustment + this.densityDy / 2;
    const maxContentHeight = containerHeight - m.top - m.bottom - topHeight - this.densityDy;
    const alignableHeight = fixAboveInput + inputHeight + fixBelowInput;
    const maxVerticalOffset = maxContentHeight - alignableHeight;
    let baseline: number;
    if (o.outline) {
      const centre = inputInternalBaseline + baselineAdjustment / 2 + (containerHeight - inputHeight) / 2;
      const bottom = topInputBaseline + maxVerticalOffset;
      const basis = this.alignY <= 0 ? Math.max(centre - topInputBaseline, 0) : Math.max(bottom - centre, 0);
      baseline = centre + basis * this.alignY;
    } else {
      baseline = topInputBaseline + maxVerticalOffset * alignFactor;
    }
    return {
      baseline,
      containerHeight,
      subtext,
      size: new Size(constraints.maxWidth, containerHeight + (subtext?.subtextHeight ?? 0)),
    };
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    return constraints.constrain(this.layoutAll(constraints, false).size);
  }

  protected override performLayout(): void {
    const constraints = this.constraints;
    const layout = this.layoutAll(constraints, true);
    const m = this.m;
    const o = this.o;
    const overallWidth = layout.size.width;
    const height = layout.containerHeight;
    const rtl = o.textDirection === TextDirection.rtl;
    const iconWidth = o.icon?.size.width ?? 0;
    const centerLayout = (box: RenderBox, x: number): number => {
      setChildOffset(box, new Offset(x, (height - box.size.height) / 2));
      return box.size.width;
    };
    const baselineLayout = (box: RenderBox, x: number): number => {
      setChildOffset(box, new Offset(x, layout.baseline - (box.getDistanceToBaseline(TextBaseline.alphabetic) ?? 0)));
      return box.size.width;
    };
    if (o.icon) {
      centerLayout(o.icon, rtl ? overallWidth - iconWidth : 0);
    }
    const subtextBaseline = (layout.subtext?.ascent ?? 0) + layout.containerHeight;
    const helperBaseline = this.helperError.getDistanceToBaseline(TextBaseline.alphabetic) ?? 0;
    const counterBaseline = o.counter?.getDistanceToBaseline(TextBaseline.alphabetic) ?? 0;
    const place: Placement = {
      layout,
      overallWidth,
      iconWidth,
      subtextBaseline,
      helperBaseline,
      counterBaseline,
      centerLayout,
      baselineLayout,
    };
    if (rtl) {
      this.placeRtl(place);
    } else {
      this.placeLtr(place);
    }
  }

  private placeRtl(p: Placement): void {
    const { m, o } = { m: this.m, o: this.o };
    const { overallWidth, iconWidth, subtextBaseline, helperBaseline, counterBaseline, centerLayout, baselineLayout } =
      p;
    let start = overallWidth - m.start - iconWidth;
    let end = m.end;
    setChildOffset(
      this.helperError,
      new Offset(start - this.helperError.size.width - m.inputGap, subtextBaseline - helperBaseline),
    );
    if (o.counter) {
      setChildOffset(o.counter, new Offset(end + m.inputGap, subtextBaseline - counterBaseline));
    }
    if (o.prefixIcon !== null) {
      start += m.start;
      start -= centerLayout(o.prefixIcon, start - o.prefixIcon.size.width);
      start -= prefixToInputGap;
    } else {
      start -= m.inputGap;
    }
    if (o.prefix) {
      start -= baselineLayout(o.prefix, start - o.prefix.size.width);
    }
    baselineLayout(o.input, start - o.input.size.width);
    if (o.hint) {
      baselineLayout(o.hint, start - o.hint.size.width);
    }
    if (o.suffixIcon !== null) {
      end -= m.end;
      end += centerLayout(o.suffixIcon, end);
      end += inputToSuffixGap;
    } else {
      end += m.inputGap;
    }
    if (o.suffix) {
      baselineLayout(o.suffix, end);
    }
  }

  private placeLtr(p: Placement): void {
    const { m, o } = { m: this.m, o: this.o };
    const { overallWidth, iconWidth, subtextBaseline, helperBaseline, counterBaseline, centerLayout, baselineLayout } =
      p;
    let start = m.start + iconWidth;
    let end = overallWidth - m.end;
    setChildOffset(this.helperError, new Offset(start + m.inputGap, subtextBaseline - helperBaseline));
    if (o.counter) {
      setChildOffset(o.counter, new Offset(end - o.counter.size.width - m.inputGap, subtextBaseline - counterBaseline));
    }
    if (o.prefixIcon !== null) {
      start -= m.start;
      start += centerLayout(o.prefixIcon, start);
      start += prefixToInputGap;
    } else {
      start += m.inputGap;
    }
    if (o.prefix) {
      start += baselineLayout(o.prefix, start);
    }
    baselineLayout(o.input, start);
    if (o.hint) {
      baselineLayout(o.hint, start);
    }
    if (o.suffixIcon !== null) {
      end += m.end;
      end -= centerLayout(o.suffixIcon, end - o.suffixIcon.size.width);
      end -= inputToSuffixGap;
    } else {
      end -= m.inputGap;
    }
    if (o.suffix) {
      baselineLayout(o.suffix, end - o.suffix.size.width);
    }
  }
}

/** What `performLayout` hands to the direction-specific placement. */
interface Placement {
  layout: { baseline: number; containerHeight: number; subtext: Subtext | null };
  overallWidth: number;
  iconWidth: number;
  subtextBaseline: number;
  helperBaseline: number;
  counterBaseline: number;
  centerLayout: (box: RenderBox, x: number) => number;
  baselineLayout: (box: RenderBox, x: number) => number;
}

/** The editable line of a `TextField`: tight to the width given, one `lineBox` high, with the line's baseline. */
export class RenderEditableLine extends RenderShiftedBox {
  constructor(line: RenderBox) {
    super(line);
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    const child = this.child as RenderBox;
    return constraints.constrain(new Size(constraints.maxWidth, child.getDryLayout(new BoxConstraints()).height));
  }

  protected override performLayout(): void {
    const child = this.child as RenderBox;
    child.layout(new BoxConstraints(), { parentUsesSize: true });
    this.size = this.constraints.constrain(new Size(this.constraints.maxWidth, child.size.height));
    setChildOffset(child, Offset.zero);
  }

  protected override computeDistanceToActualBaseline(baseline: TextBaseline): number | null {
    return (this.child as RenderBox).getDistanceToActualBaseline(baseline);
  }
}

/** An icon slot of the decorator: `Center(widthFactor: 1)` over a box at least 48 square (less under density). */
export function decoratorIcon(icon: RenderBox, density: VisualDensity): RenderBox {
  return new RenderPositionedBox({
    alignment: Alignment.center,
    widthFactor: 1,
    heightFactor: 1,
    child: new RenderConstrainedBox({
      additionalConstraints: density.effectiveConstraints(
        new BoxConstraints({ minWidth: kMinInteractiveDimension, minHeight: kMinInteractiveDimension }),
      ),
      child: icon,
    }),
  });
}

/** `InputDecorator.build`: the decoration around the input. */
export function buildInputDecorator(o: InputDecoratorOptions): RenderBox {
  return new RenderDecoration(o);
}

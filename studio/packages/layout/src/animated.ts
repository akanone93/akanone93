// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Layout at a given animation value (STU-068, timeline scrubbing): the
 * interpolation Flutter's implicit animations apply (`Tween.lerp` with a
 * linear curve, `BoxConstraints.lerp`, `EdgeInsets.lerp`, `Alignment.lerp`)
 * and the widget compositions of `Container` and `SizeTransition`, so the
 * layout at animation value `t` in [0, 1] is the layout Flutter produces
 * `t` of the way through the animation.
 *
 * An implicit animation (`AnimatedContainer`, `AnimatedPadding`, ...)
 * animates from the previous value to the new one; a property that is null
 * in the new value is not animated (null), and one that was null before
 * jumps straight to the new value, as Flutter's `ImplicitlyAnimatedWidgetState`
 * does.
 */

import { BoxConstraints } from "./flutter/constraints.ts";
import { Axis } from "./flutter/flex.ts";
import {
  Alignment,
  AlignmentDirectional,
  type AlignmentGeometry,
  EdgeInsets,
  type EdgeInsetsGeometry,
  type TextDirection,
} from "./flutter/geometry.ts";
import { RenderConstrainedBox } from "./flutter/proxy-box.ts";
import { ClipShape, RenderClip, RenderLimitedBox } from "./flutter/proxy-boxes.ts";
import type { RenderBox } from "./flutter/render-box.ts";
import { RenderPadding, RenderPositionedBox } from "./flutter/shifted-box.ts";

/** `Tween<double>.lerp`: `begin + (end - begin) * t`. */
export function lerpDouble(begin: number, end: number, t: number): number {
  return begin + (end - begin) * t;
}

/** The value of an implicit-animation property at `t`: null when `to` is null, `to` when `from` is null. */
export function lerpProperty<T>(from: T | null, to: T | null, t: number, lerp: (a: T, b: T, t: number) => T): T | null {
  if (to === null) {
    return null;
  }
  return from === null ? to : lerp(from, to, t);
}

/** `BoxConstraints.lerp` (both operands bounded, or both unbounded, per component). */
export function lerpBoxConstraints(a: BoxConstraints, b: BoxConstraints, t: number): BoxConstraints {
  const component = (x: number, y: number): number =>
    Number.isFinite(x) ? lerpDouble(x, y, t) : Number.POSITIVE_INFINITY;
  return new BoxConstraints({
    minWidth: component(a.minWidth, b.minWidth),
    maxWidth: component(a.maxWidth, b.maxWidth),
    minHeight: component(a.minHeight, b.minHeight),
    maxHeight: component(a.maxHeight, b.maxHeight),
  });
}

/** `EdgeInsets.lerp` for absolute insets. */
export function lerpEdgeInsets(a: EdgeInsets, b: EdgeInsets, t: number): EdgeInsets {
  return EdgeInsets.fromLTRB(
    lerpDouble(a.left, b.left, t),
    lerpDouble(a.top, b.top, t),
    lerpDouble(a.right, b.right, t),
    lerpDouble(a.bottom, b.bottom, t),
  );
}

/** `AlignmentGeometry.lerp` between two alignments of the same kind (both absolute or both directional). */
export function lerpAlignment(a: AlignmentGeometry, b: AlignmentGeometry, t: number): AlignmentGeometry {
  if (a instanceof Alignment && b instanceof Alignment) {
    return new Alignment(lerpDouble(a.x, b.x, t), lerpDouble(a.y, b.y, t));
  }
  if (a instanceof AlignmentDirectional && b instanceof AlignmentDirectional) {
    return new AlignmentDirectional(lerpDouble(a.start, b.start, t), lerpDouble(a.y, b.y, t));
  }
  throw new Error("cannot interpolate between an absolute and a directional alignment");
}

/** The properties of `Container` that affect layout. */
export interface ContainerLayout {
  alignment?: AlignmentGeometry | null;
  padding?: EdgeInsetsGeometry | null;
  constraints?: BoxConstraints | null;
  margin?: EdgeInsetsGeometry | null;
  textDirection: TextDirection;
  child: RenderBox | null;
}

/**
 * The render objects `Container` builds (`Container.build`), outermost
 * first: margin, constraints, padding, align, child; a childless container
 * without tight constraints expands to its constraints' maximum, collapsing
 * to zero while unbounded. Returns the outermost, or the child when no
 * property applies.
 */
export function buildContainer(layout: ContainerLayout): RenderBox | null {
  let current: RenderBox | null = layout.child;
  const constraints = layout.constraints ?? null;
  if (current === null && (constraints === null || !constraints.isTight)) {
    current = new RenderLimitedBox({
      maxWidth: 0,
      maxHeight: 0,
      child: new RenderConstrainedBox({ additionalConstraints: BoxConstraints.expand() }),
    });
  } else if (layout.alignment !== undefined && layout.alignment !== null) {
    current = new RenderPositionedBox({
      alignment: layout.alignment,
      textDirection: layout.textDirection,
      child: current,
    });
  }
  if (layout.padding !== undefined && layout.padding !== null) {
    current = new RenderPadding({ padding: layout.padding, textDirection: layout.textDirection, child: current });
  }
  if (constraints !== null) {
    current = new RenderConstrainedBox({ additionalConstraints: constraints, child: current });
  }
  if (layout.margin !== undefined && layout.margin !== null) {
    current = new RenderPadding({ padding: layout.margin, textDirection: layout.textDirection, child: current });
  }
  return current;
}

/**
 * `SizeTransition`: a clip around an `Align` whose width or height factor
 * follows `sizeFactor` (clamped at zero) along `axis`.
 */
export function buildSizeTransition(options: {
  axis: Axis;
  sizeFactor: number;
  alignment?: AlignmentGeometry | null;
  fixedCrossAxisSizeFactor?: number | null;
  textDirection: TextDirection;
  child: RenderBox | null;
}): RenderClip {
  const vertical = options.axis === Axis.vertical;
  const factor = Math.max(options.sizeFactor, 0);
  const fixed = options.fixedCrossAxisSizeFactor ?? null;
  const align = new RenderPositionedBox({
    alignment: options.alignment ?? (vertical ? new AlignmentDirectional(-1, 0) : new AlignmentDirectional(0, -1)),
    textDirection: options.textDirection,
    heightFactor: vertical ? factor : fixed,
    widthFactor: vertical ? fixed : factor,
    child: options.child,
  });
  return new RenderClip({ shape: ClipShape.rect, child: align });
}

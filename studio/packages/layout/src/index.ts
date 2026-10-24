// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * `@plux/layout`: a faithful TypeScript port of Flutter's box layout
 * protocol (STU-005, ADR-0013), so the Studio canvas lays out widgets as
 * the device does.
 */

export { BoxConstraints, type Constraints } from "./flutter/constraints.ts";
export { ContainerBoxParentData, ContainerRenderBox } from "./flutter/container.ts";
export {
  Axis,
  CrossAxisAlignment,
  FlexFit,
  FlexParentData,
  MainAxisAlignment,
  MainAxisSize,
  RenderFlex,
  type RenderFlexOptions,
  VerticalDirection,
} from "./flutter/flex.ts";
export {
  Alignment,
  AlignmentDirectional,
  type AlignmentGeometry,
  clampDouble,
  EdgeInsets,
  EdgeInsetsDirectional,
  type EdgeInsetsGeometry,
  nearEqual,
  nearZero,
  Offset,
  precisionErrorTolerance,
  Rect,
  Size,
  TextDirection,
} from "./flutter/geometry.ts";
export { ParentData, PipelineOwner, RenderObject } from "./flutter/object.ts";
export { RenderParagraph, type RenderParagraphOptions, TextOverflow } from "./flutter/paragraph.ts";
export {
  DecorationPosition,
  RenderConstrainedBox,
  RenderDecoratedBox,
  RenderProxyBox,
} from "./flutter/proxy-box.ts";
export {
  BaselineOffset,
  BoxParentData,
  RenderBox,
  RenderBoxWithChild,
  TextBaseline,
} from "./flutter/render-box.ts";
export {
  RenderAligningShiftedBox,
  RenderPadding,
  RenderPositionedBox,
  RenderShiftedBox,
} from "./flutter/shifted-box.ts";
export {
  computePaintOffsetFraction,
  TextPainter,
  type TextPainterOptions,
  TextWidthBasis,
} from "./flutter/text-painter.ts";
export {
  buildTextRuns,
  getParagraphStyle,
  getTextStyle,
  kDefaultFontSize,
  type ParagraphStyleArgs,
  plainText,
  type StrutStyle,
  TextScaler,
  type TextSpan,
  type TextStyle,
} from "./flutter/text-style.ts";
export { RenderView } from "./flutter/view.ts";
export { globalRects, goldenRects, type LayoutGolden, type NodeRect } from "./node-rects.ts";
export { RenderTestBox, type RenderTestBoxOptions } from "./test-box.ts";
export { CanvasKitParagraphEngine } from "./text/canvaskit-engine.ts";
export {
  type EngineLineMetrics,
  type EngineParagraph,
  type EngineParagraphStyle,
  type EngineStrutStyle,
  type EngineTextRun,
  type EngineTextStyle,
  type FontFeature,
  FontStyle,
  type ParagraphEngine,
  TextAlign,
  type TextHeightBehavior,
  TextLeadingDistribution,
} from "./text/paragraph-engine.ts";

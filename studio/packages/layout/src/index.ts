// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * `@plux/layout`: a faithful TypeScript port of Flutter's box layout
 * protocol (STU-005, ADR-0013), so the Studio canvas lays out widgets as
 * the device does.
 */

export { BoxConstraints, type Constraints } from "./constraints.ts";
export { ContainerBoxParentData, ContainerRenderBox } from "./container.ts";
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
} from "./geometry.ts";
export { globalRects, goldenRects, type LayoutGolden, type NodeRect } from "./node-rects.ts";
export { ParentData, PipelineOwner, RenderObject } from "./object.ts";
export {
  DecorationPosition,
  RenderConstrainedBox,
  RenderDecoratedBox,
  RenderProxyBox,
} from "./proxy-box.ts";
export {
  BaselineOffset,
  BoxParentData,
  RenderBox,
  RenderBoxWithChild,
  TextBaseline,
} from "./render-box.ts";
export {
  RenderAligningShiftedBox,
  RenderPadding,
  RenderPositionedBox,
  RenderShiftedBox,
} from "./shifted-box.ts";
export { RenderTestBox, type RenderTestBoxOptions } from "./test-box.ts";
export { RenderView } from "./view.ts";

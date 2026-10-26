// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * `@plux/layout`: a faithful TypeScript port of Flutter's box layout
 * protocol (STU-005, ADR-0013), so the Studio canvas lays out widgets as
 * the device does.
 */

export {
  buildContainer,
  buildSizeTransition,
  type ContainerLayout,
  lerpAlignment,
  lerpBoxConstraints,
  lerpDouble,
  lerpEdgeInsets,
  lerpProperty,
} from "./animated.ts";
export { applyBoxFit, BoxFit, type FittedSizes } from "./flutter/box-fit.ts";
export { ChildList, type ChildListHost, type ContainerParentData } from "./flutter/child-list.ts";
export { BoxConstraints, type Constraints } from "./flutter/constraints.ts";
export { ContainerBoxParentData, ContainerRenderBox } from "./flutter/container.ts";
export * from "./flutter/cupertino.ts";
export * from "./flutter/cupertino-sliding.ts";
export * from "./flutter/cupertino-sliver-nav.ts";
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
export * from "./flutter/list-wheel.ts";
export { Affine, isPaintTransformer, type PaintTransformer } from "./flutter/matrix.ts";
export { ParentData, PipelineOwner, RenderObject } from "./flutter/object.ts";
export { RenderParagraph, type RenderParagraphOptions, TextOverflow } from "./flutter/paragraph.ts";
export {
  DecorationPosition,
  RenderConstrainedBox,
  RenderDecoratedBox,
  RenderProxyBox,
} from "./flutter/proxy-box.ts";
export {
  applyStep,
  ClipShape,
  RenderAspectRatio,
  RenderClip,
  RenderFittedBox,
  RenderFractionalTranslation,
  RenderIntrinsicHeight,
  RenderIntrinsicWidth,
  RenderLimitedBox,
  RenderOpacity,
  RenderTransform,
} from "./flutter/proxy-boxes.ts";
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
export { RenderBaseline, RenderFractionallySizedOverflowBox } from "./flutter/shifted-boxes.ts";
export {
  AxisDirection,
  applyGrowthDirectionToAxisDirection,
  applyGrowthDirectionToScrollDirection,
  axisDirectionIsReversed,
  axisDirectionToAxis,
  type ChildPaintOffsetProvider,
  FixedViewportOffset,
  flipAxis,
  flipAxisDirection,
  GrowthDirection,
  type ItemExtentBuilder,
  PageViewportOffset,
  providesChildPaintOffset,
  RenderSliver,
  RenderSliverSingleBoxAdapter,
  RenderSliverToBoxAdapter,
  ScrollDirection,
  SliverConstraints,
  type SliverConstraintsInit,
  SliverGeometry,
  type SliverGeometryInit,
  type SliverLayoutDimensions,
  SliverLogicalParentData,
  SliverPhysicalParentData,
  textDirectionToAxisDirection,
  type ViewportOffset,
} from "./flutter/sliver.ts";
export {
  RenderSliverFillRemaining,
  RenderSliverFillRemainingAndOverscroll,
  RenderSliverFillRemainingWithScrollable,
} from "./flutter/sliver-fill.ts";
export {
  RenderSliverGrid,
  type SliverGridDelegate,
  SliverGridDelegateWithFixedCrossAxisCount,
  SliverGridDelegateWithMaxCrossAxisExtent,
  SliverGridGeometry,
  type SliverGridLayout,
  SliverGridParentData,
  SliverGridRegularTileLayout,
  type SliverGridSpacing,
} from "./flutter/sliver-grid.ts";
export {
  PrebuiltChildManager,
  type RenderSliverBoxChildManager,
  RenderSliverFillViewport,
  RenderSliverFixedExtentBoxAdaptor,
  RenderSliverFixedExtentList,
  RenderSliverList,
  RenderSliverMultiBoxAdaptor,
  RenderSliverVariedExtentList,
  SliverMultiBoxAdaptorParentData,
} from "./flutter/sliver-list.ts";
export {
  RenderSliverEdgeInsetsPadding,
  RenderSliverFractionalPadding,
  RenderSliverPadding,
} from "./flutter/sliver-padding.ts";
export {
  RenderSliverFloatingPersistentHeader,
  RenderSliverFloatingPinnedPersistentHeader,
  RenderSliverPersistentHeader,
  type RenderSliverPersistentHeaderOptions,
  RenderSliverPinnedPersistentHeader,
  RenderSliverScrollingPersistentHeader,
} from "./flutter/sliver-persistent-header.ts";
export {
  RenderIndexedStack,
  type RenderIndexedStackOptions,
  RenderStack,
  type RenderStackOptions,
  StackFit,
  StackParentData,
} from "./flutter/stack.ts";
export {
  FixedColumnWidth,
  FlexColumnWidth,
  FractionColumnWidth,
  IntrinsicColumnWidth,
  MaxColumnWidth,
  MinColumnWidth,
  RenderTable,
  type RenderTableOptions,
  TableCellParentData,
  TableCellVerticalAlignment,
  TableColumnWidth,
} from "./flutter/table.ts";
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
export {
  CacheExtentStyle,
  defaultCacheExtent,
  RenderShrinkWrappingViewport,
  RenderViewport,
  RenderViewportBase,
  type RenderViewportBaseOptions,
  type RenderViewportOptions,
  ScrollCacheExtent,
  SliverLogicalContainerParentData,
  SliverPhysicalContainerParentData,
} from "./flutter/viewport.ts";
export {
  RenderWrap,
  type RenderWrapOptions,
  WrapAlignment,
  WrapCrossAlignment,
  WrapParentData,
} from "./flutter/wrap.ts";
export { globalRects, goldenBaselines, goldenRects, type LayoutGolden, type NodeRect } from "./node-rects.ts";
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
export {
  defaultScreenTheme,
  defaultWidgetBuilders,
  type LayoutScreenOptions,
  layoutScreen,
  type Placed,
  type PlacedKind,
  type ScreenLayout,
  type ScreenTheme,
  type WidgetBuilder,
  type WidgetContext,
  type WidgetNode,
} from "./widgets/screen.ts";

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Builds `@plux/layout` render trees from the layout conformance scenes in
 * `test/layout-conformance/scenes/` (STU-005, ADR-0013). The Flutter side
 * builds the same scenes from widgets; each node type here maps to the render
 * object its Flutter widget creates, and the node's `id` becomes the render
 * object's `debugId`, so [goldenRects] yields what Flutter's goldens hold.
 */

import * as layout from "../src/index.ts";
import {
  Alignment,
  AlignmentDirectional,
  type AlignmentGeometry,
  Axis,
  BoxConstraints,
  CrossAxisAlignment,
  EdgeInsets,
  EdgeInsetsDirectional,
  type EdgeInsetsGeometry,
  FlexFit,
  FlexParentData,
  type LayoutGolden,
  MainAxisAlignment,
  MainAxisSize,
  PipelineOwner,
  type RenderBox,
  RenderConstrainedBox,
  RenderDecoratedBox,
  RenderFlex,
  RenderPadding,
  RenderPositionedBox,
  RenderView,
  Size,
  TextDirection,
  VerticalDirection,
} from "../src/index.ts";

/** One node of a scene; `props` use Flutter's parameter names. */
export interface SceneNode {
  id: string;
  type: string;
  props: Record<string, unknown>;
  children?: SceneNode[];
}

/** A conformance scene, shared verbatim with the Flutter golden writer. */
export interface Scene {
  name: string;
  root: SceneNode;
}

/** Node types the box-protocol port reproduces today. */
export const boxTypes: ReadonlySet<string> = new Set([
  "SizedBox",
  "ConstrainedBox",
  "Padding",
  "Align",
  "Center",
  "DecoratedBox",
  "TestBox",
]);

/** Node types that need `RenderFlex`. */
export const flexTypes: ReadonlySet<string> = new Set(["Row", "Column", "Flex", "Expanded", "Flexible", "Spacer"]);

/** Whether `@plux/layout` exports `RenderFlex` yet. */
export function renderFlexAvailable(): boolean {
  return "RenderFlex" in layout;
}

/** The node types a scene may use to be compared; everything else awaits an engine feature. */
export function supportedTypes(): ReadonlySet<string> {
  return renderFlexAvailable() ? new Set([...boxTypes, ...flexTypes]) : boxTypes;
}

/** Every node type the scene uses. */
export function sceneTypes(node: SceneNode, into: Set<string> = new Set()): Set<string> {
  into.add(node.type);
  for (const child of node.children ?? []) {
    sceneTypes(child, into);
  }
  return into;
}

/** A scene number: JSON cannot hold infinity, so scenes spell it `"infinity"`. */
function num(value: unknown, what: string): number {
  if (value === "infinity") {
    return Number.POSITIVE_INFINITY;
  }
  if (typeof value !== "number") {
    throw new Error(`${what}: expected a number or "infinity", got ${JSON.stringify(value)}`);
  }
  return value;
}

/** A scene field by name (scene props are open-ended records). */
function field(record: Record<string, unknown>, name: string): unknown {
  return record[name];
}

function optionalNum(props: Record<string, unknown>, name: string): number | null {
  return name in props ? num(props[name], name) : null;
}

function padding(value: unknown): EdgeInsetsGeometry {
  if (Array.isArray(value)) {
    return EdgeInsets.fromLTRB(
      num(value[0], "left"),
      num(value[1], "top"),
      num(value[2], "right"),
      num(value[3], "bottom"),
    );
  }
  const sides = value as Record<string, unknown>;
  return EdgeInsetsDirectional.fromSTEB(
    num(field(sides, "start") ?? 0, "start"),
    num(field(sides, "top") ?? 0, "top"),
    num(field(sides, "end") ?? 0, "end"),
    num(field(sides, "bottom") ?? 0, "bottom"),
  );
}

function alignment(value: unknown): AlignmentGeometry {
  if (value === undefined) {
    return Alignment.center;
  }
  if (Array.isArray(value)) {
    return new Alignment(num(value[0], "x"), num(value[1], "y"));
  }
  const parts = value as Record<string, unknown>;
  return new AlignmentDirectional(num(field(parts, "start"), "start"), num(field(parts, "y"), "y"));
}

function singleChild(node: SceneNode, children: RenderBox[]): RenderBox | null {
  if (!["Row", "Column", "Flex"].includes(node.type) && children.length > 1) {
    throw new Error(`node "${node.id}" of type ${node.type} takes at most one child`);
  }
  return children[0] ?? null;
}

function unsupported(node: SceneNode): Error {
  if (flexTypes.has(node.type)) {
    return new Error(
      `node "${node.id}": ${node.type} needs RenderFlex; ` +
        (renderFlexAvailable() ? "map the flex types in conformance/scene.ts" : "it is not exported yet"),
    );
  }
  return new Error(`node "${node.id}": unknown scene node type "${node.type}"`);
}

/** Row, Column and Flex: a [RenderFlex] with Flutter's widget defaults. */
function buildFlex(node: SceneNode, children: RenderBox[], textDirection: TextDirection): RenderBox {
  const props = node.props;
  return new RenderFlex({
    children,
    direction:
      node.type === "Row"
        ? Axis.horizontal
        : node.type === "Column"
          ? Axis.vertical
          : ((field(props, "direction") ?? Axis.horizontal) as Axis),
    mainAxisAlignment: (field(props, "mainAxisAlignment") ?? MainAxisAlignment.start) as MainAxisAlignment,
    mainAxisSize: (field(props, "mainAxisSize") ?? MainAxisSize.max) as MainAxisSize,
    crossAxisAlignment: (field(props, "crossAxisAlignment") ?? CrossAxisAlignment.center) as CrossAxisAlignment,
    textDirection,
    verticalDirection: (field(props, "verticalDirection") ?? VerticalDirection.down) as VerticalDirection,
    spacing: optionalNum(props, "spacing") ?? 0,
  });
}

/** Expanded, Flexible and Spacer: flex parent data on the child's render object (Spacer: a shrunk box). */
function applyFlexible(node: SceneNode, child: RenderBox | null): RenderBox {
  const props = node.props;
  // Flexible has no render object: it sets flex parent data on its child.
  const flexible =
    node.type === "Spacer"
      ? new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ width: 0, height: 0 }) })
      : child;
  if (flexible === null) {
    throw new Error(`node "${node.id}" of type ${node.type} needs a child`);
  }
  const parentData = new FlexParentData();
  parentData.flex = optionalNum(props, "flex") ?? 1;
  parentData.fit = node.type === "Flexible" ? ((field(props, "fit") ?? FlexFit.loose) as FlexFit) : FlexFit.tight;
  flexible.parentData = parentData;
  if (node.type === "Spacer") {
    flexible.debugId = node.id;
  }
  return flexible;
}

/**
 * Builds the render tree of `node` under `textDirection` (Flutter's ambient
 * `Directionality`). Throws on an unknown type and on a flex type, which
 * needs `RenderFlex`.
 */
export function buildNode(node: SceneNode, textDirection: TextDirection): RenderBox {
  const children = (node.children ?? []).map((child) => buildNode(child, textDirection));
  const child = singleChild(node, children);
  const props = node.props;
  let box: RenderBox;
  switch (node.type) {
    case "Row":
    case "Column":
    case "Flex":
      box = buildFlex(node, children, textDirection);
      break;
    case "Expanded":
    case "Flexible":
    case "Spacer":
      return applyFlexible(node, child);
    case "TestBox":
      // Flutter's side is a childless SizedBox, i.e. a RenderConstrainedBox.
      box = new RenderConstrainedBox({
        additionalConstraints: BoxConstraints.tightFor({
          width: num(field(props, "width"), "width"),
          height: num(field(props, "height"), "height"),
        }),
      });
      break;
    case "SizedBox":
      box = new RenderConstrainedBox({
        additionalConstraints: BoxConstraints.tightFor({
          width: optionalNum(props, "width"),
          height: optionalNum(props, "height"),
        }),
        child,
      });
      break;
    case "ConstrainedBox":
      box = new RenderConstrainedBox({
        additionalConstraints: new BoxConstraints({
          minWidth: optionalNum(props, "minWidth") ?? 0,
          maxWidth: optionalNum(props, "maxWidth") ?? Number.POSITIVE_INFINITY,
          minHeight: optionalNum(props, "minHeight") ?? 0,
          maxHeight: optionalNum(props, "maxHeight") ?? Number.POSITIVE_INFINITY,
        }),
        child,
      });
      break;
    case "Padding":
      box = new RenderPadding({ padding: padding(field(props, "padding")), textDirection, child });
      break;
    case "Align":
    case "Center":
      box = new RenderPositionedBox({
        alignment: node.type === "Center" ? Alignment.center : alignment(field(props, "alignment")),
        textDirection,
        widthFactor: optionalNum(props, "widthFactor"),
        heightFactor: optionalNum(props, "heightFactor"),
        child,
      });
      break;
    case "DecoratedBox":
      box = new RenderDecoratedBox({ decoration: { color: field(props, "color") }, child });
      break;
    default:
      throw unsupported(node);
  }
  box.debugId = node.id;
  return box;
}

/** Lays `scene` out in the golden's frame and direction, as the Flutter writer did. */
export function layOutScene(scene: Scene, golden: LayoutGolden): RenderView {
  const direction = golden.locale === "ar" ? TextDirection.rtl : TextDirection.ltr;
  const owner = new PipelineOwner();
  const view = new RenderView({
    size: new Size(golden.frame.width, golden.frame.height),
    child: buildNode(scene.root, direction),
  });
  owner.rootNode = view;
  view.prepareInitialFrame();
  owner.flushLayout();
  return view;
}

/**
 * The rects of a laid-out `scene` as Flutter's golden writer records them.
 * `Expanded` and `Flexible` are parent-data widgets without a render object of
 * their own, so Flutter reports their id with the rect of their child.
 */
export function sceneGoldenRects(scene: Scene, view: RenderView): ReturnType<typeof layout.goldenRects> {
  const rects = layout.goldenRects(view);
  const alias = (node: SceneNode): void => {
    const child = node.children?.[0];
    if ((node.type === "Expanded" || node.type === "Flexible") && child !== undefined) {
      const rect = rects[child.id];
      if (rect !== undefined) {
        rects[node.id] = rect;
      }
    }
    for (const c of node.children ?? []) {
      alias(c);
    }
  };
  alias(scene.root);
  return rects;
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { type WidgetDescriptor, widget, widgets } from "@plux/schema";
import type { IconName } from "@plux/ui";
import { type IdSource, uuidv7 } from "../plugin-canvas/screens.ts";

/** The Insert tab's groups: Layer 1 widgets, Layer 2 components, native slots and custom widgets (WGT-030). */
export type InsertGroup = "layer1" | "layer2" | "native";

export const insertGroups: readonly InsertGroup[] = ["layer1", "layer2", "native"];

export function groupOf(d: WidgetDescriptor): InsertGroup {
  return d.layer === 1 ? "layer1" : d.layer === 2 ? "layer2" : "native";
}

/** The descriptors to offer: the built-in registry plus the app's native and custom descriptors, deduplicated by type. */
export function catalogue(extra: readonly WidgetDescriptor[] = []): readonly WidgetDescriptor[] {
  const seen = new Set<string>();
  return [...widgets, ...extra].filter((d) => d.deprecated === undefined && !seen.has(d.type) && seen.add(d.type));
}

/** Case-insensitive match on type, category and description; empty query matches everything. */
export function search(list: readonly WidgetDescriptor[], query: string): readonly WidgetDescriptor[] {
  const q = query.trim().toLowerCase();
  return q === ""
    ? list
    : list.filter((d) => [d.type, d.category, d.description].some((s) => s.toLowerCase().includes(q)));
}

const zero: Readonly<Record<string, string | number | boolean>> = {
  string: "",
  int: 0,
  number: 0,
  double: 0,
  bool: false,
};

/** A new node from the descriptor: a fresh UUIDv7 id, the descriptor's defaults, and a zero for required primitives. */
export function buildNode(
  d: WidgetDescriptor,
  newId: IdSource = uuidv7,
): { id: string; type: string; props?: Record<string, unknown> } {
  const props: Record<string, unknown> = {};
  for (const p of d.props) {
    if (p.default !== undefined) {
      props[p.name] = p.default;
    } else if (p.required && zero[p.type] !== undefined) {
      props[p.name] = zero[p.type];
    }
  }
  return Object.keys(props).length === 0 ? { id: newId(), type: d.type } : { id: newId(), type: d.type, props };
}

/** The kinds the Insert gallery sorts widgets into, from their registry category, each with its icon. */
export const widgetKinds = [
  "layout",
  "content",
  "input",
  "interaction",
  "navigation",
  "motion",
  "platform",
  "more",
] as const;

export type WidgetKind = (typeof widgetKinds)[number];

const kindOfCategory: Readonly<Record<string, WidgetKind>> = {
  layout: "layout",
  structure: "layout",
  scrolling: "layout",
  display: "content",
  input: "input",
  forms: "input",
  control: "input",
  interaction: "interaction",
  feedback: "interaction",
  navigation: "navigation",
  animation: "motion",
  cupertino: "platform",
  device: "platform",
  host: "platform",
};

/** The gallery kind of a widget; widgets of a category without one are under "more". */
export function kindOf(d: WidgetDescriptor): WidgetKind {
  return kindOfCategory[d.category] ?? "more";
}

/** The icon of a gallery kind. */
export const kindIcon = {
  layout: "grid-four",
  content: "text-t",
  input: "cursor-text",
  interaction: "hand-pointing",
  navigation: "navigation-arrow",
  motion: "sparkle",
  platform: "device-mobile",
  more: "shapes",
} as const satisfies Record<WidgetKind, string>;

/** Icons of the widgets a layer tree shows most; any other widget takes its kind's icon. */
const typeIcons: Readonly<Record<string, IconName>> = {
  Text: "text-t",
  RichText: "text-t",
  Image: "image",
  Icon: "sparkle",
  Column: "rows",
  Row: "columns",
  Stack: "stack",
  ListView: "list",
  GridView: "grid-four",
  Scaffold: "frame",
  AppBar: "navigation-arrow",
  Container: "square-dashed",
  Padding: "square-dashed",
  SizedBox: "square-dashed",
  Card: "squares-four",
  TextField: "cursor-text",
  Switch: "toggle-right",
  Checkbox: "check-circle",
  Radio: "check-circle",
  Slider: "sliders",
  Align: "crosshair",
  Center: "crosshair",
  AspectRatio: "frame",
  ClipRRect: "frame",
  ClipOval: "frame",
  DecoratedBox: "palette",
  ColoredBox: "palette",
  FittedBox: "fit-view",
  Expanded: "arrows-up-down",
  Flexible: "arrows-up-down",
  Flex: "columns",
  Wrap: "rows",
  IndexedStack: "stack",
  ConstrainedBox: "square-dashed",
  LimitedBox: "square-dashed",
  FractionallySizedBox: "square-dashed",
  Baseline: "minus",
  Divider: "minus",
  VerticalDivider: "minus",
  Spacer: "minus",
  BottomAppBar: "rows",
  ForEach: "list",
  If: "flow-arrow",
  DataScope: "database",
  FormScope: "list-checks",
  CustomScrollView: "arrows-up-down",
  SingleChildScrollView: "arrows-up-down",
  ListTile: "list",
  DataTable: "table",
  Table: "table",
  Tooltip: "info",
  CircleAvatar: "user",
  FilterChip: "funnel",
  InputChip: "cursor-text",
  ExpansionTile: "caret-down",
  MaterialBanner: "bell",
  SelectableText: "text-t",
  CircularProgressIndicator: "refresh",
  LinearProgressIndicator: "pulse",
  SafeArea: "shield",
  GestureDetector: "hand-pointing",
  InkWell: "hand-pointing",
  Opacity: "eye",
  Visibility: "eye",
  PageView: "columns",
  TabBar: "columns",
  NavigationBar: "navigation-arrow",
  Drawer: "list",
  Hero: "sparkle",
};

/** The icon of a widget type in the layer tree. */
export function typeIcon(type: string): IconName {
  const own = typeIcons[type];
  if (own !== undefined) {
    return own;
  }
  if (/Button$/.test(type)) {
    return "hand-pointing";
  }
  const d = widget(type);
  return d === undefined ? "cube" : kindIcon[kindOf(d)];
}

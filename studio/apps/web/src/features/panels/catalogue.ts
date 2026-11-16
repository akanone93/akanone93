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

/**
 * Where zero is no start for a required number: an opacity or a scale of 0 hides the widget, an aspect ratio or an
 * item extent of 0 is invalid, a header 0 high shows nothing. These start at the value a person expects.
 */
const firstValue: Readonly<Record<string, number>> = {
  opacity: 1,
  scale: 1,
  aspectRatio: 1,
  itemExtent: 32,
  minExtent: 56,
  maxExtent: 120,
};

/** What a new widget shows, in Studio's language: a text, a button's label, a chip's, a list item's, a title. */
export interface StarterTexts {
  readonly text: string;
  readonly button: string;
  readonly chip: string;
  readonly item: string;
  readonly title: string;
  readonly add: string;
  /** The first and second entry of a new list: a tab, a destination, a menu item, a segment, a column. */
  readonly first: string;
  readonly second: string;
}

interface NewNode {
  readonly id: string;
  readonly type: string;
  readonly props?: Record<string, unknown>;
  readonly slots?: Record<string, NewNode | readonly NewNode[]>;
}

const text = (data: string, ids: IdSource): NewNode => ({ id: ids(), type: "Text", props: { data } });
const label = (slot: string, of: keyof StarterTexts) => (t: StarterTexts, ids: IdSource) => ({
  [slot]: text(t[of], ids),
});

/**
 * The content a new widget starts with, so it shows on the canvas at once instead of as an empty box: a button reads
 * "Button", a chip "Chip", a list tile its item, an app bar its title, a FAB its icon and label, a Dismissible a tile
 * to swipe. Widgets that wrap content the person brings (a Card, a Container) start empty.
 */
const starters: Readonly<
  Record<string, (t: StarterTexts, ids: IdSource) => Record<string, NewNode | readonly NewNode[]>>
> = {
  ElevatedButton: label("child", "button"),
  FilledButton: label("child", "button"),
  OutlinedButton: label("child", "button"),
  TextButton: label("child", "button"),
  Chip: label("label", "chip"),
  ActionChip: label("label", "chip"),
  ChoiceChip: label("label", "chip"),
  FilterChip: label("label", "chip"),
  InputChip: label("label", "chip"),
  ListTile: label("title", "item"),
  CheckboxListTile: label("title", "item"),
  RadioListTile: label("title", "item"),
  SwitchListTile: label("title", "item"),
  ExpansionTile: label("title", "title"),
  AppBar: label("title", "title"),
  FloatingActionButton: (t, ids) => ({
    icon: { id: ids(), type: "Icon", props: { icon: { name: "add" } } },
    label: text(t.add, ids),
  }),
  Dismissible: (t, ids) => ({ child: { id: ids(), type: "ListTile", slots: { title: text(t.item, ids) } } }),
  MaterialBanner: (t, ids) => ({
    content: text(t.text, ids),
    actions: [{ id: ids(), type: "TextButton", slots: { child: text(t.button, ids) } }],
  }),
  CupertinoSlidingSegmentedControl: (t, ids) => ({ segments: [text(t.first, ids), text(t.second, ids)] }),
};

/** Two entries of a list a widget needs at least one of, labelled and, where it takes one, with an icon. */
function twoEntries(type: string, t: StarterTexts): unknown[] | undefined {
  const labels = [t.first, t.second];
  const icons = ["looks_one", "looks_two"];
  switch (type) {
    case "list<Destination>":
    case "list<TabBarItem>":
      return labels.map((label, i) => ({ label, icon: { name: icons[i] } }));
    case "list<TabItem>":
      return labels.map((label) => ({ label }));
    case "list<MenuItem>":
    case "list<DropdownMenuEntry>":
    case "list<ButtonSegment>":
      return labels.map((label, i) => ({ value: `option${i + 1}`, label }));
    case "list<DataColumn>":
      return labels.map((label) => ({ label }));
    case "list<StepItem>":
      return labels.map((title) => ({ title }));
    case "list<ChoiceItem>":
      return labels.map((label, i) => ({ id: `option${i + 1}`, label }));
    case "list<string>":
      return labels;
    default:
      return undefined;
  }
}

/** A required date by its role: a range runs from 2020 to 2030, and the first shown date is in it. */
const dateOf = (name: string): string =>
  name.startsWith("first") ? "2020-01-01" : name.startsWith("last") ? "2030-12-31" : "2026-01-01";

/**
 * A valid first value for a required prop that has no default, by its type, so a widget is valid the moment it is
 * inserted (#71): an alignment in the centre, padding of 8, a 300 ms duration, two entries of a list of tabs or
 * destinations. Undefined where only the person can choose (an image's source, a chart's series).
 */
function firstOfType(name: string, type: string, t: StarterTexts | undefined): unknown {
  const fixed: Readonly<Record<string, unknown>> = {
    Alignment: "center",
    Axis: "vertical",
    CrossFadeState: "showFirst",
    TextBaseline: "alphabetic",
    EdgeInsets: { all: 8 },
    Offset: { dx: 0, dy: 0 },
    BoxConstraints: { minHeight: 0 },
    BoxDecoration: { color: "#E2E8F0" },
    RangeValues: { start: 0.25, end: 0.75 },
    color: "#E2E8F0",
    duration: 300,
  };
  if (type in fixed) {
    return fixed[type];
  }
  if (type === "date") {
    return dateOf(name);
  }
  if (t === undefined) {
    return undefined;
  }
  if (type === "TextSpan") {
    return { text: t.text };
  }
  // A list's items: two texts, so the item type is known and the template shows each.
  if (type === "list<T>") {
    return [t.first, t.second];
  }
  if (name === "selected" && type === "list<string>") {
    return ["option1"];
  }
  if (name === "rows" && type === "list<list<string>>") {
    return [[t.first, t.second]];
  }
  return twoEntries(type, t);
}

/**
 * Content for the required slots a widget would otherwise be invalid without: a list's item template is a list tile,
 * any other required single slot (a wrapper's child, a branch) a text. List slots stay empty: a person adds to them.
 */
function requiredSlots(d: WidgetDescriptor, t: StarterTexts, ids: IdSource): Record<string, NewNode> {
  const out: Record<string, NewNode> = {};
  for (const s of d.slots) {
    if (!s.required || s.list) {
      continue;
    }
    out[s.name] =
      s.name === "item"
        ? // Each item of the list, shown as a list tile titled with it.
          {
            id: ids(),
            type: "ListTile",
            slots: { title: { id: ids(), type: "Text", props: { data: { $expr: "item" } } } },
          }
        : text(t.text, ids);
  }
  return out;
}

/** A prop's first value: its default, a first value for a required primitive or type, or nothing. */
function firstProp(p: WidgetDescriptor["props"][number], starter: StarterTexts | undefined): unknown {
  if (p.default !== undefined) {
    return p.default;
  }
  if (!p.required) {
    return undefined;
  }
  const none = zero[p.type];
  if (none === undefined) {
    return firstOfType(p.name, p.type, starter);
  }
  return typeof none === "number" ? (firstValue[p.name] ?? none) : none;
}

/** A new node's props: the descriptor's defaults, and a first value for each required prop it can have one for. */
function startProps(d: WidgetDescriptor, starter: StarterTexts | undefined): Record<string, unknown> {
  const props: Record<string, unknown> = {};
  for (const p of d.props) {
    const first = firstProp(p, starter);
    if (first !== undefined) {
      props[p.name] = first;
    }
  }
  // A text starts with a word to show, not an empty string that draws nothing.
  if (starter !== undefined && (d.type === "Text" || d.type === "SelectableText") && props["data"] === "") {
    props["data"] = starter.text;
  }
  return props;
}

/**
 * A new node from the descriptor: a fresh UUIDv7 id, the descriptor's defaults, a first value for required primitives,
 * and, given `starter` texts, the content it starts with.
 */
export function buildNode(d: WidgetDescriptor, newId: IdSource = uuidv7, starter?: StarterTexts): NewNode {
  const props = startProps(d, starter);
  const id = newId();
  const make = starter === undefined ? undefined : starters[d.type];
  const own = make === undefined || starter === undefined ? undefined : make(starter, newId);
  const slots = starter === undefined ? own : { ...requiredSlots(d, starter, newId), ...own };
  return {
    id,
    type: d.type,
    ...(Object.keys(props).length > 0 ? { props } : {}),
    ...(slots === undefined || Object.keys(slots).length === 0 ? {} : { slots }),
  };
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

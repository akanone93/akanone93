// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { IconName } from "@plux/ui";

/** The inspector's sections of properties (STU-062), and which props are a widget's style (PC-21). */
export type PropGroup = "layout" | "appearance" | "content" | "behaviour" | "other";

export const groupOrder: readonly PropGroup[] = ["content", "layout", "appearance", "behaviour", "other"];

export const groupIcons: Record<PropGroup, IconName> = {
  layout: "grid-four",
  appearance: "palette",
  content: "text-t",
  behaviour: "hand-pointing",
  other: "sliders",
};

const groupPatterns: readonly (readonly [PropGroup, RegExp])[] = [
  [
    "layout",
    /^(width|height|size|padding|margin|align|alignment|mainAxis|crossAxis|spacing|runSpacing|gap|flex|fit|aspectRatio|constraints|position|top|bottom|left|right|start|end|direction|axis|clip|expand|scroll|physics|itemExtent|shrinkWrap|reverse|verticalDirection|textDirection)/i,
  ],
  [
    "appearance",
    /(color|colour|background|decoration|border|radius|elevation|shadow|style|theme|opacity|gradient|font|weight|outline|shape|tint|overlay|splash|highlight|filter|blend)/i,
  ],
  [
    "content",
    /^(data|text|label|title|subtitle|child|children|icon|image|src|url|value|placeholder|hint|items|leading|trailing|content|semanticsLabel|tooltip|message|actions|body|header|footer)/i,
  ],
  [
    "behaviour",
    /^(on[A-Z]|enabled|disabled|readOnly|autofocus|keyboard|obscure|max|min|divisions|validator|autovalidate|selected|checked|tristate|duration|curve|delay|repeat|loop|interactive|controller)/i,
  ],
];

/** The group of a property: the first whose pattern its name matches, else "other". */
export function propGroupOf(name: string): PropGroup {
  return groupPatterns.find(([, pattern]) => pattern.test(name))?.[0] ?? "other";
}

/** Whether a property is part of a widget's style (its appearance), which style copy and paste carries (PC-21). */
export const isStyleProp = (name: string): boolean => propGroupOf(name) === "appearance";

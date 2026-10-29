// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { cases as button } from "./button.cases.tsx";
import type { CaseDefinition } from "./case.ts";
import { cases as count } from "./count.cases.tsx";
import { cases as dialog } from "./dialog.cases.tsx";
import { cases as field } from "./field.cases.tsx";
import { cases as glass } from "./glass.cases.tsx";
import { cases as icon } from "./icon.cases.tsx";
import { cases as liveChip } from "./live-chip.cases.tsx";
import { cases as menu } from "./menu.cases.tsx";
import { cases as meter } from "./meter.cases.tsx";
import { cases as metricStrip } from "./metric-strip.cases.tsx";
import { cases as popover } from "./popover.cases.tsx";
import { cases as select } from "./select.cases.tsx";
import { cases as sparkline } from "./sparkline.cases.tsx";
import { cases as statusPill } from "./status-pill.cases.tsx";
import { cases as summaryCard } from "./summary-card.cases.tsx";
import { cases as tabs } from "./tabs.cases.tsx";
import { cases as toast } from "./toast.cases.tsx";
import { cases as tooltip } from "./tooltip.cases.tsx";
import { cases as tree } from "./tree.cases.tsx";
import { cases as typeTag } from "./type-tag.cases.tsx";
import { cases as virtualTable } from "./virtual-table.cases.tsx";

/** Every component's cases by name, for the development-only catalogue page. */
export const allCases: Readonly<Record<string, readonly CaseDefinition[]>> = {
  AlertDialog: dialog.filter((c) => c.name === "alert dialog"),
  Button: button,
  Dialog: dialog.filter((c) => c.name !== "alert dialog"),
  Field: field,
  Count: count,
  GlassCapsule: glass.filter((c) => c.name === "capsule"),
  FloatingPanel: glass.filter((c) => c.name !== "capsule"),
  LiveChip: liveChip,
  Meter: meter,
  MetricStrip: metricStrip,
  Sparkline: sparkline,
  StatusPill: statusPill,
  SummaryCard: summaryCard,
  Tree: tree,
  TypeTag: typeTag,
  VirtualTable: virtualTable,
  Icon: icon,
  Menu: menu,
  Popover: popover,
  Select: select,
  Tabs: tabs,
  Toast: toast,
  Tooltip: tooltip,
};

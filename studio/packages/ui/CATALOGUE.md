<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: AGPL-3.0-only
-->

# @plux/ui catalogue

Every shared component, one entry each (studio-development 5.2). Search here before writing a component. The
components below keep Studio's props and render shadcn/ui's components (ADR-0103); shadcn's own components are in
`src/components/` and are imported as `@plux/ui/components/<name>` (avatar, alert, alert-dialog, badge, breadcrumb,
button, card, checkbox, collapsible, dialog, dropdown-menu, empty, field, hover-card, input, input-group, item, kbd,
label, popover, progress, scroll-area, select, separator, sheet, sidebar, skeleton, spinner, switch, table, tabs,
textarea, toggle, toggle-group, tooltip). Every entry has a `<name>.cases.tsx` with its states; the
development-only catalogue page renders `allCases`. Built-in text (Close, Cancel, loading labels) comes in through
props from the message catalogue.

## Icon

- **Purpose:** the one icon set of Studio's chrome: lucide, shadcn's icon library, by Studio's icon names (ADR-0103).
- **Props:** `name: IconName`, `size?: "pill" | "tab" | "capsule"` (default `tab`), `label?: string`, `className?`,
  `data-icon?: "inline-start" | "inline-end"` (shadcn's placement inside a button or badge).
- **States:** decorative (`aria-hidden`, no `label`), labelled (`role="img"`), mirrored under `dir="rtl"`.
- **Tokens:** `currentColor`; sizes `size-icon-sm|md|lg` (12, 14, 17 px) unless a shadcn component sizes it.
- **Mirrors:** `mirrors: true` for `arrow-bend-up-left`, `arrow-clockwise`, `arrow-counter-clockwise`, `arrow-left`,
  `arrow-right`, `arrow-up-right`, `caret-left`, `caret-right`, `paper-plane-tilt`, `sign-out` (kept in
  `src/icon-mirrors.ts`); every other icon is `mirrors: false`.
- **Icons:** the keys of `icons` in `src/icons.ts`.
- **Add an icon:** add a row to `icons` in `src/icons.ts` with its lucide icon, and classify it in `icon-mirrors.ts`.
- **Do not use for:** the apps' own icons (Material Symbols and Cupertino, embedded by the server).

## Button

- **Purpose:** every action, on shadcn's Button: `primary` → `default`, `secondary` → `outline`, `ghost`, `danger` → `destructive`; sizes `sm`, `md` (shadcn `default`), `lg`.
- **Props:** Base UI Button props plus `variant: "primary" | "secondary" | "ghost" | "danger"`, `size`, `loading`,
  `loadingLabel`.
- **States:** rest, hover, focus-visible, disabled, loading (`aria-busy`, stays focusable), long text.
- **Keyboard:** Enter and Space.
- **Tokens:** shadcn's semantic variables (ADR-0103).
- **Do not use for:** navigation (use a link); `danger` only inside confirm dialogs.

## Field, Input, Textarea

- **Purpose:** a labelled control with description and error wired by `aria-describedby` and `aria-invalid`: shadcn's Field, FieldLabel, FieldDescription and FieldError over Base UI Field; `Input` is shadcn's Input, or its InputGroup with a leading icon or trailing control.
- **Props:** `Field { label, description?, error?, children }`; `Input` and `Textarea` take native props.
- **States:** rest, hover, focus, invalid, disabled, long label.
- **Tokens:** shadcn's semantic variables (ADR-0103).

## Select

- **Purpose:** single choice from a short list, on shadcn's Select; not modal and opened below the trigger, so the page keeps its scroll bar.
- **Props:** `options`, `value?`, `defaultValue?`, `onValueChange?`, `label?`, `placeholder?`, `disabled?`.
- **States:** placeholder, selected, open, disabled, long option.
- **Keyboard:** Arrow keys, Home, End, type-ahead, Escape (Base UI).
- **Mirrors:** `caret-down` mirrors: false, `check` mirrors: false.
- **Do not use for:** more than about 15 options (use a combobox).

## Dialog, AlertDialog

- **Purpose:** on shadcn's Dialog and AlertDialog: modal task (`Dialog`, sizes `sm`, `md`, `lg`; `full` is the full-screen overlay of the heavy editors, STU-063) and destructive confirmation (`AlertDialog`).
- **Props:** `title`, `description?`, `labels { close }` / `labels { cancel, confirm }`, `trigger?`, Base UI root props
  (`open`, `defaultOpen`, `onOpenChange`), `onConfirm` for the alert dialog.
- **States:** closed, open, long title; alert dialog is not dismissed by an outside press.
- **Keyboard:** focus trapped and returned, Escape closes (not for the alert dialog's outside press).
- **Tokens:** shadcn's semantic variables (ADR-0103).
- **Mirrors:** `x` mirrors: false.

## Popover

- **Purpose:** small anchored panel with an optional title, on shadcn's Popover.
- **Props:** `trigger`, `title?`, Base UI root props.
- **States:** closed, open. **Tokens:** shadcn's semantic variables (ADR-0103).
- **Do not use for:** actions lists (use Menu).

## Menu

- **Purpose:** list of actions behind a trigger, on shadcn's DropdownMenu; not modal.
- **Props:** `trigger`, `entries: MenuEntry[]` (item with `danger`, `disabled`; separator), Base UI root props.
- **States:** closed, open, disabled item, danger item.
- **Keyboard:** Arrow keys follow reading direction, Home, End, type-ahead, Escape.
- **Tokens:** shadcn's semantic variables (ADR-0103).

## Tabs

- **Purpose:** tabs on shadcn's Tabs.
- **Props:** `tabs: { value, label, panel }[]`, Base UI root props.
- **States:** each tab selected, focus-visible, long label.
- **Keyboard:** Arrow keys follow reading direction (8.4), Home, End.

## Tooltip

- **Purpose:** caption for an icon-only control on shadcn's Tooltip; opens after 400 ms (`tooltipDelayMs`), also on keyboard focus.
- **Props:** `content`, `children` (one focusable element).
- **Tokens:** shadcn's semantic variables (ADR-0103).
- **Do not use for:** essential information (touch users never see it).

## Toast

- **Purpose:** transient result messages on sonner, on shadcn's popover surface; 5 s, errors stay (`notify`).
- **Props:** `Toaster { direction, regionLabel }`, `notify(kind, message)`.
- **States:** success, info, error. **Tokens:** shadcn's semantic variables (ADR-0103).

## StatusPill

- **Purpose:** health under a screen, release health, test results, on shadcn's Badge; every status has an icon and a word.
- **Props:** `status: "success" | "warning" | "danger" | "neutral"`, `label`, `value?`.
- **States:** the four statuses, with and without a value.
- **Tokens:** shadcn's semantic variables (ADR-0103).
- **Do not use for:** a bare colour dot; an icon without a word.

## Count

- **Purpose:** a count on shadcn's Badge for tabs, layer rows and notifications.
- **Props:** `value`, `tone?: "neutral" | "warning"`, `label?` (accessible name).
- **States:** neutral, warning, large number.
- **Tokens:** shadcn's semantic variables (ADR-0103).

## TypeTag

- **Purpose:** node type in the layer tree and search results (U10); mono, tertiary, fixed column.
- **Props:** `children`.
- **States:** short, long (truncated).
- **Tokens:** shadcn's semantic variables (ADR-0103).

## MetricStrip

- **Purpose:** one row at the foot of a panel: the label muted, the value in tabular figures.
- **Props:** `metrics: { label, value }[]`; every value carries its unit.
- **States:** three metrics, wrapping.
- **Tokens:** shadcn's semantic variables (ADR-0103).

## Meter

- **Purpose:** budget meter in shadcn's progress style; `layout="bar"` fills its container.
- **Props:** `value`, `max`, `label`, `valueText`.
- **States:** half, empty, over budget (clamped). Role `meter` with `aria-valuenow` and `aria-valuetext`.
- **Tokens:** shadcn's semantic variables (ADR-0103).

## Sparkline

- **Purpose:** recent values as a violet line (U10); SVG elements only.
- **Props:** `values: number[]`, `label` (label and latest value in text).
- **States:** series, flat, empty. Mirrored under `dir="rtl"`.
- **Tokens:** shadcn's semantic variables (ADR-0103).

## LiveChip

- **Purpose:** paired device or live stream on shadcn's Badge; its pulsing dot stops under reduced motion.
- **Props:** `label` (device, state and latency, composed by the caller).
- **States:** live. Role `status`.
- **Tokens:** shadcn's semantic variables (ADR-0103).

## SummaryCard

- **Purpose:** three equal cells, value over caption, on shadcn's card surface, for panel footers.
- **Props:** `cells: [SummaryCell, SummaryCell, SummaryCell]`.
- **States:** screens, links, ghost.
- **Tokens:** `bg-card`, `rounded-lg`, `--density-card-padding-x`, `--density-card-padding-y`.

## GlassCapsule, FloatingPanel

- **Purpose:** the canvas's floating bars and panels on shadcn's popover surface.
- **Props:** `label`, `children`, `className?`.
- **States:** capsule (role `group`), panel (region). Solid fallback under reduced transparency and the contrast theme.
- **Do not use for:** canvas items, inputs, dialogs, anything inside another floating surface.
- **Tokens:** shadcn's semantic variables (ADR-0103).

## VirtualTable

- **Purpose:** tables of any size; only the visible rows are in the DOM (STU-120, U11 "Table"). TanStack Table 9 and Virtual.
- **Props:** `data`, `columns` (TanStack column definitions), `label`, `rowHeight` (the `density.tableRow` value), `visibleRows`, `overscan?`.
- **States:** 10,000 rows. Roles `table`, `row`, `columnheader`, `cell`; `aria-rowcount` and `aria-rowindex` give the full size.
- **Keyboard:** the scrolling body is focusable and scrolls with the arrow keys.
- **Tokens:** shadcn's semantic variables (ADR-0103).

## Tree

- **Purpose:** the layer tree (U11 "Tree row") on Headless Tree.
- **Props:** `nodes`, `rootId`, `label`, `dir?: "ltr" | "rtl"`.
- **States:** collapsed, expanded, selected, focused; LTR and RTL.
- **Keyboard:** WAI-ARIA tree. Down and Up move, Right expands (Left in RTL), Left collapses or moves to the parent (Right in RTL), Home and End.
- **Mirrors:** `caret-right` mirrors: true, `caret-down` mirrors: false.
- **Tokens:** shadcn's semantic variables (ADR-0103).

## Catalogue page

- **Purpose:** development-only route `/__catalogue` in `apps/web` that renders every `*.cases.tsx` (found with `import.meta.glob`). Absent from production builds.
- **Query:** `theme=light|dark|contrast`, `dir=rtl`, `lang=`, `only=<component>`. The dialog, menu and popover cases open an overlay on mount, so they render alone with `only`.
- **Checks:** `studio/e2e/catalogue.spec.ts` runs axe in every theme and in `ar-XB` and tests the Tooltip.

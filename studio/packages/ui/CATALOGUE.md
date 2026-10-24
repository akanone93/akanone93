<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: AGPL-3.0-only
-->

# @plux/ui catalogue

Every shared component, one entry each (studio-development 5.2). Search here before writing a component. Every
entry has a `<name>.cases.tsx` with its states; the development-only catalogue page renders `allCases`. All
styling uses token-backed Tailwind utilities and logical properties only; built-in text (Close, Cancel, loading
labels) comes in through props from the message catalogue.

## Icon

- **Purpose:** the one icon set of Studio's chrome, Phosphor duotone with the paths inlined (UI language U8).
- **Props:** `name: IconName`, `size?: "pill" | "tab" | "capsule"` (default `tab`), `label?: string`, `className?`.
- **States:** decorative (`aria-hidden`, no `label`), labelled (`role="img"`), mirrored under `dir="rtl"`.
- **Tokens:** `currentColor`; sizes `size-3`, `size-4`, `size-5` (spacing tokens).
- **Mirrors:** `mirrors: true` for `arrow-bend-up-left`, `arrow-clockwise`, `arrow-counter-clockwise`, `arrow-left`,
  `arrow-right`, `caret-left`, `caret-right`, `paper-plane-tilt`, `sign-out` (kept in `src/icon-mirrors.ts`); every
  other icon is `mirrors: false`.
- **Icons (44):** arrow-bend-up-left, arrow-clockwise, arrow-counter-clockwise, arrow-left, arrow-right, caret-down,
  caret-left, caret-right, caret-up, check, check-circle, clock, command, copy, dots-three, eye, file, folder, funnel,
  gear, globe, house, info, key, list, lock, magnifying-glass, minus, moon, package, paper-plane-tilt, pencil-simple,
  play, plus, puzzle-piece, rocket-launch, sign-out, sun, trash, user, warning, warning-circle, x, x-circle.
- **Add an icon:** extend `iconNames` in `scripts/gen-icons.ts`, run `make studio-gen-icons`, classify it in
  `icon-mirrors.ts`.
- **Do not use for:** the apps' own icons (Material Symbols and Cupertino, embedded by the server).

## Button

- **Purpose:** every action. Pill, 36 px (`md`) or field height (`sm`) from the density tokens (U11).
- **Props:** Base UI Button props plus `variant: "primary" | "secondary" | "ghost" | "danger"`, `size`, `loading`,
  `loadingLabel`.
- **States:** rest, hover, focus-visible, disabled, loading (`aria-busy`, stays focusable), long text.
- **Keyboard:** Enter and Space.
- **Tokens:** `bg-accent`, `text-on-accent`, `shadow-primary`, `bg-hover`, `bg-danger-fill`, `outline-focus`.
- **Do not use for:** navigation (use a link); `danger` only inside confirm dialogs.

## Field, Input, Textarea

- **Purpose:** a labelled control with description and error wired by `aria-describedby` and `aria-invalid`.
- **Props:** `Field { label, description?, error?, children }`; `Input` and `Textarea` take native props.
- **States:** rest, hover, focus, invalid, disabled, long label.
- **Tokens:** `bg-raised`, `border-hairline`, `border-strong`, `text-danger`, `rounded-sm`, `--density-field`.

## Select

- **Purpose:** single choice from a short list (U11 "Input, select").
- **Props:** `options`, `value?`, `defaultValue?`, `onValueChange?`, `label?`, `placeholder?`, `disabled?`.
- **States:** placeholder, selected, open, disabled, long option.
- **Keyboard:** Arrow keys, Home, End, type-ahead, Escape (Base UI).
- **Mirrors:** `caret-down` mirrors: false, `check` mirrors: false.
- **Do not use for:** more than about 15 options (use a combobox).

## Dialog, AlertDialog

- **Purpose:** modal task (`Dialog`, sizes `sm`, `md`, `lg`) and destructive confirmation (`AlertDialog`).
- **Props:** `title`, `description?`, `labels { close }` / `labels { cancel, confirm }`, `trigger?`, Base UI root props
  (`open`, `defaultOpen`, `onOpenChange`), `onConfirm` for the alert dialog.
- **States:** closed, open, long title; alert dialog is not dismissed by an outside press.
- **Keyboard:** focus trapped and returned, Escape closes (not for the alert dialog's outside press).
- **Tokens:** `bg-raised`, `rounded-xl`, `shadow-e4`, `font-serif`. Widths approximate 400 / 560 / 800 px with the
  container scale until width tokens exist.
- **Mirrors:** `x` mirrors: false.

## Popover

- **Purpose:** small anchored panel with optional serif title.
- **Props:** `trigger`, `title?`, Base UI root props.
- **States:** closed, open. **Tokens:** `bg-glass`, `rounded-xl`, `shadow-e2`.
- **Do not use for:** actions lists (use Menu).

## Menu

- **Purpose:** list of actions behind a trigger.
- **Props:** `trigger`, `entries: MenuEntry[]` (item with `danger`, `disabled`; separator), Base UI root props.
- **States:** closed, open, disabled item, danger item.
- **Keyboard:** Arrow keys follow reading direction, Home, End, type-ahead, Escape.
- **Tokens:** `bg-glass`, `shadow-e2`, `data-highlighted:bg-hover`.

## Tabs

- **Purpose:** pill tabs (U11): track on `bg-hover`, selected tab a raised pill.
- **Props:** `tabs: { value, label, panel }[]`, Base UI root props.
- **States:** each tab selected, focus-visible, long label.
- **Keyboard:** Arrow keys follow reading direction (8.4), Home, End.

## Tooltip

- **Purpose:** caption for an icon-only control; opens after 400 ms (`tooltipDelayMs`), also on keyboard focus.
- **Props:** `content`, `children` (one focusable element).
- **Tokens:** `bg-brand-ink/94`, `text-on-accent`, `rounded-md`.
- **Do not use for:** essential information (touch users never see it).

## Toast

- **Purpose:** transient result messages on sonner, token styles; 5 s, errors stay (`notify`).
- **Props:** `Toaster { direction, regionLabel }`, `notify(kind, message)`.
- **States:** success, info, error. **Tokens:** `bg-glass`, `rounded-full`, `shadow-e2`, `text-danger`.

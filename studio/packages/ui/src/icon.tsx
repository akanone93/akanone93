// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ReactElement } from "react";
import { cx } from "./cx.ts";
import { iconMirrors } from "./icon-mirrors.ts";
import { type IconName, icons } from "./icons.ts";

/** U8 sizes: pill (inside pills), tab (tabs, fields, tables), capsule (capsules). */
export type IconSize = "pill" | "tab" | "capsule";

const sizeClass: Readonly<Record<IconSize, string>> = {
  pill: "size-icon-sm",
  tab: "size-icon-md",
  capsule: "size-icon-lg",
};

export interface IconProps {
  readonly name: IconName;
  readonly size?: IconSize;
  /** Accessible name from the message catalogue. Without it the icon is decorative and hidden from assistive technology. */
  readonly label?: string;
  readonly className?: string;
  /** shadcn's placement inside a button or badge, which spaces the icon from the text. */
  readonly "data-icon"?: "inline-start" | "inline-end";
}

/** A lucide icon by its Studio name (ADR-0103). Mirrors under RTL when the icon has a direction. */
export function Icon({ name, size = "tab", label, className, "data-icon": placement }: IconProps): ReactElement {
  const mirrors = iconMirrors(name);
  const labelled = label !== undefined && label !== "";
  const Glyph = icons[name];
  return (
    <Glyph
      focusable="false"
      data-icon-name={name}
      {...(placement !== undefined && { "data-icon": placement })}
      data-mirrors={mirrors}
      {...(labelled ? { role: "img", "aria-label": label } : { "aria-hidden": true })}
      className={cx("shrink-0", sizeClass[size], mirrors && "rtl:-scale-x-100", className)}
    />
  );
}

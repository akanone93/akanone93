// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ReactElement } from "react";
import { cx } from "./cx.ts";
import { iconMirrors } from "./icon-mirrors.ts";
import { type IconName, iconPaths } from "./icons.gen.ts";

/** U8 sizes: pill (inside pills), tab (tabs, fields, tables), capsule (capsules). */
export type IconSize = "pill" | "tab" | "capsule";

const sizeClass: Readonly<Record<IconSize, string>> = {
  pill: "size-3",
  tab: "size-4",
  capsule: "size-5",
};

export interface IconProps {
  readonly name: IconName;
  readonly size?: IconSize;
  /** Accessible name from the message catalogue. Without it the icon is decorative and hidden from assistive technology. */
  readonly label?: string;
  readonly className?: string;
}

/** Phosphor duotone icon with its paths inlined as data (U8). Mirrors under RTL when the icon has a direction. */
export function Icon({ name, size = "tab", label, className }: IconProps): ReactElement {
  const mirrors = iconMirrors(name);
  const labelled = label !== undefined && label !== "";
  return (
    // biome-ignore lint/a11y/noSvgWithoutTitle: the name is aria-label when labelled; otherwise the icon is aria-hidden
    <svg
      viewBox="0 0 256 256"
      fill="currentColor"
      focusable="false"
      data-icon={name}
      data-mirrors={mirrors}
      {...(labelled ? { role: "img", "aria-label": label } : { "aria-hidden": true })}
      className={cx("shrink-0", sizeClass[size], mirrors && "rtl:-scale-x-100", className)}
    >
      {iconPaths[name].map((p) => (
        <path key={p.d} d={p.d} {...(p.faded ? { opacity: 0.2 } : {})} />
      ))}
    </svg>
  );
}

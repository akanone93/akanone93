// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { KeyboardEventHandler, ReactElement, ReactNode } from "react";
import { cx } from "./cx.ts";

export interface GlassProps {
  /** Accessible name of the region, from the message catalogue. */
  readonly label: string;
  readonly children: ReactNode;
  readonly className?: string;
  /** `toolbar` for a capsule whose controls are reached with the arrow keys; `group` otherwise. */
  readonly role?: "group" | "toolbar";
  readonly onKeyDown?: KeyboardEventHandler<HTMLDivElement>;
}

/** The canvas's top bars on shadcn's popover surface, height from the density tokens. */
export function GlassCapsule({ label, children, className, role = "group", onKeyDown }: GlassProps): ReactElement {
  return (
    // biome-ignore lint/a11y/noStaticElementInteractions: the role is a toolbar or a group; the handler only roves focus
    // biome-ignore lint/a11y/useAriaPropsSupportedByRole: the role is a toolbar or a group, both name themselves
    // biome-ignore lint/a11y/useSemanticElements: a capsule is a named group of mixed controls, not a form fieldset
    <div
      role={role}
      aria-label={label}
      onKeyDown={onKeyDown}
      className={cx(
        "flex h-(--density-capsule) items-center gap-2 rounded-xl bg-popover px-2 text-popover-foreground shadow-md ring-1 ring-foreground/10",
        className,
      )}
    >
      {children}
    </div>
  );
}

/** The canvas's floating panels on shadcn's popover surface. */
export function FloatingPanel({ label, children, className }: GlassProps): ReactElement {
  return (
    <section
      aria-label={label}
      className={cx("rounded-xl bg-popover p-3 text-popover-foreground shadow-md ring-1 ring-foreground/10", className)}
    >
      {children}
    </section>
  );
}

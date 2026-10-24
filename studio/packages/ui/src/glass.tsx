// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ReactElement, ReactNode } from "react";
import { cx } from "./cx.ts";

export interface GlassProps {
  /** Accessible name of the region, from the message catalogue. */
  readonly label: string;
  readonly children: ReactNode;
  readonly className?: string;
}

/** The three top capsules: floating glass, height from the density tokens (U3). Never nested in another glass surface. */
export function GlassCapsule({ label, children, className }: GlassProps): ReactElement {
  return (
    // biome-ignore lint/a11y/useSemanticElements: a capsule is a named group of mixed controls, not a form fieldset
    <div
      role="group"
      aria-label={label}
      className={cx("glass flex h-(--density-capsule) items-center gap-2 rounded-full px-3 text-primary", className)}
    >
      {children}
    </div>
  );
}

/** Left and right panels, menus and the bottom drawer: floating glass, `radius.xl` (U3). Never nested in another glass surface. */
export function FloatingPanel({ label, children, className }: GlassProps): ReactElement {
  return (
    <section aria-label={label} className={cx("glass rounded-xl p-3 text-primary", className)}>
      {children}
    </section>
  );
}

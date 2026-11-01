// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { AnimatePresence, domMax, LazyMotion, MotionConfig, m } from "motion/react";
import type { ReactElement, ReactNode } from "react";

/** One easing and length for every filtered list, so a search narrows the apps and the table rows alike. */
const transition = { duration: 0.22, ease: [0.2, 0, 0, 1] } as const;
const hidden = { opacity: 0, scale: 0.97 } as const;
const shown = { opacity: 1, scale: 1 } as const;

/**
 * A list whose items animate as a filter, a search or a sort changes it: an item that stays glides to its new place,
 * one that leaves fades out, and one that joins fades in. Nothing moves for a person who asks for reduced motion.
 */
export function AnimatedList<T>({
  items,
  keyOf,
  render,
  className,
  label,
}: {
  readonly items: readonly T[];
  readonly keyOf: (item: T) => string;
  readonly render: (item: T) => ReactNode;
  readonly className?: string;
  readonly label?: string;
}): ReactElement {
  return (
    <LazyMotion features={domMax} strict>
      <MotionConfig reducedMotion="user" transition={transition}>
        <ul className={className} aria-label={label}>
          <AnimatePresence initial={false} mode="popLayout">
            {items.map((item) => (
              <m.li key={keyOf(item)} layout initial={hidden} animate={shown} exit={hidden} className="relative">
                {render(item)}
              </m.li>
            ))}
          </AnimatePresence>
        </ul>
      </MotionConfig>
    </LazyMotion>
  );
}

/** A table body whose rows animate as the list is filtered, in the same way as `AnimatedList`. */
export function AnimatedRows<T>({
  rows,
  keyOf,
  render,
  className,
}: {
  readonly rows: readonly T[];
  readonly keyOf: (row: T) => string;
  /** The cells of a row. */
  readonly render: (row: T) => ReactNode;
  readonly className?: string;
}): ReactElement {
  return (
    <LazyMotion features={domMax} strict>
      <MotionConfig reducedMotion="user" transition={transition}>
        <tbody className={className}>
          <AnimatePresence initial={false}>
            {rows.map((row) => (
              <m.tr
                key={keyOf(row)}
                layout="position"
                initial={{ opacity: 0 }}
                animate={{ opacity: 1 }}
                exit={{ opacity: 0 }}
                className="border-b transition-colors hover:bg-muted/50"
              >
                {render(row)}
              </m.tr>
            ))}
          </AnimatePresence>
        </tbody>
      </MotionConfig>
    </LazyMotion>
  );
}

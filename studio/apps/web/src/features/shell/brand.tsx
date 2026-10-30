// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { m as motion, useReducedMotion } from "motion/react";
import type { ReactNode } from "react";

/** The entrance curve of the brand's headlines and blocks: a quick start that settles slowly (U13.4). */
export const enter = [0.16, 1, 0.3, 1] as const;

/** Headline words that rise out of a blur, one after another (U13.4). Needs a `LazyMotion` above it. */
export function RisingWords({ text, from, className }: { text: string; from: number; className?: string }) {
  const reduced = useReducedMotion() === true;
  return (
    <span className={className}>
      {text.split(" ").map((word, i) => (
        <motion.span
          // biome-ignore lint/suspicious/noArrayIndexKey: the words of a fixed sentence keep their order
          key={i}
          className="inline-block whitespace-pre"
          initial={reduced ? false : { opacity: 0, y: "0.45em", filter: "blur(12px)" }}
          animate={{ opacity: 1, y: 0, filter: "blur(0px)" }}
          transition={{ duration: 1.1, ease: enter, delay: reduced ? 0 : 0.1 + (from + i) * 0.07 }}
        >
          {`${word} `}
        </motion.span>
      ))}
    </span>
  );
}

/** Fades a block up into place after the headline (U13.4). Needs a `LazyMotion` above it. */
export function Rise({ delay, children, className }: { delay: number; children: ReactNode; className?: string }) {
  const reduced = useReducedMotion() === true;
  return (
    <motion.div
      className={className}
      initial={reduced ? false : { opacity: 0, y: 24 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 1, ease: enter, delay: reduced ? 0 : delay }}
    >
      {children}
    </motion.div>
  );
}

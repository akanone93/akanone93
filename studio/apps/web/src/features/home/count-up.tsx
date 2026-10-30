// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { animate, useMotionValue, useMotionValueEvent, useReducedMotion } from "motion/react";
import { useEffect, useState } from "react";

/**
 * A number that counts up to its value when it first shows and glides to each new value (U13.4); with reduced motion,
 * or while animations are off, it shows the value at once.
 */
export function CountUp({ value, format }: { value: number; format: (v: number) => string }) {
  const reduced = useReducedMotion() === true;
  const shown = useMotionValue(reduced ? value : 0);
  const [text, setText] = useState(() => format(shown.get()));
  useMotionValueEvent(shown, "change", (v) => setText(format(v)));
  useEffect(() => {
    if (reduced) {
      shown.set(value);
      return;
    }
    const controls = animate(shown, value, { duration: 1.6, ease: [0.16, 1, 0.3, 1] });
    return () => controls.stop();
  }, [reduced, shown, value]);
  // The final value is what assistive technology reads; the counting is only drawn.
  return (
    <>
      <span aria-hidden="true">{text}</span>
      <span className="sr-only">{format(value)}</span>
    </>
  );
}

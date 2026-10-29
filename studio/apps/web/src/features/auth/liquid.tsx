// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { m, useReducedMotion } from "motion/react";
import { useId } from "react";

/** The drawing box of the liquid form; the SVG scales it to its container. */
const SIZE = 800;
const CENTRE = SIZE / 2;
const RADIUS = 300;

/**
 * Radius factors of the six control points of each shape the liquid passes through. Every shape
 * has the same structure, so Motion interpolates the path numbers one by one.
 */
const SHAPES: readonly (readonly number[])[] = [
  [1, 0.84, 1.08, 0.9, 1.04, 0.86],
  [0.88, 1.08, 0.86, 1.06, 0.84, 1.02],
  [1.08, 0.9, 0.98, 0.82, 1.1, 0.94],
  [0.9, 1.02, 1.06, 0.88, 0.94, 1.08],
];

/** A closed Catmull-Rom curve through points around a centre, written as cubic Béziers. */
function blobPath(factors: readonly number[], scale = 1, dx = 0, dy = 0): string {
  const count = factors.length;
  const points = factors.map((f, i) => {
    const angle = (i / count) * Math.PI * 2 - Math.PI / 2;
    const r = RADIUS * f * scale;
    return [CENTRE + dx + Math.cos(angle) * r, CENTRE + dy + Math.sin(angle) * r] as const;
  });
  const at = (i: number) => points[(i + count) % count] ?? ([CENTRE, CENTRE] as const);
  const round = (n: number) => Math.round(n * 10) / 10;
  const [sx, sy] = at(0);
  let d = `M${round(sx)} ${round(sy)}`;
  for (let i = 0; i < count; i++) {
    const [x0, y0] = at(i - 1);
    const [x1, y1] = at(i);
    const [x2, y2] = at(i + 1);
    const [x3, y3] = at(i + 2);
    const c1x = x1 + (x2 - x0) / 6;
    const c1y = y1 + (y2 - y0) / 6;
    const c2x = x2 - (x3 - x1) / 6;
    const c2y = y2 - (y3 - y1) / 6;
    d += ` C${round(c1x)} ${round(c1y)} ${round(c2x)} ${round(c2y)} ${round(x2)} ${round(y2)}`;
  }
  return `${d} Z`;
}

function cycle(paths: readonly string[]): string[] {
  return [...paths, paths[0] ?? ""];
}

const body = cycle(SHAPES.map((s) => blobPath(s)));
/** The reflection: a smaller copy of the body, up and to the left, where the light falls. */
const reflection = cycle(SHAPES.map((s) => blobPath(s, 0.42, -70, -110)));

/** The drop lands and wobbles before it settles: an underdamped spring. */
const landing = { type: "spring", stiffness: 70, damping: 8, mass: 1, delay: 0.15 } as const;
const morphing = { duration: 18, ease: "easeInOut", repeat: Number.POSITIVE_INFINITY } as const;
const turning = { duration: 26, ease: "easeInOut", repeat: Number.POSITIVE_INFINITY } as const;

/**
 * The liquid form behind the sign-in picture (U16.2): a glossy body that lands as a drop, swells
 * with a wobbling spring and then keeps changing shape and turning slowly. Its colours are the
 * hero tokens; a soft reflection and a specular spot make it read as liquid glass. Decorative.
 */
export function Liquid() {
  const id = useId();
  const reduced = useReducedMotion() === true;
  const fill = `${id}-fill`;
  const shine = `${id}-shine`;
  const spot = `${id}-spot`;
  return (
    <svg aria-hidden="true" viewBox={`0 0 ${SIZE} ${SIZE}`} className="hero-liquid">
      <defs>
        <linearGradient id={fill} x1="0.1" y1="0" x2="0.9" y2="1">
          <stop offset="0" style={{ stopColor: "var(--color-hero-liquid-a)" }} />
          <stop offset="0.55" style={{ stopColor: "var(--color-hero-liquid-b)" }} />
          <stop offset="1" style={{ stopColor: "var(--color-hero-liquid-c)" }} />
        </linearGradient>
        <linearGradient id={shine} x1="0.2" y1="0" x2="0.7" y2="1">
          <stop offset="0" style={{ stopColor: "var(--color-raised)", stopOpacity: 0.7 }} />
          <stop offset="1" style={{ stopColor: "var(--color-raised)", stopOpacity: 0 }} />
        </linearGradient>
        <radialGradient id={spot} cx="0.3" cy="0.22" r="0.16">
          <stop offset="0" style={{ stopColor: "var(--color-raised)", stopOpacity: 0.85 }} />
          <stop offset="1" style={{ stopColor: "var(--color-raised)", stopOpacity: 0 }} />
        </radialGradient>
      </defs>
      <m.g
        style={{ originX: "50%", originY: "50%" }}
        initial={reduced ? false : { scale: 0.05, y: -260, opacity: 0 }}
        animate={{ scale: 1, y: 0, opacity: 1 }}
        transition={{ scale: landing, y: landing, opacity: { duration: 0.25, delay: 0.15 } }}
      >
        <m.g
          style={{ originX: "50%", originY: "50%" }}
          animate={reduced ? { rotate: 0 } : { rotate: [0, 14, -6, 0] }}
          transition={turning}
        >
          <m.path
            fill={`url(#${fill})`}
            initial={false}
            animate={reduced ? { d: body[0] ?? "" } : { d: body }}
            transition={morphing}
          />
          <m.path
            fill={`url(#${shine})`}
            initial={false}
            animate={reduced ? { d: reflection[0] ?? "" } : { d: reflection }}
            transition={morphing}
          />
          <circle cx={CENTRE} cy={CENTRE} r={RADIUS} fill={`url(#${spot})`} />
        </m.g>
      </m.g>
    </svg>
  );
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ReactElement } from "react";
import { encode } from "uqr";

const quietZone = 2;

/** One horizontal run of dark modules. */
interface Run {
  readonly x: number;
  readonly y: number;
  readonly width: number;
}

/** Joins neighbouring dark modules of each row into runs, so the SVG has far fewer elements. */
export function runsOf(modules: readonly (readonly boolean[])[]): Run[] {
  const runs: Run[] = [];
  modules.forEach((row, y) => {
    let start = -1;
    for (let x = 0; x <= row.length; x += 1) {
      const dark = x < row.length && row[x] === true;
      if (dark && start < 0) {
        start = x;
      } else if (!dark && start >= 0) {
        runs.push({ x: start, y, width: x - start });
        start = -1;
      }
    }
  });
  return runs;
}

/** A QR code drawn as SVG rectangles (L-2): never a markup string, so nothing for the CSP to allow. */
export function QrCode({ text, label }: { readonly text: string; readonly label: string }): ReactElement {
  const { data, size } = encode(text, { ecc: "M", border: 0 });
  const full = size + quietZone * 2;
  return (
    <svg
      role="img"
      aria-label={label}
      viewBox={`0 0 ${full} ${full}`}
      shapeRendering="crispEdges"
      className="size-48 rounded-md bg-white"
    >
      {runsOf(data).map((r) => (
        <rect key={`${r.y}-${r.x}`} x={r.x + quietZone} y={r.y + quietZone} width={r.width} height={1} fill="black" />
      ))}
    </svg>
  );
}

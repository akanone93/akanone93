// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Playing a widget's enter or exit transition on the canvas (PC-29, ANI-003): the frame at each animation frame,
 * about thirty a second, until the transition ends; only the drawn copy moves, the document never changes.
 */

import { useCallback, useEffect, useRef, useState } from "react";
import { frameAt, type MotionFrame } from "./motion.ts";

/** A transition being played: on node `id`, at `frame`. */
export interface Playing {
  readonly id: string;
  readonly frame: MotionFrame;
}

interface Run {
  readonly id: string;
  readonly phase: "enter" | "exit";
  readonly transition: Record<string, unknown>;
  readonly size: { readonly w: number; readonly h: number };
  readonly start: number;
}

/** The step between drawn frames, in ms (about thirty a second: each is a layout of one screen). */
const STEP_MS = 33;

export function useMotionPlayer(): {
  readonly playing: Playing | undefined;
  readonly play: (
    id: string,
    phase: "enter" | "exit",
    transition: Record<string, unknown>,
    size: { readonly w: number; readonly h: number },
  ) => void;
} {
  const [playing, setPlaying] = useState<Playing | undefined>();
  const run = useRef<Run | null>(null);
  const handle = useRef<number | null>(null);
  const tick = useCallback((now: number) => {
    const r = run.current;
    if (r === null) {
      return;
    }
    const frame = frameAt(r.transition, r.phase, now - r.start, r.size);
    if (frame === undefined) {
      run.current = null;
      setPlaying(undefined);
      return;
    }
    setPlaying({ id: r.id, frame });
    handle.current = window.setTimeout(() => requestAnimationFrame(tick), STEP_MS);
  }, []);
  const play = useCallback(
    (id: string, phase: "enter" | "exit", transition: Record<string, unknown>, size: { w: number; h: number }) => {
      if (handle.current !== null) {
        window.clearTimeout(handle.current);
      }
      run.current = { id, phase, transition, size, start: performance.now() };
      requestAnimationFrame(tick);
    },
    [tick],
  );
  useEffect(
    () => () => {
      run.current = null;
      if (handle.current !== null) {
        window.clearTimeout(handle.current);
      }
    },
    [],
  );
  return { playing, play };
}

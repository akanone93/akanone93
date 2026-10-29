// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ReactElement } from "react";

export interface LiveChipProps {
  /** Device, state and latency in one string, e.g. "Pixel 9 · live · 41 ms", composed by the caller's i18n. */
  readonly label: string;
}

/** Paired device or live stream. Its pulsing dot is the one looping animation; reduced motion stops it (U10, U13). */
export function LiveChip({ label }: LiveChipProps): ReactElement {
  return (
    <span
      role="status"
      className="inline-flex items-center gap-2 rounded-full bg-success-surface px-2 py-0.5 font-mono text-tag text-success"
    >
      <span aria-hidden="true" className="size-1 animate-pulse rounded-full bg-success motion-reduce:animate-none" />
      <span>{label}</span>
    </span>
  );
}

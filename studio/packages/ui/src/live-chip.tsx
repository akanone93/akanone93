// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ReactElement } from "react";
import { Badge } from "./components/badge";

export interface LiveChipProps {
  /** Device, state and latency in one string, e.g. "Pixel 9 · live · 41 ms", composed by the caller's i18n. */
  readonly label: string;
}

/** A paired device or live stream as shadcn's Badge; its pulsing dot stops under reduced motion. */
export function LiveChip({ label }: LiveChipProps): ReactElement {
  return (
    <Badge variant="outline" role="status">
      <span aria-hidden="true" className="size-1.5 animate-pulse rounded-full bg-success motion-reduce:animate-none" />
      <span>{label}</span>
    </Badge>
  );
}

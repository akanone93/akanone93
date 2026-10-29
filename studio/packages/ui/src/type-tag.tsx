// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ReactElement, ReactNode } from "react";

export interface TypeTagProps {
  readonly children: ReactNode;
}

/** Node type in the layer tree and search results: mono, tertiary, fixed column (U10). */
export function TypeTag({ children }: TypeTagProps): ReactElement {
  return <span className="inline-block w-6 shrink-0 truncate font-mono text-tag text-tertiary">{children}</span>;
}

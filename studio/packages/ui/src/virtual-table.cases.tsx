// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { CaseDefinition } from "./case.ts";
import { caseRowHeight } from "./case.ts";
import { VirtualTable } from "./virtual-table.tsx";

interface Row {
  readonly path: string;
  readonly size: string;
}

export const tableRows: readonly Row[] = Array.from({ length: 10_000 }, (_, i) => ({
  path: `/pages/page-${i}`,
  size: `${i} B`,
}));

export const cases: readonly CaseDefinition[] = [
  {
    name: "10,000 rows",
    element: (
      <VirtualTable
        label="Pages"
        data={tableRows}
        rowHeight={caseRowHeight}
        visibleRows={8}
        columns={[
          { accessorKey: "path", header: "Path" },
          { accessorKey: "size", header: "Size" },
        ]}
      />
    ),
  },
];

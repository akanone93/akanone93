// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Meter } from "@plux/ui";
import type { ReactElement } from "react";
import { Table } from "../security/parts.tsx";
import { localisationAreaMessages } from "./area.messages.ts";
import { AreaFrame } from "./area-frame.tsx";
import { useArea } from "./area-scope.tsx";
import { missingByPlugin } from "./grid-model.ts";

function Rows(): ReactElement {
  const m = localisationAreaMessages(useLingui().i18n);
  const area = useArea();
  const locales = area.sessions.locales;
  const rows = missingByPlugin(area.usedByPlugin, locales, area.messages);
  if (rows.length === 0) {
    return <p className="text-body text-secondary">{m.heatEmpty}</p>;
  }
  return (
    <Table
      caption={m.heatTable}
      heads={[m.heatPlugin, ...locales]}
      rows={rows.map((r) => ({
        key: r.plugin,
        cells: [
          r.plugin,
          ...locales.map((l) => {
            const missing = r.missing[l] ?? 0;
            const text = m.heatCell({ missing, total: r.total });
            return (
              <Meter
                key={l}
                value={r.total - missing}
                max={Math.max(r.total, 1)}
                label={`${r.plugin} ${l}`}
                valueText={text}
              />
            );
          }),
        ],
      }))}
    />
  );
}

/** A14: the translations each plugin still lacks, per locale (STU-111, I18N-007). */
export function HeatmapPage(): ReactElement {
  const m = localisationAreaMessages(useLingui().i18n);
  return (
    <AreaFrame id="heatmap-title" title={m.heatTitle} caption={m.heatCaption}>
      <Rows />
    </AreaFrame>
  );
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { AppLimitUsage, LimitUsage } from "@plux/api";
import { limits as registry } from "@plux/schema";
import { Meter, StatusPill } from "@plux/ui";
import type { ReactElement } from "react";
import { appsMessages } from "./apps.messages.ts";
import { formatAmount, type LimitLevel, levelOf, type UnitWords } from "./apps-model.ts";

type Messages = ReturnType<typeof appsMessages>;

function unitWords(m: Messages): UnitWords {
  return {
    operations: m.unitOperations,
    codepoints: m.unitCodepoints,
    microseconds: m.unitMicroseconds,
    milliseconds: m.unitMilliseconds,
  };
}

/** The badge of a limit that is near or at its limit; a limit within bounds shows none, and the meter carries the number. */
function LevelPill({ level, m }: { readonly level: LimitLevel; readonly m: Messages }): ReactElement | null {
  if (level === "ok") {
    return null;
  }
  return level === "over" ? (
    <StatusPill status="danger" label={m.limitOver} />
  ) : (
    <StatusPill status="warning" label={m.limitWarning} />
  );
}

/**
 * The usage of an app against its limits on its card (STU-025): one meter per limit the summary reports, with the
 * number in text, and a badge once the usage is near or at the limit in force.
 */
export function CardLimits({ limits }: { readonly limits: readonly AppLimitUsage[] }): ReactElement | null {
  const { i18n } = useLingui();
  const m = appsMessages(i18n);
  if (limits.length === 0) {
    return null;
  }
  return (
    <ul aria-label={m.limitsUsage} className="flex flex-col gap-1">
      {limits.map((l) => {
        const name = m.limitName(l.key) ?? l.key;
        const unit = registry.find((r) => r.key === l.key)?.unit ?? "count";
        const text = m.limitUsageText(
          formatAmount(l.used, unit, i18n.locale, unitWords(m)),
          formatAmount(l.limit, unit, i18n.locale, unitWords(m)),
        );
        return (
          <li key={l.key} className="flex flex-wrap items-center gap-2">
            <span className="text-caption text-secondary">{name}</span>
            <Meter value={Number(l.used)} max={Number(l.limit)} label={name} valueText={text} />
            <LevelPill level={levelOf(l.used, l.limit)} m={m} />
          </li>
        );
      })}
    </ul>
  );
}

/** The table of every limit of the app with its usage and the limit in force (STU-025, LIM-005). */
export function LimitsTable({ limits }: { readonly limits: readonly LimitUsage[] }): ReactElement {
  const { i18n } = useLingui();
  const m = appsMessages(i18n);
  if (limits.length === 0) {
    return <p className="text-body text-secondary">{m.limitsEmpty}</p>;
  }
  return (
    <table className="w-full text-start text-body">
      <caption className="sr-only">{m.limitsTitle}</caption>
      <thead>
        <tr>
          {[m.colLimit, m.colUsed, m.colEffective, m.colUsage].map((h) => (
            <th key={h} scope="col" className="p-2 text-start text-caption text-secondary">
              {h}
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {limits.map((l) => {
          const used = formatAmount(l.value, l.unit, i18n.locale, unitWords(m));
          const effective = formatAmount(l.effective, l.unit, i18n.locale, unitWords(m));
          const computed = levelOf(l.value, l.effective);
          const level: LimitLevel = computed === "ok" && l.warning ? "warning" : computed;
          const friendly = m.limitName(l.key);
          return (
            <tr key={l.key}>
              <th scope="row" className="p-2 text-start font-normal">
                {friendly !== undefined && <span className="block">{friendly}</span>}
                <span className="font-mono text-data text-secondary">{l.key}</span>
              </th>
              <td className="p-2 font-mono text-data">{used}</td>
              <td className="p-2 font-mono text-data">{effective}</td>
              <td className="p-2">
                <span className="flex flex-wrap items-center gap-2">
                  <Meter
                    value={Number(l.value)}
                    max={Number(l.effective)}
                    label={friendly ?? l.key}
                    valueText={m.limitUsageText(used, effective)}
                  />
                  <LevelPill level={level} m={m} />
                </span>
              </td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}

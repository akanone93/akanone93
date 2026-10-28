// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Field, Input } from "@plux/ui";
import type { ReactElement } from "react";
import { type DesignMessages, designMessages } from "./design.messages.ts";
import { families, groupRows, putToken, SCRIPTS } from "./design-model.ts";
import { Frame, useDesign } from "./scope.tsx";

const scriptName = (m: DesignMessages, s: (typeof SCRIPTS)[number]): string =>
  ({
    latin: m.scriptLatin,
    cyrillic: m.scriptCyrillic,
    arabic: m.scriptArabic,
    devanagari: m.scriptDevanagari,
    ethiopic: m.scriptEthiopic,
  })[s];

function Script({ script }: { readonly script: (typeof SCRIPTS)[number] }): ReactElement {
  const m = designMessages(useLingui().i18n);
  const ctx = useDesign();
  const row = groupRows(ctx.rows, "typography").find((r) => r.path[0] === script);
  const list = families(row?.light);
  const name = scriptName(m, script);
  return (
    <li className="flex flex-col gap-1">
      <Field label={m.familiesLabel(name)}>
        <Input
          value={list.join(", ")}
          disabled={!ctx.canEdit}
          onChange={(e) =>
            ctx.write(`typography:${script}`, (t) =>
              putToken(
                t,
                ["typography", script],
                e.target.value
                  .split(",")
                  .map((f) => f.trim())
                  .filter((f) => f !== ""),
                "fontFamily",
              ),
            )
          }
        />
      </Field>
      <span className="text-caption text-secondary">
        {list.length < 2 ? m.noFallback(name) : m.fallbackOk(name, list.length)}
      </span>
    </li>
  );
}

/** A17: the font families of each script, with fallbacks (STU-112, THM-004). */
export function TypographyPage(): ReactElement {
  const m = designMessages(useLingui().i18n);
  return (
    <Frame id="design-typography" title={m.typographyTitle} caption={m.typographyCaption}>
      <ul className="flex flex-col gap-3">
        {SCRIPTS.map((s) => (
          <Script key={s} script={s} />
        ))}
      </ul>
    </Frame>
  );
}

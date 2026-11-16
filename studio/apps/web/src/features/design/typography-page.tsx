// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Field, Input } from "@plux/ui";
import { type ReactElement, useEffect, useState } from "react";
import { type DesignMessages, designMessages } from "./design.messages.ts";
import { families, putToken, SCRIPTS, scriptPath } from "./design-model.ts";
import { HostFonts } from "./host-fonts.tsx";
import { Frame, useDesign } from "./scope.tsx";

const scriptName = (m: DesignMessages, s: (typeof SCRIPTS)[number]): string =>
  ({
    latin: m.scriptLatin,
    cyrillic: m.scriptCyrillic,
    arabic: m.scriptArabic,
    devanagari: m.scriptDevanagari,
    ethiopic: m.scriptEthiopic,
  })[s];

/** The families typed as a comma-separated list, empty entries left out. */
const parseFamilies = (text: string): string[] =>
  text
    .split(",")
    .map((f) => f.trim())
    .filter((f) => f !== "");

/**
 * One script's families, the `fontFamily` token at `font.script.<code>` the device appends to every text style's
 * fallbacks (THM-004). The list is kept as typed and saved when the field is left or Enter is pressed, so a comma
 * starts the next family instead of being dropped.
 */
function Script({ script }: { readonly script: (typeof SCRIPTS)[number] }): ReactElement {
  const m = designMessages(useLingui().i18n);
  const ctx = useDesign();
  const path = scriptPath(script);
  const row = ctx.rows.find((r) => r.path.join(".") === path.join("."));
  const list = families(row?.light);
  const saved = list.join(", ");
  const [text, setText] = useState(saved);
  useEffect(() => setText(saved), [saved]);
  const name = scriptName(m, script);
  const save = () => {
    const next = parseFamilies(text);
    if (next.join(", ") !== saved) {
      ctx.write(`font:${script}`, (t) => putToken(t, path, next, "fontFamily"));
    }
  };
  const shown = parseFamilies(text);
  return (
    <li className="flex flex-col gap-1">
      <Field label={m.familiesLabel(name)}>
        <Input
          value={text}
          disabled={!ctx.canEdit}
          onChange={(e) => setText(e.target.value)}
          onBlur={save}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              save();
            }
          }}
        />
      </Field>
      <span className="text-caption text-muted-foreground">
        {shown.length === 0
          ? m.noFonts(name)
          : shown.length < 2
            ? m.noFallback(name)
            : m.fallbackOk(name, shown.length)}
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
      <HostFonts />
    </Frame>
  );
}

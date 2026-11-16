// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { parseHexColor, type Rgba } from "@plux/layout";
import type { ReactElement } from "react";
import { contrast } from "../plugin-canvas/a11y-marks.ts";
import { tokenValue } from "../plugin-canvas/paint-theme.ts";
import { designMessages } from "./design.messages.ts";
import type { Mode } from "./design-model.ts";
import { modeLabel } from "./parts.tsx";

type Obj = Record<string, unknown>;

/** The colour roles shown as pairs: a fill and the colour of what is drawn on it (THM-001, A11Y-003). */
const PAIRS = [
  ["primary", "onPrimary"],
  ["primaryContainer", "onPrimaryContainer"],
  ["secondary", "onSecondary"],
  ["secondaryContainer", "onSecondaryContainer"],
  ["tertiary", "onTertiary"],
  ["tertiaryContainer", "onTertiaryContainer"],
  ["error", "onError"],
  ["errorContainer", "onErrorContainer"],
  ["surfaceContainer", "onSurface"],
  ["inverseSurface", "onInverseSurface"],
] as const;

/** WCAG 2.2 AA for normal text (A11Y-003). */
const AA_TEXT = 4.5;

/**
 * A colour token's value in a mode, as the device reads it: dark from `$extensions`, high contrast from the
 * `contrast.high` group over the light value (ADR-0032), aliases followed.
 */
export function modeColour(tokens: Obj, path: string, mode: Mode): string | undefined {
  const read = (p: string, dark: boolean) => {
    const v = tokenValue(tokens, p, dark, "");
    return typeof v === "string" ? v : undefined;
  };
  if (mode === "contrast") {
    return read(`contrast.high.${path}`, false) ?? read(path, false);
  }
  return read(path, mode === "dark");
}

const rgba = (hex: string | undefined): Rgba | undefined => (hex === undefined ? undefined : parseHexColor(hex));

/** The contrast of `on` over `fill`, or undefined when either is not a colour the page knows. */
export function pairContrast(fill: string | undefined, on: string | undefined): number | undefined {
  const a = rgba(fill);
  const b = rgba(on);
  return a === undefined || b === undefined ? undefined : contrast(a, b);
}

function PairTile(props: { readonly tokens: Obj; readonly mode: Mode; readonly fill: string; readonly on: string }) {
  const m = designMessages(useLingui().i18n);
  const fill = modeColour(props.tokens, `color.${props.fill}`, props.mode);
  const on = modeColour(props.tokens, `color.${props.on}`, props.mode);
  if (fill === undefined) {
    return null;
  }
  const ratio = pairContrast(fill, on);
  const passes = ratio !== undefined && ratio >= AA_TEXT;
  return (
    <li className="mode-pair" style={{ backgroundColor: fill, color: on }}>
      <span className="mode-pair-sample" aria-hidden="true">
        {m.textSample}
      </span>
      <span className="mode-pair-name">{props.fill}</span>
      {ratio !== undefined && (
        <span className="mode-pair-ratio" data-passes={passes}>
          {passes ? m.contrastPasses(ratio.toFixed(1)) : m.contrastFails(ratio.toFixed(1))}
        </span>
      )}
    </li>
  );
}

/**
 * One mode of the theme as a screen would show it (STU-112): on the mode's own surface, each colour role as a tile
 * with text in its on-colour and the contrast between them, so a pair below WCAG AA shows here before a check flags
 * it; the colours without a pair follow as swatches.
 */
export function ModePreview(props: { readonly tokens: Obj; readonly mode: Mode }): ReactElement {
  const m = designMessages(useLingui().i18n);
  const { tokens, mode } = props;
  const surface = modeColour(tokens, "color.surface", mode);
  const onSurface = modeColour(tokens, "color.onSurface", mode);
  const paired = new Set<string>(PAIRS.flat());
  const colour = tokens["color"];
  const others = (colour !== null && typeof colour === "object" ? Object.keys(colour) : []).filter(
    (k) => !k.startsWith("$") && !paired.has(k) && k !== "surface",
  );
  const label = modeLabel(m, mode);
  return (
    <section
      aria-label={label}
      className="mode-preview"
      style={{ backgroundColor: surface, color: onSurface }}
      data-mode={mode}
    >
      <h3 className="mode-preview-title">{label}</h3>
      <ul className="mode-pairs">
        {PAIRS.map(([fill, on]) => (
          <PairTile key={fill} tokens={tokens} mode={mode} fill={fill} on={on} />
        ))}
      </ul>
      {others.length > 0 && (
        <ul aria-label={m.swatches(label)} className="mode-swatches">
          {others.map((k) => {
            const v = modeColour(tokens, `color.${k}`, mode);
            return v === undefined ? null : (
              <li key={k}>
                <span aria-hidden="true" className="mode-swatch" style={{ backgroundColor: v }} />
                <span>{k}</span>
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}

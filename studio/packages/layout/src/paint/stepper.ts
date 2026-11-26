// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * A vertical `Stepper` as Material draws it (build log row 123): each step a 72-high header — a 24-pixel circle 24
 * from the start, between 16-pixel connector lines, holding the step's number, a check when complete, a pencil when
 * editing, or a red warning when in error — with its title (and subtitle) 60 from the start; below the current step,
 * its content's room, crossed by the line to the next step. The scene lays the stepper out as Flutter does; this draws
 * it, from the steps the document or the design-time evaluation gives (ADR-0112).
 */

import type { PaintText, PlacedGlyph } from "./fields.ts";
import { type BoxPaint, type PaintTheme, type Rect, type Rgba, roleColor, withOpacity } from "./paint.ts";
import { M3_TYPE_SCALE, type ResolvedText, typeScaleOf } from "./text-style.ts";

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/** Flutter's `Stepper` geometry: header height, circle, line, the title's start and the current step's content. */
const HEADER = 72;
const CIRCLE = 24;
const LINE = 16;
const MARGIN = 24;
const TITLE_START = 60;
const END_PAD = 24;
const CONTENT = 88;
/** `Colors.grey.shade400`, the connector; `Colors.red` and `Colors.red.shade400`, a step in error. */
const LINE_COLOR: Rgba = 0xbdbdbdff;
const ERROR_LIGHT: Rgba = 0xf44336ff;
const ERROR_DARK: Rgba = 0xef5350ff;

/** What a stepper paints: its circles and lines, its titles and numbers, and its icons. */
export interface StepperPaint {
  readonly boxes: readonly BoxPaint[];
  readonly texts: readonly PaintText[];
  readonly glyphs: readonly PlacedGlyph[];
}

interface Step {
  readonly title: string;
  readonly subtitle: string | undefined;
  readonly state: string;
  readonly active: boolean;
}

/** The steps of `props`; none when `steps` is not a list. */
function stepsOf(props: Obj): Step[] {
  const steps = Array.isArray(props["steps"]) ? props["steps"] : [];
  return steps.filter(isObj).map((s) => ({
    title: typeof s["title"] === "string" ? s["title"] : "",
    subtitle: typeof s["subtitle"] === "string" ? s["subtitle"] : undefined,
    state: typeof s["state"] === "string" ? s["state"] : "indexed",
    active: s["isActive"] === true,
  }));
}

/** What every step shares. */
interface Frame {
  readonly r: Rect;
  readonly rtl: boolean;
  readonly dark: boolean;
  readonly theme: PaintTheme;
  readonly title: ResolvedText;
  readonly subtitle: ResolvedText;
  readonly number: ResolvedText;
}

/** A step's circle colour and what it draws on it, as `Stepper._circleColor` and `_buildCircleChild` have them. */
function circleOf(step: Step, f: Frame): { fill: Rgba; on: Rgba } {
  if (f.dark) {
    return {
      fill: step.active ? roleColor(f.theme, "secondary") : roleColor(f.theme, "surface"),
      on: roleColor(f.theme, "onSecondary"),
    };
  }
  return {
    fill: step.active ? roleColor(f.theme, "primary") : withOpacity(roleColor(f.theme, "onSurface"), 0.38),
    on: roleColor(f.theme, "onPrimary"),
  };
}

/** A step's text colour: red in error, faded when disabled, else the theme's. */
function textColor(step: Step, f: Frame): Rgba {
  if (step.state === "error") {
    return f.dark ? ERROR_DARK : ERROR_LIGHT;
  }
  const onSurface = roleColor(f.theme, "onSurface");
  return step.state === "disabled" ? withOpacity(onSurface, 0.38) : onSurface;
}

/** The circle, or the warning that stands for it in error, with what it shows. */
function drawCircle(step: Step, index: number, cx: number, cy: number, f: Frame, out: Out): void {
  const box: Rect = { x: cx - CIRCLE / 2, y: cy - CIRCLE / 2, w: CIRCLE, h: CIRCLE };
  if (step.state === "error") {
    out.glyphs.push({ name: "warning", set: "material", size: CIRCLE, color: textColor(step, f), ...box });
    return;
  }
  const { fill, on } = circleOf(step, f);
  out.boxes.push({ ...box, radii: [CIRCLE / 2, CIRCLE / 2, CIRCLE / 2, CIRCLE / 2], fill });
  const icon = step.state === "complete" ? "check" : step.state === "editing" ? "edit" : undefined;
  if (icon !== undefined) {
    out.glyphs.push({ name: icon, set: "material", size: 18, color: on, x: cx - 9, y: cy - 9, w: 18, h: 18 });
    return;
  }
  const h = f.number.size * f.number.height;
  out.texts.push({
    text: String(index + 1),
    x: box.x,
    y: cy - h / 2,
    w: CIRCLE,
    style: f.number,
    color: on,
    align: "center",
  });
}

/** The title and subtitle, centred in the header beside the circle. */
function drawTitle(step: Step, top: number, f: Frame, out: Out): void {
  const titleH = f.title.size * f.title.height;
  const subH = step.subtitle === undefined ? 0 : 2 + f.subtitle.size * f.subtitle.height;
  const y = top + (HEADER - titleH - subH) / 2;
  const x = f.rtl ? f.r.x + END_PAD : f.r.x + TITLE_START;
  const w = Math.max(0, f.r.w - TITLE_START - END_PAD);
  const color = textColor(step, f);
  const align = f.rtl ? { align: "end" as const } : {};
  out.texts.push({ text: step.title, x, y, w, style: f.title, color, ...align });
  if (step.subtitle !== undefined) {
    out.texts.push({ text: step.subtitle, x, y: y + titleH + 2, w, style: f.subtitle, color, ...align });
  }
}

interface Out {
  boxes: BoxPaint[];
  texts: PaintText[];
  glyphs: PlacedGlyph[];
}

/**
 * A horizontal `Stepper` (build log row 295): each step's circle and title where the layout put them (its parts
 * `hcircle<i>`, `htitle<i>`), and the connectors between them (`hline<i>`).
 */
function horizontalPaint(steps: readonly Step[], f: Frame, part: (name: string) => Rect | undefined): StepperPaint {
  const out: Out = { boxes: [], texts: [], glyphs: [] };
  steps.forEach((step, index) => {
    const circle = part(`hcircle${index}`);
    if (circle !== undefined) {
      drawCircle(step, index, circle.x + circle.w / 2, circle.y + circle.h / 2, f, out);
    }
    const title = part(`htitle${index}`);
    if (title !== undefined) {
      out.texts.push({
        text: step.title,
        x: title.x,
        y: title.y,
        w: title.w + 1,
        style: f.title,
        color: textColor(step, f),
      });
    }
    const line = part(`hline${index}`);
    if (line !== undefined) {
      out.boxes.push({ ...line, fill: LINE_COLOR });
    }
  });
  return out;
}

/**
 * The paint of a `Stepper` in `r`; undefined when it has no steps, or runs across and its parts are not laid out
 * (`part` gives the rect of one, by name).
 */
export function stepperPaint(
  props: Obj,
  r: Rect,
  theme: PaintTheme,
  part?: (name: string) => Rect | undefined,
): StepperPaint | undefined {
  const steps = stepsOf(props);
  if (steps.length === 0 || (props["type"] === "horizontal" && part === undefined)) {
    return undefined;
  }
  const scale = typeScaleOf(theme);
  const bodySmall = scale.bodySmall ?? M3_TYPE_SCALE.bodySmall;
  const f: Frame = {
    r,
    rtl: theme.rtl === true,
    dark: theme.scheme.brightness === "dark",
    theme,
    title: { ...(scale.bodyLarge ?? M3_TYPE_SCALE.bodyLarge), italic: false },
    subtitle: { ...bodySmall, italic: false },
    number: { ...bodySmall, size: 12, height: 1, letterSpacing: 0, italic: false },
  };
  if (props["type"] === "horizontal" && part !== undefined) {
    return horizontalPaint(steps, f, part);
  }
  const current = typeof props["currentStep"] === "number" ? props["currentStep"] : 0;
  const cx = f.rtl ? r.x + r.w - MARGIN - CIRCLE / 2 : r.x + MARGIN + CIRCLE / 2;
  const line = (y: number, h: number): BoxPaint => ({ x: cx - 0.5, y, w: 1, h, fill: LINE_COLOR });
  const out: Out = { boxes: [], texts: [], glyphs: [] };
  let top = r.y;
  steps.forEach((step, index) => {
    const last = index === steps.length - 1;
    if (index > 0) {
      out.boxes.push(line(top, LINE));
    }
    if (!last) {
      out.boxes.push(line(top + HEADER - LINE, LINE));
    }
    drawCircle(step, index, cx, top + HEADER / 2, f, out);
    drawTitle(step, top, f, out);
    top += HEADER;
    if (index === current) {
      if (!last) {
        out.boxes.push(line(top, CONTENT));
      }
      top += CONTENT;
    }
  });
  return out;
}

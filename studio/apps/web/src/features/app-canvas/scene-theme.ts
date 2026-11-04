// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ArrowStyle, ColorRgba, GridStyle } from "@plux/canvas";
import { useEffect, useState } from "react";
import { arrowStyle } from "../plugin-canvas/scene.ts";
import { type AppSceneStyle, appSceneStyle } from "./app-canvas.scene.ts";

/** The colours the canvas worker draws the app canvas with, taken from the page's theme. */
export interface SceneTheme {
  /** A key that changes when the theme does, so the surface is made again with the new background. */
  readonly key: string;
  readonly background: ColorRgba;
  /** The dot grid on the background (STU-032): what boxes snap and align on. */
  readonly grid: GridStyle;
  readonly style: AppSceneStyle;
  readonly arrows: ArrowStyle;
}

/** The grid's geometry: a dot every 24 world units, 1 CSS pixel in radius, never closer than 14 CSS pixels. */
const GRID = { spacing: 24, radius: 1, minGap: 14 } as const;

/** The light theme's colours, for a page that cannot read its own (tests, a browser without a 2D canvas). */
export const fallbackTheme: SceneTheme = {
  key: "fallback",
  background: 0xf4f4f5ff,
  grid: { ...GRID, color: 0xd4d4d8ff },
  style: appSceneStyle,
  arrows: arrowStyle,
};

/**
 * Reads CSS colours as RGBA through a one-pixel 2D canvas: the browser resolves any CSS colour (OKLCH, `color-mix`,
 * system colours under forced colours) to sRGB bytes, which is what the canvas worker takes.
 */
function colourReader(): ((css: string, alpha?: number) => ColorRgba) | undefined {
  if (typeof document === "undefined") {
    return undefined;
  }
  let context: CanvasRenderingContext2D | null = null;
  try {
    const canvas = document.createElement("canvas");
    canvas.width = 1;
    canvas.height = 1;
    context = canvas.getContext("2d", { willReadFrequently: true });
  } catch {
    return undefined;
  }
  if (context === null || typeof context.getImageData !== "function") {
    return undefined;
  }
  const ctx = context;
  return (css, alpha) => {
    ctx.clearRect(0, 0, 1, 1);
    ctx.fillStyle = css;
    ctx.fillRect(0, 0, 1, 1);
    const [r = 0, g = 0, b = 0, a = 255] = ctx.getImageData(0, 0, 1, 1).data;
    return ((r << 24) | (g << 16) | (b << 8) | (alpha ?? a)) >>> 0;
  };
}

/**
 * The scene's colours from the theme's tokens on `root` (ADR-0103): the canvas a step off the page with a quiet dot
 * grid, boxes on the card colour with the border, the plugin's solid mark in the primary, titles in the foreground,
 * keys muted, the focus ring and selection in the ring colour, the pills in the status colours, and arrows in the
 * muted foreground with the primary for the focused bundle.
 */
export function readSceneTheme(root: HTMLElement): SceneTheme {
  const colour = colourReader();
  const styles = getComputedStyle(root);
  const token = (name: string) => styles.getPropertyValue(`--${name}`).trim();
  if (colour === undefined || token("card") === "") {
    return fallbackTheme;
  }
  const mix = (name: string, percent: number, base = "card") =>
    colour(`color-mix(in oklch, ${token(name)} ${percent}%, ${token(base)})`);
  const pill = (status: "success" | "warning" | "danger") => ({
    fill: colour(token(`color-${status}-surface`) || token("muted")),
    text: colour(token(`color-${status}`)),
    stroke: mix(`color-${status}`, 35),
  });
  const style: AppSceneStyle = {
    fill: colour(token("card")),
    archivedFill: colour(token("muted")),
    nativeFill: mix("primary", 10),
    stroke: colour(token("border")),
    title: colour(token("card-foreground")),
    secondary: colour(token("muted-foreground")),
    divider: colour(token("border")),
    tile: colour(token("primary")),
    tileText: colour(token("primary-foreground")),
    mutedTile: colour(token("muted")),
    mutedTileText: colour(token("muted-foreground")),
    frameFill: colour(`color-mix(in oklch, ${token("foreground")} 3%, transparent)`),
    frameStroke: mix("muted-foreground", 35, "background"),
    focus: colour(token("ring")),
    pills: {
      neutral: { fill: colour(token("muted")), text: colour(token("foreground")), stroke: colour(token("border")) },
      success: pill("success"),
      warning: pill("warning"),
      danger: pill("danger"),
    },
  };
  const line = colour(token("muted-foreground"));
  const arrows: ArrowStyle = {
    ...arrowStyle,
    stroke: { color: line, width: 1.5 },
    dimmed: { color: (line & 0xffffff00) | 0x50, width: 1.5 },
    focus: { color: colour(token("primary")), width: 2 },
    badgeFill: colour(token("primary")),
    badgeText: colour(token("primary-foreground")),
    labelText: colour(token("foreground")),
  };
  // A step darker than the page in light (white cards stand off it) and a step lighter in dark; the grid's dots a
  // quiet step further, so the canvas reads as a surface without competing with the cards on it.
  const background = mix("muted-foreground", 7, "background");
  const grid: GridStyle = { ...GRID, color: mix("muted-foreground", 30, "background") };
  const key = [background, grid.color, style.fill, style.title, line].join(":");
  return { key, background, grid, style, arrows };
}

/** The scene theme of the page, read again whenever the theme or the system's colour scheme changes. */
export function useSceneTheme(): SceneTheme {
  const [theme, setTheme] = useState<SceneTheme>(() =>
    typeof document === "undefined" ? fallbackTheme : readSceneTheme(document.documentElement),
  );
  useEffect(() => {
    const root = document.documentElement;
    const update = () =>
      setTheme((was) => {
        const next = readSceneTheme(root);
        return next.key === was.key ? was : next;
      });
    const observer = typeof MutationObserver === "undefined" ? undefined : new MutationObserver(update);
    observer?.observe(root, { attributes: true, attributeFilter: ["data-theme", "class", "style"] });
    const scheme = window.matchMedia?.("(prefers-color-scheme: dark)");
    scheme?.addEventListener?.("change", update);
    return () => {
      observer?.disconnect();
      scheme?.removeEventListener?.("change", update);
    };
  }, []);
  return theme;
}

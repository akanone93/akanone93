// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Where a canvas was left, per browser tab (build log #249): coming back to the app canvas from a plugin, or to a
 * plugin from the app canvas, shows exactly the place and zoom the person left, not the opening view again. The tab's
 * sessionStorage holds only numbers under the canvas's key; it goes with the tab, and a browser that blocks storage
 * simply opens canvases at their opening view.
 */

/** A canvas's view: the canvas point at the centre of the view, and the zoom (1 is 100 %). */
export interface CanvasView {
  readonly x: number;
  readonly y: number;
  readonly zoom: number;
}

const PREFIX = "plux.canvas-view.";

/** The key of a canvas: the app canvas of an app, or one plugin's canvas. */
export function canvasViewKey(org: string, app: string, plugin?: string): string {
  return plugin === undefined ? `${PREFIX}${org}.${app}` : `${PREFIX}${org}.${app}.${plugin}`;
}

function tab(): Storage | undefined {
  try {
    return globalThis.sessionStorage;
  } catch {
    return undefined;
  }
}

/** The view the canvas was left at in this tab, or undefined. A stored value that is not a view is ignored. */
export function rememberedView(key: string): CanvasView | undefined {
  try {
    const raw = tab()?.getItem(key);
    if (raw === null || raw === undefined) {
      return undefined;
    }
    const v: unknown = JSON.parse(raw);
    if (typeof v !== "object" || v === null) {
      return undefined;
    }
    const { x, y, zoom } = v as Record<string, unknown>;
    if (
      typeof x !== "number" ||
      typeof y !== "number" ||
      typeof zoom !== "number" ||
      !Number.isFinite(x) ||
      !Number.isFinite(y) ||
      !Number.isFinite(zoom) ||
      zoom <= 0
    ) {
      return undefined;
    }
    return { x, y, zoom };
  } catch {
    return undefined;
  }
}

/** Keeps the view a canvas is at, for this tab. A view that is not finite is not kept. */
export function rememberView(key: string, view: CanvasView): void {
  if (!(Number.isFinite(view.x) && Number.isFinite(view.y) && Number.isFinite(view.zoom) && view.zoom > 0)) {
    return;
  }
  try {
    tab()?.setItem(key, JSON.stringify({ x: view.x, y: view.y, zoom: view.zoom }));
  } catch {
    // Storage full or blocked: the canvas opens at its opening view next time.
  }
}

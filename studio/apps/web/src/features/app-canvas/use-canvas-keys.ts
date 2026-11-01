// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { type MutableRefObject, type RefObject, useEffect, useRef } from "react";
import type { CanvasTool } from "./use-canvas-gestures.ts";

export interface CanvasKeys {
  readonly canvas: RefObject<HTMLCanvasElement | null>;
  /** Whether Space is held; the gestures pan while it is. */
  readonly space: MutableRefObject<boolean>;
  readonly fit: () => void;
  readonly zoomBy: (factor: number) => void;
  readonly resetZoom: () => void;
  readonly setTool: (tool: CanvasTool) => void;
  readonly clear: () => void;
}

/** Typing in a field or working in a dialog keeps its keys. */
const typing = (t: EventTarget | null): boolean =>
  t instanceof HTMLElement &&
  (t.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(t.tagName) || t.closest("[role=dialog]") !== null);

/** The action of a key with Ctrl or Cmd: zoom in, out, or back to 100%. */
function withModifier(e: KeyboardEvent, keys: CanvasKeys): (() => void) | undefined {
  if (e.key === "=" || e.key === "+") {
    return () => keys.zoomBy(1.25);
  }
  if (e.key === "-") {
    return () => keys.zoomBy(0.8);
  }
  return e.key === "0" ? keys.resetZoom : undefined;
}

/** The action of a plain key: the tools, zoom, fit and clearing the selection. */
function plain(e: KeyboardEvent, keys: CanvasKeys): (() => void) | undefined {
  const k = e.key.toLowerCase();
  const actions: Record<string, () => void> = {
    v: () => keys.setTool("select"),
    h: () => keys.setTool("hand"),
    "+": () => keys.zoomBy(1.25),
    "=": () => keys.zoomBy(1.25),
    "-": () => keys.zoomBy(0.8),
    escape: keys.clear,
  };
  if (e.shiftKey && (e.key === "!" || e.code === "Digit1")) {
    return keys.fit;
  }
  return actions[k];
}

/**
 * The canvas's keys, as design tools have them: V and H pick the tool, Space held pans, + and - zoom, Shift+1 fits,
 * Ctrl+0 resets the zoom and Escape clears the selection. Typing in a field or a dialog is left alone.
 */
export function useCanvasKeys(input: CanvasKeys): void {
  const keys = useRef(input);
  keys.current = input;
  useEffect(() => {
    const down = (e: KeyboardEvent) => {
      if (typing(e.target) || e.altKey) {
        return;
      }
      const current = keys.current;
      if (e.key === " " && !(e.target instanceof HTMLButtonElement)) {
        e.preventDefault();
        current.space.current = true;
        current.canvas.current?.setAttribute("data-space", "true");
        return;
      }
      const action = e.ctrlKey || e.metaKey ? withModifier(e, current) : plain(e, current);
      if (action !== undefined) {
        e.preventDefault();
        action();
      }
    };
    const up = (e: KeyboardEvent) => {
      if (e.key === " ") {
        keys.current.space.current = false;
        keys.current.canvas.current?.removeAttribute("data-space");
      }
    };
    window.addEventListener("keydown", down);
    window.addEventListener("keyup", up);
    return () => {
      window.removeEventListener("keydown", down);
      window.removeEventListener("keyup", up);
    };
  }, []);
}

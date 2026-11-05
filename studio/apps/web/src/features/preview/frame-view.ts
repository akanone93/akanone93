// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useCallback, useState } from "react";
import type { Orientation } from "./frames.ts";

/**
 * How the device frame is shown (STU-057, PC-11): its orientation and whether its chrome (status bar, cutout, home
 * indicator) is drawn. A viewer's convenience, kept in this browser only; the stored preview switches are unchanged.
 */
export interface FrameView {
  readonly orientation: Orientation;
  readonly chrome: boolean;
}

export const defaultFrameView: FrameView = { orientation: "portrait", chrome: true };

const KEY = "plux.preview.frameView";

function storage(): Storage | undefined {
  try {
    return globalThis.localStorage;
  } catch {
    return undefined;
  }
}

/** The stored view, or the default when none is stored, it is unreadable or storage is blocked. */
export function readFrameView(store: Storage | undefined = storage()): FrameView {
  try {
    const raw = JSON.parse(store?.getItem(KEY) ?? "null") as Partial<FrameView> | null;
    return {
      orientation: raw?.orientation === "landscape" ? "landscape" : "portrait",
      chrome: raw?.chrome !== false,
    };
  } catch {
    return defaultFrameView;
  }
}

/** The frame view and its setter, remembered in this browser. */
export function useFrameView(): readonly [FrameView, (next: FrameView) => void] {
  const [view, setView] = useState(readFrameView);
  const set = useCallback((next: FrameView) => {
    setView(next);
    try {
      storage()?.setItem(KEY, JSON.stringify(next));
    } catch {
      // Storage blocked: the view lasts for this page.
    }
  }, []);
  return [view, set] as const;
}

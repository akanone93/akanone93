// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Spinner } from "@plux/ui/components/spinner";
import { type ReactNode, useRef } from "react";

/** What a canvas's first load waits for (#211). */
export interface FirstContent {
  /** The content failed to load: the canvas says so instead. */
  readonly failed: boolean;
  readonly loaded: boolean;
  /** How many things the canvas draws: with none there is nothing to lay out. */
  readonly items: number;
  /** The things are laid out and drawn; `linked` too when the layout waits for their links. */
  readonly drawn: boolean;
  readonly linked?: boolean;
}

/** The first content is in: it failed, or it loaded and there is nothing to draw or it is drawn. */
export function firstContentIn(c: FirstContent): boolean {
  return c.failed || (c.loaded && (c.items === 0 || (c.drawn && c.linked !== false)));
}

/**
 * Whether a canvas is still on its first load on this page: true until its first content is in once, then false for
 * good, so a later refresh of the same content never brings the loading state back (#211).
 */
export function useFirstLoad(content: FirstContent): boolean {
  const done = useRef(false);
  if (firstContentIn(content)) {
    done.current = true;
  }
  return !done.current;
}

/**
 * A canvas's first load (#211): a small glass pill in the middle of the stage while its content loads and is laid out,
 * and only then. It takes no pointer and covers nothing, fades in after a moment so a quick load shows nothing, and is
 * announced politely.
 */
export function CanvasLoading({ label, shown }: { readonly label: string; readonly shown: boolean }): ReactNode {
  return shown ? (
    <div role="status" className="canvas-loading canvas-panel glass">
      <Spinner
        aria-hidden="true"
        role="presentation"
        aria-label={undefined}
        className="size-3.5 motion-reduce:animate-none"
      />
      <span>{label}</span>
    </div>
  ) : null;
}

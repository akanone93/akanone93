// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { z } from "zod";

/** Zoom bounds in percent (ADR-0013). */
export const MIN_ZOOM_PERCENT = 10;
export const MAX_ZOOM_PERCENT = 400;

const finite = z.number().refine(Number.isFinite);

/**
 * C2 search: `?x&y&z`, the viewport centre in world units and the zoom in percent (10-400).
 * Bad values fall back to "fit the content" rather than failing the page.
 */
export const canvasSearchSchema = z.object({
  x: finite.optional().catch(undefined),
  y: finite.optional().catch(undefined),
  /** A screen to pan to and pulse once the canvas opens (STU-053). */
  focus: z.string().optional().catch(undefined),
  z: finite
    .transform((v) => Math.min(MAX_ZOOM_PERCENT, Math.max(MIN_ZOOM_PERCENT, v)))
    .optional()
    .catch(undefined),
});

export type CanvasSearch = z.infer<typeof canvasSearchSchema>;

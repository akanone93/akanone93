// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/** Device frames and the device matrix (STU-057). */

import { limitDefault } from "@plux/schema";

export type FramePlatform = "material" | "cupertino";
export type FrameKind = "phone" | "tablet" | "desktop";
export type SizeClass = "compact" | "medium" | "expanded";
export type Orientation = "portrait" | "landscape";

export interface DeviceFrame {
  readonly id: string;
  readonly kind: FrameKind;
  readonly platform: FramePlatform;
  /** Logical pixels, portrait. */
  readonly width: number;
  readonly height: number;
  readonly density: number;
}

/** Presets named by STU-057, plus a tablet and a desktop window. */
export const frameCatalogue: readonly DeviceFrame[] = [
  { id: "android-small", kind: "phone", platform: "material", width: 360, height: 800, density: 2 },
  { id: "pixel", kind: "phone", platform: "material", width: 412, height: 915, density: 2.625 },
  { id: "galaxy-a", kind: "phone", platform: "material", width: 384, height: 854, density: 2.8125 },
  { id: "android-entry", kind: "phone", platform: "material", width: 320, height: 640, density: 2 },
  { id: "iphone-se", kind: "phone", platform: "cupertino", width: 375, height: 667, density: 2 },
  { id: "iphone", kind: "phone", platform: "cupertino", width: 390, height: 844, density: 3 },
  { id: "iphone-pro-max", kind: "phone", platform: "cupertino", width: 430, height: 932, density: 3 },
  { id: "foldable", kind: "phone", platform: "material", width: 673, height: 841, density: 2.625 },
  { id: "ipad", kind: "tablet", platform: "cupertino", width: 820, height: 1180, density: 2 },
  { id: "android-tablet", kind: "tablet", platform: "material", width: 800, height: 1280, density: 2 },
  { id: "desktop", kind: "desktop", platform: "material", width: 1280, height: 800, density: 1 },
];

export const defaultFrameId = "android-small";

/** Material window size classes: compact below 600, medium below 840, expanded from 840 (WGT-010). */
export function sizeClassOf(width: number): SizeClass {
  if (width < 600) {
    return "compact";
  }
  return width < 840 ? "medium" : "expanded";
}

/** Frame size in the given orientation. */
export function orientedSize(frame: Pick<DeviceFrame, "width" | "height">, orientation: Orientation) {
  return orientation === "portrait"
    ? { width: frame.width, height: frame.height }
    : { width: frame.height, height: frame.width };
}

export interface CustomFrameInput {
  readonly width: number;
  readonly height: number;
  readonly density: number;
}

/** The limit a custom frame breaks: its size, or its pixel density (D40). */
export type CustomFrameRefusal = "size" | "density";

/** The largest width or height and the largest density of a custom frame (studio.customFrame*). */
export function customFrameLimits(): { readonly maxPixels: number; readonly maxDensity: number } {
  return {
    maxPixels: limitDefault("studio.customFrameMaxPixels"),
    maxDensity: limitDefault("studio.customFrameMaxDensity"),
  };
}

/** The limit the frame breaks, if any; the first one checked is reported. */
export function customFrameRefusal(input: CustomFrameInput): CustomFrameRefusal | undefined {
  const { maxPixels, maxDensity } = customFrameLimits();
  if (input.width > maxPixels || input.height > maxPixels) {
    return "size";
  }
  if (input.density < 1 || input.density > maxDensity) {
    return "density";
  }
  return undefined;
}

/**
 * A custom frame (D40): whole logical pixels for width and height, and a density from 1 up to the
 * configured maximum. A frame beyond studio.customFrameMaxPixels or studio.customFrameMaxDensity is refused.
 */
export function customFrame(id: string, input: CustomFrameInput): DeviceFrame | "invalid" {
  const { width, height, density } = input;
  const ok = Number.isInteger(width) && Number.isInteger(height) && width > 0 && height > 0;
  if (!ok || !Number.isFinite(density) || customFrameRefusal(input) !== undefined) {
    return "invalid";
  }
  return {
    id,
    kind: width >= 840 ? "desktop" : width >= 600 ? "tablet" : "phone",
    platform: "material",
    width,
    height,
    density,
  };
}

/** Looks a frame up by ID in the catalogue, then among the user's custom frames. */
export function resolveFrame(id: string, custom: readonly DeviceFrame[] = []): DeviceFrame {
  const all = [...frameCatalogue, ...custom];
  return all.find((f) => f.id === id) ?? (all.find((f) => f.id === defaultFrameId) as DeviceFrame);
}

/** The stored device_frame string: a preset ID, or "custom:WxH@D" for a custom frame. */
export function encodeCustom(frame: Pick<DeviceFrame, "width" | "height" | "density">): string {
  return `custom:${frame.width}x${frame.height}@${frame.density}`;
}

export function decodeCustom(value: string): DeviceFrame | undefined {
  const m = /^custom:(\d+)x(\d+)@([0-9.]+)$/.exec(value);
  if (m === null) {
    return undefined;
  }
  const made = customFrame(value, { width: Number(m[1]), height: Number(m[2]), density: Number(m[3]) });
  return made === "invalid" ? undefined : made;
}

export interface MatrixCell {
  readonly frame: DeviceFrame;
  readonly width: number;
  readonly height: number;
  readonly sizeClass: SizeClass;
  /** Left edge of the cell when the frames sit in one row with the given gap. */
  readonly x: number;
}

/** The device matrix: one screen in several frames, side by side in the order given. */
export function matrixLayout(frames: readonly DeviceFrame[], orientation: Orientation, gap = 24): MatrixCell[] {
  let x = 0;
  return frames.map((frame) => {
    const { width, height } = orientedSize(frame, orientation);
    const cell = { frame, width, height, sizeClass: sizeClassOf(width), x };
    x += width + gap;
    return cell;
  });
}

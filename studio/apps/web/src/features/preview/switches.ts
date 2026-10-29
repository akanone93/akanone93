// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { create } from "@bufbuild/protobuf";
import { type PreviewSwitches, PreviewSwitchesSchema } from "@plux/api";
import { type Direction, directionOf, pseudoAccent, pseudoMirror } from "@plux/i18n";
import type { FramePlatform } from "./frames.ts";

/** Canvas-wide preview switches (STU-058); they change how screens look while designing, not the document. */

export type PreviewTheme = "light" | "dark";
export type MockState = "success" | "loading" | "empty" | "error";

export const mockStates: readonly MockState[] = ["success", "loading", "empty", "error"];
export const textScales: readonly number[] = [0.85, 1, 1.15, 1.3, 1.5, 2];

export interface PreviewState {
  readonly deviceFrame: string;
  readonly theme: PreviewTheme;
  readonly brandOverlay: string;
  readonly locale: string;
  readonly textScale: number;
  /** Empty follows the device frame. */
  readonly platform: FramePlatform | "";
  readonly mockStates: Readonly<Record<string, MockState>>;
}

export const defaultPreview: PreviewState = {
  deviceFrame: "android-small",
  theme: "light",
  brandOverlay: "",
  locale: "en",
  textScale: 1,
  platform: "",
  mockStates: {},
};

const asMock = (v: string): MockState => ((mockStates as readonly string[]).includes(v) ? (v as MockState) : "success");

/** Reads the stored switches of one app; unknown values fall back to the defaults. */
export function readPreview(stored: PreviewSwitches | undefined): PreviewState {
  if (stored === undefined) {
    return defaultPreview;
  }
  const scale = stored.textScale;
  return {
    deviceFrame: stored.deviceFrame || defaultPreview.deviceFrame,
    theme: stored.theme === "dark" ? "dark" : "light",
    brandOverlay: stored.brandOverlay,
    locale: stored.locale || defaultPreview.locale,
    textScale: scale >= 0.5 && scale <= 3 ? scale : 1,
    platform: stored.platform === "material" || stored.platform === "cupertino" ? stored.platform : "",
    mockStates: Object.fromEntries(Object.entries(stored.mockStates).map(([k, v]) => [k, asMock(v)])),
  };
}

export function writePreview(state: PreviewState): PreviewSwitches {
  return create(PreviewSwitchesSchema, {
    deviceFrame: state.deviceFrame,
    theme: state.theme,
    brandOverlay: state.brandOverlay,
    locale: state.locale,
    textScale: state.textScale,
    platform: state.platform,
    mockStates: { ...state.mockStates },
  });
}

/** What the canvas render applies from the switches. */
export interface PreviewRender {
  readonly direction: Direction;
  readonly textScale: number;
  readonly theme: PreviewTheme;
  readonly platform: FramePlatform;
  readonly locale: string;
  readonly brandOverlay: string;
  /** The mock state selected for each data source (DAT-080). */
  readonly mockStates: Readonly<Record<string, MockState>>;
  /** Transforms a catalogue string for the pseudo-locales (I18N-010); other locales leave it unchanged. */
  readonly localize: (text: string) => string;
}

export function previewRender(state: PreviewState, framePlatform: FramePlatform): PreviewRender {
  const localize =
    state.locale === "en-XA" ? pseudoAccent : state.locale === "ar-XB" ? pseudoMirror : (text: string) => text;
  return {
    direction: directionOf(state.locale),
    textScale: state.textScale,
    theme: state.theme,
    platform: state.platform === "" ? framePlatform : state.platform,
    locale: state.locale,
    brandOverlay: state.brandOverlay,
    mockStates: state.mockStates,
    localize,
  };
}

/** Mock state of one data source; sources without an entry succeed. */
export function mockStateOf(state: PreviewState, source: string): MockState {
  return state.mockStates[source] ?? "success";
}

export function withMock(state: PreviewState, source: string, mock: MockState): PreviewState {
  return { ...state, mockStates: { ...state.mockStates, [source]: mock } };
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ScreenStyle } from "@plux/canvas";
import { useMemo } from "react";
import type { PreviewRender } from "../preview/switches.ts";
import { localizeTree, lookedStyle, messagesFor, resolveTheme } from "./look.ts";
import type { PlacedScreen } from "./scene.ts";
import { screenStyle } from "./scene.ts";

interface DocLike {
  readonly kind: string;
  readonly content: Uint8Array;
}

/** What the preview switches draw (STU-058): the screens with translated texts and the style with theme, scale and direction. */
export function usePreviewScene(
  screens: readonly PlacedScreen[],
  appDocs: readonly DocLike[] | undefined,
  pluginDocs: readonly DocLike[] | undefined,
  look: PreviewRender,
): { shown: readonly PlacedScreen[]; styled: ScreenStyle } {
  const everything = useMemo(() => [...(appDocs ?? []), ...(pluginDocs ?? [])], [appDocs, pluginDocs]);
  const styled = useMemo(() => lookedStyle(screenStyle, resolveTheme(everything, look), look), [everything, look]);
  const shown = useMemo(() => {
    const messages = messagesFor(everything, look);
    return screens.map((s) => ({ ...s, root: localizeTree(s.root, messages, look.localize) }));
  }, [screens, everything, look]);
  return { shown, styled };
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ScreenStyle } from "@plux/canvas";
import { useMemo, useRef } from "react";
import type { PreviewRender } from "../preview/switches.ts";
import { localizeTree, lookedStyle, messagesFor, resolveTheme } from "./look.ts";
import type { Node } from "./plugin-model.ts";
import type { PlacedScreen } from "./scene.ts";
import { screenStyle } from "./scene.ts";

interface DocLike {
  readonly kind: string;
  readonly content: Uint8Array;
}

interface LocalizedRoots {
  readonly everything: readonly DocLike[];
  readonly look: PreviewRender;
  readonly byRoot: WeakMap<Node, Node | undefined>;
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
  const cache = useRef<LocalizedRoots | undefined>(undefined);
  const shown = useMemo(() => {
    const kept = cache.current;
    const byRoot =
      kept !== undefined && kept.everything === everything && kept.look === look
        ? kept.byRoot
        : new WeakMap<Node, Node | undefined>();
    cache.current = { everything, look, byRoot };
    const messages = messagesFor(everything, look);
    return screens.map((s) => {
      if (s.root === undefined) {
        return s;
      }
      if (!byRoot.has(s.root)) {
        byRoot.set(s.root, localizeTree(s.root, messages, look.localize));
      }
      return { ...s, root: byRoot.get(s.root) };
    });
  }, [screens, everything, look]);
  return { shown, styled };
}

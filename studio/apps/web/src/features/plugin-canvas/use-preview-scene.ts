// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ScreenStyle } from "@plux/canvas";
import type { PaintTheme } from "@plux/layout";
import { useMemo, useRef } from "react";
import { useSceneTheme } from "../app-canvas/scene-theme.ts";
import { withoutHidden } from "../panels/studio-meta.ts";
import type { PreviewRender } from "../preview/switches.ts";
import { dataSourcesOf, localizeTree, lookedStyle, messagesFor, resolveTheme } from "./look.ts";
import { bindMocks } from "./mock-bindings.ts";
import { paintedStyle, paintThemeOf, themeTokens } from "./paint-theme.ts";
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

/** What the preview switches draw (STU-058, DAT-080): the screens with translated texts, data bound to the selected mock states, and the style with theme, scale and direction. */
export function usePreviewScene(
  screens: readonly PlacedScreen[],
  appDocs: readonly DocLike[] | undefined,
  pluginDocs: readonly DocLike[] | undefined,
  look: PreviewRender,
): { shown: readonly PlacedScreen[]; styled: ScreenStyle; paint: PaintTheme } {
  const everything = useMemo(() => [...(appDocs ?? []), ...(pluginDocs ?? [])], [appDocs, pluginDocs]);
  // Each screen's name above its frame takes the canvas theme's label colour, so it reads on a light and a dark canvas.
  const title = useSceneTheme().arrows.labelText;
  // The screens are painted in the theme's colours for the previewed mode and brand (STU-005, THM-001–THM-003).
  const tokens = useMemo(() => themeTokens(everything), [everything]);
  const paint = useMemo(
    () => paintThemeOf(tokens, look.theme === "dark", look.brandOverlay),
    [tokens, look.theme, look.brandOverlay],
  );
  const styled = useMemo(
    () => ({ ...paintedStyle(lookedStyle(screenStyle, resolveTheme(everything, look), look), paint), title }),
    [everything, paint, look, title],
  );
  const cache = useRef<LocalizedRoots | undefined>(undefined);
  const shown = useMemo(() => {
    const kept = cache.current;
    const byRoot =
      kept !== undefined && kept.everything === everything && kept.look === look
        ? kept.byRoot
        : new WeakMap<Node, Node | undefined>();
    cache.current = { everything, look, byRoot };
    const messages = messagesFor(everything, look);
    const mocks = new Map(dataSourcesOf(everything).map((src) => [src.id, src.mock]));
    return screens.map((s) => {
      if (s.root === undefined) {
        return s;
      }
      if (!byRoot.has(s.root)) {
        // A layer hidden in Layers is left out of what the canvas draws (STU-061); the device still shows it.
        const drawn = withoutHidden(s.root);
        byRoot.set(s.root, bindMocks(localizeTree(drawn, messages, look.localize), mocks, look.mockStates));
      }
      return { ...s, root: byRoot.get(s.root) };
    });
  }, [screens, everything, look]);
  return { shown, styled, paint };
}

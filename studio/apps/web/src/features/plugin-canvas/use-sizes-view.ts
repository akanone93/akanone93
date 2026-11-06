// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The Size classes view of the canvas (PC-33, WGT-010): each screen's window size class and width, the widgets a
 * size-class layer changes there, the widgets shown only in some size classes, and on a foldable the controls and
 * images that cross the fold — worked out from the drawn screens and, for the fold, their laid-out boxes.
 */

import type { I18n } from "@lingui/core";
import { type SizeClass, sizeClassOf } from "@plux/layout";
import { useMemo } from "react";
import type { PreviewDevice } from "../preview/switches.ts";
import { pluginCanvasMessages } from "./plugin-canvas.messages.ts";
import type { Node } from "./plugin-model.ts";
import { foldMarks, type SizeMark, type SizesOverlay, sizeMarks } from "./responsive.ts";
import type { Box } from "./select/geometry.ts";

export function useSizesView(input: {
  readonly on: boolean;
  readonly screens: readonly { readonly id: string; readonly root?: Node | undefined }[];
  readonly device: PreviewDevice;
  readonly types: ReadonlyMap<string, string>;
  readonly screenOf: ReadonlyMap<string, string>;
  readonly boxOf: () => ((id: string) => Box | undefined) | undefined;
  /** Changes when the screens are laid out again, so the fold is checked against the new boxes. */
  readonly laidOut: unknown;
  readonly i18n: I18n;
}): SizesOverlay | undefined {
  const { on, screens, device, types, screenOf, boxOf, laidOut, i18n } = input;
  return useMemo(() => {
    if (!on) {
      return undefined;
    }
    const m = pluginCanvasMessages(i18n);
    const sc = sizeClassOf(device.width);
    const name = (c: SizeClass) => m.sizeClassName[c];
    const marks = sizeMarks(
      screens.map((s) => s.root),
      sc,
    );
    const box = boxOf();
    if (device.fold !== undefined && box !== undefined && laidOut !== undefined) {
      const rootOf = new Map(screens.map((s) => [s.id, s.root?.id]));
      const boxes = new Map([...types.keys()].flatMap((id) => (box(id) === undefined ? [] : [[id, box(id) as Box]])));
      const screenBox = (id: string) => {
        const root = rootOf.get(screenOf.get(id) ?? "");
        return root === undefined ? undefined : box(root);
      };
      for (const id of foldMarks(boxes, types, screenBox, device.fold)) {
        marks.set(id, { kind: "fold" });
      }
    }
    const text = (mark: SizeMark): string =>
      mark.kind === "layer"
        ? m.sizeLayer(name(mark.layer), mark.props.join(", "))
        : mark.kind === "only"
          ? m.sizeOnly(mark.classes.map(name).join(", "))
          : m.sizeFold;
    const label = m.sizeScreen(name(sc), Math.round(device.width));
    return {
      marks,
      screens: screens.flatMap((s) => (s.root === undefined ? [] : [{ root: s.root.id, label }])),
      text,
    };
  }, [on, screens, device, types, screenOf, boxOf, laidOut, i18n]);
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { Document } from "@plux/api";
import { Button, Icon, Popover } from "@plux/ui";
import { type ReactNode, useMemo } from "react";
import { readLocales } from "../localisation/l10n-model.ts";
import { dataSourcesOf, jsonOf } from "../plugin-canvas/look.ts";
import { useBrands } from "../plugin-canvas/paint-theme.ts";
import { pluginCanvasMessages } from "../plugin-canvas/plugin-canvas.messages.ts";
import { usePreviewBar } from "../preview/preview-bar.tsx";

/**
 * The preview switches on the app canvas (#286): the device, locale, light or dark, size class and text scale every
 * plugin canvas of the app draws its screens in. They are one set per person and app, stored in the person's
 * preferences (STU-058), so setting them here once shows each plugin in that look, and the dev session's device tools
 * follow them.
 */
export function AppPreview(props: {
  readonly appId: string;
  readonly project: readonly Document[] | undefined;
}): ReactNode {
  const docs = props.project;
  const sources = useMemo(() => dataSourcesOf(docs ?? []), [docs]);
  const sourceIds = useMemo(() => sources.map((s) => s.id), [sources]);
  const sourceNames = useMemo(() => Object.fromEntries(sources.map((s) => [s.id, s.name])), [sources]);
  const brands = useBrands(docs);
  const locales = useMemo(() => readLocales(jsonOf(docs ?? [], "app")[0])?.supported ?? [], [docs]);
  const { switcher } = usePreviewBar(props.appId, sourceIds, brands, sourceNames, locales);
  const label = pluginCanvasMessages(useLingui().i18n).preview;
  return (
    <Popover
      title={label}
      trigger={
        <Button size="sm" variant="ghost" aria-label={label}>
          <Icon name="monitor" size="capsule" />
          <span>{label}</span>
          <Icon name="caret-down" size="pill" />
        </Button>
      }
    >
      <div className="flex flex-col gap-3">{switcher}</div>
    </Popover>
  );
}

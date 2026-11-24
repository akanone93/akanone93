// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { PluginSummary } from "@plux/api";
import { Icon } from "@plux/ui";
import { Button } from "@plux/ui/components/button";
import { InputGroup, InputGroupAddon, InputGroupInput } from "@plux/ui/components/input-group";
import { type ReactNode, useMemo, useState } from "react";
import { buildWhereUsed, type LoadedDoc } from "../safety/index.ts";
import { appCanvasMessages } from "./app-canvas.messages.ts";
import { type Hit, type HitKind, type Place, type SearchInput, searchApp, searchables } from "./app-search.ts";

/** How many hits of a group are listed before "N more". */
const SHOWN_PER_GROUP = 12;

/** Where a hit opens: a plugin on the canvas, or a screen of a plugin. */
export interface SearchActions {
  readonly onPlugin: (pluginId: string) => void;
  readonly onScreen: (pluginId: string, pageId: string | undefined) => void;
  readonly pluginName: (id: string) => string;
}

function PlaceButton(props: { readonly place: Place; readonly actions: SearchActions }): ReactNode {
  const { place, actions } = props;
  const where = place.pageId === undefined ? place.title : `${actions.pluginName(place.pluginId)} › ${place.title}`;
  return (
    <Button
      size="sm"
      variant="ghost"
      className="h-auto justify-start px-1 py-0.5 text-start"
      onClick={() =>
        place.pageId === undefined ? actions.onPlugin(place.pluginId) : actions.onScreen(place.pluginId, place.pageId)
      }
    >
      <span className="truncate">{where}</span>
    </Button>
  );
}

function HitRow(props: { readonly hit: Hit; readonly actions: SearchActions }): ReactNode {
  const { i18n } = useLingui();
  const m = appCanvasMessages(i18n);
  const { hit, actions } = props;
  if (hit.kind === "plugin") {
    return (
      <li>
        <Button
          size="sm"
          variant="ghost"
          className="h-auto w-full justify-start px-1 py-0.5"
          onClick={() => actions.onPlugin(hit.id)}
        >
          <span className="truncate font-medium">{hit.title}</span>
          <span className="truncate font-mono text-caption text-muted-foreground">{hit.detail}</span>
        </Button>
      </li>
    );
  }
  if (hit.kind === "screen") {
    return (
      <li>
        <Button
          size="sm"
          variant="ghost"
          className="h-auto w-full justify-start px-1 py-0.5"
          onClick={() => actions.onScreen(hit.pluginId, hit.pageId)}
        >
          <span className="truncate text-muted-foreground">{actions.pluginName(hit.pluginId)} ›</span>
          <span className="truncate font-medium">{hit.title}</span>
          <span className="truncate font-mono text-caption text-muted-foreground">{hit.detail}</span>
        </Button>
      </li>
    );
  }
  return (
    <li className="flex flex-col gap-0.5 border-b border-border pb-1.5">
      <span className="truncate font-mono text-label">{hit.title}</span>
      {hit.uses.length === 0 ? (
        <span className="text-caption text-muted-foreground">{m.searchUnused}</span>
      ) : (
        <>
          <span className="text-caption text-muted-foreground">{m.searchUsedIn(hit.uses.length)}</span>
          {hit.uses.map((p) => (
            <PlaceButton key={`${p.pluginId}|${p.pageId ?? p.title}`} place={p} actions={actions} />
          ))}
        </>
      )}
    </li>
  );
}

/** The Search tab of the app canvas's dock (#265). */
export function AppSearch(props: { readonly all: readonly Hit[]; readonly actions: SearchActions }): ReactNode {
  const { i18n } = useLingui();
  const m = appCanvasMessages(i18n);
  const [query, setQuery] = useState("");
  const groups = useMemo(() => searchApp(props.all, query), [props.all, query]);
  return (
    <div className="flex flex-col gap-3">
      <InputGroup>
        <InputGroupAddon>
          <Icon name="magnifying-glass" />
        </InputGroupAddon>
        <InputGroupInput
          aria-label={m.searchApp}
          placeholder={m.searchApp}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
      </InputGroup>
      {query.trim() === "" ? (
        <p className="dock-note">{m.searchHint}</p>
      ) : groups.size === 0 ? (
        <p className="dock-note">{m.searchNothing}</p>
      ) : (
        [...groups].map(([kind, hits]) => (
          <section key={kind} aria-label={m.searchGroup(kind)} className="flex flex-col gap-1">
            <h3 className="text-label font-medium">
              {m.searchGroup(kind)} <span className="tabular-nums text-muted-foreground">{hits.length}</span>
            </h3>
            <ul className="flex flex-col gap-1">
              {hits.slice(0, SHOWN_PER_GROUP).map((h) => (
                <HitRow key={`${h.kind}:${"id" in h ? h.id : h.pageId}`} hit={h} actions={props.actions} />
              ))}
            </ul>
            {hits.length > SHOWN_PER_GROUP && (
              <p className="text-caption text-muted-foreground">{m.searchMore(hits.length - SHOWN_PER_GROUP)}</p>
            )}
          </section>
        ))
      )}
    </div>
  );
}

/** The Search pane of the app canvas's dock: its label and the search over everything the app's documents hold. */
export function useSearchPane(
  docs: readonly LoadedDoc[],
  summaries: readonly PluginSummary[],
  actions: SearchActions,
): { readonly label: string; readonly panel: ReactNode } {
  const { i18n } = useLingui();
  const plugins = useMemo<SearchInput["plugins"]>(
    () =>
      summaries.flatMap((s) =>
        s.plugin === undefined
          ? []
          : [{ id: s.plugin.id, key: s.plugin.key, name: s.plugin.name, functionsUsed: s.functionsUsed }],
      ),
    [summaries],
  );
  const all = useMemo(() => searchables({ docs, index: buildWhereUsed(docs), plugins }), [docs, plugins]);
  return { label: appCanvasMessages(i18n).searchTab, panel: <AppSearch all={all} actions={actions} /> };
}

export type { HitKind };

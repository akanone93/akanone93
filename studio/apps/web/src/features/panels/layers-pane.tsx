// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Icon, type LayerNode, LayerTree, layerMatches } from "@plux/ui";
import { Button } from "@plux/ui/components/button";
import { InputGroup, InputGroupAddon, InputGroupInput } from "@plux/ui/components/input-group";
import { Tooltip, TooltipContent, TooltipTrigger } from "@plux/ui/components/tooltip";
import type { ReactElement } from "react";
import { panelsMessages } from "./panels.messages.ts";

/** The Layers pane's tools, which stay at the top while the tree scrolls: the filter, expand all and collapse all. */
export function LayersToolbar({
  filter,
  onFilter,
  onExpandAll,
  onCollapseAll,
}: {
  readonly filter: string;
  readonly onFilter: (text: string) => void;
  readonly onExpandAll: () => void;
  readonly onCollapseAll: () => void;
}): ReactElement {
  const m = panelsMessages(useLingui().i18n);
  const tool = (label: string, icon: "expand-all" | "collapse-all", run: () => void) => (
    <Tooltip>
      <TooltipTrigger render={<Button variant="ghost" size="icon-sm" aria-label={label} onClick={run} />}>
        <Icon name={icon} />
      </TooltipTrigger>
      <TooltipContent side="bottom">{label}</TooltipContent>
    </Tooltip>
  );
  return (
    <div className="flex items-center gap-1">
      <InputGroup className="flex-1">
        <InputGroupAddon>
          <Icon name="funnel" />
        </InputGroupAddon>
        <InputGroupInput
          aria-label={m.filterLayers}
          placeholder={m.filterLayers}
          value={filter}
          onChange={(e) => onFilter(e.target.value)}
        />
      </InputGroup>
      {tool(m.expandAll, "expand-all", onExpandAll)}
      {tool(m.collapseAll, "collapse-all", onCollapseAll)}
    </div>
  );
}

/**
 * The Layers pane (STU-061), which also holds the screens and the selection: a card for what is selected (how many,
 * which, show it, clear it) over the tree of the screens and their widgets.
 */
export function LayersPane({
  nodes,
  selected,
  filter,
  expanded,
  dir,
  onExpanded,
  onSelect,
  onReveal,
  onClear,
  onDrop,
}: {
  readonly nodes: Readonly<Record<string, LayerNode>>;
  readonly selected: readonly string[];
  readonly filter: string;
  readonly expanded: readonly string[];
  readonly dir: "ltr" | "rtl";
  readonly onExpanded: (ids: readonly string[]) => void;
  readonly onSelect: (id: string, toggle: boolean) => void;
  readonly onReveal: (id: string) => void;
  readonly onClear: () => void;
  readonly onDrop: ((dragged: string, target: string, half: "before" | "after") => void) | undefined;
}): ReactElement {
  const m = panelsMessages(useLingui().i18n);
  const types = selected.map((id) => nodes[id]?.type ?? id);
  const summary = [...new Set(types)].map((t) => `${types.filter((x) => x === t).length} ${t}`).join(", ");
  const empty = layerMatches(nodes, "plugin", filter)?.size === 0;
  const anchor = selected[selected.length - 1];
  return (
    <div className="flex flex-col gap-2">
      {selected.length > 0 && (
        <section aria-label={m.selectionTab} className="layer-selection">
          <div className="flex items-center gap-2">
            <span className="layer-selection-count">{selected.length}</span>
            <p className="min-w-0 flex-1 truncate text-xs font-medium">
              {selected.length > 1 ? `${m.selectedCount(selected.length)} (${summary})` : (types[0] ?? "")}
            </p>
            {anchor !== undefined && (
              <Button variant="ghost" size="icon-xs" aria-label={m.showSelection} onClick={() => onReveal(anchor)}>
                <Icon name="crosshair" />
              </Button>
            )}
            <Button variant="ghost" size="icon-xs" aria-label={m.clearSelection} onClick={onClear}>
              <Icon name="x" />
            </Button>
          </div>
          <ul aria-label={m.selectionLabel} className="flex flex-wrap gap-1">
            {selected.map((id) => (
              <li key={id} className="layer-selection-chip">
                <Icon name={nodes[id]?.icon ?? "cube"} size="pill" />
                <span className="truncate">{nodes[id]?.preview ?? nodes[id]?.name ?? id}</span>
                <code className="truncate opacity-60">{id}</code>
              </li>
            ))}
          </ul>
        </section>
      )}
      {empty && <p className="dock-note">{m.noLayers}</p>}
      <LayerTree
        nodes={nodes}
        rootId="plugin"
        label={m.layersLabel}
        dir={dir}
        selected={selected}
        onSelect={onSelect}
        filter={filter}
        expanded={expanded}
        onExpandedChange={onExpanded}
        onDrop={onDrop}
      />
    </div>
  );
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import uFuzzy from "@leeoniya/ufuzzy";
import { useLingui } from "@lingui/react";
import { Dialog } from "@plux/ui";
import { type KeyboardEvent, useId, useMemo, useState } from "react";
import { type CommandState, commandMessages, useCommands } from "../../lib/commands/index.ts";
import {
  type OrgHit,
  type OrgSearchFn,
  parseScope,
  pushRecent,
  readRecents,
  recentsFirst,
  useOrgSearch,
} from "./palette-search.ts";

const fuzzy = new uFuzzy({ intraMode: 1 });

/** Commands whose title matches `needle`, best first; everything, in registry order, for an empty needle. */
export function filterCommands(states: readonly CommandState[], needle: string): readonly CommandState[] {
  if (needle.trim() === "") {
    return states;
  }
  const haystack = states.map((s) => s.command.title);
  const [idxs, info, order] = fuzzy.search(haystack, needle);
  if (idxs === null || idxs.length === 0) {
    return [];
  }
  const ordered = info !== null && order !== null ? order.map((o) => info.idx[o]) : idxs;
  return ordered.flatMap((i) => (i === undefined ? [] : [states[i]])).filter((s): s is CommandState => s !== undefined);
}

export interface PaletteProps {
  readonly open: boolean;
  readonly onOpenChange: (open: boolean) => void;
  /** Organisation-wide search (F15); absent in contexts without a server. */
  readonly search?: OrgSearchFn;
  readonly onOpenHit?: (hit: OrgHit) => void;
}

type Row =
  | { readonly type: "command"; readonly key: string; readonly state: CommandState }
  | { readonly type: "hit"; readonly key: string; readonly hit: OrgHit };

/** D1: the local command palette. Arrows move, Enter runs, Escape closes and focus returns to the opener. */
export function CommandPalette({ open, onOpenChange, search, onOpenHit }: PaletteProps) {
  const { i18n } = useLingui();
  const m = commandMessages(i18n);
  const { states } = useCommands();
  const [needle, setNeedle] = useState("");
  const [active, setActive] = useState(0);
  const listId = useId();
  const { scope, text } = parseScope(needle);
  const org = useOrgSearch(search, scope, text);
  const rows = useMemo<Row[]>(() => {
    const local = scope === "screens" ? [] : filterCommands(states, text);
    const ordered = text === "" ? recentsFirst(local, (s) => s.command.id, readRecents()) : local;
    return [
      ...ordered.map((state): Row => ({ type: "command", key: state.command.id, state })),
      ...org.hits.map((hit): Row => ({ type: "hit", key: `org:${hit.kind}:${hit.id}`, hit })),
    ];
  }, [states, scope, text, org.hits]);
  const results = rows;
  const current = results[Math.min(active, Math.max(results.length - 1, 0))];

  const run = (row: Row | undefined) => {
    if (row?.type === "hit") {
      onOpenChange(false);
      onOpenHit?.(row.hit);
      return;
    }
    if (row?.state.enabled !== true) {
      return;
    }
    pushRecent(row.state.command.id);
    onOpenChange(false);
    void row.state.command.run();
  };
  const onKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      const step = event.key === "ArrowDown" ? 1 : -1;
      setActive((a) => (results.length === 0 ? 0 : (a + step + results.length) % results.length));
    } else if (event.key === "Enter") {
      event.preventDefault();
      run(current);
    }
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setNeedle("");
        setActive(0);
        onOpenChange(next);
      }}
      title={m.paletteTitle}
      labels={{ close: m.close }}
    >
      <input
        type="text"
        role="combobox"
        aria-expanded="true"
        aria-controls={listId}
        aria-activedescendant={current === undefined ? undefined : `${listId}-${current.key}`}
        aria-label={m.paletteSearch}
        placeholder={m.paletteSearch}
        value={needle}
        onChange={(e) => {
          setNeedle(e.target.value);
          setActive(0);
        }}
        onKeyDown={onKeyDown}
        className="rounded-md border border-subtle bg-sunken px-3 py-2 text-body text-primary"
      />
      <div
        id={listId}
        role="listbox"
        aria-label={m.paletteTitle}
        className="flex max-h-80 flex-col gap-1 overflow-auto"
      >
        {results.length === 0 && !org.pending && (
          <div className="px-3 py-2 text-body text-secondary">{m.paletteEmpty}</div>
        )}
        {results.map((row) => (
          <div
            key={row.key}
            id={`${listId}-${row.key}`}
            role="option"
            aria-selected={row === current}
            aria-disabled={row.type === "command" && !row.state.enabled}
            tabIndex={-1}
            onClick={() => run(row)}
            onKeyDown={undefined}
            className={`flex flex-col rounded-md px-3 py-2 text-body ${row === current ? "bg-sunken" : ""} ${row.type === "hit" || row.state.enabled ? "text-primary" : "text-secondary"}`}
          >
            {row.type === "hit" ? (
              <>
                <span>{row.hit.name}</span>
                <span className="text-caption text-secondary">{row.hit.kind}</span>
              </>
            ) : (
              <>
                <span>{row.state.command.title}</span>
                {!row.state.enabled && row.state.missing !== undefined && (
                  <span className="text-caption">{m.missing(row.state.missing)}</span>
                )}
                {!row.state.enabled && row.state.reason !== undefined && (
                  <span className="text-caption">{row.state.reason}</span>
                )}
              </>
            )}
          </div>
        ))}
        {org.pending && <div className="px-3 py-2 text-caption text-secondary">{m.paletteSearching}</div>}
        {org.failed && (
          <div role="alert" className="px-3 py-2 text-caption text-danger">
            {m.paletteSearchFailed}
          </div>
        )}
      </div>
    </Dialog>
  );
}

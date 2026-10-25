// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import uFuzzy from "@leeoniya/ufuzzy";
import { useLingui } from "@lingui/react";
import { Dialog } from "@plux/ui";
import { type KeyboardEvent, useId, useMemo, useState } from "react";
import { type CommandState, commandMessages, useCommands } from "../../lib/commands/index.ts";

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
}

/** D1: the local command palette. Arrows move, Enter runs, Escape closes and focus returns to the opener. */
export function CommandPalette({ open, onOpenChange }: PaletteProps) {
  const { i18n } = useLingui();
  const m = commandMessages(i18n);
  const { states } = useCommands();
  const [needle, setNeedle] = useState("");
  const [active, setActive] = useState(0);
  const listId = useId();
  const results = useMemo(() => filterCommands(states, needle), [states, needle]);
  const current = results[Math.min(active, Math.max(results.length - 1, 0))];

  const run = (state: CommandState | undefined) => {
    if (state?.enabled !== true) {
      return;
    }
    onOpenChange(false);
    void state.command.run();
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
        aria-activedescendant={current === undefined ? undefined : `${listId}-${current.command.id}`}
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
        {results.length === 0 && <div className="px-3 py-2 text-body text-secondary">{m.paletteEmpty}</div>}
        {results.map((s) => (
          <div
            key={s.command.id}
            id={`${listId}-${s.command.id}`}
            role="option"
            aria-selected={s === current}
            aria-disabled={!s.enabled}
            tabIndex={-1}
            onClick={() => run(s)}
            onKeyDown={undefined}
            className={`flex flex-col rounded-md px-3 py-2 text-body ${s === current ? "bg-sunken" : ""} ${s.enabled ? "text-primary" : "text-secondary"}`}
          >
            <span>{s.command.title}</span>
            {!s.enabled && s.missing !== undefined && <span className="text-caption">{m.missing(s.missing)}</span>}
          </div>
        ))}
      </div>
    </Dialog>
  );
}

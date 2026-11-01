// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import uFuzzy from "@leeoniya/ufuzzy";
import { useLingui } from "@lingui/react";
import { Icon, type IconName } from "@plux/ui";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@plux/ui/components/dialog";
import { Kbd } from "@plux/ui/components/kbd";
import { Spinner } from "@plux/ui/components/spinner";
import { ToggleGroup, ToggleGroupItem } from "@plux/ui/components/toggle-group";
import { type KeyboardEvent, type ReactElement, useEffect, useId, useMemo, useState } from "react";
import {
  type CommandGroup,
  type CommandState,
  commandMessages,
  shortcutKeys,
  useCommands,
} from "../../lib/commands/index.ts";
import {
  type OrgHit,
  type OrgSearchFn,
  type PaletteScope,
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

interface Section {
  readonly key: string;
  readonly title: string;
  readonly rows: readonly Row[];
}

const groupOrder: readonly CommandGroup[] = ["navigate", "organisation", "screens", "edit", "appearance", "account"];

const kindOrder = ["app", "plugin", "page", "component", "template", "function", "translation_key"];

const kindIcon: Readonly<Record<string, IconName>> = {
  app: "squares-four",
  plugin: "puzzle-piece",
  page: "file",
  component: "stack",
  template: "copy",
  function: "lightning",
  translation_key: "globe",
};

const groupIcon: Readonly<Record<CommandGroup, IconName>> = {
  navigate: "arrow-right",
  organisation: "users",
  screens: "file",
  edit: "pencil-simple",
  appearance: "palette",
  account: "user",
};

const prefixOf: Readonly<Record<PaletteScope, string>> = { all: "", commands: ">", screens: "#" };

/** The keys of a shortcut as small key caps. */
function Keys({ shortcut, apple }: { readonly shortcut: string; readonly apple: boolean }): ReactElement {
  return (
    <span className="ms-auto flex gap-0.5">
      {shortcutKeys(shortcut, apple).map((key, i) => (
        // biome-ignore lint/suspicious/noArrayIndexKey: a key may repeat, the order is fixed
        <Kbd key={i}>{key}</Kbd>
      ))}
    </span>
  );
}

/** The palette's sections: recent and grouped commands without text, matching commands and the hits by kind with it. */
function buildSections(
  m: ReturnType<typeof commandMessages>,
  states: readonly CommandState[],
  scope: PaletteScope,
  text: string,
  hits: readonly OrgHit[],
): Section[] {
  const out: Section[] = [];
  if (scope !== "screens") {
    out.push(...commandSections(m, states, text));
  }
  const kinds = [...new Set([...kindOrder, ...hits.map((h) => h.kind)])];
  for (const kind of kinds) {
    const ofKind = hits.filter((h) => h.kind === kind);
    if (ofKind.length > 0) {
      out.push({
        key: `kind:${kind}`,
        title: m.kind(kind),
        rows: ofKind.map((hit) => ({ type: "hit", key: `org:${hit.kind}:${hit.id}`, hit })),
      });
    }
  }
  return out;
}

function commandSections(
  m: ReturnType<typeof commandMessages>,
  states: readonly CommandState[],
  text: string,
): Section[] {
  const local = filterCommands(states, text);
  const row = (state: CommandState, prefix = ""): Row => ({
    type: "command",
    key: `${prefix}${state.command.id}`,
    state,
  });
  if (text !== "") {
    return local.length === 0 ? [] : [{ key: "commands", title: m.paletteCommands, rows: local.map((s) => row(s)) }];
  }
  const recents = readRecents();
  const out: Section[] = [];
  const recent = recentsFirst(local, (s) => s.command.id, recents).filter((s) => recents.includes(s.command.id));
  if (recent.length > 0) {
    out.push({ key: "recent", title: m.paletteRecent, rows: recent.map((s) => row(s, "recent:")) });
  }
  for (const group of groupOrder) {
    const rows = local.filter((s) => s.command.group === group && !recents.includes(s.command.id));
    if (rows.length > 0) {
      out.push({ key: group, title: groupTitle(m, group), rows: rows.map((s) => row(s)) });
    }
  }
  return out;
}

/** One option of the palette: a command with its shortcut, or an organisation hit with its kind's icon and path. */
function PaletteRow({
  row,
  id,
  selected,
  apple,
  m,
  onRun,
  onHover,
}: {
  readonly row: Row;
  readonly id: string;
  readonly selected: boolean;
  readonly apple: boolean;
  readonly m: ReturnType<typeof commandMessages>;
  readonly onRun: () => void;
  readonly onHover: () => void;
}): ReactElement {
  const disabled = row.type === "command" && !row.state.enabled;
  return (
    <div
      id={id}
      role="option"
      aria-selected={selected}
      aria-disabled={disabled}
      tabIndex={-1}
      onClick={onRun}
      onKeyDown={undefined}
      onMouseMove={onHover}
      className={`flex cursor-default items-center gap-3 rounded-md px-2 py-2 text-sm ${selected ? "bg-accent text-accent-foreground" : ""} ${disabled ? "opacity-60" : ""}`}
    >
      {row.type === "hit" ? (
        <>
          <Icon name={kindIcon[row.hit.kind] ?? "file"} className="text-muted-foreground" />
          <span className="flex min-w-0 flex-col">
            <span className="truncate">{row.hit.name}</span>
            {row.hit.path !== undefined && row.hit.path !== "" && (
              <span className="truncate text-xs text-muted-foreground">{row.hit.path}</span>
            )}
          </span>
          <span className="sr-only">{row.hit.kind}</span>
        </>
      ) : (
        <>
          <Icon name={groupIcon[row.state.command.group]} className="text-muted-foreground" />
          <span className="flex min-w-0 flex-col">
            <span className="truncate">{row.state.command.title}</span>
            {disabled && row.state.missing !== undefined && (
              <span className="text-xs text-muted-foreground">{m.missing(row.state.missing)}</span>
            )}
            {disabled && row.state.reason !== undefined && (
              <span className="text-xs text-muted-foreground">{row.state.reason}</span>
            )}
          </span>
          {row.state.command.shortcut !== undefined && <Keys shortcut={row.state.command.shortcut} apple={apple} />}
        </>
      )}
    </div>
  );
}

/**
 * D1: Studio's search and command palette in one place (F15, STU-013). Without text it lists the commands, the most
 * recent first; with text it narrows the commands and searches the organisation's apps, plugins, screens and the rest,
 * each kind under its own heading. `>` or the Commands filter keeps to commands, `#` or the Screens filter to screens.
 * Arrows move, Enter opens, Escape closes and focus returns to the opener.
 */
export function CommandPalette({ open, onOpenChange, search, onOpenHit }: PaletteProps) {
  const { i18n } = useLingui();
  const m = commandMessages(i18n);
  const { states } = useCommands();
  const [needle, setNeedle] = useState("");
  const [active, setActive] = useState(0);
  const listId = useId();
  const apple = typeof navigator !== "undefined" && /mac|iphone|ipad/i.test(navigator.platform);
  const { scope, text } = parseScope(needle);
  const org = useOrgSearch(search, scope, text);

  const sections = useMemo(() => buildSections(m, states, scope, text, org.hits), [m, states, scope, text, org.hits]);
  const rows = sections.flatMap((s) => s.rows);
  const current = rows[Math.min(active, Math.max(rows.length - 1, 0))];

  // The active option stays in view as the arrows move through a long list.
  const currentKey = current?.key;
  useEffect(() => {
    if (currentKey !== undefined) {
      document.getElementById(`${listId}-${currentKey}`)?.scrollIntoView?.({ block: "nearest" });
    }
  }, [currentKey, listId]);

  const close = (next: boolean) => {
    setNeedle("");
    setActive(0);
    onOpenChange(next);
  };
  const run = (row: Row | undefined) => {
    if (row?.type === "hit") {
      close(false);
      onOpenHit?.(row.hit);
      return;
    }
    if (row?.state.enabled !== true) {
      return;
    }
    pushRecent(row.state.command.id);
    close(false);
    void row.state.command.run();
  };
  const onKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      const step = event.key === "ArrowDown" ? 1 : -1;
      setActive((a) => (rows.length === 0 ? 0 : (a + step + rows.length) % rows.length));
    } else if (event.key === "Enter") {
      event.preventDefault();
      run(current);
    }
  };
  const setScope = (next: PaletteScope) => {
    setNeedle(`${prefixOf[next]}${text}`);
    setActive(0);
  };

  return (
    <Dialog open={open} onOpenChange={close}>
      <DialogContent showCloseButton={false} className="top-1/4 translate-y-0 gap-0 overflow-hidden p-0 sm:max-w-xl">
        <DialogTitle className="sr-only">{m.paletteTitle}</DialogTitle>
        <DialogDescription className="sr-only">{m.paletteSearch}</DialogDescription>
        <div className="flex items-center gap-2 border-b px-3">
          <Icon name="magnifying-glass" className="text-muted-foreground" />
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
            className="h-12 min-w-0 flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
          />
          {org.pending && <Spinner className="text-muted-foreground" />}
        </div>
        {search !== undefined && (
          <div className="flex items-center gap-2 border-b px-3 py-2">
            <ToggleGroup
              aria-label={m.scopeLabel}
              size="sm"
              variant="outline"
              value={[scope]}
              onValueChange={(v) => {
                const next = v[0] as PaletteScope | undefined;
                if (next !== undefined) {
                  setScope(next);
                }
              }}
            >
              <ToggleGroupItem value="all">{m.scopeAll}</ToggleGroupItem>
              <ToggleGroupItem value="commands">{m.scopeCommands}</ToggleGroupItem>
              <ToggleGroupItem value="screens">{m.scopeScreens}</ToggleGroupItem>
            </ToggleGroup>
          </div>
        )}
        <div
          id={listId}
          role="listbox"
          aria-label={m.paletteTitle}
          className="flex max-h-96 flex-col overflow-y-auto p-2"
        >
          {rows.length === 0 && !org.pending && (
            <div className="flex flex-col items-center gap-2 px-3 py-10 text-sm text-muted-foreground">
              <Icon name="magnifying-glass" />
              {m.paletteEmpty}
            </div>
          )}
          {sections.map((section) => (
            // biome-ignore lint/a11y/useSemanticElements: a group of options inside a listbox; a fieldset may not stand there
            <div key={section.key} role="group" aria-label={section.title} className="flex flex-col pb-2">
              <div className="px-2 py-1.5 text-xs font-medium text-muted-foreground" aria-hidden="true">
                {section.title}
              </div>
              {section.rows.map((row) => (
                <PaletteRow
                  key={row.key}
                  row={row}
                  id={`${listId}-${row.key}`}
                  selected={row === current}
                  apple={apple}
                  m={m}
                  onRun={() => run(row)}
                  onHover={() => {
                    const at = rows.indexOf(row);
                    if (at !== active) {
                      setActive(at);
                    }
                  }}
                />
              ))}
            </div>
          ))}
          {org.failed && (
            <p role="alert" className="px-2 py-2 text-xs text-destructive">
              {m.paletteSearchFailed}
            </p>
          )}
        </div>
        <div className="flex items-center gap-4 border-t bg-muted/40 px-3 py-2 text-xs text-muted-foreground">
          <span className="flex items-center gap-1.5">
            <Kbd>
              <Icon name="arrows-up-down" />
            </Kbd>
            {m.hintMove}
          </span>
          <span className="flex items-center gap-1.5">
            <Kbd>
              <Icon name="enter" />
            </Kbd>
            {m.hintOpen}
          </span>
          <span className="flex items-center gap-1.5">
            <Kbd>{m.keyEscape}</Kbd>
            {m.hintClose}
          </span>
          {org.pending && <span className="ms-auto">{m.paletteSearching}</span>}
        </div>
      </DialogContent>
    </Dialog>
  );
}

function groupTitle(m: ReturnType<typeof commandMessages>, group: CommandGroup): string {
  return {
    navigate: m.groupNavigate,
    organisation: m.groupOrganisation,
    screens: m.groupScreens,
    edit: m.groupEdit,
    appearance: m.groupAppearance,
    account: m.groupAccount,
  }[group];
}

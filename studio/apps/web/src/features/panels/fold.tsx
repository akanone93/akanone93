// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Icon, type IconName } from "@plux/ui";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@plux/ui/components/collapsible";
import { createContext, type ReactElement, type ReactNode, useCallback, useContext, useMemo, useState } from "react";
import "./fold.css";

/**
 * Which folding sections of a list are open (MR-3): the list's first section is open and every other closed until the
 * person opens it; what they open or close is kept for this browser, by section, so the inspector keeps the same
 * sections open from one selection to the next. A section outside any list keeps its own state, open.
 */
interface FoldState {
  readonly first: string | undefined;
  readonly isOpen: (id: string) => boolean;
  readonly setOpen: (id: string, open: boolean) => void;
}

const FoldContext = createContext<FoldState | null>(null);

/** Inside another section, a folding card is shown as a plain subsection: its heading, then its body (MR-3). */
const InlineContext = createContext(false);

/** Shows the folding cards inside it as plain subsections of the section around them. */
export function FoldInline({ children }: { readonly children: ReactNode }): ReactElement {
  return <InlineContext.Provider value={true}>{children}</InlineContext.Provider>;
}

/** The key the folds of `scope` are kept under in this browser. */
const storageKey = (scope: string) => `plux.folds.${scope}`;

function readStored(scope: string): Readonly<Record<string, boolean>> {
  try {
    const raw = JSON.parse(globalThis.localStorage?.getItem(storageKey(scope)) ?? "{}") as unknown;
    return raw !== null && typeof raw === "object" && !Array.isArray(raw)
      ? Object.fromEntries(Object.entries(raw).filter((e): e is [string, boolean] => typeof e[1] === "boolean"))
      : {};
  } catch {
    return {};
  }
}

/** Whether a section is open: what the person chose for it, else open only when it is the list's first. */
export function foldOpen(stored: Readonly<Record<string, boolean>>, id: string, first: string | undefined): boolean {
  return stored[id] ?? id === first;
}

/**
 * A list of folding sections: `first` names the one open by default; `scope` keeps the person's choices apart; `all`
 * opens every section while it holds (a search, "set only"), so nothing found is hidden in a closed one.
 */
export function FoldList(props: {
  readonly scope: string;
  readonly first: string | undefined;
  readonly all?: boolean;
  readonly children: ReactNode;
}): ReactElement {
  const [stored, setStored] = useState(() => readStored(props.scope));
  const setOpen = useCallback(
    (id: string, open: boolean) => {
      setStored((was) => {
        const next = { ...was, [id]: open };
        try {
          globalThis.localStorage?.setItem(storageKey(props.scope), JSON.stringify(next));
        } catch {
          // Storage blocked: the choice holds for this page only.
        }
        return next;
      });
    },
    [props.scope],
  );
  const value = useMemo<FoldState>(
    () => ({
      first: props.first,
      isOpen: (id) => props.all === true || foldOpen(stored, id, props.first),
      setOpen,
    }),
    [stored, props.first, props.all, setOpen],
  );
  return <FoldContext.Provider value={value}>{props.children}</FoldContext.Provider>;
}

/** A section's open state: from its list when it is in one, else its own, open unless `alone` says otherwise. */
export function useFold(id: string, alone = true): [boolean, (open: boolean) => void] {
  const list = useContext(FoldContext);
  const [own, setOwn] = useState(alone);
  return list === null ? [own, setOwn] : [list.isOpen(id), (open) => list.setOpen(id, open)];
}

/**
 * A folding card of a pane (MR-3): a head with its icon, name, an optional note at the end (a count, a summary), and a
 * caret; its body below. Closed, it is one line high.
 */
export function FoldCard(props: {
  readonly id: string;
  readonly icon: IconName;
  readonly title: ReactNode;
  /** The card's accessible name when the title is not plain text. */
  readonly label?: string;
  /** What the head shows at its end: a count, how many are set, a short summary. */
  readonly note?: ReactNode;
  readonly className?: string;
  readonly bodyClassName?: string;
  readonly children: ReactNode;
}): ReactElement {
  const [open, setOpen] = useFold(props.id);
  const inline = useContext(InlineContext);
  if (inline) {
    return (
      <section
        className="fold-sub"
        data-fold={props.id}
        aria-label={props.label ?? (typeof props.title === "string" ? props.title : undefined)}
      >
        <h4 className="fold-sub-title">
          <Icon name={props.icon} size="pill" className="fold-icon" />
          <span className="fold-title">{props.title}</span>
          {props.note !== undefined && <span className="fold-note">{props.note}</span>}
        </h4>
        <div className={`fold-sub-body${props.bodyClassName === undefined ? "" : ` ${props.bodyClassName}`}`}>
          {props.children}
        </div>
      </section>
    );
  }
  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className={`fold-card${props.className === undefined ? "" : ` ${props.className}`}`}
      data-fold={props.id}
      render={<section aria-label={props.label ?? (typeof props.title === "string" ? props.title : undefined)} />}
    >
      <CollapsibleTrigger className="fold-head">
        <Icon name={props.icon} size="pill" className="fold-icon" />
        <span className="fold-title">{props.title}</span>
        {props.note !== undefined && <span className="fold-note">{props.note}</span>}
        <Icon name="caret-down" size="pill" className="fold-caret" />
      </CollapsibleTrigger>
      <CollapsibleContent className={`fold-body${props.bodyClassName === undefined ? "" : ` ${props.bodyClassName}`}`}>
        {props.children}
      </CollapsibleContent>
    </Collapsible>
  );
}

/**
 * A folding row inside a section (MR-3), for a composite value such as a text style: its name, a dot when it is set,
 * a short summary of its value, and the editor below once opened. `defaultOpen` opens it at first.
 */
export function FoldRow(props: {
  readonly title: string;
  readonly set: boolean;
  readonly summary?: string;
  readonly defaultOpen?: boolean;
  readonly action?: ReactNode;
  readonly children: ReactNode;
}): ReactElement {
  const [open, setOpen] = useState(props.defaultOpen === true);
  return (
    <Collapsible open={open} onOpenChange={setOpen} className="fold-row" data-set={props.set || undefined}>
      <div className="fold-row-head">
        <CollapsibleTrigger className="fold-row-trigger">
          <Icon name="caret-down" size="pill" className="fold-caret" />
          <span className="prop-dot" aria-hidden="true" />
          <span className="fold-row-title">{props.title}</span>
          {props.summary !== undefined && props.summary !== "" && (
            <span className="fold-row-summary">{props.summary}</span>
          )}
        </CollapsibleTrigger>
        {props.action}
      </div>
      <CollapsibleContent className="fold-row-body">{props.children}</CollapsibleContent>
    </Collapsible>
  );
}

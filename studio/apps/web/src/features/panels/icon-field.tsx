// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { IconNames } from "@plux/canvas";
import { Input } from "@plux/ui/components/input";
import { Popover, PopoverContent, PopoverTrigger } from "@plux/ui/components/popover";
import { type ReactElement, useEffect, useMemo, useState } from "react";
import { loadIconNames } from "../plugin-canvas/icons.ts";
import { styleMessages } from "./style.messages.ts";
import "./icon-field.css";

type IconSet = "material" | "cupertino";
const SETS: readonly IconSet[] = ["material", "cupertino"];
/** The most icons the grid shows at once; a search narrows the rest. */
const SHOWN = 96;

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/** An `IconData` value's name and set, when it has a name. */
export function iconValue(value: unknown): { readonly name: string; readonly set: IconSet } | undefined {
  if (!isObj(value) || typeof value["name"] !== "string") {
    return undefined;
  }
  return { name: value["name"], set: value["set"] === "cupertino" ? "cupertino" : "material" };
}

/** The everyday icons of each set, shown first before a search. */
const COMMON: Readonly<Record<IconSet, readonly string[]>> = {
  material: [
    "home",
    "search",
    "settings",
    "person",
    "favorite",
    "add",
    "close",
    "menu",
    "arrow_back",
    "arrow_forward",
    "check",
    "delete",
    "edit",
    "share",
    "more_vert",
    "notifications",
    "shopping_cart",
    "star",
    "info",
    "help",
    "mail",
    "call",
    "calendar_today",
    "schedule",
    "location_on",
    "photo_camera",
    "image",
    "lock",
    "visibility",
    "download",
    "upload",
    "refresh",
    "filter_list",
    "sort",
    "chevron_right",
    "expand_more",
    "logout",
    "account_circle",
  ],
  cupertino: [
    "home",
    "search",
    "settings",
    "person",
    "heart",
    "add",
    "clear",
    "bars",
    "back",
    "forward",
    "check_mark",
    "delete",
    "pencil",
    "share",
    "ellipsis_vertical",
    "bell",
    "cart",
    "star",
    "info",
    "question",
    "mail",
    "phone",
    "calendar",
    "clock",
    "location",
    "camera",
    "photo",
    "lock",
    "eye",
    "cloud_download",
    "cloud_upload",
    "refresh",
    "chevron_right",
    "chevron_down",
    "square_arrow_right",
    "person_circle",
  ],
};

/**
 * The names of `set` that contain every word of `query` (spaces and hyphens read as underscores), names that start
 * with it first, then shorter names; without a query, the set's everyday icons, then the rest by name.
 */
export function matchIcons(
  names: Readonly<Record<string, number>>,
  query: string,
  set: IconSet = "material",
): string[] {
  const q = query
    .trim()
    .toLowerCase()
    .replace(/[\s-]+/g, "_");
  const all = Object.keys(names);
  if (q === "") {
    const common = COMMON[set].filter((n) => n in names);
    const first = new Set(common);
    return [...common, ...all.filter((n) => !first.has(n)).sort()];
  }
  const words = q.split("_").filter((w) => w !== "");
  return all
    .filter((n) => words.every((w) => n.includes(w)))
    .sort((a, b) => Number(!a.startsWith(q)) - Number(!b.startsWith(q)) || a.length - b.length || (a < b ? -1 : 1));
}

/** The glyph of an icon in the canvas's icon fonts. */
function Glyph({ name, set, names }: { name: string; set: IconSet; names: IconNames | null }): ReactElement {
  const point = names?.[set][name];
  return (
    <span className="icon-glyph" data-set={set} aria-hidden="true">
      {point === undefined ? "" : String.fromCodePoint(point)}
    </span>
  );
}

/**
 * An `IconData` property (THM-005): a field with the icon's glyph and name that opens a picker of every icon of
 * Material Symbols and the Cupertino icons, searched by name.
 */
export function IconField({
  name,
  value,
  readOnly = false,
  onChange,
  onEnd,
}: {
  readonly name: string;
  readonly value: unknown;
  readonly readOnly?: boolean;
  readonly onChange: (value: unknown) => void;
  readonly onEnd: () => void;
}): ReactElement {
  const m = styleMessages(useLingui().i18n);
  const current = iconValue(value);
  const [names, setNames] = useState<IconNames | null>(null);
  const [set, setSet] = useState<IconSet>(current?.set ?? "material");
  const [query, setQuery] = useState("");
  useEffect(() => {
    let live = true;
    loadIconNames()
      .then((n) => live && setNames(n))
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, []);
  const matches = useMemo(() => (names === null ? [] : matchIcons(names[set], query, set)), [names, set, query]);
  const pick = (next: unknown) => {
    onChange(next);
    onEnd();
  };
  return (
    <Popover>
      <PopoverTrigger
        disabled={readOnly}
        render={
          <button type="button" className="color-field" aria-label={`${m.pick(name)}: ${current?.name ?? m.none}`}>
            {current === undefined ? null : <Glyph name={current.name} set={current.set} names={names} />}
            <span className="min-w-0 flex-1 truncate text-start">{current?.name ?? m.none}</span>
          </button>
        }
      />
      <PopoverContent align="start" className="color-popover icon-popover">
        <div className="icon-sets">
          {SETS.map((s) => (
            <button key={s} type="button" aria-pressed={s === set} className="icon-set" onClick={() => setSet(s)}>
              {m.iconSet(s)}
            </button>
          ))}
        </div>
        <Input
          aria-label={m.iconSearch}
          placeholder={m.iconSearch}
          className="h-8"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        {names === null ? (
          <p className="color-popover-title">{m.iconLoading}</p>
        ) : (
          <IconGrid
            matches={matches}
            set={set}
            names={names}
            current={current}
            onPick={(n) => pick({ name: n, set })}
          />
        )}
        <button type="button" className="color-clear" onClick={() => pick(undefined)}>
          {m.clear}
        </button>
      </PopoverContent>
    </Popover>
  );
}

function IconGrid({
  matches,
  set,
  names,
  current,
  onPick,
}: {
  readonly matches: readonly string[];
  readonly set: IconSet;
  readonly names: IconNames;
  readonly current: { readonly name: string; readonly set: IconSet } | undefined;
  readonly onPick: (name: string) => void;
}): ReactElement {
  const m = styleMessages(useLingui().i18n);
  if (matches.length === 0) {
    return <p className="color-popover-title">{m.iconNoMatch}</p>;
  }
  return (
    <>
      <div className="icon-grid">
        {matches.slice(0, SHOWN).map((n) => (
          <button
            key={n}
            type="button"
            className="icon-chip"
            title={n}
            aria-label={n}
            aria-pressed={current?.set === set && current.name === n}
            onClick={() => onPick(n)}
          >
            <Glyph name={n} set={set} names={names} />
          </button>
        ))}
      </div>
      {matches.length > SHOWN ? <p className="color-popover-title">{m.iconMore(matches.length - SHOWN)}</p> : null}
    </>
  );
}

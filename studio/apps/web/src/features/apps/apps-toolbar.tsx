// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Icon, type IconName, Input, Select } from "@plux/ui";
import { Toggle } from "@plux/ui/components/toggle";
import { ToggleGroup, ToggleGroupItem } from "@plux/ui/components/toggle-group";
import type { ReactElement } from "react";
import { appsMessages } from "./apps.messages.ts";
import type { AppSort, AppStatus, AppsSearch } from "./apps.schema.ts";

/** The Select value that stands for "no filter"; a Select option cannot be an empty string. */
const ANY = "any";

/**
 * One choice out of a few, as shadcn's ToggleGroup (2 to 7 options). Each option is a toggle button whose
 * `aria-pressed` tells the state; pressing the chosen one again keeps it chosen.
 */
export function Choice<T extends string>({
  label,
  value,
  options,
  onChange,
}: {
  readonly label: string;
  readonly value: T;
  readonly options: readonly { readonly value: T; readonly label: string; readonly icon?: IconName }[];
  readonly onChange: (value: T) => void;
}): ReactElement {
  return (
    <ToggleGroup
      aria-label={label}
      variant="outline"
      size="sm"
      spacing={0}
      value={[value]}
      onValueChange={(next) => {
        const chosen = options.find((o) => next.includes(o.value) && o.value !== value);
        if (chosen !== undefined) {
          onChange(chosen.value);
        }
      }}
    >
      {options.map((o) => (
        <ToggleGroupItem key={o.value} value={o.value}>
          {o.icon !== undefined && <Icon name={o.icon} data-icon="inline-start" />}
          {o.label}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  );
}

const asSort = (v: string): AppSort => (v === "activity" || v === "health" ? v : "name");

/** Search, team and tag filters, sort order, status and "my apps" (STU-023); every change goes to the URL. */
export function AppsToolbar({
  search,
  teams,
  tags,
  change,
}: {
  readonly search: AppsSearch;
  readonly teams: readonly { readonly id: string; readonly name: string }[];
  readonly tags: readonly string[];
  readonly change: (patch: AppsSearch) => void;
}): ReactElement {
  const m = appsMessages(useLingui().i18n);
  const team = search.team ?? "";
  const tag = search.tag ?? "";
  const mine = search.mine === true;
  const status: AppStatus = search.status ?? "active";
  return (
    <fieldset aria-label={m.filtersLabel} className="m-0 flex min-w-0 flex-wrap items-center gap-2 border-0 p-0">
      <div className="min-w-48 flex-1">
        <Input
          type="search"
          leading="magnifying-glass"
          aria-label={m.searchLabel}
          placeholder={m.searchLabel}
          value={search.q ?? ""}
          autoComplete="off"
          onChange={(e) => change({ q: e.target.value })}
        />
      </div>
      <div className="w-36">
        <Select
          label={m.teamLabel}
          options={[{ value: ANY, label: m.allTeams }, ...teams.map((t) => ({ value: t.id, label: t.name }))]}
          value={team === "" ? ANY : team}
          onValueChange={(v) => change({ team: v === ANY ? undefined : v })}
        />
      </div>
      <div className="w-36">
        <Select
          label={m.tagLabel}
          options={[{ value: ANY, label: m.allTags }, ...tags.map((t) => ({ value: t, label: t }))]}
          value={tag === "" ? ANY : tag}
          onValueChange={(v) => change({ tag: v === ANY ? undefined : v })}
        />
      </div>
      <div className="w-36">
        <Select
          label={m.sortLabel}
          options={[
            { value: "name", label: m.sortName },
            { value: "activity", label: m.sortActivity },
            { value: "health", label: m.sortHealth },
          ]}
          value={search.sort ?? "name"}
          onValueChange={(v) => change({ sort: asSort(v) })}
        />
      </div>
      <Choice
        label={m.statusLabel}
        value={status}
        options={[
          { value: "active", label: m.statusActive },
          { value: "archived", label: m.statusArchived },
          { value: "all", label: m.statusAll },
        ]}
        onChange={(v) => change({ status: v })}
      />
      <Toggle
        variant="outline"
        size="sm"
        pressed={mine}
        onPressedChange={(on) => change({ mine: on ? true : undefined })}
      >
        <Icon name="user" data-icon="inline-start" />
        {m.mine}
      </Toggle>
    </fieldset>
  );
}

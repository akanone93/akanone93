// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, cx, Icon, type IconName, Input, Select } from "@plux/ui";
import type { ReactElement } from "react";
import { appsMessages } from "./apps.messages.ts";
import type { AppSort, AppStatus, AppsSearch } from "./apps.schema.ts";

/** The Select value that stands for "no filter"; a Select option cannot be an empty string. */
const ANY = "any";

const pressed = "bg-raised text-primary shadow-soft";

/** A button of a toggle group: `aria-pressed` tells the state, the raised pill shows it. */
export function Toggle({
  on,
  onClick,
  icon,
  children,
}: {
  readonly on: boolean;
  readonly onClick: () => void;
  readonly icon?: IconName;
  readonly children: string;
}): ReactElement {
  return (
    <Button size="sm" variant="ghost" aria-pressed={on} className={cx(on && pressed)} onClick={onClick}>
      {icon !== undefined && <Icon name={icon} size="tab" />}
      {children}
    </Button>
  );
}

const asSort = (v: string): AppSort => (v === "activity" || v === "health" ? v : "name");

/** The status filter: active, archived or all apps. */
function StatusToggles({ status, change }: { readonly status: AppStatus; readonly change: (p: AppsSearch) => void }) {
  const m = appsMessages(useLingui().i18n);
  const options: readonly [AppStatus, string][] = [
    ["active", m.statusActive],
    ["archived", m.statusArchived],
    ["all", m.statusAll],
  ];
  return (
    <fieldset aria-label={m.statusLabel} className="apps-segments">
      {options.map(([value, label]) => (
        <Toggle key={value} on={status === value} onClick={() => change({ status: value })}>
          {label}
        </Toggle>
      ))}
    </fieldset>
  );
}

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
  return (
    <fieldset aria-label={m.filtersLabel} className="apps-toolbar">
      <span className="apps-search">
        <Input
          type="search"
          size="lg"
          leading="magnifying-glass"
          aria-label={m.searchLabel}
          placeholder={m.searchLabel}
          value={search.q ?? ""}
          autoComplete="off"
          onChange={(e) => change({ q: e.target.value })}
        />
      </span>
      <span className="apps-filter">
        <Select
          label={m.teamLabel}
          options={[{ value: ANY, label: m.allTeams }, ...teams.map((t) => ({ value: t.id, label: t.name }))]}
          value={team === "" ? ANY : team}
          onValueChange={(v) => change({ team: v === ANY ? undefined : v })}
        />
      </span>
      <span className="apps-filter">
        <Select
          label={m.tagLabel}
          options={[{ value: ANY, label: m.allTags }, ...tags.map((t) => ({ value: t, label: t }))]}
          value={tag === "" ? ANY : tag}
          onValueChange={(v) => change({ tag: v === ANY ? undefined : v })}
        />
      </span>
      <span className="apps-filter">
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
      </span>
      <StatusToggles status={search.status ?? "active"} change={change} />
      <span className="apps-segments">
        <Toggle on={mine} icon="user" onClick={() => change({ mine: mine ? undefined : true })}>
          {m.mine}
        </Toggle>
      </span>
    </fieldset>
  );
}

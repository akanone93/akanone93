// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { AlertDialog, Button, Input, Select } from "@plux/ui";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { type ReactElement, useEffect, useMemo, useState } from "react";
import { localisationAreaMessages } from "./area.messages.ts";
import { type LocalisationSearch, setStates, writeMessages } from "./area-actions.ts";
import { AreaFrame } from "./area-frame.tsx";
import { type Area, useArea } from "./area-scope.tsx";
import { AddLocaleDialog, ExportDialog, ImportDialog } from "./dialogs.tsx";
import { type CellStatus, filterKeys, statusOf } from "./grid-model.ts";

type Bulk = "approve" | "review" | "copy";

function useSearchState(): [LocalisationSearch, (change: Partial<LocalisationSearch>) => void] {
  const search = useSearch({ strict: false }) as LocalisationSearch;
  const navigate = useNavigate();
  return [search, (change) => void navigate({ to: ".", search: (prev: object) => ({ ...prev, ...change }) })];
}

function useStatusLabels() {
  const m = localisationAreaMessages(useLingui().i18n);
  return {
    missing: m.statusMissing,
    new: m.statusNew,
    translated: m.statusTranslated,
    reviewed: m.statusReviewed,
    approved: m.statusApproved,
  } satisfies Record<CellStatus, string>;
}

function Filters(props: {
  readonly search: LocalisationSearch;
  readonly set: (c: Partial<LocalisationSearch>) => void;
}) {
  const m = localisationAreaMessages(useLingui().i18n);
  const area = useArea();
  const labels = useStatusLabels();
  const { search, set } = props;
  return (
    <div className="flex flex-wrap items-end gap-3">
      <Select
        label={m.pluginFilter}
        options={[{ value: "", label: m.allPlugins }, ...area.plugins.map((p) => ({ value: p, label: p }))]}
        value={area.plugin ?? ""}
        onValueChange={(v) => set({ plugin: v === "" ? undefined : v })}
      />
      <Select
        label={m.localeFilter}
        options={area.sessions.locales.map((l) => ({ value: l, label: l }))}
        value={area.sessions.locale}
        onValueChange={(locale) => set({ locale })}
      />
      <Select
        label={m.statusFilter}
        options={[
          { value: "all", label: m.statusAll },
          ...(Object.keys(labels) as CellStatus[]).map((s) => ({ value: s, label: labels[s] })),
        ]}
        value={search.status ?? "all"}
        onValueChange={(v) => set({ status: v as LocalisationSearch["status"] })}
      />
      <Input
        aria-label={m.search}
        placeholder={m.search}
        value={search.q ?? ""}
        onChange={(e) => set({ q: e.target.value === "" ? undefined : e.target.value })}
      />
    </div>
  );
}

function Toolbar() {
  const m = localisationAreaMessages(useLingui().i18n);
  const [open, setOpen] = useState<"locale" | "import" | "export" | null>(null);
  const area = useArea();
  return (
    <div className="flex flex-wrap gap-2">
      <Button onClick={() => setOpen("locale")} disabled={!area.sessions.canEditKeys}>
        {m.addLocale}
      </Button>
      <Button onClick={() => setOpen("import")} disabled={!area.canEdit}>
        {m.importOpen}
      </Button>
      <Button onClick={() => setOpen("export")}>{m.exportOpen}</Button>
      <AddLocaleDialog open={open === "locale"} onClose={() => setOpen(null)} />
      <ImportDialog open={open === "import"} onClose={() => setOpen(null)} />
      <ExportDialog open={open === "export"} onClose={() => setOpen(null)} />
    </div>
  );
}

/** D56: a bulk action over the selected keys, confirmed before it changes anything. */
function BulkBar(props: { readonly ids: readonly string[]; readonly done: () => void }): ReactElement {
  const m = localisationAreaMessages(useLingui().i18n);
  const area = useArea();
  const [pending, setPending] = useState<Bulk | null>(null);
  const { locale, defaultLocale } = area.sessions;
  const text = {
    approve: m.bulkApproveText({ locale }),
    review: m.bulkReviewText({ locale }),
    copy: m.bulkCopyText({ locale }),
    none: "",
  };
  const run = () => {
    if (pending === "approve") {
      setStates(area, props.ids, "approved");
    } else if (pending === "review") {
      setStates(area, props.ids, "translated");
    } else if (pending === "copy") {
      const source = area.messages.get(defaultLocale) ?? {};
      const mine = area.messages.get(locale) ?? {};
      const texts = Object.fromEntries(
        props.ids.flatMap((id) => ((mine[id] ?? "") === "" && source[id] !== undefined ? [[id, source[id]]] : [])),
      );
      void writeMessages(area, texts, "new");
    }
    setPending(null);
    props.done();
  };
  const none = props.ids.length === 0;
  return (
    <div className="flex flex-wrap items-center gap-2">
      <span className="text-body">{m.selected({ count: props.ids.length })}</span>
      <Button disabled={none || !area.canApprove} onClick={() => setPending("approve")}>
        {m.bulkApprove}
      </Button>
      <Button disabled={none || !area.canEdit} onClick={() => setPending("review")}>
        {m.bulkReview}
      </Button>
      <Button disabled={none || !area.canEdit || locale === defaultLocale} onClick={() => setPending("copy")}>
        {m.bulkCopy}
      </Button>
      {area.canEdit && !area.canApprove && <span className="text-caption text-secondary">{m.noApprove}</span>}
      <AlertDialog
        open={pending !== null}
        onOpenChange={(o) => !o && setPending(null)}
        title={m.bulkTitle({ count: props.ids.length })}
        description={text[pending ?? "none"]}
        labels={{ cancel: m.bulkCancel, confirm: m.bulkConfirm }}
        onConfirm={run}
      />
    </div>
  );
}

function Rows(props: {
  readonly rows: ReturnType<typeof filterKeys>;
  readonly area: Area;
  readonly selected: ReadonlySet<string>;
  readonly toggle: (ids: readonly string[], on: boolean) => void;
}): ReactElement {
  const m = localisationAreaMessages(useLingui().i18n);
  const labels = useStatusLabels();
  const { area, rows } = props;
  const { locale, defaultLocale } = area.sessions;
  const source = area.messages.get(defaultLocale);
  const mine = area.messages.get(locale);
  const all = rows.length > 0 && rows.every((r) => props.selected.has(r.id));
  return (
    <table aria-label={m.gridTable} className="w-full border-collapse text-start">
      <thead>
        <tr>
          <th scope="col" className="p-2">
            <input
              type="checkbox"
              aria-label={m.selectAll}
              checked={all}
              onChange={(e) =>
                props.toggle(
                  rows.map((r) => r.id),
                  e.target.checked,
                )
              }
            />
          </th>
          {[m.gridTitle, m.sourceColumn, m.targetColumn({ locale }), m.statusColumn, m.contextColumn].map((h) => (
            <th key={h} scope="col" className="p-2 text-start text-label text-secondary">
              {h}
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {rows.map((k) => {
          const screens = (area.screens.get(k.id) ?? []).filter((s) => area.contextScreens.has(s));
          return (
            <tr key={k.id} className="border-t border-hairline">
              <td className="p-2">
                <input
                  type="checkbox"
                  aria-label={m.selectRow({ key: k.key })}
                  checked={props.selected.has(k.id)}
                  onChange={(e) => props.toggle([k.id], e.target.checked)}
                />
              </td>
              <th scope="row" className="p-2 text-start font-mono text-data font-normal" dir="ltr">
                {k.key}
              </th>
              <td className="p-2 text-body">{source?.[k.id] ?? ""}</td>
              <td className="p-2 text-body">{mine?.[k.id] ?? ""}</td>
              <td className="p-2 text-body">{labels[statusOf(k.id, mine, area.states)]}</td>
              <td className="p-2 text-caption text-secondary">
                {screens.length > 0 ? m.context({ screens: screens.join(", ") }) : m.noContext}
              </td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}

function Grid(): ReactElement {
  const m = localisationAreaMessages(useLingui().i18n);
  const area = useArea();
  const [search, set] = useSearchState();
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
  const { locale, setLocale } = area.sessions;
  useEffect(() => {
    if (search.locale !== undefined && search.locale !== locale && area.sessions.locales.includes(search.locale)) {
      setLocale(search.locale);
    }
  }, [search.locale, locale, setLocale, area.sessions.locales]);
  const mine = area.messages.get(locale);
  const rows = useMemo(
    () =>
      filterKeys(
        area.keys,
        {
          status: search.status ?? "all",
          query: search.q ?? "",
          used: area.plugin === undefined ? undefined : (area.usedByPlugin.get(area.plugin) ?? new Set()),
        },
        (id) => statusOf(id, mine, area.states),
        (id) => [...area.messages.values()].flatMap((msgs) => msgs[id] ?? []),
      ),
    [area, search.status, search.q, mine],
  );
  const toggle = (ids: readonly string[], on: boolean) =>
    setSelected((cur) => {
      const next = new Set(cur);
      for (const id of ids) {
        on ? next.add(id) : next.delete(id);
      }
      return next;
    });
  return (
    <div className="flex flex-col gap-4">
      <Filters search={search} set={set} />
      <Toolbar />
      {!area.canEdit && <p className="text-caption text-secondary">{m.readOnlyGrid}</p>}
      <BulkBar ids={rows.filter((r) => selected.has(r.id)).map((r) => r.id)} done={() => setSelected(new Set())} />
      {rows.length === 0 ? (
        <p className="text-body text-secondary">{m.emptyGrid}</p>
      ) : (
        <Rows rows={rows} area={area} selected={selected} toggle={toggle} />
      )}
    </div>
  );
}

/** A13: the translation keys by locale, with filters, search, status, screenshot context and bulk actions (STU-111, I18N-007, I18N-008). */
export function LocalisationPage(): ReactElement {
  const m = localisationAreaMessages(useLingui().i18n);
  return (
    <AreaFrame id="localisation-title" title={m.gridTitle} caption={m.gridCaption}>
      <Grid />
    </AreaFrame>
  );
}

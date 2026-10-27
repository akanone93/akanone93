// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { LibraryItem } from "@plux/api";
import { Button, Field, Input, notify, Select } from "@plux/ui";
import { Link, useNavigate, useParams, useSearch } from "@tanstack/react-router";
import { type ChangeEvent, type ReactElement, useState } from "react";
import { libraryMessages } from "./library.messages.ts";
import type { LibrarySearch } from "./library.schema.ts";
import { readFile } from "./library-model.ts";
import { type LibraryKind, useCan, useLibraryClients, useLibraryItems } from "./use-library.ts";

const ANY = "";

type Messages = ReturnType<typeof libraryMessages>;

function Row({
  item,
  org,
  m,
}: {
  readonly item: LibraryItem;
  readonly org: string;
  readonly m: Messages;
}): ReactElement {
  const vis =
    { private: m.visPrivate, organization: m.visOrganization, public: m.visPublic }[item.visibility] ?? item.visibility;
  const link =
    item.kind === "component" ? (
      <Link to="/o/$org/library/components/$id" params={{ org, id: item.id }}>
        {item.name}
      </Link>
    ) : (
      <Link to="/o/$org/library/templates/$id" params={{ org, id: item.id }}>
        {item.name}
      </Link>
    );
  return (
    <li className="flex flex-wrap items-baseline gap-3 py-2">
      <span className="text-body font-medium">{link}</span>
      <code className="text-caption text-secondary">{item.key}</code>
      <span className="text-caption">{item.kind === "component" ? m.kindComponent : m.kindTemplate}</span>
      <span className="text-caption">{vis}</span>
      <span className="text-caption text-secondary">{m.headVersion(item.headVersion)}</span>
    </li>
  );
}

function Filters({ search, m }: { readonly search: LibrarySearch; readonly m: Messages }): ReactElement {
  const navigate = useNavigate();
  const set = (next: Partial<Record<keyof LibrarySearch, string | undefined>>) =>
    void navigate({ to: ".", search: ((prev: object) => ({ ...prev, ...next })) as never });
  const text = (key: "q" | "owner" | "category" | "tag", label: string) => (
    <Field label={label}>
      <Input value={search[key] ?? ""} onChange={(e) => set({ [key]: e.target.value || undefined })} />
    </Field>
  );
  return (
    <div className="flex flex-wrap items-end gap-4">
      {text("q", m.searchLabel)}
      <Select
        label={m.kindLabel}
        value={search.kind ?? ANY}
        placeholder={m.kindAll}
        options={[
          { value: ANY, label: m.kindAll },
          { value: "component", label: m.kindComponent },
          { value: "template", label: m.kindTemplate },
        ]}
        onValueChange={(v) => set({ kind: v || undefined })}
      />
      <Select
        label={m.visibilityLabel}
        value={search.visibility ?? ANY}
        placeholder={m.visibilityAll}
        options={[
          { value: ANY, label: m.visibilityAll },
          { value: "private", label: m.visPrivate },
          { value: "organization", label: m.visOrganization },
          { value: "public", label: m.visPublic },
        ]}
        onValueChange={(v) => set({ visibility: v || undefined })}
      />
      {text("owner", m.ownerLabel)}
      {text("category", m.categoryLabel)}
      {text("tag", m.tagLabel)}
    </div>
  );
}

function ImportButton({ kind, m }: { readonly kind: LibraryKind; readonly m: Messages }): ReactElement {
  const clients = useLibraryClients();
  const [busy, setBusy] = useState(false);
  const onFile = async (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    e.target.value = "";
    if (file === undefined) {
      return;
    }
    setBusy(true);
    try {
      const bytes = await readFile(file);
      const res =
        kind === "component"
          ? await clients.components.importComponent({ file: bytes })
          : await clients.templates.importTemplate({ file: bytes });
      await clients.invalidate();
      notify("success", m.imported(res.item?.name ?? file.name));
    } catch {
      notify("error", m.importFailed);
    } finally {
      setBusy(false);
    }
  };
  const label = kind === "component" ? m.importComponent : m.importTemplate;
  return (
    <label className="inline-flex">
      <span className="sr-only">{label}</span>
      <input
        type="file"
        className="sr-only"
        data-testid={`import-${kind}`}
        disabled={busy}
        onChange={(e) => void onFile(e)}
      />
      <Button
        variant="secondary"
        disabled={busy}
        onClick={(e) => (e.currentTarget.previousElementSibling as HTMLInputElement | null)?.click()}
      >
        {label}
      </Button>
    </label>
  );
}

/** O4: components and templates with search and filters held in the URL (STU-086); import needs library.manage. */
export function LibraryPage(): ReactElement {
  const m = libraryMessages(useLingui().i18n);
  const org = (useParams({ strict: false }) as { org?: string }).org ?? "";
  const search = useSearch({ strict: false }) as LibrarySearch;
  const items = useLibraryItems(search);
  const canManage = useCan("library.manage");
  return (
    <section className="flex flex-col gap-4" aria-labelledby="library-title">
      <h1 id="library-title" className="text-screen">
        {m.title}
      </h1>
      <Filters search={search} m={m} />
      {canManage && (
        <div className="flex gap-2">
          <ImportButton kind="component" m={m} />
          <ImportButton kind="template" m={m} />
        </div>
      )}
      {items.isPending && <p className="text-body text-secondary">{m.loading}</p>}
      {items.isError && (
        <p role="alert" className="text-body text-danger">
          {m.loadFailed}
        </p>
      )}
      {items.data?.length === 0 && <p className="text-body text-secondary">{m.empty}</p>}
      <ul>
        {(items.data ?? []).map((item) => (
          <Row key={item.id} item={item} org={org} m={m} />
        ))}
      </ul>
    </section>
  );
}

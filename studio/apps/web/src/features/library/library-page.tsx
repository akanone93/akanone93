// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { LibraryItem } from "@plux/api";
import { Field, FileDrop, Icon, Input, notify, Select } from "@plux/ui";
import { Badge } from "@plux/ui/components/badge";
import { Link, useNavigate, useParams, useSearch } from "@tanstack/react-router";
import { type ReactElement, useState } from "react";
import { AnimatedList } from "../../components/animated-list.tsx";
import { PageEmpty, PageHeader, PageStatus } from "../../components/page.tsx";
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
    <div className="page-row">
      <span className="page-row-mark" aria-hidden="true">
        <Icon name={item.kind === "component" ? "component" : "package"} />
      </span>
      <span className="flex min-w-0 flex-col">
        <span className="truncate text-sm font-medium">{link}</span>
        <code className="truncate text-xs text-muted-foreground">{item.key}</code>
      </span>
      <span className="ms-auto flex flex-wrap items-center gap-2">
        <Badge variant="secondary">{item.kind === "component" ? m.kindComponent : m.kindTemplate}</Badge>
        <Badge variant="outline">{vis}</Badge>
        <span className="text-xs text-muted-foreground">{m.headVersion(item.headVersion)}</span>
      </span>
    </div>
  );
}

function Filters({ search, m }: { readonly search: LibrarySearch; readonly m: Messages }): ReactElement {
  const navigate = useNavigate();
  const set = (next: Partial<Record<keyof LibrarySearch, string | undefined>>) =>
    void navigate({ to: ".", resetScroll: false, search: ((prev: object) => ({ ...prev, ...next })) as never });
  const text = (key: "q" | "owner" | "category" | "tag", label: string) => (
    <Field label={label}>
      <Input value={search[key] ?? ""} onChange={(e) => set({ [key]: e.target.value || undefined })} />
    </Field>
  );
  return (
    <div className="page-toolbar">
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
  const onFile = async (file: File | undefined) => {
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
    <FileDrop
      variant="button"
      label={label}
      disabled={busy}
      testId={`import-${kind}`}
      icon={kind === "component" ? "component" : "package"}
      onFiles={(files) => void onFile(files[0])}
    />
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
    <section className="page-frame" aria-labelledby="library-title">
      <PageHeader
        id="library-title"
        title={m.title}
        caption={m.caption}
        actions={
          canManage && (
            <>
              <ImportButton kind="component" m={m} />
              <ImportButton kind="template" m={m} />
            </>
          )
        }
      />
      <div className="page-body">
        <Filters search={search} m={m} />
        <PageStatus loading={items.isPending} failed={items.isError} error={items.error} labels={m} />
        {items.data?.length === 0 && <PageEmpty icon="book" text={m.empty} />}
        <AnimatedList
          items={items.data ?? []}
          keyOf={(item) => item.id}
          className="flex flex-col gap-2"
          render={(item) => <Row item={item} org={org} m={m} />}
        />
      </div>
    </section>
  );
}

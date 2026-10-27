// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Dialog, Field, Input, Textarea } from "@plux/ui";
import { Link, useParams } from "@tanstack/react-router";
import { type ReactElement, useState } from "react";
import { objects, PreviewPanel, splitList, strings, usePreview } from "../designers/db-schema-overlay.tsx";
import { designerMessages } from "../designers/designers.messages.ts";
import type { Json } from "../designers/use-document.ts";
import { Table } from "../security/parts.tsx";
import { type DataMessages, dataMessages } from "./data.messages.ts";
import { Frame } from "./frame.tsx";
import { CommitBar } from "./parts.tsx";
import { useDataContext } from "./scope.tsx";
import { useCommit, usePluginJson, useSeededList } from "./use-draft.ts";

/** An index as the text of one line, and the indexes of a text of lines; empty lines are dropped. */
export const indexLine = (index: unknown): string => strings(index).join(", ");
export function parseIndexes(text: string): string[][] {
  return text
    .split("\n")
    .map(splitList)
    .filter((fields) => fields.length > 0);
}

/** A22: the collections of the plugin's on-device database (STU-113, DB-004). */
export function CollectionsPage(): ReactElement {
  const m = dataMessages(useLingui().i18n);
  return (
    <Frame id="collections-title" title={m.collectionsTitle} caption={m.collectionsCaption}>
      <CollectionRows />
    </Frame>
  );
}

function CollectionRows(): ReactElement {
  const m = dataMessages(useLingui().i18n);
  const ctx = useDataContext();
  const collections = objects(usePluginJson().value?.["collections"]);
  if (collections.length === 0) {
    return <p className="text-body text-secondary">{m.noCollections}</p>;
  }
  return (
    <Table
      caption={m.collectionsTitle}
      heads={[m.colName, m.colVersion, m.colFields, m.colIndexes, ""]}
      rows={collections.map((c) => ({
        key: String(c["id"]),
        cells: [
          String(c["key"] ?? ""),
          String(c["version"] ?? 1),
          String(objects(c["fields"]).length),
          Array.isArray(c["indexes"]) ? c["indexes"].map(indexLine).join("; ") : "",
          <Link
            key="open"
            to="/o/$org/apps/$app/data/collections/$collectionId"
            params={{ org: ctx.org, app: ctx.app, collectionId: String(c["id"]) } as never}
            search={{ env: ctx.env, plugin: ctx.plugin }}
          >
            {m.open}
          </Link>,
        ],
      }))}
    />
  );
}

/** D61: what publishing the draft schema does to devices on the last release, from the Go planner (DB-005). */
function PreviewDialog(props: {
  readonly open: boolean;
  readonly onClose: () => void;
  readonly next: Json | undefined;
  readonly m: DataMessages;
}): ReactElement {
  const ctx = useDataContext();
  const d = designerMessages(useLingui().i18n);
  const { path } = usePluginJson();
  const preview = usePreview(props.open, path, props.next, ctx.tool, ctx.loadPublished);
  return (
    <Dialog
      open={props.open}
      onOpenChange={(open) => {
        if (!open) {
          props.onClose();
        }
      }}
      title={props.m.previewTitle}
      labels={{ close: props.m.previewClose }}
    >
      <PreviewPanel preview={preview} m={d} />
    </Dialog>
  );
}

function Schema(props: { collection: Json; canEdit: boolean; change: (patch: Json) => void; m: DataMessages }) {
  const { collection: c, canEdit, change, m } = props;
  const fields = objects(c["fields"]);
  const plans = objects(c["migrations"]);
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap gap-3">
        <Field label={m.key}>
          <Input value={String(c["key"] ?? "")} disabled={!canEdit} onChange={(e) => change({ key: e.target.value })} />
        </Field>
        <Field label={m.version}>
          <Input
            type="number"
            min={1}
            value={String(c["version"] ?? 1)}
            disabled={!canEdit}
            onChange={(e) => change({ version: Math.max(1, Number.parseInt(e.target.value, 10) || 1) })}
          />
        </Field>
        <Field label={m.primaryKey}>
          <Input
            value={strings(c["primaryKey"]).join(", ")}
            disabled={!canEdit}
            onChange={(e) => change({ primaryKey: splitList(e.target.value) })}
          />
        </Field>
      </div>
      <Field label={m.indexes} description={m.indexesHelp}>
        <Textarea
          rows={3}
          defaultValue={(Array.isArray(c["indexes"]) ? c["indexes"] : []).map(indexLine).join("\n")}
          disabled={!canEdit}
          onChange={(e) => {
            const indexes = parseIndexes(e.target.value);
            change({ indexes: indexes.length === 0 ? undefined : indexes });
          }}
        />
      </Field>
      <section aria-label={m.fields} className="flex flex-col gap-2">
        <h2 className="text-label font-medium">{m.fields}</h2>
        <Table
          caption={m.fields}
          heads={[m.colName, m.colType, ""]}
          rows={fields.map((f, i) => ({
            key: `${i}${String(f["name"])}`,
            cells: [String(f["name"] ?? ""), String(f["type"] ?? ""), f["sensitive"] === true ? m.sensitive : ""],
          }))}
        />
      </section>
      <section aria-label={m.plans} className="flex flex-col gap-2">
        <h2 className="text-label font-medium">{m.plans}</h2>
        {plans.length === 0 && <p className="text-body text-secondary">{m.noPlans}</p>}
        <ul>
          {plans.map((p) => (
            <li key={String(p["from"])} className="text-body">
              {m.planFrom(Number(p["from"]))}
            </li>
          ))}
        </ul>
      </section>
    </div>
  );
}

function Editor(props: { collectionId: string }): ReactElement {
  const i18n = useLingui().i18n;
  const m = dataMessages(i18n);
  const d = designerMessages(i18n);
  const { value } = usePluginJson();
  const [collections, setCollections, seeded] = useSeededList("collections");
  const [previewing, setPreviewing] = useState(false);
  const next = value === undefined || !seeded ? undefined : { ...value, collections };
  const state = useCommit(next, "collection");
  const current = collections.find((c) => String(c["id"]) === props.collectionId);
  if (current === undefined) {
    return <p className="text-body text-secondary">{seeded ? m.notFoundCollection : m.loading}</p>;
  }
  const change = (patch: Json) =>
    setCollections((all) =>
      all.map((c) => {
        if (c !== current) {
          return c;
        }
        const out: Json = { ...c, ...patch };
        for (const [k, v] of Object.entries(patch)) {
          if (v === undefined) {
            delete out[k];
          }
        }
        return out;
      }),
    );
  return (
    <div className="flex flex-col gap-4">
      <Schema collection={current} canEdit={state.canEdit} change={change} m={m} />
      <div>
        <Button variant="secondary" onClick={() => setPreviewing(true)}>
          {m.previewButton}
        </Button>
      </div>
      <PreviewDialog open={previewing} onClose={() => setPreviewing(false)} next={next} m={m} />
      <CommitBar state={state} m={m} d={d} />
    </div>
  );
}

/** A23: one collection's schema, indexes and migration plans, with the migration preview (STU-113). */
export function CollectionPage(): ReactElement {
  const m = dataMessages(useLingui().i18n);
  const { collectionId } = useParams({ strict: false }) as { collectionId?: string };
  return (
    <Frame
      id="collection-title"
      title={m.collectionTitle}
      caption={m.collectionCaption}
      back={{ to: "/o/$org/apps/$app/data/collections", label: m.collectionsBack }}
    >
      <Editor collectionId={collectionId ?? ""} />
    </Frame>
  );
}

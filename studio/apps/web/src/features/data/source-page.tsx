// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Dialog, Field, Input, Select } from "@plux/ui";
import { useParams } from "@tanstack/react-router";
import { type ReactElement, useState } from "react";
import { objects } from "../designers/db-schema-overlay.tsx";
import { designerMessages } from "../designers/designers.messages.ts";
import { isObject, type Json } from "../designers/use-document.ts";
import { Table } from "../security/parts.tsx";
import { type DataMessages, dataMessages } from "./data.messages.ts";
import { Frame } from "./frame.tsx";
import { CommitBar, JsonField } from "./parts.tsx";
import { useCommit, usePluginJson, useSeededList } from "./use-draft.ts";

const KINDS = ["rest", "graphql", "websocket", "sse", "function", "database", "static"] as const;
const ROUTES = ["default", "plux", "direct"] as const;

/** A source without the keys whose patch value is `undefined`. */
export function patched(source: Json, patch: Json): Json {
  const out: Json = { ...source, ...patch };
  for (const [k, v] of Object.entries(patch)) {
    if (v === undefined) {
      delete out[k];
    }
  }
  return out;
}

/** The operations the configuration names: `config.operations` as a list of objects, or as an object keyed by name. */
export function operationsOf(config: unknown): Json[] {
  const ops = isObject(config) ? config["operations"] : undefined;
  if (Array.isArray(ops)) {
    return ops.filter(isObject);
  }
  return isObject(ops) ? Object.entries(ops).map(([name, op]) => ({ name, ...(isObject(op) ? op : {}) })) : [];
}

/** The per-environment URLs: `config.urls` as an object from environment to URL. */
export function urlsOf(config: unknown): [string, string][] {
  const urls = isObject(config) ? config["urls"] : undefined;
  return isObject(urls) ? Object.entries(urls).map(([env, url]) => [env, String(url)]) : [];
}

/** The types of the plugin that the source's type expression names. */
export function schemasOf(types: readonly Json[], expression: string): Json[] {
  const used = new Set(expression.match(/[A-Za-z_][A-Za-z0-9_]*/g) ?? []);
  return types.filter((t) => used.has(String(t["name"])));
}

/** D60: edit the design-time mock of a source (DAT-080). */
function MockDialog(props: {
  readonly open: boolean;
  readonly onClose: () => void;
  readonly source: Json;
  readonly canEdit: boolean;
  readonly onUse: (mock: unknown) => void;
  readonly m: DataMessages;
}): ReactElement {
  const { m, source } = props;
  const [draft, setDraft] = useState<unknown>(source["mock"]);
  return (
    <Dialog
      open={props.open}
      onOpenChange={(open) => {
        if (!open) {
          props.onClose();
        }
      }}
      title={m.mockTitle}
      labels={{ close: m.mockClose }}
    >
      <div className="flex flex-col gap-3">
        <JsonField
          label={m.mockLabel}
          error={m.mockInvalid}
          value={source["mock"]}
          seedKey={`${String(source["id"])}:${props.open}`}
          disabled={!props.canEdit}
          objectOnly={false}
          allowEmpty={false}
          rows={10}
          onValid={setDraft}
        />
        <div>
          <Button
            disabled={!props.canEdit}
            onClick={() => {
              props.onUse(draft);
              props.onClose();
            }}
          >
            {m.mockSave}
          </Button>
        </div>
      </div>
    </Dialog>
  );
}

function Tables(props: { source: Json; types: readonly Json[]; m: DataMessages }): ReactElement {
  const { source, types, m } = props;
  const config = source["config"];
  const ops = operationsOf(config);
  const urls = urlsOf(config);
  const schemas = schemasOf(types, String(source["type"] ?? ""));
  return (
    <>
      <section aria-label={m.operations} className="flex flex-col gap-2">
        <h2 className="text-label font-medium">{m.operations}</h2>
        {ops.length === 0 ? (
          <p className="text-body text-secondary">{m.noOperations}</p>
        ) : (
          <Table
            caption={m.operations}
            heads={[m.colOperation, m.colMethod, m.colPath]}
            rows={ops.map((o, i) => ({
              key: `${i}${String(o["name"])}`,
              cells: [String(o["name"] ?? o["id"] ?? ""), String(o["method"] ?? ""), String(o["path"] ?? "")],
            }))}
          />
        )}
      </section>
      <section aria-label={m.schemas} className="flex flex-col gap-2">
        <h2 className="text-label font-medium">{m.schemas}</h2>
        {schemas.length === 0 ? (
          <p className="text-body text-secondary">{m.noSchemas}</p>
        ) : (
          <ul>
            {schemas.map((t) => (
              <li key={String(t["name"])} className="text-body">
                {String(t["name"])}
              </li>
            ))}
          </ul>
        )}
      </section>
      <section aria-label={m.urls} className="flex flex-col gap-2">
        <h2 className="text-label font-medium">{m.urls}</h2>
        {urls.length === 0 ? (
          <p className="text-body text-secondary">{m.noUrls}</p>
        ) : (
          <Table
            caption={m.urls}
            heads={[m.colEnvironment, m.colUrl]}
            rows={urls.map(([env, url]) => ({ key: env, cells: [env, url] }))}
          />
        )}
      </section>
    </>
  );
}

function Fields(props: {
  source: Json;
  canEdit: boolean;
  change: (patch: Json) => void;
  m: DataMessages;
}): ReactElement {
  const { source, canEdit, change, m } = props;
  const id = String(source["id"]);
  return (
    <div className="flex flex-col gap-3">
      <Field label={m.name}>
        <Input
          value={String(source["name"] ?? "")}
          disabled={!canEdit}
          onChange={(e) => change({ name: e.target.value })}
        />
      </Field>
      <Select
        label={m.kind}
        options={KINDS.map((k) => ({ value: k, label: k }))}
        value={String(source["kind"] ?? "rest")}
        disabled={!canEdit}
        onValueChange={(kind) => change({ kind })}
      />
      <Field label={m.type}>
        <Input
          value={String(source["type"] ?? "")}
          disabled={!canEdit}
          onChange={(e) => change({ type: e.target.value })}
        />
      </Field>
      <Select
        label={m.route}
        options={ROUTES.map((r) => ({
          value: r,
          label: { default: m.routeDefault, plux: m.routePlux, direct: m.routeDirect }[r],
        }))}
        value={String(source["route"] ?? "default")}
        disabled={!canEdit}
        onValueChange={(route) => change({ route: route === "default" ? undefined : route })}
      />
      <JsonField
        label={m.config}
        error={m.configInvalid}
        value={source["config"]}
        seedKey={id}
        disabled={!canEdit}
        objectOnly
        allowEmpty
        onValid={(config) => change({ config })}
      />
      <JsonField
        label={m.transaction}
        help={m.transactionHelp}
        error={m.transactionInvalid}
        value={source["transaction"]}
        seedKey={id}
        disabled={!canEdit}
        objectOnly
        allowEmpty
        rows={4}
        onValid={(transaction) => change({ transaction })}
      />
    </div>
  );
}

function Editor(props: { sourceId: string }): ReactElement {
  const i18n = useLingui().i18n;
  const m = dataMessages(i18n);
  const d = designerMessages(i18n);
  const { value } = usePluginJson();
  const [sources, setSources, seeded] = useSeededList("dataSources");
  const [mocking, setMocking] = useState(false);
  const next = value === undefined || !seeded ? undefined : { ...value, dataSources: sources };
  const state = useCommit(next, "data-source");
  const current = sources.find((s) => String(s["id"]) === props.sourceId);
  if (current === undefined) {
    return <p className="text-body text-secondary">{seeded ? m.notFound : m.loading}</p>;
  }
  const change = (patch: Json) => setSources((all) => all.map((s) => (s === current ? patched(s, patch) : s)));
  return (
    <div className="flex flex-col gap-4">
      <Fields source={current} canEdit={state.canEdit} change={change} m={m} />
      <Tables source={current} types={objects(value?.["types"])} m={m} />
      <section aria-label={m.mock} className="flex flex-col gap-2">
        <h2 className="text-label font-medium">{m.mock}</h2>
        <pre className="max-h-40 overflow-auto text-code">{JSON.stringify(current["mock"] ?? null, null, 2)}</pre>
        <div>
          <Button variant="secondary" onClick={() => setMocking(true)}>
            {m.editMock}
          </Button>
        </div>
      </section>
      <MockDialog
        key={String(mocking)}
        open={mocking}
        onClose={() => setMocking(false)}
        source={current}
        canEdit={state.canEdit}
        onUse={(mock) => change({ mock })}
        m={m}
      />
      <CommitBar state={state} m={m} d={d} />
    </div>
  );
}

/** A21: one data source with its operations, schemas, per-environment URLs and mock (STU-113). */
export function SourcePage(): ReactElement {
  const m = dataMessages(useLingui().i18n);
  const { sourceId } = useParams({ strict: false }) as { sourceId?: string };
  return (
    <Frame
      id="source-title"
      title={m.sourceTitle}
      caption={m.sourceCaption}
      back={{ to: "/o/$org/apps/$app/data", label: m.back }}
    >
      <Editor sourceId={sourceId ?? ""} />
    </Frame>
  );
}

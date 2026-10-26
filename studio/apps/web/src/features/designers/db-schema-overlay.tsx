// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Dialog, Field, Icon, Input, TypeTag } from "@plux/ui";
import type { Problem } from "@plux/validator";
import { useEffect, useMemo, useState } from "react";
import { jsonEqual } from "../editor/json-patch.ts";
import type { LoadPublished } from "../json-editor/use-published.ts";
import type { ValidatorClient } from "../problems/validator-client.ts";
import { type DesignerMessages, designerMessages } from "./designers.messages.ts";
import { ProblemList, Status } from "./parts.tsx";
import {
  type DesignerDocument,
  hasErrors,
  isObject,
  type Json,
  useApply,
  useCheck,
  useDocumentValue,
} from "./use-document.ts";
import { uuid7 } from "./uuid7.ts";

export interface DbSchemaOverlayProps {
  readonly open: boolean;
  readonly onClose: () => void;
  readonly documents: readonly DesignerDocument[] | undefined;
  readonly validatePage: ((file: string, data: string) => Promise<readonly Problem[]>) | undefined;
  /** The Go migration planner (L-12). */
  readonly tool: ValidatorClient["tool"];
  readonly loadPublished: LoadPublished;
}

const objects = (v: unknown): Json[] => (Array.isArray(v) ? v.filter(isObject) : []);
const strings = (v: unknown): string[] => (Array.isArray(v) ? v.filter((s): s is string => typeof s === "string") : []);

/** A comma-separated list as names; empty items are dropped. */
export const splitList = (text: string): string[] =>
  text
    .split(",")
    .map((s) => s.trim())
    .filter((s) => s !== "");

/** A plan's renames as `new=old` pairs and back. */
export const renameText = (rename: unknown): string =>
  isObject(rename)
    ? Object.entries(rename)
        .map(([n, o]) => `${n}=${String(o)}`)
        .join(", ")
    : "";
export function parseRename(text: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const pair of splitList(text)) {
    const [n, o] = pair.split("=").map((s) => s.trim());
    if (n !== undefined && o !== undefined && n !== "" && o !== "") {
      out[n] = o;
    }
  }
  return out;
}

/**
 * The draft of the plugin document: the edited collections, and `droppedCollections` extended by every collection
 * the document had and the draft no longer has (DB-005), minus any the draft declares again.
 */
export function draftDocument(base: Json, collections: readonly Json[]): Json {
  const next: Json = { ...base };
  const had = objects(base["collections"]);
  if (collections.length > 0 || "collections" in base) {
    next["collections"] = collections;
  }
  const live = new Set(collections.map((c) => c["id"]));
  const dropped = new Set([...strings(base["droppedCollections"]), ...had.map((c) => String(c["id"]))]);
  const list = [...dropped].filter((id) => !live.has(id)).sort();
  if (list.length > 0 || "droppedCollections" in base) {
    next["droppedCollections"] = list;
  }
  if (list.length === 0) {
    delete next["droppedCollections"];
  }
  return next;
}

type Preview =
  | { readonly state: "none" }
  | { readonly state: "checking" }
  | { readonly state: "unavailable" }
  | { readonly state: "done"; readonly findings: readonly Problem[] };

/** The migration of the draft against the last release, computed by the Go planner in the validator module. */
function usePreview(
  open: boolean,
  path: string | undefined,
  next: Json | undefined,
  tool: ValidatorClient["tool"],
  loadPublished: LoadPublished,
): Preview {
  const [released, setReleased] = useState<Json[] | null | undefined>(undefined);
  const [preview, setPreview] = useState<Preview>({ state: "checking" });
  useEffect(() => {
    if (!open || path === undefined) {
      return undefined;
    }
    let live = true;
    loadPublished(path)
      .then((text) => {
        const doc: unknown = text === null ? null : JSON.parse(text);
        if (live) {
          setReleased(isObject(doc) ? objects(doc["collections"]) : null);
        }
      })
      .catch(() => live && setReleased(null));
    return () => {
      live = false;
    };
  }, [open, path, loadPublished]);
  useEffect(() => {
    if (!open || next === undefined || released === undefined) {
      return undefined;
    }
    if (released === null) {
      setPreview({ state: "none" });
      return undefined;
    }
    if (tool === undefined) {
      setPreview({ state: "unavailable" });
      return undefined;
    }
    let live = true;
    setPreview({ state: "checking" });
    const timer = setTimeout(() => {
      tool("collections", [
        JSON.stringify(released),
        JSON.stringify(objects(next["collections"])),
        JSON.stringify(strings(next["droppedCollections"])),
      ])
        .then((found) => {
          if (live) {
            setPreview({ state: "done", findings: objects(found).map(finding) });
          }
        })
        .catch(() => live && setPreview({ state: "unavailable" }));
    }, 300);
    return () => {
      live = false;
      clearTimeout(timer);
    };
  }, [open, next, released, tool]);
  return preview;
}

function finding(d: Json): Problem {
  const sev = String(d["severity"]);
  return {
    code: String(d["code"] ?? ""),
    severity: sev === "warning" || sev === "info" ? sev : "error",
    file: "",
    path: "",
    message: String(d["message"] ?? ""),
  };
}

function PreviewPanel({ preview, m }: { preview: Preview; m: DesignerMessages }) {
  let body: React.ReactNode;
  if (preview.state === "none") {
    body = <p className="text-body text-secondary">{m.dbPreviewNoRelease}</p>;
  } else if (preview.state === "checking") {
    body = <p className="text-body text-secondary">{m.dbPreviewChecking}</p>;
  } else if (preview.state === "unavailable") {
    body = <p className="text-body">{m.dbPreviewUnavailable}</p>;
  } else if (preview.findings.length === 0) {
    body = <p className="text-body text-secondary">{m.dbPreviewClean}</p>;
  } else {
    body = (
      <ul className="flex flex-col gap-1">
        {preview.findings.map((f) => (
          <li key={`${f.code}${f.message}`} className="text-body">
            <TypeTag>{f.code}</TypeTag> {f.message}
          </li>
        ))}
      </ul>
    );
  }
  return (
    <section aria-label={m.dbPreview} className="max-h-40 overflow-y-auto border-t border-hairline pt-3">
      <h3 className="text-label font-medium">{m.dbPreview}</h3>
      {body}
    </section>
  );
}

function CollectionEditor(props: {
  collection: Json;
  onChange: (c: Json) => void;
  canEdit: boolean;
  m: DesignerMessages;
}) {
  const { collection: c, onChange, canEdit, m } = props;
  const fields = objects(c["fields"]);
  const plans = objects(c["migrations"]);
  const set = (patch: Json) => onChange({ ...c, ...patch });
  const setField = (i: number, patch: Json) =>
    set({ fields: fields.map((f, j) => (j === i ? { ...f, ...patch } : f)) });
  const setPlan = (i: number, patch: Json) => {
    const merged = plans.map((p, j) => (j === i ? { ...p, ...patch } : p));
    set({ migrations: merged.map(tidyPlan) });
  };
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap gap-3">
        <Field label={m.dbKey}>
          <Input value={String(c["key"] ?? "")} disabled={!canEdit} onChange={(e) => set({ key: e.target.value })} />
        </Field>
        <Field label={m.dbVersion}>
          <Input
            type="number"
            min={1}
            value={String(c["version"] ?? 1)}
            disabled={!canEdit}
            onChange={(e) => set({ version: Math.max(1, Number.parseInt(e.target.value, 10) || 1) })}
          />
        </Field>
        <Field label={m.dbPrimaryKey}>
          <Input
            value={strings(c["primaryKey"]).join(", ")}
            disabled={!canEdit}
            onChange={(e) => set({ primaryKey: splitList(e.target.value) })}
          />
        </Field>
      </div>
      <section aria-label={m.dbFields} className="flex flex-col gap-2">
        <h3 className="text-label font-medium">{m.dbFields}</h3>
        {fields.map((f, i) => (
          // The field's position is its identity: names are edited in place.
          // biome-ignore lint/suspicious/noArrayIndexKey: rows are edited in place
          <div key={i} className="flex flex-wrap items-end gap-3">
            <Field label={m.dbFieldName}>
              <Input
                value={String(f["name"] ?? "")}
                disabled={!canEdit}
                onChange={(e) => setField(i, { name: e.target.value })}
              />
            </Field>
            <Field label={m.dbFieldType}>
              <Input
                value={String(f["type"] ?? "")}
                disabled={!canEdit}
                onChange={(e) => setField(i, { type: e.target.value })}
              />
            </Field>
            <label className="flex items-center gap-2 text-body">
              <input
                type="checkbox"
                checked={f["sensitive"] === true}
                disabled={!canEdit}
                onChange={(e) => setField(i, { sensitive: e.target.checked ? true : undefined })}
              />
              {m.dbSensitive}
            </label>
            <Button
              variant="secondary"
              disabled={!canEdit}
              onClick={() => set({ fields: fields.filter((_, j) => j !== i) })}
            >
              {m.dbRemoveField}
            </Button>
          </div>
        ))}
        <div>
          <Button
            variant="secondary"
            disabled={!canEdit}
            onClick={() => set({ fields: [...fields, { name: "", type: "string" }] })}
          >
            <Icon name="plus" />
            {m.dbAddField}
          </Button>
        </div>
      </section>
      <section aria-label={m.dbPlans} className="flex flex-col gap-2">
        <h3 className="text-label font-medium">{m.dbPlans}</h3>
        {plans.map((p, i) => (
          // biome-ignore lint/suspicious/noArrayIndexKey: rows are edited in place
          <div key={i} className="flex flex-wrap items-end gap-3">
            <Field label={m.dbPlanFrom}>
              <Input
                type="number"
                min={1}
                value={String(p["from"] ?? 1)}
                disabled={!canEdit}
                onChange={(e) => setPlan(i, { from: Math.max(1, Number.parseInt(e.target.value, 10) || 1) })}
              />
            </Field>
            <Field label={m.dbPlanDrop}>
              <Input
                value={strings(p["drop"]).join(", ")}
                disabled={!canEdit}
                onChange={(e) => setPlan(i, { drop: splitList(e.target.value) })}
              />
            </Field>
            <Field label={m.dbPlanReset}>
              <Input
                value={strings(p["reset"]).join(", ")}
                disabled={!canEdit}
                onChange={(e) => setPlan(i, { reset: splitList(e.target.value) })}
              />
            </Field>
            <Field label={m.dbPlanRename}>
              <Input
                defaultValue={renameText(p["rename"])}
                disabled={!canEdit}
                onChange={(e) => setPlan(i, { rename: parseRename(e.target.value) })}
              />
            </Field>
            <Button
              variant="secondary"
              disabled={!canEdit}
              onClick={() => set({ migrations: plans.filter((_, j) => j !== i) })}
            >
              {m.dbRemovePlan}
            </Button>
          </div>
        ))}
        <div>
          <Button
            variant="secondary"
            disabled={!canEdit}
            onClick={() => set({ migrations: [...plans, { from: Math.max(1, Number(c["version"] ?? 1) - 1) }] })}
          >
            <Icon name="plus" />
            {m.dbAddPlan}
          </Button>
        </div>
      </section>
    </div>
  );
}

/** A plan without empty lists or renames, as the schema wants it. */
function tidyPlan(p: Json): Json {
  const out: Json = { from: p["from"] };
  for (const k of ["drop", "reset"]) {
    if (strings(p[k]).length > 0) {
      out[k] = p[k];
    }
  }
  if (isObject(p["rename"]) && Object.keys(p["rename"]).length > 0) {
    out["rename"] = p["rename"];
  }
  return out;
}

/** The DB schema designer V7 (STU-063): collections and fields of the on-device database, with the migration preview from the Go planner (DB-005). */
export function DbSchemaOverlay(props: DbSchemaOverlayProps) {
  const { i18n } = useLingui();
  const m = useMemo(() => designerMessages(i18n), [i18n]);
  const { path, value } = useDocumentValue(props.documents, "plugin.json");
  const { canEdit, apply } = useApply(path, "db-schema");
  const [collections, setCollections] = useState<Json[]>([]);
  const [selected, setSelected] = useState(0);
  const [note, setNote] = useState("");
  // biome-ignore lint/correctness/useExhaustiveDependencies: the draft is seeded when the overlay opens
  useEffect(() => {
    if (props.open) {
      setCollections(structuredClone(objects(value?.["collections"])));
      setSelected(0);
    }
  }, [props.open]);
  // biome-ignore lint/correctness/useExhaustiveDependencies: a note belongs to the draft it was made for
  useEffect(() => setNote(""), [collections]);
  const next = useMemo(
    () => (value === undefined ? undefined : draftDocument(value, collections)),
    [value, collections],
  );
  const check = useCheck(props.open, path, next, props.validatePage);
  const preview = usePreview(props.open, path, next, props.tool, props.loadPublished);
  const changed = next !== undefined && !jsonEqual(next, value);
  const canApply = canEdit && changed && !hasErrors(check);
  const current = collections[selected];
  return (
    <Dialog
      open={props.open}
      onOpenChange={(open) => {
        if (!open) {
          props.onClose();
        }
      }}
      size="full"
      title={m.dbTitle}
      labels={{ close: m.dbClose }}
    >
      <div className="flex min-h-0 flex-1 flex-col gap-3">
        <div className="flex flex-wrap items-center gap-3">
          <Button
            variant="secondary"
            disabled={!canEdit}
            onClick={() => {
              setCollections((all) => [
                ...all,
                {
                  id: uuid7(),
                  key: `collection-${all.length + 1}`,
                  version: 1,
                  primaryKey: ["id"],
                  fields: [{ name: "id", type: "string" }],
                },
              ]);
              setSelected(collections.length);
            }}
          >
            <Icon name="plus" />
            {m.dbAdd}
          </Button>
          <Button
            variant="secondary"
            disabled={!canEdit || current === undefined}
            onClick={() => {
              setCollections((all) => all.filter((_, i) => i !== selected));
              setSelected(0);
            }}
          >
            {m.dbRemove}
          </Button>
          <Button
            disabled={!canApply}
            onClick={() => {
              if (next !== undefined) {
                apply(next);
                setNote(m.applied);
              }
            }}
          >
            {m.apply}
          </Button>
          <Status check={check} valid={!hasErrors(check)} changed={changed} note={note} m={m} />
        </div>
        {!canEdit && <p className="text-caption text-secondary">{m.readOnly}</p>}
        <div className="flex min-h-0 flex-1 gap-6 overflow-y-auto">
          <nav aria-label={m.dbList} className="flex w-56 shrink-0 flex-col gap-1">
            {collections.length === 0 && <p className="text-body text-secondary">{m.dbNone}</p>}
            {collections.map((c, i) => (
              <Button
                key={String(c["id"])}
                variant={i === selected ? "primary" : "secondary"}
                aria-pressed={i === selected}
                onClick={() => setSelected(i)}
              >
                {String(c["key"] ?? "")}
              </Button>
            ))}
          </nav>
          <div className="min-w-0 flex-1">
            {current === undefined ? (
              <p className="text-body text-secondary">{m.dbSelect}</p>
            ) : (
              <CollectionEditor
                collection={current}
                canEdit={canEdit}
                m={m}
                onChange={(c) => setCollections((all) => all.map((x, i) => (i === selected ? c : x)))}
              />
            )}
          </div>
        </div>
        <PreviewPanel preview={preview} m={m} />
        <ProblemList problems={check.state === "done" ? check.problems : []} m={m} />
      </div>
    </Dialog>
  );
}

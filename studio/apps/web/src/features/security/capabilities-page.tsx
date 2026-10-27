// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Dialog, Field, Textarea } from "@plux/ui";
import { type ReactElement, useState } from "react";
import { Frame } from "./parts.tsx";
import { securityMessages } from "./security.messages.ts";
import { useAppDocument, usePermissions, useSaveCapabilities, useScope } from "./use-security.ts";

const lists = ["deviceApis", "networkDomains", "functions"] as const;
type ListKey = (typeof lists)[number];
type Grants = Partial<Record<ListKey, string[]>>;

const split = (text: string) =>
  text
    .split("\n")
    .map((s) => s.trim())
    .filter((s) => s !== "");

/** A36: the capability grants of the app document (SEC-080); editing is D65 through the document RPCs. */
export function CapabilitiesPage(): ReactElement {
  const m = securityMessages(useLingui().i18n);
  const { app } = useScope();
  const doc = useAppDocument(app);
  const save = useSaveCapabilities();
  const can = usePermissions().has("app.manage");
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState<Record<ListKey, string>>({ deviceApis: "", networkDomains: "", functions: "" });
  const [failed, setFailed] = useState(false);
  const grants = ((doc.data?.json as { capabilities?: Grants } | undefined)?.capabilities ?? {}) as Grants;
  const labels: Record<ListKey, string> = {
    deviceApis: m.deviceApis,
    networkDomains: m.networkDomains,
    functions: m.functions,
  };
  const edit = () => {
    setDraft({
      deviceApis: (grants.deviceApis ?? []).join("\n"),
      networkDomains: (grants.networkDomains ?? []).join("\n"),
      functions: (grants.functions ?? []).join("\n"),
    });
    setOpen(true);
  };
  const submit = () => {
    if (doc.data === undefined) {
      return;
    }
    const next: Grants = {};
    for (const k of lists) {
      const items = split(draft[k]);
      if (items.length > 0 || grants[k] !== undefined) {
        next[k] = items;
      }
    }
    setOpen(false);
    save(app, { ...doc.data.json, capabilities: next }, doc.data.revision).then(
      () => setFailed(false),
      () => setFailed(true),
    );
  };
  return (
    <Frame id="caps-title" title={m.capsTitle} caption={m.capsCaption} loading={doc.isPending} failed={doc.isError}>
      <dl className="flex flex-col gap-1 text-body">
        {lists.map((k) => (
          <div key={k} className="flex gap-4">
            <dt className="w-48 text-secondary">{labels[k]}</dt>
            <dd className="font-mono">{(grants[k] ?? []).join(", ") || m.none}</dd>
          </div>
        ))}
      </dl>
      <div>
        <Button disabled={!can} title={can ? undefined : m.revokeNoPermission} onClick={edit}>
          {m.editGrants}
        </Button>
      </div>
      {failed && (
        <p role="alert" className="text-body text-danger">
          {m.saveFailed}
        </p>
      )}
      <Dialog
        open={open}
        onOpenChange={setOpen}
        size="sm"
        title={m.editGrants}
        description={m.editGrantsDescription}
        labels={{ close: m.close }}
      >
        {lists.map((k) => (
          <Field key={k} label={labels[k]}>
            <Textarea value={draft[k]} onChange={(e) => setDraft({ ...draft, [k]: e.target.value })} />
          </Field>
        ))}
        <div className="flex justify-end gap-2">
          <Button variant="secondary" onClick={() => setOpen(false)}>
            {m.cancel}
          </Button>
          <Button onClick={submit}>{m.save}</Button>
        </div>
      </Dialog>
    </Frame>
  );
}

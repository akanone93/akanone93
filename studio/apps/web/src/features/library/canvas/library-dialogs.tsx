// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { LibraryItem } from "@plux/api";
import { Button, Dialog, Field, Input, Select } from "@plux/ui";
import { type FormEvent, type ReactElement, useState } from "react";
import { canvasLibraryMessages } from "./canvas.messages.ts";
import type { TemplateParameter } from "./template-doc.ts";

const KEY = /^[a-z][a-z0-9-]*$/;
const slug = (name: string) =>
  name
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");

type Details = { readonly name: string; readonly key: string; readonly description: string };

function useDetails(initial: string) {
  const [name, setName] = useState(initial);
  const [key, setKey] = useState<string | undefined>();
  const [description, setDescription] = useState("");
  const effectiveKey = key ?? slug(name);
  return {
    details: { name: name.trim(), key: effectiveKey, description } satisfies Details,
    valid: name.trim() !== "" && KEY.test(effectiveKey),
    set: { name: setName, key: setKey, description: setDescription },
    values: { name, key: effectiveKey, description },
  };
}

function DetailFields({ form }: { readonly form: ReturnType<typeof useDetails> }): ReactElement {
  const m = canvasLibraryMessages(useLingui().i18n);
  return (
    <>
      <Field label={m.name}>
        <Input value={form.values.name} onChange={(e) => form.set.name(e.target.value)} />
      </Field>
      <Field label={m.key}>
        <Input value={form.values.key} onChange={(e) => form.set.key(e.target.value)} />
      </Field>
      <Field label={m.description}>
        <Input value={form.values.description} onChange={(e) => form.set.description(e.target.value)} />
      </Field>
    </>
  );
}

/** D28: save the selection as a template in two steps: details, then exposed parameters and visibility. */
export function SaveTemplateDialog({
  candidates,
  save,
  onClose,
}: {
  readonly candidates: readonly TemplateParameter[];
  readonly save: (details: Details & { visibility: string }, parameters: TemplateParameter[]) => Promise<void>;
  readonly onClose: () => void;
}): ReactElement {
  const m = canvasLibraryMessages(useLingui().i18n);
  const form = useDetails("");
  const [step, setStep] = useState<1 | 2>(1);
  const [exposed, setExposed] = useState<ReadonlySet<string>>(new Set());
  const [visibility, setVisibility] = useState("private");
  const [failed, setFailed] = useState(false);
  const submit = async () => {
    try {
      await save(
        { ...form.details, visibility },
        candidates.filter((c) => exposed.has(c.path)),
      );
      onClose();
    } catch {
      setFailed(true);
    }
  };
  const toggle = (path: string) =>
    setExposed((s) => {
      const next = new Set(s);
      if (!next.delete(path)) {
        next.add(path);
      }
      return next;
    });
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()} title={m.saveTitle} labels={{ close: m.close }}>
      {step === 1 ? (
        <div className="flex flex-col gap-3">
          <p className="text-caption text-secondary">{m.stepDetails}</p>
          <DetailFields form={form} />
          <Button disabled={!form.valid} onClick={() => setStep(2)}>
            {m.next}
          </Button>
        </div>
      ) : (
        <div className="flex flex-col gap-3">
          <p className="text-caption text-secondary">{m.stepExpose}</p>
          <h3 className="text-body font-medium">{m.exposeHeading}</h3>
          {candidates.length === 0 && <p className="text-body text-secondary">{m.exposeNone}</p>}
          {candidates.map((c) => (
            <label key={c.path} className="flex items-center gap-2 text-body">
              <input type="checkbox" checked={exposed.has(c.path)} onChange={() => toggle(c.path)} />
              {c.name}
            </label>
          ))}
          <p className="text-caption text-secondary">{m.flaggedNote}</p>
          <Select
            label={m.visibility}
            value={visibility}
            options={[
              { value: "private", label: m.private },
              { value: "organization", label: m.organization },
            ]}
            onValueChange={setVisibility}
          />
          {failed && (
            <p role="alert" className="text-body text-danger">
              {m.saveFailed}
            </p>
          )}
          <div className="flex justify-end gap-2">
            <Button variant="secondary" onClick={() => setStep(1)}>
              {m.back}
            </Button>
            <Button onClick={() => void submit()}>{m.save}</Button>
          </div>
        </div>
      )}
    </Dialog>
  );
}

/** D30: name the component the selection becomes. */
export function ExtractComponentDialog({
  extract,
  onClose,
}: {
  readonly extract: (details: Details) => Promise<void>;
  readonly onClose: () => void;
}): ReactElement {
  const m = canvasLibraryMessages(useLingui().i18n);
  const form = useDetails("");
  const [failed, setFailed] = useState(false);
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    try {
      await extract(form.details);
      onClose();
    } catch {
      setFailed(true);
    }
  };
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()} title={m.extractTitle} labels={{ close: m.close }}>
      <form className="flex flex-col gap-3" onSubmit={(e) => void submit(e)}>
        <DetailFields form={form} />
        {failed && (
          <p role="alert" className="text-body text-danger">
            {m.saveFailed}
          </p>
        )}
        <Button type="submit" disabled={!form.valid}>
          {m.extractConfirm}
        </Button>
      </form>
    </Dialog>
  );
}

/** D29 and STU-055: pick a template, then fill its parameters. */
export function InsertTemplateDialog({
  templates,
  state,
  load,
  insert,
  onClose,
}: {
  readonly templates: readonly LibraryItem[];
  readonly state: "loading" | "ready" | "failed";
  readonly load: (item: LibraryItem) => Promise<Record<string, unknown>>;
  readonly insert: (doc: Record<string, unknown>, args: Record<string, string>) => void;
  readonly onClose: () => void;
}): ReactElement {
  const m = canvasLibraryMessages(useLingui().i18n);
  const [doc, setDoc] = useState<Record<string, unknown> | undefined>();
  const [args, setArgs] = useState<Record<string, string>>({});
  const [failed, setFailed] = useState(false);
  const parameters = Array.isArray(doc?.["parameters"]) ? (doc["parameters"] as TemplateParameter[]) : [];
  const pick = async (id: string) => {
    const item = templates.find((t) => t.id === id);
    try {
      setDoc(item === undefined ? undefined : await load(item));
      setFailed(false);
    } catch {
      setFailed(true);
    }
  };
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()} title={m.insertTitle} labels={{ close: m.close }}>
      <div className="flex flex-col gap-3">
        {state === "loading" && <p className="text-body text-secondary">{m.insertLoading}</p>}
        {(state === "failed" || failed) && (
          <p role="alert" className="text-body text-danger">
            {m.insertFailed}
          </p>
        )}
        {state === "ready" && templates.length === 0 && <p className="text-body text-secondary">{m.insertNone}</p>}
        {templates.length > 0 && (
          <Select
            label={m.insertPick}
            value=""
            placeholder={m.insertPick}
            options={templates.map((t) => ({ value: t.id, label: t.name }))}
            onValueChange={(id) => void pick(id)}
          />
        )}
        {parameters.map((p) => (
          <Field key={p.name} label={p.name}>
            <Input value={args[p.name] ?? ""} onChange={(e) => setArgs({ ...args, [p.name]: e.target.value })} />
          </Field>
        ))}
        <p className="text-caption text-secondary">{m.insertCopyNote}</p>
        <Button disabled={doc === undefined} onClick={() => doc !== undefined && insert(doc, args)}>
          {m.insertConfirm}
        </Button>
      </div>
    </Dialog>
  );
}

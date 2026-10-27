// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { limitDefault } from "@plux/schema";
import { Button, Dialog, Field, Input, Select, Textarea } from "@plux/ui";
import { useNavigate } from "@tanstack/react-router";
import { type ReactElement, useState } from "react";
import { localisationAreaMessages } from "./area.messages.ts";
import { addLocale, localeProblem, writeMessages } from "./area-actions.ts";
import { useArea } from "./area-scope.tsx";
import { matchKeys } from "./grid-model.ts";
import {
  type Entries,
  exportTranslations,
  FORMATS,
  formatOfName,
  parseTranslations,
  type TranslationFormat,
} from "./translation-formats.ts";

const formatOptions = FORMATS.map((f) => ({ value: f, label: f.toUpperCase() }));

/** D53: adds a locale to the app. */
export function AddLocaleDialog(props: { readonly open: boolean; readonly onClose: () => void }): ReactElement {
  const m = localisationAreaMessages(useLingui().i18n);
  const area = useArea();
  const navigate = useNavigate();
  const [code, setCode] = useState("");
  const [shown, setShown] = useState(false);
  const problem = localeProblem(code, area.sessions.locales);
  const submit = () => {
    setShown(true);
    if (problem === undefined && area.sessions.canEditKeys) {
      addLocale(area, code);
      void navigate({ to: ".", search: (prev: object) => ({ ...prev, locale: code }) });
      setCode("");
      setShown(false);
      props.onClose();
    }
  };
  return (
    <Dialog
      open={props.open}
      onOpenChange={(o) => !o && props.onClose()}
      title={m.addLocaleTitle}
      labels={{ close: m.close }}
    >
      <div className="flex flex-col gap-3">
        <Field
          label={m.localeCode}
          error={shown && problem !== undefined ? (problem === "taken" ? m.localeTaken : m.localeInvalid) : undefined}
        >
          <Input value={code} dir="ltr" onChange={(e) => setCode(e.target.value.trim())} />
        </Field>
        <Button onClick={submit} disabled={!area.sessions.canEditKeys}>
          {m.addLocaleConfirm}
        </Button>
      </div>
    </Dialog>
  );
}

type Read =
  | { readonly state: "none" }
  | { readonly state: "error"; readonly reason: "tooLarge" | "malformed" | "empty" }
  | { readonly state: "ready"; readonly matched: Entries; readonly unknown: number };

/** Reads an untrusted file as data: its size is capped before it is read, and its text is only ever shown as text. */
async function readFile(
  file: File,
  format: TranslationFormat,
  keys: ReturnType<typeof useArea>["keys"],
): Promise<Read> {
  const max = limitDefault("translation.importSize");
  if (file.size > max) {
    return { state: "error", reason: "tooLarge" };
  }
  const parsed = parseTranslations(format, await file.text(), max);
  if (!parsed.ok) {
    return { state: "error", reason: parsed.reason };
  }
  const { matched, unknown } = matchKeys(parsed.entries, keys);
  return { state: "ready", matched, unknown: unknown.length };
}

/** D54: imports translations from an XLIFF, ARB, JSON or CSV file into the chosen locale. */
export function ImportDialog(props: { readonly open: boolean; readonly onClose: () => void }): ReactElement {
  const m = localisationAreaMessages(useLingui().i18n);
  const area = useArea();
  const [format, setFormat] = useState<TranslationFormat>("json");
  const [read, setRead] = useState<Read>({ state: "none" });
  const reasons = { tooLarge: m.importTooLarge, malformed: m.importMalformed, empty: m.importEmpty };
  const choose = async (file: File | undefined) => {
    if (file === undefined) {
      return;
    }
    const f = formatOfName(file.name) ?? format;
    setFormat(f);
    setRead(await readFile(file, f, area.keys));
  };
  const apply = async () => {
    if (read.state === "ready" && area.canEdit) {
      await writeMessages(area, read.matched, "translated");
      props.onClose();
    }
  };
  return (
    <Dialog
      open={props.open}
      onOpenChange={(o) => !o && props.onClose()}
      title={m.importTitle}
      labels={{ close: m.close }}
    >
      <div className="flex flex-col gap-3">
        <p className="text-body">{m.importInto({ locale: area.sessions.locale })}</p>
        <Select
          label={m.importFormat}
          options={formatOptions}
          value={format}
          onValueChange={(v) => setFormat(v as TranslationFormat)}
        />
        <Field label={m.importFile}>
          <input
            type="file"
            accept=".xlf,.xliff,.arb,.json,.csv"
            aria-label={m.importFile}
            onChange={(e) => void choose(e.target.files?.[0])}
          />
        </Field>
        {read.state === "error" && (
          <p role="alert" className="text-body text-danger">
            {reasons[read.reason]}
          </p>
        )}
        {read.state === "ready" && (
          <p role="status" className="text-body">
            {m.importPreview({ matched: Object.keys(read.matched).length, unknown: read.unknown })}
          </p>
        )}
        <Button onClick={() => void apply()} disabled={read.state !== "ready" || !area.canEdit}>
          {m.importApply}
        </Button>
      </div>
    </Dialog>
  );
}

function download(name: string, text: string): void {
  const url = URL.createObjectURL(new Blob([text], { type: "text/plain;charset=utf-8" }));
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  a.click();
  URL.revokeObjectURL(url);
}

/** D55: exports the chosen locale's translations as XLIFF, ARB, JSON or CSV. */
export function ExportDialog(props: { readonly open: boolean; readonly onClose: () => void }): ReactElement {
  const m = localisationAreaMessages(useLingui().i18n);
  const area = useArea();
  const [format, setFormat] = useState<TranslationFormat>("json");
  const { locale, defaultLocale } = area.sessions;
  const text = exportTranslations(format, {
    locale,
    sourceLocale: defaultLocale,
    keys: area.keys.map((k) => k.key),
    source: byName(area, defaultLocale),
    target: byName(area, locale),
  });
  const ext = { xliff: "xlf", arb: "arb", json: "json", csv: "csv" }[format];
  return (
    <Dialog
      open={props.open}
      onOpenChange={(o) => !o && props.onClose()}
      title={m.exportTitle}
      labels={{ close: m.close }}
    >
      <div className="flex flex-col gap-3">
        <Select
          label={m.importFormat}
          options={formatOptions}
          value={format}
          onValueChange={(v) => setFormat(v as TranslationFormat)}
        />
        <Textarea aria-label={m.exportTitle} readOnly dir="ltr" rows={10} value={text} />
        <Button onClick={() => download(`${locale}.${ext}`, text)}>{m.exportDownload}</Button>
      </div>
    </Dialog>
  );
}

function byName(area: ReturnType<typeof useArea>, locale: string): Entries {
  const messages = area.messages.get(locale) ?? {};
  return Object.fromEntries(area.keys.map((k) => [k.key, messages[k.id] ?? ""]));
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Dialog, Field, Icon, Input, Select, Textarea } from "@plux/ui";
import { type ReactElement, useState } from "react";
import { tokenValue } from "../designers/theme-overlay.tsx";
import { type DesignMessages, designMessages } from "./design.messages.ts";
import {
  contrastOf,
  dropToken,
  type Mode,
  mergeTokens,
  parseDtcg,
  putToken,
  setMode,
  TOKEN_TYPES,
  validName,
} from "./design-model.ts";
import { useDesign } from "./scope.tsx";

export const show = (v: unknown): string => (v === undefined ? "" : typeof v === "string" ? v : JSON.stringify(v));

/** The label of a mode. */
export const modeLabel = (m: DesignMessages, mode: Mode): string =>
  mode === "light" ? m.colLight : mode === "dark" ? m.colDark : m.colContrast;

/** D57: edit the values of one token in every mode; each change goes through the edit pipeline at once. */
export function EditTokenDialog(props: {
  readonly path: readonly string[];
  readonly onClose: () => void;
}): ReactElement {
  const m = designMessages(useLingui().i18n);
  const ctx = useDesign();
  const name = props.path.join(".");
  const tokens = ctx.value?.["tokens"];
  const row = ctx.rows.find((r) => r.path.join(".") === name);
  const current: Record<Mode, unknown> = {
    light: row?.light,
    dark: row?.dark,
    contrast: contrastOf(tokens, props.path),
  };
  const change = (mode: Mode, text: string) =>
    ctx.write(`${name}:${mode}`, (t) =>
      setMode(t, props.path, mode, text === "" && mode !== "light" ? undefined : tokenValue(text, current[mode])),
    );
  return (
    <Dialog open onOpenChange={(o) => !o && props.onClose()} title={m.editTitle(name)} labels={{ close: m.close }}>
      <div className="flex flex-col gap-3">
        {(["light", "dark", "contrast"] as const).map((mode) => (
          <Field key={mode} label={m.valueOf(modeLabel(m, mode))}>
            <Input value={show(current[mode])} disabled={!ctx.canEdit} onChange={(e) => change(mode, e.target.value)} />
          </Field>
        ))}
      </div>
    </Dialog>
  );
}

const typeOptions = [...TOKEN_TYPES].map((t) => ({ value: t, label: t }));

/** The fields that add a token at a path below `prefix`. */
export function AddToken(props: { readonly prefix: readonly string[]; readonly typed: boolean }): ReactElement {
  const m = designMessages(useLingui().i18n);
  const ctx = useDesign();
  const [name, setName] = useState("");
  const [value, setValue] = useState("");
  const [type, setType] = useState("color");
  const addable = ctx.canEdit && validName(name) && value !== "";
  return (
    <div className="flex flex-wrap items-end gap-3">
      <Field label={m.newName}>
        <Input value={name} disabled={!ctx.canEdit} onChange={(e) => setName(e.target.value)} />
      </Field>
      <Field label={m.newValue}>
        <Input value={value} disabled={!ctx.canEdit} onChange={(e) => setValue(e.target.value)} />
      </Field>
      {props.typed && <Select label={m.newType} options={typeOptions} value={type} onValueChange={setType} />}
      <Button
        variant="secondary"
        disabled={!addable}
        onClick={() => {
          ctx.write(`add:${name}`, (t) =>
            putToken(t, [...props.prefix, ...name.split(".")], value, props.typed ? type : undefined),
          );
          setName("");
          setValue("");
        }}
      >
        <Icon name="plus" />
        {m.addToken}
      </Button>
    </div>
  );
}

/** A button that removes the token at `path`. */
export function RemoveToken(props: { readonly path: readonly string[] }): ReactElement {
  const m = designMessages(useLingui().i18n);
  const ctx = useDesign();
  const name = props.path.join(".");
  return (
    <Button
      variant="secondary"
      aria-label={m.removeToken(name)}
      disabled={!ctx.canEdit}
      onClick={() => ctx.write(`${name}:remove`, (t) => dropToken(t, props.path))}
    >
      <Icon name="trash" />
    </Button>
  );
}

const ERRORS = { tooLarge: "importTooLarge", notJson: "importNotJson", notObject: "importNotObject" } as const;
const REASONS = { name: "skipName", type: "skipType", value: "skipValue", depth: "skipDepth" } as const;

/** D58: import design tokens in the DTCG format. The text is parsed as data only; what is skipped is listed. */
export function ImportDialog(props: { readonly onClose: () => void }): ReactElement {
  const m = designMessages(useLingui().i18n);
  const ctx = useDesign();
  const [text, setText] = useState("");
  const [result, setResult] = useState<ReturnType<typeof parseDtcg> | undefined>(undefined);
  const [done, setDone] = useState(false);
  const apply = () => {
    if (result?.ok === true) {
      ctx.write("import", (t) => mergeTokens(t, result.tokens));
      setDone(true);
    }
  };
  return (
    <Dialog open onOpenChange={(o) => !o && props.onClose()} title={m.importTitle} labels={{ close: m.close }}>
      <div className="flex flex-col gap-3">
        <Field label={m.importLabel}>
          <Textarea
            value={text}
            rows={8}
            onChange={(e) => {
              setText(e.target.value);
              setResult(undefined);
              setDone(false);
            }}
          />
        </Field>
        <Button variant="secondary" disabled={text === ""} onClick={() => setResult(parseDtcg(text))}>
          {m.importRead}
        </Button>
        {result?.ok === false && (
          <p role="alert" className="text-body">
            {m[ERRORS[result.error]]}
          </p>
        )}
        {result?.ok === true && <Summary result={result} m={m} />}
        <Button disabled={result?.ok !== true || result.imported === 0 || !ctx.canEdit || done} onClick={apply}>
          {m.importApply}
        </Button>
        {done && (
          <p role="status" className="text-body">
            {m.importDone}
          </p>
        )}
      </div>
    </Dialog>
  );
}

function Summary({
  result,
  m,
}: {
  readonly result: Extract<ReturnType<typeof parseDtcg>, { ok: true }>;
  readonly m: DesignMessages;
}) {
  return (
    <div className="flex flex-col gap-1">
      <p role="status" className="text-body">
        {m.importSummary(result.imported, result.skipped.length)}
      </p>
      {result.skipped.length > 0 && (
        <>
          <h3 className="text-label font-medium">{m.skippedHeading}</h3>
          <ul className="text-caption">
            {result.skipped.slice(0, 50).map((s) => (
              <li key={`${s.path}:${s.reason}`}>
                <span className="font-mono">{s.path}</span>: {m[REASONS[s.reason]]}
              </li>
            ))}
          </ul>
        </>
      )}
    </div>
  );
}

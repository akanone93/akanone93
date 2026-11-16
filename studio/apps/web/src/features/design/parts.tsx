// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { limitDefault } from "@plux/schema";
import { Button, Dialog, Field, FileDrop, Icon, Input, Select, Textarea } from "@plux/ui";
import { type ReactElement, useState } from "react";
import { ColourField } from "../../components/colour-field.tsx";
import { tokenValue } from "../designers/theme-overlay.tsx";
import { type DesignMessages, designMessages } from "./design.messages.ts";
import {
  contrastOf,
  dropBaseToken,
  dropToken,
  importChanges,
  type Mode,
  mergeTokens,
  parseDtcg,
  putToken,
  setMode,
  TOKEN_TYPES,
  typedValue,
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
            {row?.type === "color" ? (
              <ColourField
                value={show(current[mode])}
                pickLabel={m.pickColour(modeLabel(m, mode))}
                disabled={!ctx.canEdit}
                onChange={(v) => change(mode, v)}
              />
            ) : (
              <Input
                value={show(current[mode])}
                disabled={!ctx.canEdit}
                onChange={(e) => change(mode, e.target.value)}
              />
            )}
          </Field>
        ))}
      </div>
    </Dialog>
  );
}

const typeOptions = [...TOKEN_TYPES].map((t) => ({ value: t, label: t }));

/** The fields that add a token at a path below `prefix`: its path, its type when `typed`, and its value. */
export function AddToken(props: { readonly prefix: readonly string[]; readonly typed: boolean }): ReactElement {
  const m = designMessages(useLingui().i18n);
  const ctx = useDesign();
  const [name, setName] = useState("");
  const [value, setValue] = useState("");
  const [type, setType] = useState("color");
  const addable = ctx.canEdit && validName(name) && value !== "";
  const colour = props.typed && type === "color";
  return (
    <div className="token-add">
      <Field label={m.newName}>
        <Input
          value={name}
          disabled={!ctx.canEdit}
          placeholder="color.brand"
          spellCheck={false}
          onChange={(e) => setName(e.target.value)}
        />
      </Field>
      {props.typed && (
        <Field label={m.newType}>
          <Select
            label={m.newType}
            options={typeOptions}
            value={type}
            onValueChange={setType}
            disabled={!ctx.canEdit}
          />
        </Field>
      )}
      <Field label={m.newValue}>
        {colour ? (
          <ColourField value={value} pickLabel={m.pickColour(m.newValue)} disabled={!ctx.canEdit} onChange={setValue} />
        ) : (
          <Input value={value} disabled={!ctx.canEdit} onChange={(e) => setValue(e.target.value)} />
        )}
      </Field>
      <Button
        variant="secondary"
        disabled={!addable}
        onClick={() => {
          ctx.write(`add:${name}`, (t) =>
            putToken(
              t,
              [...props.prefix, ...name.split(".")],
              typedValue(props.typed ? type : undefined, value),
              props.typed ? type : undefined,
            ),
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

/**
 * A button that removes the token at `path`; a `base` token goes with its high-contrast and brand values, in the same
 * undo step.
 */
export function RemoveToken(props: { readonly path: readonly string[]; readonly base?: boolean }): ReactElement {
  const m = designMessages(useLingui().i18n);
  const ctx = useDesign();
  const name = props.path.join(".");
  return (
    <Button
      variant="ghost"
      size="sm"
      aria-label={m.removeToken(name)}
      disabled={!ctx.canEdit}
      onClick={() =>
        ctx.write(`${name}:remove`, (t) =>
          props.base === true ? dropBaseToken(t, props.path) : dropToken(t, props.path),
        )
      }
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
  const [file, setFile] = useState<string | undefined>(undefined);
  // Measured when the file is read, against the theme as it was then: applying it must not turn the counts into
  // "nothing would change".
  const [changes, setChanges] = useState<ReturnType<typeof importChanges> | undefined>(undefined);
  const read = (t: string) => {
    const parsed = parseDtcg(t);
    setResult(parsed);
    setChanges(parsed.ok ? importChanges(ctx.value?.["tokens"], parsed.tokens) : undefined);
  };
  const take = (files: readonly File[]) => {
    const f = files[0];
    if (f === undefined) {
      return;
    }
    setFile(f.name);
    setDone(false);
    // A file larger than a document may be is refused before it is read into memory.
    if (f.size > limitDefault("document.fileSize")) {
      setText("");
      setResult({ ok: false, error: "tooLarge" });
      return;
    }
    void f.text().then((t) => {
      setText(t);
      read(t);
    });
  };
  const apply = () => {
    if (result?.ok === true) {
      ctx.write("import", (t) => mergeTokens(t, result.tokens));
      setDone(true);
    }
  };
  return (
    <Dialog open onOpenChange={(o) => !o && props.onClose()} title={m.importTitle} labels={{ close: m.close }}>
      <div className="flex flex-col gap-3">
        <FileDrop
          label={m.importFile}
          hint={m.importFileHint}
          chosen={file}
          accept=".json,application/json"
          icon="upload"
          onFiles={take}
        />
        <Field label={m.importLabel}>
          <Textarea
            value={text}
            rows={8}
            onChange={(e) => {
              setText(e.target.value);
              setResult(undefined);
              setChanges(undefined);
              setDone(false);
            }}
          />
        </Field>
        <Button variant="secondary" disabled={text === ""} onClick={() => read(text)}>
          {m.importRead}
        </Button>
        {result?.ok === false && (
          <p role="alert" className="text-body">
            {m[ERRORS[result.error]]}
          </p>
        )}
        {result?.ok === true && <Summary result={result} changes={changes} m={m} />}
        <Button
          disabled={changes === undefined || changes.added + changes.changed === 0 || !ctx.canEdit || done}
          onClick={apply}
        >
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
  changes,
  m,
}: {
  readonly result: Extract<ReturnType<typeof parseDtcg>, { ok: true }>;
  readonly changes: ReturnType<typeof importChanges> | undefined;
  readonly m: DesignMessages;
}) {
  const c = changes ?? { added: 0, changed: 0, same: 0 };
  return (
    <div className="flex flex-col gap-1">
      <p role="status" className="text-body">
        {m.importSummary(c.added, c.changed, c.same, result.skipped.length)}
      </p>
      {result.imported > 0 && c.added + c.changed === 0 && (
        <p className="text-caption text-muted-foreground">{m.importNothing}</p>
      )}
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

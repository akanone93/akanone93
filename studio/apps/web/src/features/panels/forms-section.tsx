// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Icon, Menu, Select } from "@plux/ui";
import { Input } from "@plux/ui/components/input";
import { Popover, PopoverContent, PopoverTrigger } from "@plux/ui/components/popover";
import { createContext, type ReactElement, useContext } from "react";
import { type FormsMessages, formsMessages } from "./forms.messages.ts";
import {
  addFieldRecipe,
  addFormRecipe,
  addValidatorRecipe,
  FIELD_TYPES,
  FORM_MODES,
  type Form,
  type FormField,
  type FormMode,
  formsOf,
  removeFieldRecipe,
  removeFormRecipe,
  removeValidatorRecipe,
  renameFormRecipe,
  setFieldKeyRecipe,
  setValidatorRecipe,
  VALIDATOR_KINDS,
  VALIDATOR_OPTIONS,
  type ValidatorKind,
} from "./forms-model.ts";
import { textOfValue, valueOfText } from "./page-settings-model.ts";
import "./forms-section.css";

/**
 * The forms of a page (PC-26, STA-020): each form's name, fields (name, type, initial value) and checks, and the state
 * the canvas previews it in. A form's state is read by bindings as `page.<form>.values.<field>`, `.errors.<field>`,
 * `.status` and `.valid`.
 */

type Obj = Record<string, unknown>;
type Change = (recipe: (draft: Obj) => void, field: string) => void;

/** The preview mode of each form on the canvas (PC-26); null outside a canvas. */
export interface FormPreviewControl {
  readonly modeOf: (page: string, form: string) => FormMode | undefined;
  readonly set: (page: string, form: string, mode: FormMode | undefined) => void;
}
export const FormPreviewContext = createContext<FormPreviewControl | null>(null);

/** A text input that commits on blur or Enter. */
function Commit(p: {
  readonly label: string;
  readonly value: string;
  readonly readOnly: boolean;
  readonly mono?: boolean;
  readonly placeholder?: string;
  readonly onCommit: (text: string) => void;
}) {
  return (
    <Input
      key={p.value}
      aria-label={p.label}
      placeholder={p.placeholder}
      defaultValue={p.value}
      readOnly={p.readOnly}
      className="form-input"
      data-mono={p.mono === true || undefined}
      onBlur={(e) => {
        if (e.target.value !== p.value) {
          p.onCommit(e.target.value);
        }
      }}
      onKeyDown={(e) => {
        if (e.key === "Enter") {
          e.currentTarget.blur();
        }
      }}
    />
  );
}

export function FormModes(p: { readonly page: string; readonly form: string; readonly m: FormsMessages }) {
  const preview = useContext(FormPreviewContext);
  if (preview === null) {
    return null;
  }
  const mode = preview.modeOf(p.page, p.form) ?? "pristine";
  return (
    <div className="state-chips" role="radiogroup" aria-label={p.m.preview}>
      {FORM_MODES.map((x) => (
        // biome-ignore lint/a11y/useSemanticElements: a chip of a segmented choice
        <button
          key={x}
          type="button"
          role="radio"
          aria-checked={mode === x}
          className="state-chip"
          onClick={() => preview.set(p.page, p.form, x === "pristine" ? undefined : x)}
        >
          {p.m.mode(x)}
        </button>
      ))}
    </div>
  );
}

function ValidatorChip(p: {
  readonly v: Obj;
  readonly at: readonly [number, number, number];
  readonly m: FormsMessages;
  readonly canEdit: boolean;
  readonly change: Change;
}) {
  const kind = String(p.v["kind"]) as ValidatorKind;
  const [form, field, index] = p.at;
  const keys = [...(VALIDATOR_OPTIONS[kind] ?? []), "message"];
  return (
    <Popover>
      <PopoverTrigger
        render={
          <button type="button" className="form-check" data-kind={kind}>
            {p.m.validatorName(kind)}
          </button>
        }
      />
      <PopoverContent align="start" className="form-check-pop">
        {keys.map((k) => {
          const raw = p.v[k];
          const text = typeof raw === "object" ? "" : textOfValue(raw);
          return (
            <div key={k} className="form-option">
              <span>{p.m.option(k)}</span>
              <Commit
                label={p.m.option(k)}
                value={text}
                mono={k === "pattern"}
                readOnly={!p.canEdit}
                onCommit={(t) =>
                  p.change(
                    setValidatorRecipe(
                      form,
                      field,
                      index,
                      k,
                      ["min", "max", "maxScale", "maxIntegerDigits", "debounceMs"].includes(k)
                        ? valueOfText("double", t)
                        : t,
                    ),
                    `validator:${form}:${field}:${index}:${k}`,
                  )
                }
              />
            </div>
          );
        })}
        {p.canEdit && (
          <button
            type="button"
            className="form-remove-check"
            onClick={() => p.change(removeValidatorRecipe(form, field, index), `validator:${form}:${field}:remove`)}
          >
            <Icon name="trash" size="pill" />
            {p.m.removeValidator}
          </button>
        )}
      </PopoverContent>
    </Popover>
  );
}

function FieldRow(p: {
  readonly f: FormField;
  readonly at: readonly [number, number];
  readonly only: boolean;
  readonly m: FormsMessages;
  readonly canEdit: boolean;
  readonly change: Change;
}) {
  const [form, field] = p.at;
  const ro = !p.canEdit;
  const key = (k: string) => `field:${form}:${field}:${k}`;
  return (
    <div className="form-field">
      <div className="form-field-row">
        <Commit
          label={p.m.fieldName}
          value={p.f.name}
          mono
          readOnly={ro}
          onCommit={(t) =>
            /^[a-z][A-Za-z0-9]*$/.test(t) && p.change(setFieldKeyRecipe(form, field, "name", t), key("name"))
          }
        />
        <div className="form-type">
          <Select
            label={p.m.fieldType}
            options={[...new Set([...FIELD_TYPES, p.f.type])].map((t) => ({ value: t, label: t }))}
            value={p.f.type}
            disabled={ro}
            onValueChange={(t) => p.change(setFieldKeyRecipe(form, field, "type", t), key("type"))}
          />
        </div>
        {!p.only && !ro && (
          <button
            type="button"
            className="form-icon"
            aria-label={p.m.removeField(p.f.name)}
            onClick={() => p.change(removeFieldRecipe(form, field), key("remove"))}
          >
            <Icon name="x" size="pill" />
          </button>
        )}
      </div>
      <Commit
        label={p.m.initial}
        placeholder={p.m.initial}
        value={textOfValue(p.f.initial)}
        readOnly={ro}
        onCommit={(t) => p.change(setFieldKeyRecipe(form, field, "initial", valueOfText(p.f.type, t)), key("initial"))}
      />
      <div className="form-checks">
        {p.f.validators.map((v, i) => (
          <ValidatorChip
            // biome-ignore lint/suspicious/noArrayIndexKey: validators have no id; their order is their identity
            key={i}
            v={v}
            at={[form, field, i]}
            m={p.m}
            canEdit={p.canEdit}
            change={p.change}
          />
        ))}
        {!ro && (
          <Menu
            trigger={
              <button type="button" className="form-check" data-add="">
                <Icon name="plus" size="pill" />
                {p.m.addValidator}
              </button>
            }
            entries={VALIDATOR_KINDS.map((k) => ({
              kind: "item" as const,
              id: k,
              label: p.m.validatorName(k),
              onSelect: () => p.change(addValidatorRecipe(form, field, k), key(`add:${k}`)),
            }))}
          />
        )}
      </div>
    </div>
  );
}

function FormCard(p: {
  readonly page: string;
  readonly form: Form;
  readonly index: number;
  readonly m: FormsMessages;
  readonly canEdit: boolean;
  readonly change: Change;
}) {
  const { m, form, index } = p;
  return (
    <section className="form-card" aria-label={form.name}>
      <div className="form-card-head">
        <Icon name="list-checks" size="pill" />
        <Commit
          label={m.formName}
          value={form.name}
          mono
          readOnly={!p.canEdit}
          onCommit={(t) => /^[a-z][A-Za-z0-9]*$/.test(t) && p.change(renameFormRecipe(index, t), `form:${index}:name`)}
        />
        {p.canEdit && (
          <button
            type="button"
            className="form-icon"
            aria-label={m.removeForm(form.name)}
            onClick={() => p.change(removeFormRecipe(index), `form:${index}:remove`)}
          >
            <Icon name="trash" size="pill" />
          </button>
        )}
      </div>
      <FormModes page={p.page} form={form.name} m={m} />
      {form.fields.map((f, i) => (
        <FieldRow
          key={f.name}
          f={f}
          at={[index, i]}
          only={form.fields.length === 1}
          m={m}
          canEdit={p.canEdit}
          change={p.change}
        />
      ))}
      {p.canEdit && (
        <button
          type="button"
          className="form-add"
          onClick={() =>
            p.change(
              addFieldRecipe(
                index,
                freshName(
                  form.fields.map((f) => f.name),
                  "field",
                ),
              ),
              `form:${index}:add`,
            )
          }
        >
          <Icon name="plus" size="pill" />
          {m.addField}
        </button>
      )}
    </section>
  );
}

/** `base`, else `base2`, `base3`… whichever is not taken. */
function freshName(taken: readonly string[], base: string): string {
  let n = 1;
  let name = base;
  while (taken.includes(name)) {
    n += 1;
    name = `${base}${n}`;
  }
  return name;
}

export function FormsSection(p: {
  readonly pageId: string;
  readonly page: unknown;
  readonly canEdit: boolean;
  readonly change: Change;
  readonly newId: () => string;
}): ReactElement {
  const m = formsMessages(useLingui().i18n);
  const forms = formsOf(p.page);
  return (
    <div className="forms-section">
      {forms.map((f, i) => (
        <FormCard key={f.id} page={p.pageId} form={f} index={i} m={m} canEdit={p.canEdit} change={p.change} />
      ))}
      {p.canEdit && (
        <button
          type="button"
          className="form-add"
          onClick={() =>
            p.change(
              addFormRecipe(
                p.newId(),
                freshName(
                  forms.map((f) => f.name),
                  "form",
                ),
              ),
              "forms:add",
            )
          }
        >
          <Icon name="plus" size="pill" />
          {m.addForm}
        </button>
      )}
    </div>
  );
}

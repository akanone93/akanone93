// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Icon, Select } from "@plux/ui";
import type { ReactElement } from "react";
import { formsMessages } from "./forms.messages.ts";
import { boundField, type Form } from "./forms-model.ts";
import { FormModes } from "./forms-section.tsx";

/**
 * The form field panel of an input widget (PC-26, STA-020): which field of the page's forms it is bound to, a select
 * that binds it to another in one step — value, change, touched and error text — and the form's preview state.
 */

type Obj = Record<string, unknown>;

export function FormFieldPanel(p: {
  readonly node: Obj;
  readonly pageId: string;
  readonly forms: readonly Form[];
  readonly canEdit: boolean;
  readonly bind: (form: string, field: string) => void;
}): ReactElement {
  const m = formsMessages(useLingui().i18n);
  const bound = boundField(p.node);
  const options = p.forms.flatMap((f) =>
    f.fields.map((x) => ({ value: `${f.name}.${x.name}`, label: `${f.name}.${x.name}` })),
  );
  return (
    <section className="lay-card" aria-label={m.formField}>
      <h3 className="lay-title">
        <Icon name="list-checks" size="pill" />
        {m.formField}
      </h3>
      {p.forms.length === 0 ? (
        <p className="lay-note">{m.noForms}</p>
      ) : (
        <>
          {bound !== undefined && (
            <p className="cond-worked" data-result="true">
              {m.bound(bound.form, bound.field)}
            </p>
          )}
          <Select
            label={m.bindTo}
            options={options}
            value={bound === undefined ? "" : `${bound.form}.${bound.field}`}
            disabled={!p.canEdit}
            onValueChange={(v) => {
              const [form, field] = v.split(".");
              if (form !== undefined && field !== undefined) {
                p.bind(form, field);
              }
            }}
          />
          {bound !== undefined && <FormModes page={p.pageId} form={bound.form} m={m} />}
        </>
      )}
    </section>
  );
}

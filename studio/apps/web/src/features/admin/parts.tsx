// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Timestamp } from "@bufbuild/protobuf/wkt";
import { timestampDate } from "@bufbuild/protobuf/wkt";
import { useLingui } from "@lingui/react";
import { Button, Field, Input, Select, Tooltip } from "@plux/ui";
import { Link, useParams } from "@tanstack/react-router";
import { type FormEvent, type ReactElement, type ReactNode, useState } from "react";
import { type DataRow, DataTable, PageFrame } from "../../components/page.tsx";
import { adminMessages } from "./admin.messages.ts";

/** Heading, caption and the loading and error states shared by every administration page; the sidebar lists the pages. */
export function Frame({
  id,
  title,
  caption,
  loading = false,
  failed = false,
  error,
  saveFailed = false,
  saveError,
  children,
}: {
  readonly id: string;
  readonly title: string;
  readonly caption?: string;
  readonly loading?: boolean;
  readonly failed?: boolean;
  readonly error?: unknown;
  readonly saveFailed?: boolean;
  /** The refused change, whose code and reason are shown under the failure. */
  readonly saveError?: unknown;
  readonly children: ReactNode;
}): ReactElement {
  const m = adminMessages(useLingui().i18n);
  return (
    <PageFrame
      id={id}
      title={title}
      caption={caption}
      loading={loading}
      failed={failed}
      error={error}
      saveFailed={saveFailed}
      saveError={saveError}
      labels={m}
    >
      {children}
    </PageFrame>
  );
}

export function Table({
  caption,
  heads,
  rows,
  empty,
}: {
  readonly caption: string;
  readonly heads: readonly string[];
  readonly rows: readonly DataRow[];
  readonly empty: string;
}): ReactElement {
  return <DataTable caption={caption} heads={heads} rows={rows} empty={empty} />;
}

export function fmt(time: Timestamp | undefined, never: string): string {
  return time === undefined ? never : timestampDate(time).toLocaleString();
}

export interface FormField {
  readonly name: string;
  readonly label: string;
  readonly type?: "text" | "date" | "number" | "password";
  readonly required?: boolean;
  readonly options?: readonly { readonly value: string; readonly label: string }[];
  readonly initial?: string;
}

function initialValues(fields: readonly FormField[]): Record<string, string> {
  return Object.fromEntries(fields.map((f) => [f.name, f.initial ?? f.options?.[0]?.value ?? ""]));
}

/**
 * A small create form: text, date, number and single-choice fields, and `children` for fields whose choices depend on
 * one another, which the page holds; it clears its own fields after a successful change.
 */
export function CreateForm({
  fields,
  submit,
  disabled,
  deniedReason,
  onSubmit,
  children,
}: {
  readonly fields: readonly FormField[];
  readonly submit: string;
  readonly disabled: boolean;
  readonly deniedReason: string;
  readonly onSubmit: (value: (name: string) => string) => Promise<boolean>;
  readonly children?: ReactNode;
}): ReactElement {
  const [values, setValues] = useState(() => initialValues(fields));
  // Options may arrive after the first render, so an unchosen field holds its first option.
  const current = (field: FormField) => values[field.name] || field.options?.[0]?.value || "";
  const send = async (event: FormEvent) => {
    event.preventDefault();
    if (await onSubmit((name) => current(fields.find((f) => f.name === name) ?? { name, label: name }))) {
      setValues(initialValues(fields));
    }
  };
  return (
    <form className="flex flex-wrap items-end gap-3" onSubmit={send}>
      {fields.map((f) => (
        <Field key={f.name} label={f.label}>
          {f.options === undefined ? (
            <Input
              type={f.type ?? "text"}
              required={f.required === true}
              value={values[f.name] ?? ""}
              onChange={(e) => setValues({ ...values, [f.name]: e.target.value })}
            />
          ) : (
            <Select
              label={f.label}
              options={f.options}
              value={current(f)}
              onValueChange={(v) => setValues({ ...values, [f.name]: v })}
            />
          )}
        </Field>
      ))}
      {children}
      <Tooltip content={disabled ? deniedReason : undefined} disabledTrigger={disabled}>
        <Button type="submit" disabled={disabled}>
          {submit}
        </Button>
      </Tooltip>
    </form>
  );
}

/** A secret the server returns once: it lives in the page's state only, is never stored and goes away with Dismiss. */
export function SecretPanel({
  heading,
  note,
  secret,
  dismiss,
  onDismiss,
}: {
  readonly heading: string;
  readonly note: string;
  readonly secret: string;
  readonly dismiss: string;
  readonly onDismiss: () => void;
}): ReactElement {
  return (
    <div role="status" className="flex flex-col gap-2 rounded-sm border border-hairline p-3">
      <h2 className="text-label font-medium">{heading}</h2>
      <output className="font-mono text-data break-all">{secret}</output>
      <p className="text-caption text-muted-foreground">{note}</p>
      <Button variant="secondary" size="sm" onClick={onDismiss}>
        {dismiss}
      </Button>
    </div>
  );
}

/**
 * Says that a feature needs an enterprise licence, in place of the form that would change it, so nothing is typed in
 * vain (GOV-033); it links to the licence page, which the person can open since they could read the licence.
 */
export function LicenceNote({ text }: { readonly text: string }): ReactElement {
  const m = adminMessages(useLingui().i18n);
  const org = (useParams({ strict: false }) as { org?: string }).org ?? "";
  return (
    <p role="note" className="text-body text-muted-foreground">
      {text}{" "}
      <Link to="/o/$org/admin/license" params={{ org }} className="text-primary underline-offset-4 hover:underline">
        {m.licenceLink}
      </Link>
    </p>
  );
}

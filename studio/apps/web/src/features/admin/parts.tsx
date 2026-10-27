// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Timestamp } from "@bufbuild/protobuf/wkt";
import { timestampDate } from "@bufbuild/protobuf/wkt";
import { useLingui } from "@lingui/react";
import { Button, Field, Input, Select } from "@plux/ui";
import { Link, useParams } from "@tanstack/react-router";
import { type FormEvent, type ReactElement, type ReactNode, useState } from "react";
import { adminMessages } from "./admin.messages.ts";

type Messages = ReturnType<typeof adminMessages>;

/** The administration pages in navigation order; the path decides which page is current. */
function navItems(m: Messages) {
  return [
    { to: "/o/$org/admin", label: m.navSettings },
    { to: "/o/$org/admin/members", label: m.navMembers },
    { to: "/o/$org/admin/teams", label: m.navTeams },
    { to: "/o/$org/admin/roles", label: m.navRoles },
    { to: "/o/$org/admin/restrictions", label: m.navRestrictions },
    { to: "/o/$org/admin/access", label: m.navAccess },
    { to: "/o/$org/admin/sso", label: m.navSso },
    { to: "/o/$org/admin/tokens", label: m.navTokens },
    { to: "/o/$org/admin/sessions", label: m.navSessions },
    { to: "/o/$org/admin/audit", label: m.navAudit },
    { to: "/o/$org/admin/compliance", label: m.navCompliance },
    { to: "/o/$org/admin/retention", label: m.navRetention },
    { to: "/o/$org/admin/license", label: m.navLicense },
    { to: "/o/$org/admin/limits", label: m.navLimits },
  ] as const;
}

function AdminNav({ m }: { readonly m: Messages }): ReactElement {
  const org = (useParams({ strict: false }) as { org?: string }).org ?? "";
  return (
    <nav aria-label={m.navLabel} className="flex flex-wrap gap-3 text-body">
      {navItems(m).map((item) => (
        <Link key={item.to} to={item.to} params={{ org }} activeOptions={{ exact: true }}>
          {item.label}
        </Link>
      ))}
    </nav>
  );
}

/** Heading, caption, the area's navigation and the loading and error states shared by every administration page. */
export function Frame({
  id,
  title,
  caption,
  loading = false,
  failed = false,
  saveFailed = false,
  children,
}: {
  readonly id: string;
  readonly title: string;
  readonly caption?: string;
  readonly loading?: boolean;
  readonly failed?: boolean;
  readonly saveFailed?: boolean;
  readonly children: ReactNode;
}): ReactElement {
  const m = adminMessages(useLingui().i18n);
  return (
    <section className="flex flex-col gap-4" aria-labelledby={id}>
      <AdminNav m={m} />
      <h1 id={id} className="text-screen">
        {title}
      </h1>
      {caption !== undefined && <p className="text-body text-secondary">{caption}</p>}
      {loading && !failed && (
        <p role="status" className="text-body text-secondary">
          {m.loading}
        </p>
      )}
      {failed && (
        <p role="alert" className="text-body text-danger">
          {m.loadFailed}
        </p>
      )}
      {saveFailed && (
        <p role="alert" className="text-body text-danger">
          {m.saveFailed}
        </p>
      )}
      {!loading && !failed && children}
    </section>
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
  readonly rows: readonly { readonly key: string; readonly cells: readonly ReactNode[] }[];
  readonly empty: string;
}): ReactElement {
  if (rows.length === 0) {
    return <p className="text-body text-secondary">{empty}</p>;
  }
  return (
    <table className="w-full text-left text-body">
      <caption className="sr-only">{caption}</caption>
      <thead>
        <tr>
          {heads.map((h) => (
            <th key={h} scope="col" className="p-2 text-caption text-secondary">
              {h}
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {rows.map((r) => (
          <tr key={r.key}>
            {r.cells.map((c, i) => (
              // biome-ignore lint/suspicious/noArrayIndexKey: cells have a fixed order
              <td key={i} className="p-2">
                {c}
              </td>
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  );
}

export function fmt(time: Timestamp | undefined, never: string): string {
  return time === undefined ? never : timestampDate(time).toLocaleString();
}

export interface FormField {
  readonly name: string;
  readonly label: string;
  readonly type?: "text" | "date" | "number";
  readonly required?: boolean;
  readonly options?: readonly { readonly value: string; readonly label: string }[];
  readonly initial?: string;
}

function initialValues(fields: readonly FormField[]): Record<string, string> {
  return Object.fromEntries(fields.map((f) => [f.name, f.initial ?? f.options?.[0]?.value ?? ""]));
}

/** A small create form: text, date, number and single-choice fields; it clears itself after a successful change. */
export function CreateForm({
  fields,
  submit,
  disabled,
  deniedReason,
  onSubmit,
}: {
  readonly fields: readonly FormField[];
  readonly submit: string;
  readonly disabled: boolean;
  readonly deniedReason: string;
  readonly onSubmit: (value: (name: string) => string) => Promise<boolean>;
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
      <Button type="submit" disabled={disabled} title={disabled ? deniedReason : undefined}>
        {submit}
      </Button>
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
      <p className="text-caption text-secondary">{note}</p>
      <Button variant="secondary" size="sm" onClick={onDismiss}>
        {dismiss}
      </Button>
    </div>
  );
}

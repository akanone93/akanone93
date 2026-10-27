// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { GovernanceService } from "@plux/api";
import { Button, Field, Input } from "@plux/ui";
import { useNavigate, useParams, useSearch } from "@tanstack/react-router";
import { type ReactElement, useState } from "react";
import { adminMessages } from "./admin.messages.ts";
import type { AccessSearch } from "./admin.schema.ts";
import { Frame } from "./parts.tsx";
import { useAdminQuery, useOrgId, useRpc } from "./use-admin.ts";

/** O23: why a person holds or lacks a permission, naming the grant or restriction (F14); `?user&app&env&permission` hold the question. */
export function AccessPage(): ReactElement {
  const m = adminMessages(useLingui().i18n);
  const organizationId = useOrgId();
  const org = (useParams({ strict: false }) as { org?: string }).org ?? "";
  const search = useSearch({ strict: false }) as AccessSearch;
  const navigate = useNavigate();
  const governance = useRpc(GovernanceService);
  const [draft, setDraft] = useState<AccessSearch>(search);
  const asked = search.user !== undefined && search.permission !== undefined;
  const answer = useAdminQuery(
    ["explain", organizationId, search.user, search.permission, search.app, search.env],
    () =>
      governance.explainAccess({
        organizationId,
        userId: search.user ?? "",
        permission: search.permission ?? "",
        appId: search.app ?? "",
        environment: search.env ?? "",
      }),
    asked,
  );
  const set = (field: keyof AccessSearch) => (e: { target: { value: string } }) =>
    setDraft({ ...draft, [field]: e.target.value === "" ? undefined : e.target.value });
  return (
    <Frame id="admin-access" title={m.accessTitle} caption={m.accessCaption}>
      <form
        className="flex flex-wrap items-end gap-3"
        onSubmit={(e) => {
          e.preventDefault();
          navigate({ to: "/o/$org/admin/access", params: { org }, search: draft });
        }}
      >
        <Field label={m.accessUser}>
          <Input required value={draft.user ?? ""} onChange={set("user")} />
        </Field>
        <Field label={m.accessPermission}>
          <Input required value={draft.permission ?? ""} onChange={set("permission")} />
        </Field>
        <Field label={m.accessApp}>
          <Input value={draft.app ?? ""} onChange={set("app")} />
        </Field>
        <Field label={m.accessEnv}>
          <Input value={draft.env ?? ""} onChange={set("env")} />
        </Field>
        <Button type="submit">{m.explain}</Button>
      </form>
      {answer.isError && (
        <p role="alert" className="text-body text-danger">
          {m.loadFailed}
        </p>
      )}
      {answer.data !== undefined && (
        <div role="status" className="flex flex-col gap-1 text-body">
          <p className="font-medium">{answer.data.decision === "allow" ? m.allowed : m.denied}</p>
          {answer.data.allowingGrantIds.length > 0 && (
            <p>
              {m.allowedBy}: <span className="font-mono">{answer.data.allowingGrantIds.join(", ")}</span>
            </p>
          )}
          {answer.data.removingRestrictionIds.length > 0 && (
            <p>
              {m.removedBy}: <span className="font-mono">{answer.data.removingRestrictionIds.join(", ")}</span>
            </p>
          )}
          {answer.data.ownerFloor && <p>{m.ownerFloor}</p>}
        </div>
      )}
    </Frame>
  );
}

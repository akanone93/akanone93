// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { AppService, GovernanceService, OrgService, type Restriction, type RoleGrant } from "@plux/api";
import { Button, Field, Select } from "@plux/ui";
import { useNavigate, useParams, useSearch } from "@tanstack/react-router";
import { type ReactElement, useState } from "react";
import { Refusal } from "../../components/refusal.tsx";
import { adminMessages } from "./admin.messages.ts";
import type { AccessSearch } from "./admin.schema.ts";
import { Frame } from "./parts.tsx";
import { pageSize, useAdminQuery, useOrgId, useRpc } from "./use-admin.ts";

/** The choice of every app, or of every environment of the chosen app: no identifier or key can be `*`. */
const every = "*";

/**
 * O23: why a person holds or lacks a permission in the organisation, an app or one of its environments (F14, GOV-006).
 * The person, permission, app and environment are chosen by name; `?user&permission&app&env` hold the question, and the
 * answer names the grants and restrictions it rests on.
 */
export function AccessPage(): ReactElement {
  const m = adminMessages(useLingui().i18n);
  const organizationId = useOrgId();
  const org = (useParams({ strict: false }) as { org?: string }).org ?? "";
  const search = useSearch({ strict: false }) as AccessSearch;
  const navigate = useNavigate();
  const governance = useRpc(GovernanceService);
  const orgApi = useRpc(OrgService);
  const appApi = useRpc(AppService);
  const [draft, setDraft] = useState<AccessSearch>(search);
  const members = useAdminQuery(["members", organizationId], () =>
    orgApi.listMembers({ organizationId, page: { pageSize } }),
  );
  const catalogue = useAdminQuery(["permissions"], () => governance.listPermissions({}));
  const apps = useAdminQuery(["apps", organizationId], () => appApi.listApps({ organizationId, page: { pageSize } }));
  const environments = useAdminQuery(
    ["environments", draft.app],
    () => appApi.listEnvironments({ appId: draft.app ?? "", page: { pageSize } }),
    draft.app !== undefined,
  );
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
  // The grants and restrictions the answer names, read only when it names some.
  const grants = useAdminQuery(
    ["grants", search.user],
    () => governance.listGrants({ userId: search.user ?? "", teamId: "", appId: "", page: { pageSize } }),
    (answer.data?.allowingGrantIds.length ?? 0) > 0,
  );
  const restrictions = useAdminQuery(
    ["restrictions"],
    () => governance.listRestrictions({ page: { pageSize } }),
    (answer.data?.removingRestrictionIds.length ?? 0) > 0,
  );
  const people = [...new Map((members.data?.members ?? []).map((p) => [p.userId, p.displayName || p.email])).entries()];
  const appName = (id: string): string => apps.data?.apps.find((a) => a.id === id)?.name ?? id;
  const whereOf = (appId: string, environment: string): string =>
    appId === "" ? m.everyApp : environment === "" ? appName(appId) : m.restrictionWhere(appName(appId), environment);
  const grantText = (id: string): string => {
    const g: RoleGrant | undefined = grants.data?.grants.find((x) => x.id === id);
    return g === undefined ? id : m.grantText(g.role, g.appId === "" ? m.kindOrg : appName(g.appId));
  };
  const restrictionText = (id: string): string => {
    const r: Restriction | undefined = restrictions.data?.restrictions.find((x) => x.id === id);
    return r === undefined ? id : m.restrictionText(whereOf(r.appId, r.environment), r.patterns.join(", "), r.reason);
  };
  const pick = (field: keyof AccessSearch) => (value: string) =>
    setDraft({ ...draft, [field]: value === every ? undefined : value, ...(field === "app" && { env: undefined }) });
  return (
    <Frame
      id="admin-access"
      title={m.accessTitle}
      caption={m.accessCaption}
      failed={members.isError || catalogue.isError}
      error={members.error ?? catalogue.error}
    >
      <form
        className="flex flex-wrap items-end gap-3"
        onSubmit={(e) => {
          e.preventDefault();
          navigate({ to: "/o/$org/admin/access", params: { org }, search: draft });
        }}
      >
        <Field label={m.accessUser}>
          <Select
            label={m.accessUser}
            placeholder={m.accessChoose}
            options={people.map(([value, label]) => ({ value, label }))}
            value={draft.user ?? ""}
            onValueChange={pick("user")}
          />
        </Field>
        <Field label={m.accessPermission}>
          <Select
            label={m.accessPermission}
            placeholder={m.accessChoose}
            options={(catalogue.data?.permissions ?? []).map((p) => ({ value: p.name, label: p.name }))}
            value={draft.permission ?? ""}
            onValueChange={pick("permission")}
          />
        </Field>
        <Field label={m.restrictionApp}>
          <Select
            label={m.restrictionApp}
            options={[
              { value: every, label: m.everyApp },
              ...(apps.data?.apps ?? []).map((a) => ({ value: a.id, label: a.name })),
            ]}
            value={draft.app ?? every}
            onValueChange={pick("app")}
          />
        </Field>
        <Field label={m.restrictionEnvironment}>
          <Select
            label={m.restrictionEnvironment}
            options={[
              { value: every, label: m.everyEnvironment },
              ...(environments.data?.environments ?? []).map((e) => ({ value: e.key, label: e.name || e.key })),
            ]}
            value={draft.env ?? every}
            disabled={draft.app === undefined}
            onValueChange={pick("env")}
          />
        </Field>
        <Button type="submit" disabled={draft.user === undefined || draft.permission === undefined}>
          {m.explain}
        </Button>
      </form>
      {answer.isError && <Refusal error={answer.error} failedTitle={m.loadFailed} />}
      {answer.data !== undefined && (
        <div role="status" className="flex flex-col gap-1 text-body">
          <p className="font-medium">{answer.data.decision === "allow" ? m.allowed : m.denied}</p>
          {answer.data.allowingGrantIds.length > 0 && (
            <p>
              {m.allowedBy}: {answer.data.allowingGrantIds.map(grantText).join("; ")}
            </p>
          )}
          {answer.data.removingRestrictionIds.length > 0 && (
            <p>
              {m.removedBy}: {answer.data.removingRestrictionIds.map(restrictionText).join("; ")}
            </p>
          )}
          {answer.data.ownerFloor && <p>{m.ownerFloor}</p>}
        </div>
      )}
    </Frame>
  );
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { GovernanceService } from "@plux/api";
import { Button, Field, Input } from "@plux/ui";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import { type ReactElement, useEffect, useState } from "react";
import { TypeConfirmDialog } from "../../components/dialogs/index.ts";
import { adminMessages } from "./admin.messages.ts";
import { Frame, Table } from "./parts.tsx";
import { buildTree, type TreeArea, toggleAll } from "./permission-tree.ts";
import { pageSize, useAdminQuery, useCan, useRpc, useRun } from "./use-admin.ts";

/** O20: predefined and custom roles (GOV-002). */
export function RolesPage(): ReactElement {
  const m = adminMessages(useLingui().i18n);
  const org = (useParams({ strict: false }) as { org?: string }).org ?? "";
  const governance = useRpc(GovernanceService);
  const roles = useAdminQuery(["roles"], () => governance.listRoles({ page: { pageSize } }));
  return (
    <Frame
      id="admin-roles"
      title={m.rolesTitle}
      caption={m.rolesCaption}
      loading={roles.isPending}
      failed={roles.isError}
    >
      <Table
        caption={m.rolesTitle}
        heads={[m.colName, m.colKey, m.colPredefined]}
        empty={m.nothing}
        rows={(roles.data?.roles ?? []).map((r) => ({
          key: r.key,
          cells: [
            <Link key="n" to="/o/$org/admin/roles/$roleId" params={{ org, roleId: r.key }}>
              {r.name}
            </Link>,
            r.key,
            r.predefined ? m.yes : m.no,
          ],
        }))}
      />
      <Link to="/o/$org/admin/roles/$roleId" params={{ org, roleId: "new" }}>
        {m.newRole}
      </Link>
    </Frame>
  );
}

function PermissionTree({
  tree,
  selected,
  locked,
  onChange,
}: {
  readonly tree: readonly TreeArea[];
  readonly selected: ReadonlySet<string>;
  readonly locked: boolean;
  readonly onChange: (next: Set<string>) => void;
}): ReactElement {
  const m = adminMessages(useLingui().i18n);
  return (
    <div className="flex flex-col gap-3">
      {tree.map((a) => (
        <fieldset key={a.area} className="flex flex-col gap-2 rounded-sm border border-hairline p-3">
          <legend className="px-1 text-label font-medium">{a.area}</legend>
          {a.features.map((f) => {
            const names = f.actions.map((x) => x.name);
            const all = names.every((n) => selected.has(n));
            return (
              <fieldset key={f.feature} className="flex flex-col gap-1 ps-3">
                <legend className="flex items-center gap-2 text-body">
                  <input
                    type="checkbox"
                    aria-label={`${a.area} ${f.feature === "" ? m.generalFeature : f.feature}`}
                    checked={all}
                    disabled={locked}
                    onChange={(e) => onChange(toggleAll(selected, names, e.target.checked))}
                  />
                  {f.feature === "" ? m.generalFeature : f.feature}
                </legend>
                {f.actions.map((x) => (
                  <label key={x.name} title={x.description} className="flex items-center gap-2 ps-6 text-body">
                    <input
                      type="checkbox"
                      aria-label={x.name}
                      checked={selected.has(x.name)}
                      disabled={locked}
                      onChange={(e) => onChange(toggleAll(selected, [x.name], e.target.checked))}
                    />
                    <span>{x.action}</span>
                    {x.stepUp && <span className="text-caption text-secondary">{m.needsStepUp}</span>}
                  </label>
                ))}
              </fieldset>
            );
          })}
        </fieldset>
      ))}
    </div>
  );
}

/** O21: the role builder; the permission catalogue is a tree of area, feature and action (F14, GOV-002). */
export function RolePage(): ReactElement {
  const m = adminMessages(useLingui().i18n);
  const { org = "", roleId = "" } = useParams({ strict: false }) as { org?: string; roleId?: string };
  const isNew = roleId === "new";
  const navigate = useNavigate();
  const governance = useRpc(GovernanceService);
  const can = useCan("roles.manage");
  const { run, failed } = useRun();
  const catalogue = useAdminQuery(["permissions"], () => governance.listPermissions({}));
  const role = useAdminQuery(["role", roleId], () => governance.getRole({ key: roleId }), !isNew);
  const [key, setKey] = useState("");
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [deleting, setDeleting] = useState(false);
  const loaded = role.data?.role;
  useEffect(() => {
    if (loaded !== undefined) {
      setKey(loaded.key);
      setName(loaded.name);
      setDescription(loaded.description);
      setSelected(new Set(loaded.permissions));
    }
  }, [loaded]);
  const locked = !can || loaded?.predefined === true;
  const save = async () => {
    const permissions = [...selected].sort();
    const ok = await run(() =>
      isNew
        ? governance.createRole({ key, name, description, permissions })
        : governance.updateRole({ key, name, description, permissions }),
    );
    if (ok && isNew) {
      await navigate({ to: "/o/$org/admin/roles", params: { org } });
    }
  };
  return (
    <Frame
      id="admin-role"
      title={isNew ? m.newRole : (loaded?.name ?? m.builderTitle)}
      caption={m.builderCaption}
      loading={catalogue.isPending || (!isNew && role.isPending)}
      failed={catalogue.isError || (!isNew && role.isError)}
      saveFailed={failed}
    >
      {loaded?.predefined === true && <p className="text-body text-secondary">{m.predefinedLocked}</p>}
      <div className="flex flex-wrap gap-3">
        <Field label={m.colKey}>
          <Input value={key} disabled={!isNew} onChange={(e) => setKey(e.target.value)} />
        </Field>
        <Field label={m.colName}>
          <Input value={name} disabled={locked} onChange={(e) => setName(e.target.value)} />
        </Field>
        <Field label={m.roleDescription}>
          <Input value={description} disabled={locked} onChange={(e) => setDescription(e.target.value)} />
        </Field>
      </div>
      <h2 className="text-label font-medium">{m.permissionsTree}</h2>
      <PermissionTree
        tree={buildTree(catalogue.data?.permissions ?? [])}
        selected={selected}
        locked={locked}
        onChange={setSelected}
      />
      <div className="flex gap-3">
        <Button disabled={locked || key === "" || name === ""} onClick={save}>
          {m.save}
        </Button>
        {!isNew && (
          <Button variant="danger" disabled={locked} onClick={() => setDeleting(true)}>
            {m.deleteRole}
          </Button>
        )}
      </div>
      <TypeConfirmDialog
        open={deleting}
        onOpenChange={setDeleting}
        phrase={roleId}
        description={m.deleteRoleDesc}
        onConfirm={async () => {
          setDeleting(false);
          if (await run(() => governance.deleteRole({ key: roleId }))) {
            await navigate({ to: "/o/$org/admin/roles", params: { org } });
          }
        }}
      />
    </Frame>
  );
}

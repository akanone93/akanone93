// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button } from "@plux/ui";
import { type ReactElement, useState } from "react";
import { TypeConfirmDialog } from "../../components/dialogs/index.ts";
import { Frame, Table } from "./parts.tsx";
import { securityMessages } from "./security.messages.ts";
import { useEffectiveSecurity, useEnvironment, usePermissions, useScope, useSetProfile } from "./use-security.ts";

const profiles = ["standard", "strict", "maximum"] as const;

function settingsOf(bytes: Uint8Array | undefined): [string, string][] {
  if (bytes === undefined || bytes.length === 0) {
    return [];
  }
  const parsed = JSON.parse(new TextDecoder().decode(bytes)) as Record<string, unknown>;
  return Object.entries(parsed)
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([k, v]) => [k, typeof v === "string" ? v : JSON.stringify(v)]);
}

/** A33: the profile and the effective setting values of one environment (SEC-180); changing it is D66 and then a typed confirm. */
export function SecurityProfilePage(): ReactElement {
  const m = securityMessages(useLingui().i18n);
  const { app, env } = useScope();
  const environment = useEnvironment(app, env);
  const config = useEffectiveSecurity(app, environment.environment?.id);
  const setProfile = useSetProfile();
  const can = usePermissions().has("app.manage");
  const [chosen, setChosen] = useState<string>();
  const [confirm, setConfirm] = useState(false);
  const [failed, setFailed] = useState(false);
  const names: Record<string, string> = {
    standard: m.profileStandard,
    strict: m.profileStrict,
    maximum: m.profileMaximum,
  };
  const current = config.data?.profile ?? "";
  const target = chosen ?? current;
  return (
    <Frame
      id="security-title"
      title={m.profileTitle}
      caption={m.profileCaption}
      loading={environment.loading || config.isPending}
      failed={environment.failed || config.isError}
    >
      <p className="text-body">
        {m.profileVersion}: <span className="font-mono">{String(config.data?.version ?? 0n)}</span>
      </p>
      <div className="flex items-end gap-3">
        <fieldset className="flex gap-2">
          <legend className="sr-only">{m.profileLabel}</legend>
          {profiles.map((p) => (
            <Button
              key={p}
              variant={p === target ? "primary" : "secondary"}
              aria-pressed={p === target}
              disabled={!can}
              onClick={() => setChosen(p)}
            >
              {names[p] ?? p}
            </Button>
          ))}
        </fieldset>
        <Button disabled={!can || target === current} onClick={() => setConfirm(true)}>
          {m.changeProfile}
        </Button>
      </div>
      {failed && (
        <p role="alert" className="text-body text-danger">
          {m.saveFailed}
        </p>
      )}
      <h2 className="text-section">{m.effective}</h2>
      <Table
        caption={m.effective}
        heads={[m.colSetting, m.colValue]}
        rows={settingsOf(config.data?.effectiveJson).map(([k, v]) => ({
          key: k,
          cells: [
            <span key="k" className="font-mono">
              {k}
            </span>,
            <span key="v" className="font-mono">
              {v}
            </span>,
          ],
        }))}
      />
      <TypeConfirmDialog
        open={confirm}
        onOpenChange={setConfirm}
        phrase={env}
        description={m.profileConfirm}
        onConfirm={() => {
          setConfirm(false);
          const id = environment.environment?.id;
          if (id !== undefined) {
            setProfile(app, id, target, config.data?.version ?? 0n).then(
              () => setFailed(false),
              () => setFailed(true),
            );
          }
        }}
      />
    </Frame>
  );
}

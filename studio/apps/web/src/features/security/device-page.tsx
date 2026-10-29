// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { timestampDate } from "@bufbuild/protobuf/wkt";
import { useLingui } from "@lingui/react";
import { Button, StatusPill } from "@plux/ui";
import { useParams } from "@tanstack/react-router";
import { type ReactElement, useState } from "react";
import { JustificationDialog } from "../../components/dialogs/index.ts";
import { Frame, fmt } from "./parts.tsx";
import { securityMessages } from "./security.messages.ts";
import { useDevice, usePermissions, useRevokeDevice } from "./use-security.ts";

/** A32: one device with its attestation; revoking asks for a reason that the server audits (D62). */
export function DevicePage(): ReactElement {
  const m = securityMessages(useLingui().i18n);
  const id = (useParams({ strict: false }) as { deviceId?: string }).deviceId ?? "";
  const device = useDevice(id);
  const revoke = useRevokeDevice();
  const can = usePermissions().has("app.manage");
  const [open, setOpen] = useState(false);
  const [failed, setFailed] = useState(false);
  const d = device.data?.device;
  const rows: readonly (readonly [string, string])[] =
    d === undefined
      ? []
      : [
          [m.colPlatform, d.platform],
          [m.osVersion, d.osVersion],
          [m.runtimeVersion, d.runtimeVersion],
          [m.colBuild, d.hostBuild],
          [m.colAssurance, d.assuranceLevel],
          [m.keyId, d.dpopJkt],
          [m.provider, d.attestation?.provider ?? ""],
          [m.colAttested, fmt(d.attestation?.verifiedAt && timestampDate(d.attestation.verifiedAt), m.never)],
          [m.registered, fmt(d.registeredAt && timestampDate(d.registeredAt), m.never)],
          [m.lastSeen, fmt(d.lastSeenAt && timestampDate(d.lastSeenAt), m.never)],
        ];
  return (
    <Frame id="device-title" title={m.deviceOf(id)} loading={device.isPending} failed={device.isError}>
      {d !== undefined && (
        <>
          <dl className="flex flex-col gap-1 text-body">
            {rows.map(([k, v]) => (
              <div key={k} className="flex gap-4">
                <dt className="w-48 text-secondary">{k}</dt>
                <dd className="font-mono">{v}</dd>
              </div>
            ))}
          </dl>
          <div className="flex items-center gap-3">
            {d.revokedAt === undefined ? (
              <StatusPill status="success" label={m.stateActive} />
            ) : (
              <StatusPill status="danger" label={m.stateRevoked} />
            )}
            <Button
              variant="danger"
              disabled={!can || d.revokedAt !== undefined}
              title={can ? undefined : m.revokeNoPermission}
              onClick={() => setOpen(true)}
            >
              {m.revoke}
            </Button>
          </div>
          {failed && (
            <p role="alert" className="text-body text-danger">
              {m.saveFailed}
            </p>
          )}
          <JustificationDialog
            open={open}
            onOpenChange={setOpen}
            description={m.revokeDescription}
            onSubmit={(reason) => {
              setOpen(false);
              revoke(id, reason).then(
                () => setFailed(false),
                () => setFailed(true),
              );
            }}
          />
        </>
      )}
    </Frame>
  );
}

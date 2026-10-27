// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { timestampMs } from "@bufbuild/protobuf/wkt";
import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { type DevPairing, DevSessionService } from "@plux/api";
import { Button, Dialog } from "@plux/ui";
import { useMutation } from "@tanstack/react-query";
import { type ReactNode, useEffect, useMemo, useState } from "react";
import { useApp } from "../../lib/use-app.tsx";
import { devicesMessages } from "./devices.messages.ts";
import { QrCode } from "./qr-code.tsx";

/** Whole seconds left until `expiresAtMs`, never below zero. */
export function secondsLeft(expiresAtMs: number, nowMs: number): number {
  return Math.max(0, Math.ceil((expiresAtMs - nowMs) / 1000));
}

/** Minutes and seconds as `m:ss`. */
export function clock(seconds: number): string {
  return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, "0")}`;
}

function useNow(): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, []);
  return now;
}

function PairingBody({ pairing }: { readonly pairing: DevPairing }): ReactNode {
  const m = devicesMessages(useLingui().i18n);
  const now = useNow();
  const expiresAt = pairing.expiresAt === undefined ? 0 : timestampMs(pairing.expiresAt);
  const left = secondsLeft(expiresAt, now);
  return (
    <div className="flex flex-col items-center gap-3">
      <p className="text-body text-secondary">{m.pairHelp}</p>
      {left > 0 && <QrCode text={pairing.uri} label={m.qrLabel} />}
      <div className="flex w-full flex-col gap-1 text-body">
        <span className="text-secondary">{m.code}</span>
        <span className="font-mono text-primary">{pairing.code}</span>
        <span className="text-secondary">{m.link}</span>
        <span className="break-all font-mono text-caption text-primary">{pairing.uri}</span>
      </div>
      <p role="timer" className="text-body text-secondary">
        {left > 0 ? m.expiresIn(clock(left)) : m.expired}
      </p>
    </div>
  );
}

/**
 * D39: pairs a device (DEV-002). The QR holds the pairing link; the short code is the typed alternative.
 * The token lives at most two minutes, counted from the server's `expiresAt`, and a new one is one click away.
 */
export function PairingDialog({
  sessionId,
  onClose,
}: {
  readonly sessionId: string;
  readonly onClose: () => void;
}): ReactNode {
  const m = devicesMessages(useLingui().i18n);
  const { session } = useApp();
  const client = useMemo(() => createClient(DevSessionService, session.transport), [session.transport]);
  const mint = useMutation({ mutationFn: async () => (await client.getPairing({ sessionId })).pairing });
  const { mutate } = mint;
  // biome-ignore lint/correctness/useExhaustiveDependencies: one token per opening of the dialog
  useEffect(() => mutate(), []);
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()} title={m.pairTitle} labels={{ close: m.close }}>
      {mint.data !== undefined && <PairingBody key={mint.data.token} pairing={mint.data} />}
      {mint.isError && (
        <p role="alert" className="text-body">
          {m.failed}
        </p>
      )}
      <div className="mt-3 flex justify-end">
        <Button variant="secondary" size="sm" loading={mint.isPending} onClick={() => mutate()}>
          {m.newCode}
        </Button>
      </div>
    </Dialog>
  );
}

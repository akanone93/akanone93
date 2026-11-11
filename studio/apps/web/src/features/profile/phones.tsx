// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { timestampDate, timestampMs } from "@bufbuild/protobuf/wkt";
import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { type DevPhone, DevPhoneService, PhoneSignInState } from "@plux/api";
import { AlertDialog, Dialog, Icon } from "@plux/ui";
import { Button } from "@plux/ui/components/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@plux/ui/components/card";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { type ReactElement, useEffect, useMemo, useState } from "react";
import { useApp } from "../../lib/use-app.tsx";
import { secondsLeft } from "../devices/pairing-dialog.tsx";
import { QrCode } from "../devices/qr-code.tsx";
import { useGuarded } from "../releases/guarded.tsx";
import { phonesMessages } from "./phones.messages.ts";

type Messages = ReturnType<typeof phonesMessages>;

/** A sign-in being shown: its QR code's link, once, and when it stops working. */
type Shown = { readonly id: string; readonly link: string; readonly expiresAtMs: number };

/** How often the dialog asks where its sign-in stands, in milliseconds. */
const pollMs = 1000;

function usePhonesClient() {
  const { session } = useApp();
  return useMemo(() => createClient(DevPhoneService, session.transport), [session.transport]);
}

function useNow(): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, []);
  return now;
}

/**
 * The person's phones signed in to the Plux Dev app (DEV-002, ADR-0104): connect one with a QR code after a step-up
 * with the security key (SEC-103) and a number picked on the phone, see when each was last used, sign one out.
 */
export function PhonesCard(): ReactElement {
  const { i18n } = useLingui();
  const m = phonesMessages(i18n);
  const client = usePhonesClient();
  const queryClient = useQueryClient();
  const phones = useQuery({ queryKey: ["dev-phones"], queryFn: async () => (await client.listPhones({})).phones });
  const [shown, setShown] = useState<Shown | null>(null);
  const [signingOut, setSigningOut] = useState<DevPhone | null>(null);
  const { guard, dialog, failed } = useGuarded();
  const start = () =>
    guard(async () => {
      const r = await client.createPhoneSignIn({});
      setShown({
        id: r.signInId,
        link: r.link,
        expiresAtMs: r.expiresAt === undefined ? Date.now() : timestampMs(r.expiresAt),
      });
    });
  const refresh = () => queryClient.invalidateQueries({ queryKey: ["dev-phones"] });
  const list = phones.data ?? [];
  return (
    <Card>
      <CardHeader>
        <CardTitle>{m.title}</CardTitle>
        <CardDescription>{m.caption}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {phones.isSuccess && list.length === 0 && <p className="text-sm text-muted-foreground">{m.empty}</p>}
        {list.length > 0 && (
          <ul aria-label={m.title} className="flex flex-col gap-2">
            {list.map((p) => (
              <PhoneRow key={p.id} phone={p} m={m} locale={i18n.locale} onSignOut={() => setSigningOut(p)} />
            ))}
          </ul>
        )}
        <div>
          <Button onClick={start}>
            <Icon name="device-mobile" data-icon="inline-start" />
            {m.connect}
          </Button>
        </div>
        {failed && (
          <p role="alert" className="text-sm">
            {m.connectFailed}
          </p>
        )}
      </CardContent>
      {dialog}
      {shown !== null && (
        <ConnectPhoneDialog
          shown={shown}
          m={m}
          onAgain={() => {
            setShown(null);
            start();
          }}
          onClose={() => {
            setShown(null);
            void refresh();
          }}
          onConnected={() => void refresh()}
        />
      )}
      <AlertDialog
        open={signingOut !== null}
        onOpenChange={(open) => {
          if (!open) {
            setSigningOut(null);
          }
        }}
        title={m.signOutTitle}
        description={m.signOutBody}
        labels={{ cancel: m.cancel, confirm: m.signOutConfirm }}
        onConfirm={() => {
          const phone = signingOut;
          setSigningOut(null);
          if (phone !== null) {
            void client.revokePhone({ phoneId: phone.id }).finally(() => void refresh());
          }
        }}
      />
    </Card>
  );
}

function PhoneRow(p: {
  readonly phone: DevPhone;
  readonly m: Messages;
  readonly locale: string;
  readonly onSignOut: () => void;
}): ReactElement {
  const { phone, m } = p;
  const used =
    phone.lastUsedAt === undefined || timestampMs(phone.lastUsedAt) === 0
      ? m.neverUsed
      : m.lastUsed(
          new Intl.DateTimeFormat(p.locale, { dateStyle: "medium", timeStyle: "short" }).format(
            timestampDate(phone.lastUsedAt),
          ),
        );
  return (
    <li className="flex flex-wrap items-center gap-3 rounded-lg border px-3 py-2">
      <Icon name="device-mobile" />
      <div className="flex min-w-0 flex-1 flex-col">
        <span className="truncate text-sm font-medium">{phone.name}</span>
        <span className="text-xs text-muted-foreground">
          {phone.platform !== "" && `${phone.platform} · `}
          {used}
        </span>
      </div>
      <Button variant="ghost" size="icon" aria-label={m.signOut(phone.name)} onClick={p.onSignOut}>
        <Icon name="sign-out" />
      </Button>
    </li>
  );
}

/**
 * The QR code that connects a phone, then, once a phone scanned it, the number to pick on that phone: it asks the
 * server where the sign-in stands every second. Closed before the phone is connected, the sign-in is cancelled.
 */
function ConnectPhoneDialog(p: {
  readonly shown: Shown;
  readonly m: Messages;
  readonly onAgain: () => void;
  readonly onClose: () => void;
  readonly onConnected: () => void;
}): ReactElement {
  const { shown, m, onConnected } = p;
  const client = usePhonesClient();
  const now = useNow();
  const status = useQuery({
    queryKey: ["dev-phone-sign-in", shown.id],
    queryFn: () => client.getPhoneSignIn({ signInId: shown.id }),
    refetchInterval: (q) => {
      const state = q.state.data?.state;
      return state === undefined || state === PhoneSignInState.WAITING || state === PhoneSignInState.CLAIMED
        ? pollMs
        : false;
    },
  });
  const state = status.data?.state ?? PhoneSignInState.WAITING;
  const done = state === PhoneSignInState.SIGNED_IN;
  useEffect(() => {
    if (done) {
      onConnected();
    }
  }, [done, onConnected]);
  const close = () => {
    if (!done) {
      void client.cancelPhoneSignIn({ signInId: shown.id }).catch(() => undefined);
    }
    p.onClose();
  };
  const left = secondsLeft(shown.expiresAtMs, now);
  const phone = status.data?.phoneName ?? "";
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) {
          close();
        }
      }}
      size="sm"
      title={m.dialogTitle}
      labels={{ close: m.close }}
    >
      <div className="flex flex-col items-center gap-4 py-2 text-center" aria-live="polite">
        {state === PhoneSignInState.WAITING && left > 0 && (
          <>
            <QrCode text={shown.link} label={m.qrLabel} />
            <p className="text-sm">{m.scan}</p>
            <p className="text-xs text-muted-foreground">{m.expiresIn(left)}</p>
          </>
        )}
        {state === PhoneSignInState.CLAIMED && left > 0 && (
          <>
            <p className="text-sm font-medium">{m.pickOn(phone)}</p>
            <output
              aria-label={m.number(status.data?.number ?? 0)}
              className="rounded-2xl bg-primary/10 px-8 py-3 text-5xl font-bold tracking-widest text-primary tabular-nums"
            >
              {String(status.data?.number ?? "")}
            </output>
            <p className="text-xs text-muted-foreground">{m.pickHint}</p>
          </>
        )}
        {done && (
          <p role="status" className="flex items-center gap-2 text-sm font-medium">
            <Icon name="check-circle" />
            {m.connected(phone)}
          </p>
        )}
        {(state === PhoneSignInState.FAILED ||
          state === PhoneSignInState.EXPIRED ||
          (!done && state !== PhoneSignInState.CANCELLED && left === 0)) && (
          <>
            <p role="alert" className="text-sm">
              {state === PhoneSignInState.FAILED ? m.wrong : m.expired}
            </p>
            <Button onClick={p.onAgain}>{m.again}</Button>
          </>
        )}
      </div>
    </Dialog>
  );
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { DevSessionService, DevThemeMode } from "@plux/api";
import { Button, Dialog, Field, Input, Select } from "@plux/ui";
import { useMutation, useQuery } from "@tanstack/react-query";
import { type ReactNode, useMemo, useState } from "react";
import { useApp } from "../../lib/use-app.tsx";
import { devicesMessages } from "./devices.messages.ts";

const themes = {
  light: DevThemeMode.LIGHT,
  dark: DevThemeMode.DARK,
  system: DevThemeMode.SYSTEM,
} as const;

/**
 * D38: sends a screen of the stored draft to a paired device (STU-060, STU-130, DEV-011). The session
 * follows the draft (contract #17), so this sends only the command that opens the route with the parameters.
 */
export function SendToDeviceDialog({
  sessionId,
  initialRoute = "",
  onClose,
}: {
  readonly sessionId: string;
  readonly initialRoute?: string;
  readonly onClose: () => void;
}): ReactNode {
  const m = devicesMessages(useLingui().i18n);
  const { session } = useApp();
  const client = useMemo(() => createClient(DevSessionService, session.transport), [session.transport]);
  const devices = useQuery({
    queryKey: ["dev-devices", sessionId],
    queryFn: async () => (await client.listDevices({ sessionId })).devices,
  });
  const [deviceId, setDeviceId] = useState("");
  const [route, setRoute] = useState(initialRoute);
  const [locale, setLocale] = useState("");
  const [theme, setTheme] = useState<keyof typeof themes>("system");
  const [scale, setScale] = useState("1");
  const send = useMutation({
    mutationFn: async () =>
      client.sendCommand({
        sessionId,
        deviceId,
        command: {
          case: "openScreen",
          value: { route, locale, theme: themes[theme], textScale: Number(scale) || 1, params: {}, mocks: [] },
        },
      }),
  });
  const list = devices.data ?? [];
  const chosen = deviceId === "" ? (list[0]?.id ?? "") : deviceId;
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()} title={m.sendTitle} labels={{ close: m.close }}>
      {devices.isSuccess && list.length === 0 && <p className="text-body text-secondary">{m.noDevices}</p>}
      {list.length > 0 && (
        <form
          className="flex flex-col gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            setDeviceId(chosen);
            send.mutate();
          }}
        >
          <Select
            label={m.device}
            value={chosen}
            onValueChange={setDeviceId}
            options={list.map((d) => ({ value: d.id, label: m.deviceName(d.name, d.connected) }))}
          />
          <Field label={m.route}>
            <Input value={route} onChange={(e) => setRoute(e.target.value)} />
          </Field>
          <Field label={m.locale}>
            <Input value={locale} onChange={(e) => setLocale(e.target.value)} />
          </Field>
          <Select
            label={m.theme}
            value={theme}
            onValueChange={(v) => setTheme(v as keyof typeof themes)}
            options={[
              { value: "light", label: m.themeLight },
              { value: "dark", label: m.themeDark },
              { value: "system", label: m.themeSystem },
            ]}
          />
          <Field label={m.textScale}>
            <Input inputMode="decimal" value={scale} onChange={(e) => setScale(e.target.value)} />
          </Field>
          <Button type="submit" loading={send.isPending}>
            {m.send}
          </Button>
          {send.isSuccess && (
            <p role="status" className="text-body text-secondary">
              {m.sent}
            </p>
          )}
        </form>
      )}
      {(send.isError || devices.isError) && (
        <p role="alert" className="text-body">
          {m.failed}
        </p>
      )}
    </Dialog>
  );
}

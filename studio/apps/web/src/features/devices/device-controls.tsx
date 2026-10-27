// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Field, Input, Select } from "@plux/ui";
import { useMutation } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";
import { devicesMessages } from "./devices.messages.ts";
import { themes, useDevClient } from "./use-dev-session.ts";

/**
 * The commands a paired device understands for its look and place (STU-132, DEV-011): reload the screen, the
 * locale, the text scale, the theme and the route. All of them are one OpenScreen command.
 */
export function DeviceControls({
  sessionId,
  deviceId,
  initialRoute = "",
}: {
  readonly sessionId: string;
  /** Empty sends to every device of the session. */
  readonly deviceId: string;
  readonly initialRoute?: string;
}): ReactNode {
  const m = devicesMessages(useLingui().i18n);
  const client = useDevClient();
  const [route, setRoute] = useState(initialRoute);
  const [locale, setLocale] = useState("");
  const [theme, setTheme] = useState<keyof typeof themes>("system");
  const [scale, setScale] = useState("1");
  const open = useMutation({
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
  return (
    <fieldset aria-label={m.controlsTitle} className="flex flex-wrap items-end gap-2">
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
      <Button size="sm" loading={open.isPending} onClick={() => open.mutate()}>
        {m.apply}
      </Button>
      <Button size="sm" variant="secondary" loading={open.isPending} onClick={() => open.mutate()}>
        {m.reload}
      </Button>
      {open.isError && (
        <span role="alert" className="text-body">
          {m.failed}
        </span>
      )}
    </fieldset>
  );
}

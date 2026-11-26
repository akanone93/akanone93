// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Field, Select } from "@plux/ui";
import { useNavigate } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { useEnvironments } from "../functions/use-functions.ts";
import { defaultEnvironment } from "../security/security.schema.ts";
import { useScope } from "../security/use-security.ts";
import { releasesMessages } from "./releases.messages.ts";

/**
 * The environment of a page whose content is per environment (releases, kill switches, experiments, flags), written to
 * `?env` so the address keeps it; the default environment leaves the address without one.
 */
export function EnvironmentPicker(): ReactElement | null {
  const m = releasesMessages(useLingui().i18n);
  const { app, env } = useScope();
  const navigate = useNavigate();
  const environments = useEnvironments(app);
  const list = environments.data?.environments ?? [];
  if (list.length === 0) {
    return null;
  }
  return (
    <Field label={m.environmentLabel}>
      <Select
        options={list.map((e) => ({ value: e.key, label: e.name || e.key }))}
        value={env}
        onValueChange={(key) =>
          void navigate({
            to: ".",
            resetScroll: false,
            replace: true,
            search: (s: object) => ({ ...s, env: key === defaultEnvironment ? undefined : key }),
          } as never)
        }
      />
    </Field>
  );
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { StatusPill } from "@plux/ui";
import { Link } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { shellSettingsMessages } from "./shell-settings.messages.ts";
import { useShellStatus } from "./use-shell.ts";

/**
 * The link from an app's settings to its native shell settings (`/o/:org/apps/:app/settings/shell`, A38), with a
 * warning pill when the shell needs an update (GEN-004).
 */
export function ShellSettingsLink({ org, appId }: { readonly org: string; readonly appId: string }): ReactElement {
  const m = shellSettingsMessages(useLingui().i18n);
  const status = useShellStatus(appId);
  return (
    <span className="inline-flex items-center gap-2">
      <Link to="/o/$org/apps/$app/settings/shell" params={{ org, app: appId }} className="text-body">
        {m.linkLabel}
      </Link>
      {status.data?.updateRequired === true && <StatusPill status="warning" label={m.statusRequired} />}
    </span>
  );
}

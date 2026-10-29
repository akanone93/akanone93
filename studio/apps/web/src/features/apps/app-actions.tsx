// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { App } from "@plux/api";
import { Button, Icon, Menu, type MenuEntry } from "@plux/ui";
import { useRouter } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { appsMessages } from "./apps.messages.ts";
import type { AppEntry } from "./apps-model.ts";

/** The permissions the card actions need; an action without its permission is disabled, never hidden (F14). */
export interface AppPermissions {
  /** `app.manage`: archive, unarchive and, with `readPlugins`, duplicate. */
  readonly manage: boolean;
  /** `plugin.read`: export and duplicate read the app's drafts. */
  readonly readPlugins: boolean;
}

/** What the actions do; the area implements them over the API. */
export interface AppActionHandlers {
  readonly onPin: (id: string) => void;
  readonly onArchive: (app: App, archived: boolean) => void;
  readonly onDuplicate: (app: App) => void;
  readonly onExport: (app: App) => void;
}

/** The action menu of a card or row (STU-023): open, settings, duplicate, export a `.plux` archive, archive or unarchive. */
export function AppActions({
  entry,
  org,
  permissions,
  handlers,
}: {
  readonly entry: AppEntry;
  readonly org: string;
  readonly permissions: AppPermissions;
  readonly handlers: AppActionHandlers;
}): ReactElement {
  const m = appsMessages(useLingui().i18n);
  const router = useRouter();
  const { app } = entry;
  const go = (path: string) => () => router.history.push(`/o/${org}/apps/${app.id}${path}`);
  const entries: MenuEntry[] = [
    { kind: "item", id: "open", label: m.actionOpen, onSelect: go("") },
    { kind: "item", id: "settings", label: m.actionSettings, onSelect: go("/settings") },
    { kind: "separator", id: "sep" },
    {
      kind: "item",
      id: "duplicate",
      label: m.actionDuplicate,
      disabled: !(permissions.manage && permissions.readPlugins),
      onSelect: () => handlers.onDuplicate(app),
    },
    {
      kind: "item",
      id: "export",
      label: m.actionExport,
      disabled: !permissions.readPlugins,
      onSelect: () => handlers.onExport(app),
    },
    app.archived
      ? {
          kind: "item",
          id: "unarchive",
          label: m.actionUnarchive,
          disabled: !permissions.manage,
          onSelect: () => handlers.onArchive(app, false),
        }
      : {
          kind: "item",
          id: "archive",
          label: m.actionArchive,
          disabled: !permissions.manage,
          onSelect: () => handlers.onArchive(app, true),
        },
  ];
  return (
    <Menu
      trigger={
        <Button size="sm" variant="ghost" aria-label={m.actions(app.name)}>
          <Icon name="dots-three" size="capsule" />
        </Button>
      }
      entries={entries}
    />
  );
}

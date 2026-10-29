// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { type Plugin, PluginService } from "@plux/api";
import { AlertDialog } from "@plux/ui";
import { useNavigate, useParams } from "@tanstack/react-router";
import { type ReactNode, useMemo, useState } from "react";
import { useApp } from "../../lib/use-app.tsx";
import { tabEditorSession } from "../editor/document-transport.ts";
import { useAppDocuments } from "../plugin-canvas/use-plugin-documents.ts";
import {
  BlockedDeletePanel,
  buildWhereUsed,
  type ChangeSet,
  checkPluginDelete,
  isBlocked,
  loadedDocs,
  redirectPluginIncoming,
  removeOutgoing,
  TargetPicker,
  TypeToConfirmDialog,
  type Use,
  useChangeSets,
  useProductionRelease,
} from "../safety/index.ts";
import { appCanvasMessages } from "./app-canvas.messages.ts";

/** What the toolbar asked of the selected plugin. */
export type SafetyMode = "delete" | "archive";

export interface PluginSafetyProps {
  readonly appId: string;
  readonly plugin: Plugin;
  readonly mode: SafetyMode;
  readonly onClose: () => void;
  /** The change went through; the plugin summaries are stale. */
  readonly onDone: () => void;
  readonly onFailed: (message: string) => void;
}

/**
 * The safety rules of F13 for a plugin of the app canvas: deleting checks the where-used index
 * (D19, STU-070, STU-071) and offers the assisted fixes of a blocked delete, a published plugin is
 * deprecated instead of deleted (STU-073), the rest goes to the trash (GOV-031), and an app with a
 * production release asks for the plugin's key first (STU-074).
 */
export function PluginSafety(props: PluginSafetyProps): ReactNode {
  const { appId, plugin, mode, onClose, onDone, onFailed } = props;
  const m = appCanvasMessages(useLingui().i18n);
  const { session } = useApp();
  const navigate = useNavigate();
  const params = useParams({ strict: false }) as { org?: string };
  const production = useProductionRelease(appId);
  const docs = useAppDocuments(appId);
  const [editorSession] = useState(() => tabEditorSession(undefined));
  const applyChanges = useChangeSets(appId, editorSession, undefined);
  const [redirecting, setRedirecting] = useState(false);
  const loaded = useMemo(() => loadedDocs(docs.data ?? []), [docs.data]);
  const index = useMemo(() => buildWhereUsed(loaded), [loaded]);
  const check = useMemo(() => checkPluginDelete(index, plugin.id), [index, plugin.id]);
  const plugins = useMemo(() => createClient(PluginService, session.transport), [session.transport]);

  const published = plugin.latestVersion > 0n && !plugin.archived;
  const deprecating = mode === "delete" && published;
  const blocked = mode === "delete" && !deprecating && isBlocked(check);
  const ready = docs.isSuccess && !blocked;

  const run = (call: () => Promise<unknown>) => {
    void call().then(onDone, () => onFailed(m.failed));
  };
  const apply = (changeSet: ChangeSet) => {
    void applyChanges(changeSet).then((ok) => (ok ? onDone() : onFailed(m.changeRefused)));
  };
  const confirm = () =>
    run(() =>
      deprecating || mode === "archive"
        ? plugins.archivePlugin({ id: plugin.id, archived: true })
        : plugins.deletePlugin({ id: plugin.id }),
    );
  const goTo = (use: Use) => {
    onClose();
    void navigate({
      to: "/o/$org/apps/$app/plugins/$plugin",
      params: { org: params.org ?? "", app: appId, plugin: use.pluginId },
      search: { focus: use.fromId },
    });
  };
  const incomingPages = check.incoming.filter((u) => u.kind === "navigates");
  const description = deprecating
    ? m.deprecateConfirm(plugin.name, check.incoming.length)
    : m.deleteConfirm(plugin.name);
  const action = deprecating ? m.deprecate : m.deletePlugin;

  return (
    <>
      <BlockedDeletePanel
        open={blocked && !redirecting}
        check={check}
        onNavigate={goTo}
        onRedirect={() => setRedirecting(true)}
        onRemoveOutgoing={() => apply(removeOutgoing(loaded, check.outgoing))}
        onOpenChange={(o) => !o && onClose()}
      />
      <TargetPicker
        open={blocked && redirecting}
        title={m.redirectTitle}
        label={m.redirectTarget}
        options={[...index.pages.values()]
          .filter((p) => p.pluginId !== plugin.id)
          .map((p) => ({ value: p.route, label: p.title }))}
        action={m.redirect}
        onConfirm={(route) => apply(redirectPluginIncoming(loaded, incomingPages, index.pages, route))}
        onOpenChange={(o) => !o && onClose()}
      />
      {ready && !production && (
        <AlertDialog
          open
          onOpenChange={(o) => !o && onClose()}
          title={action}
          description={description}
          labels={{ cancel: m.cancel, confirm: action }}
          onConfirm={confirm}
        />
      )}
      {ready && production && (
        <TypeToConfirmDialog
          open
          title={action}
          description={`${description} ${m.productionNote(plugin.key)}`}
          name={plugin.key}
          action={action}
          onConfirm={confirm}
          onOpenChange={(o) => !o && onClose()}
        />
      )}
    </>
  );
}

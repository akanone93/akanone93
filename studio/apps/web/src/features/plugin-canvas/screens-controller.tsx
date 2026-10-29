// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { Document } from "@plux/api";
import { AlertDialog, Button, Dialog, Field, Input } from "@plux/ui";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate, useParams } from "@tanstack/react-router";
import { type ReactNode, useCallback, useMemo, useState } from "react";
import { type Command, useScopedCommands } from "../../lib/commands/index.ts";
import { useApp } from "../../lib/use-app.tsx";
import { useEditSession, useLock } from "../editor/editor-provider.tsx";
import { instantiateScreen, TemplateStartField } from "../library/canvas/template-start.tsx";
import type { Validation } from "../problems/use-validation.ts";
import {
  BlockedDeletePanel,
  buildWhereUsed,
  type ChangeSet,
  checkScreenDelete,
  isBlocked,
  loadedDocs,
  moveScreen,
  type PageInfo,
  pluginRefs,
  RenameWarning,
  redirectIncoming,
  routeChangeEffects,
  TargetPicker,
  TypeToConfirmDialog,
  type Use,
  useChangeSets,
  useProductionRelease,
} from "../safety/index.ts";
import { ActionBar } from "./action-bar.tsx";
import { pluginCanvasMessages } from "./plugin-canvas.messages.ts";
import {
  createScreen,
  deleteScreen,
  duplicateScreen,
  renamedRoute,
  renameScreen,
  type ScreenContext,
  type ScreenResult,
} from "./screen-actions.ts";

/** What renaming `page` to `title` breaks outside Studio: its route may change with it (D27). */
function renameEffects(ctx: ScreenContext, page: PageInfo | undefined, id: string | undefined, title: string) {
  if (page === undefined || id === undefined || title === "") {
    return [];
  }
  return routeChangeEffects(page.route, renamedRoute(ctx, id, title) ?? page.route);
}

type Dialogue =
  | { kind: "new" }
  | { kind: "rename" }
  | { kind: "delete" }
  | { kind: "move" }
  | { kind: "redirect" }
  | null;

export interface ScreensControllerProps {
  readonly appId: string;
  readonly pluginId: string;
  readonly editorSessionId: string;
  readonly documents: readonly Document[];
  readonly appDocuments: readonly Document[] | undefined;
  readonly screens: readonly { readonly id: string; readonly title: string }[];
  readonly entryPage: string | undefined;
  readonly validation: Validation;
  readonly percent: number;
  readonly onZoom: (factor: number) => void;
  readonly onFit: () => void;
  /** Opens the history overlay (STU-069); the action bar button shows only when given. */
  readonly onHistory?: (() => void) | undefined;
  /** The frame, theme, locale and text scale controls of the preview (STU-058). */
  readonly preview?: ReactNode;
  readonly canSend?: boolean | undefined;
  readonly canPublish?: boolean | undefined;
  /** Opens the publish flow (A1). */
  readonly onPublish: () => void;
}

/**
 * The action bar, the screen dialogs and the screen commands of one open plugin (S2.7), with the
 * safety rules of F13: every delete checks the where-used index and opens the blocked-delete panel
 * with its assisted fixes (STU-070, STU-071), a rename lists what a route change breaks (D27),
 * deleting in an app with a production release asks for the name (STU-074), and a screen moves to
 * another plugin as one change set (D23).
 */
export function ScreensController(props: ScreensControllerProps): ReactNode {
  const m = pluginCanvasMessages(useLingui().i18n);
  const { session: app } = useApp();
  const transport = app.transport;
  const session = useEditSession();
  const lock = useLock();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const params = useParams({ strict: false }) as { org?: string };
  const production = useProductionRelease(props.appId);
  const openPlugin = useMemo(
    () => ({ pluginId: props.pluginId, lock, edit: session }),
    [props.pluginId, lock, session],
  );
  const applyChanges = useChangeSets(props.appId, props.editorSessionId, openPlugin);
  const [picked, setPicked] = useState<string | undefined>();
  const [dialogue, setDialogue] = useState<Dialogue>(null);
  const [title, setTitle] = useState("");
  const [templateId, setTemplateId] = useState("");
  const [message, setMessage] = useState<string | null>(null);
  const current = props.screens.some((s) => s.id === picked) ? picked : (props.entryPage ?? props.screens[0]?.id);
  const currentTitle = props.screens.find((s) => s.id === current)?.title ?? "";
  const ctx: ScreenContext = useMemo(
    () => ({ plugin: [...props.documents], app: props.appDocuments ?? [] }),
    [props.documents, props.appDocuments],
  );
  const loaded = useMemo(
    () => loadedDocs(props.appDocuments ?? props.documents),
    [props.appDocuments, props.documents],
  );
  const index = useMemo(() => buildWhereUsed(loaded), [loaded]);
  const page = current === undefined ? undefined : index.pages.get(current);
  const check = useMemo(
    () => (current === undefined ? undefined : checkScreenDelete(index, current)),
    [index, current],
  );

  const finish = useCallback(
    async (result: ScreenResult | Promise<ScreenResult>) => {
      const r = await result;
      if (r.ok) {
        setMessage(null);
        await Promise.all([
          queryClient.invalidateQueries({ queryKey: ["plugin-documents", props.appId, props.pluginId] }),
          queryClient.invalidateQueries({ queryKey: ["project-documents", props.appId] }),
        ]);
        return true;
      }
      setMessage(r.reason === "offline" ? m.offline : r.reason === "pending" ? m.pendingSave : m.writeFailed);
      return false;
    },
    [queryClient, props.appId, props.pluginId, m.offline, m.pendingSave, m.writeFailed],
  );

  const applyAndClose = useCallback(
    async (changeSet: ChangeSet) => {
      const ok = await applyChanges(changeSet);
      setMessage(ok ? null : m.changeRefused);
      if (ok) {
        setDialogue(null);
      }
    },
    [applyChanges, m.changeRefused],
  );

  const open = useCallback(
    (kind: "new" | "rename" | "delete" | "move") => {
      setMessage(null);
      setTemplateId("");
      setTitle(kind === "new" ? m.untitled : kind === "rename" ? currentTitle : "");
      setDialogue({ kind });
    },
    [m.untitled, currentTitle],
  );

  const duplicate = useCallback(() => {
    if (current !== undefined) {
      void finish(duplicateScreen(session, ctx, current, m.copySuffix));
    }
  }, [session, ctx, current, finish, m.copySuffix]);

  const commands = useMemo<readonly Command[]>(
    () => [
      { id: "screen.new", title: m.newScreen, group: "screens", permission: null, run: () => open("new") },
      { id: "screen.duplicate", title: m.duplicateScreen, group: "screens", permission: null, run: duplicate },
      { id: "screen.rename", title: m.renameScreen, group: "screens", permission: null, run: () => open("rename") },
      { id: "screen.move", title: m.moveScreen, group: "screens", permission: null, run: () => open("move") },
      { id: "screen.delete", title: m.deleteScreen, group: "screens", permission: null, run: () => open("delete") },
    ],
    [m.newScreen, m.duplicateScreen, m.renameScreen, m.moveScreen, m.deleteScreen, open, duplicate],
  );
  useScopedCommands("screens", commands);

  const submit = async () => {
    const text = title.trim();
    if (text === "") {
      return;
    }
    const ok =
      dialogue?.kind === "new"
        ? await finish(
            templateId === ""
              ? createScreen(session, ctx, text)
              : instantiateScreen(transport, {
                  templateId,
                  appId: props.appId,
                  pluginId: props.pluginId,
                  editorSessionId: props.editorSessionId,
                  key: text
                    .toLowerCase()
                    .replace(/[^a-z0-9]+/g, "-")
                    .replace(/^-+|-+$/g, ""),
                }),
          )
        : current !== undefined && (await finish(Promise.resolve(renameScreen(session, ctx, current, text))));
    if (ok) {
      setDialogue(null);
    }
  };

  const blocked = dialogue?.kind === "delete" && check !== undefined && isBlocked(check);
  const effects = dialogue?.kind === "rename" ? renameEffects(ctx, page, current, title.trim()) : [];
  const otherPlugins = pluginRefs(loaded).filter((p) => p.id !== props.pluginId);
  const goTo = (use: Use) => {
    setDialogue(null);
    void navigate({
      to: "/o/$org/apps/$app/plugins/$plugin",
      params: { org: params.org ?? "", app: props.appId, plugin: use.pluginId },
      search: { focus: use.fromId },
    });
  };
  const doMove = (target: string) => {
    if (current === undefined) {
      return;
    }
    const moved = moveScreen(loaded, index, current, target);
    if (moved.ok) {
      void applyAndClose(moved.changeSet);
      return;
    }
    setMessage(m.moveRefused(m.moveReason(moved.reason)));
    setDialogue(null);
  };
  const doDelete = () => {
    if (current !== undefined) {
      void finish(deleteScreen(session, ctx, current)).then((ok) => ok && setDialogue(null));
    }
  };

  const labels = { close: m.close };
  const titleField = (
    <form
      className="flex flex-col gap-4"
      onSubmit={(e) => {
        e.preventDefault();
        void submit();
      }}
    >
      <Field label={m.screenTitle}>
        <Input value={title} onChange={(e) => setTitle(e.target.value)} />
      </Field>
      {dialogue?.kind === "new" && <TemplateStartField value={templateId} onChange={setTemplateId} />}
      <RenameWarning effects={effects} />
      <Button type="submit">{dialogue?.kind === "new" ? m.create : m.rename}</Button>
    </form>
  );

  return (
    <>
      <ActionBar
        validation={props.validation}
        percent={props.percent}
        onZoom={props.onZoom}
        onFit={props.onFit}
        screens={props.screens}
        current={current}
        onCurrent={setPicked}
        onNewScreen={() => open("new")}
        onScreenAction={open}
        onHistory={props.onHistory}
        preview={props.preview}
        canSend={props.canSend}
        canPublish={props.canPublish}
        onPublish={props.onPublish}
      />
      {message !== null && (
        <p role="alert" className="absolute inset-x-0 bottom-20 text-center text-body">
          {message}
        </p>
      )}
      <Dialog
        open={dialogue?.kind === "new" || dialogue?.kind === "rename"}
        onOpenChange={(o) => !o && setDialogue(null)}
        title={dialogue?.kind === "new" ? m.newScreen : m.renameScreen}
        labels={labels}
      >
        {titleField}
      </Dialog>
      {check !== undefined && (
        <BlockedDeletePanel
          open={blocked}
          check={check}
          onNavigate={goTo}
          onRedirect={() => setDialogue({ kind: "redirect" })}
          onOpenChange={(o) => !o && setDialogue(null)}
        />
      )}
      <TargetPicker
        open={dialogue?.kind === "move"}
        title={m.moveScreen}
        label={m.moveTarget}
        options={otherPlugins.map((p) => ({ value: p.id, label: p.label }))}
        action={m.move}
        onConfirm={doMove}
        onOpenChange={(o) => !o && setDialogue(null)}
      />
      <TargetPicker
        open={dialogue?.kind === "redirect"}
        title={m.redirectTitle}
        label={m.redirectTarget}
        options={[...index.pages.values()]
          .filter((p) => p.id !== current)
          .map((p) => ({ value: p.route, label: p.title }))}
        action={m.redirect}
        onConfirm={(route) => {
          if (page !== undefined && check !== undefined) {
            void applyAndClose(redirectIncoming(loaded, check.incoming, page, route));
          }
        }}
        onOpenChange={(o) => !o && setDialogue(null)}
      />
      <AlertDialog
        open={dialogue?.kind === "delete" && !blocked && !production}
        onOpenChange={(o) => !o && setDialogue(null)}
        title={m.deleteScreen}
        description={m.confirmDelete(currentTitle)}
        labels={{ cancel: m.cancel, confirm: m.delete }}
        onConfirm={doDelete}
      />
      <TypeToConfirmDialog
        open={dialogue?.kind === "delete" && !blocked && production}
        title={m.deleteScreen}
        description={m.deleteProductionNote(currentTitle)}
        name={page?.key ?? currentTitle}
        action={m.deleteConfirmAction}
        onConfirm={doDelete}
        onOpenChange={(o) => !o && setDialogue(null)}
      />
    </>
  );
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { Document } from "@plux/api";
import { AlertDialog, Button, Dialog, Field, Input } from "@plux/ui";
import { useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useCallback, useMemo, useState } from "react";
import { type Command, useContributeCommands } from "../../lib/commands/index.ts";
import { useEditSession } from "../editor/editor-provider.tsx";
import type { Validation } from "../problems/use-validation.ts";
import { ActionBar } from "./action-bar.tsx";
import { pluginCanvasMessages } from "./plugin-canvas.messages.ts";
import {
  blockersOf,
  createScreen,
  deleteScreen,
  duplicateScreen,
  renameScreen,
  type ScreenContext,
  type ScreenResult,
} from "./screen-actions.ts";
import type { Blocker } from "./screens.ts";

type Dialogue = { kind: "new" } | { kind: "rename" } | { kind: "delete" } | null;

export interface ScreensControllerProps {
  readonly appId: string;
  readonly pluginId: string;
  readonly documents: readonly Document[];
  readonly appDocuments: readonly Document[] | undefined;
  readonly screens: readonly { readonly id: string; readonly title: string }[];
  readonly entryPage: string | undefined;
  readonly validation: Validation;
  readonly percent: number;
  readonly onZoom: (factor: number) => void;
  readonly onFit: () => void;
}

/** The action bar, the screen dialogs and the screen commands of one open plugin (S2.7). */
export function ScreensController(props: ScreensControllerProps): ReactNode {
  const m = pluginCanvasMessages(useLingui().i18n);
  const session = useEditSession();
  const queryClient = useQueryClient();
  const [picked, setPicked] = useState<string | undefined>();
  const [dialogue, setDialogue] = useState<Dialogue>(null);
  const [title, setTitle] = useState("");
  const [message, setMessage] = useState<string | null>(null);
  const current = props.screens.some((s) => s.id === picked) ? picked : (props.entryPage ?? props.screens[0]?.id);
  const currentTitle = props.screens.find((s) => s.id === current)?.title ?? "";
  const ctx: ScreenContext = useMemo(
    () => ({ plugin: [...props.documents], app: props.appDocuments ?? [] }),
    [props.documents, props.appDocuments],
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

  const open = useCallback(
    (kind: "new" | "rename" | "delete") => {
      setMessage(null);
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
      { id: "screen.delete", title: m.deleteScreen, group: "screens", permission: null, run: () => open("delete") },
    ],
    [m.newScreen, m.duplicateScreen, m.renameScreen, m.deleteScreen, open, duplicate],
  );
  useContributeCommands("plugin-canvas", commands);

  const submit = async () => {
    const text = title.trim();
    if (text === "") {
      return;
    }
    const ok =
      dialogue?.kind === "new"
        ? await finish(createScreen(session, ctx, text))
        : current !== undefined && (await finish(Promise.resolve(renameScreen(session, ctx, current, text))));
    if (ok) {
      setDialogue(null);
    }
  };

  const blockers: Blocker[] = dialogue?.kind === "delete" && current !== undefined ? blockersOf(ctx, current) : [];
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
      <Dialog
        open={dialogue?.kind === "delete" && blockers.length > 0}
        onOpenChange={(o) => !o && setDialogue(null)}
        title={m.blockedTitle}
        labels={labels}
      >
        <ul className="flex flex-col gap-2 text-body">
          {blockers.map((b) => (
            <li key={`${b.kind}-${b.pageId ?? ""}`}>
              {b.kind === "entry" ? m.blockedEntry : m.blockedNavigate(b.title ?? "")}
            </li>
          ))}
        </ul>
      </Dialog>
      <AlertDialog
        open={dialogue?.kind === "delete" && blockers.length === 0}
        onOpenChange={(o) => !o && setDialogue(null)}
        title={m.deleteScreen}
        description={m.confirmDelete(currentTitle)}
        labels={{ cancel: m.cancel, confirm: m.delete }}
        onConfirm={() => {
          if (current !== undefined) {
            void finish(deleteScreen(session, ctx, current)).then((ok) => ok && setDialogue(null));
          }
        }}
      />
    </>
  );
}

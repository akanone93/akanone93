// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { ComponentService, type LibraryItem, TemplateService } from "@plux/api";
import { AlertDialog } from "@plux/ui";
import { useQuery } from "@tanstack/react-query";
import { type ReactElement, useCallback, useMemo, useState } from "react";
import { type Command, useScopedCommands } from "../../../lib/commands/index.ts";
import { useApp } from "../../../lib/use-app.tsx";
import {
  applyResolutions,
  type Envelope,
  planReport,
  type ReportItem,
  type Resolution,
} from "../../plugin-canvas/clipboard/envelope.ts";
import { ClipboardReportDialog } from "../../plugin-canvas/clipboard/report-dialog.tsx";
import { type ClipboardInput, pasteRecipe, removeRecipe } from "../../plugin-canvas/clipboard/use-clipboard.tsx";
import { uuidv7 } from "../../plugin-canvas/screens.ts";
import { findNode } from "../../plugin-canvas/select/edits.ts";
import { canvasLibraryMessages } from "./canvas.messages.ts";
import { ExtractComponentDialog, InsertTemplateDialog, SaveTemplateDialog } from "./library-dialogs.tsx";
import {
  buildComponentDoc,
  buildTemplateDoc,
  candidateParameters,
  detachedNodes,
  instanceNode,
  instantiateRoot,
  rootOf,
  templateEnvelope,
} from "./template-doc.ts";

type Mode = "save" | "insert" | "extract" | "detach";
const enc = (doc: unknown) => new TextEncoder().encode(JSON.stringify(doc));
const dec = (bytes: Uint8Array) => JSON.parse(new TextDecoder().decode(bytes)) as Record<string, unknown>;

/** The page templates the caller may insert (STU-055, STU-083); fetched only while the dialog is open. */
export function useInsertableTemplates(enabled: boolean) {
  const { session } = useApp();
  return useQuery({
    queryKey: ["library", "insertable-templates"],
    enabled,
    queryFn: async () =>
      (await createClient(TemplateService, session.transport).listLibraryTemplates({ page: { pageSize: 200 } })).items,
  });
}

/** The head or given version of a template, parsed. */
export async function loadTemplateDoc(
  client: ReturnType<typeof createClient<typeof TemplateService>>,
  item: LibraryItem,
): Promise<Record<string, unknown>> {
  const versions = await client.listTemplateVersions({ id: item.id });
  const head = versions.versions.find((v) => v.version === item.headVersion) ?? versions.versions.at(-1);
  if (head === undefined) {
    throw new Error("no version");
  }
  return dec(head.content);
}

/**
 * The library commands of the plugin canvas (STU-080, STU-083, STU-084): save the selection as a template (D28),
 * insert a template with parameters and the dependency report of a paste (D29), extract to a component (D30) and
 * detach an instance (D33). Each edit to the plugin is one undo step. Returns the open dialog.
 */
export function useLibraryActions(input: ClipboardInput): ReactElement | null {
  const { i18n } = useLingui();
  const m = canvasLibraryMessages(i18n);
  const { session } = useApp();
  const [mode, setMode] = useState<Mode | undefined>();
  const [pending, setPending] = useState<{
    envelope: Envelope;
    nodes: readonly unknown[];
    items: readonly ReportItem[];
  }>();
  const { screens, screenOf, selected, edit, source, has } = input;
  const templates = createClient(TemplateService, session.transport);
  const components = createClient(ComponentService, session.transport);
  const list = useInsertableTemplates(mode === "insert");

  const selectedNodes = useCallback(() => {
    return selected().flatMap((id) => {
      const root = screens.find((s) => s.id === screenOf.get(id))?.root;
      const place = root === undefined ? null : findNode(root, id);
      return place === null ? [] : [place.node];
    });
  }, [selected, screens, screenOf]);

  const anchorDoc = useCallback(() => {
    const anchor = selected()[0];
    return { anchor, doc: (anchor === undefined ? undefined : screenOf.get(anchor)) ?? screens[0]?.id };
  }, [selected, screenOf, screens]);

  const place = useCallback(
    (nodes: readonly unknown[], step: string, remove: readonly string[] = []) => {
      const { anchor, doc } = anchorDoc();
      if (doc === undefined) {
        return;
      }
      const group = `${step}-${uuidv7()}`;
      edit(doc, pasteRecipe(anchor, nodes), { group, step });
      for (const id of remove) {
        edit(screenOf.get(id) ?? doc, removeRecipe(id), { group, step });
      }
    },
    [anchorDoc, edit, screenOf],
  );

  const saveTemplate = async (
    details: { name: string; key: string; description: string; visibility: string },
    parameters: Parameters<typeof buildTemplateDoc>[1],
  ) => {
    const doc = buildTemplateDoc({ ...details, category: "page" }, parameters, selectedNodes(), source);
    const created = await templates.createTemplate({
      draft: {
        key: details.key,
        name: details.name,
        description: details.description,
        templateKind: "page",
        content: enc(doc),
      },
    });
    if (details.visibility !== "private" && created.item !== undefined) {
      await templates.setTemplateVisibility({ id: created.item.id, visibility: details.visibility });
    }
  };

  const extract = async (details: { name: string; key: string; description: string }) => {
    const nodes = selectedNodes();
    const doc = buildComponentDoc(details, nodes, source);
    const created = await components.createComponent({
      draft: { key: details.key, name: details.name, description: details.description, content: enc(doc) },
    });
    place([instanceNode(String(doc["id"]), created.version?.version ?? 1)], "extract", selected());
  };

  const insert = (doc: Record<string, unknown>, args: Record<string, string>) => {
    const nodes = instantiateRoot(doc, args);
    const envelope = templateEnvelope(doc, nodes);
    const items = planReport(envelope, source, has);
    if (items.length > 0) {
      setPending({ envelope, nodes, items });
    } else {
      place(nodes, "insert-template");
      setMode(undefined);
    }
  };

  const detach = async () => {
    const node = selectedNodes()[0] as { id?: string; component?: { id: string; version: number } } | undefined;
    if (node?.component === undefined) {
      return;
    }
    const found = (await components.listLibraryComponents({ page: { pageSize: 200 } })).items.find(
      (i) => i.documentId === node.component?.id,
    );
    const versions = found === undefined ? undefined : await components.listComponentVersions({ id: found.id });
    const content = versions?.versions.find((v) => v.version === node.component?.version)?.content;
    if (content !== undefined && node.id !== undefined) {
      place(detachedNodes(node, dec(content)["root"] ?? rootOf([])), "detach", [node.id]);
    }
  };

  const commands = useMemo<readonly Command[]>(() => {
    const reason = input.canEdit ? undefined : "read-only";
    const make = (id: string, title: string, run: Mode, edits: boolean): Command => ({
      id,
      title,
      group: "edit",
      permission: "plugin.edit",
      ...(edits && reason !== undefined ? { disabledReason: reason } : {}),
      run: () => setMode(run),
    });
    return [
      make("library.saveTemplate", m.saveCommand, "save", false),
      make("library.insertTemplate", m.insertCommand, "insert", true),
      make("library.extractComponent", m.extractCommand, "extract", true),
      make("library.detach", m.detachCommand, "detach", true),
    ];
  }, [input.canEdit, m.saveCommand, m.insertCommand, m.extractCommand, m.detachCommand]);
  useScopedCommands("plugin-library", commands);

  const close = () => {
    setMode(undefined);
    setPending(undefined);
  };
  if (pending !== undefined) {
    return (
      <ClipboardReportDialog
        items={pending.items}
        candidates={has}
        refusal={undefined}
        onConfirm={(choices: ReadonlyMap<string, Resolution>) => {
          place(applyResolutions(pending.nodes, choices), "insert-template");
          close();
        }}
        onClose={close}
      />
    );
  }
  if (mode === "save") {
    return (
      <SaveTemplateDialog
        candidates={candidateParameters(rootOf(selectedNodes()))}
        save={saveTemplate}
        onClose={close}
      />
    );
  }
  if (mode === "extract") {
    return <ExtractComponentDialog extract={extract} onClose={close} />;
  }
  if (mode === "insert") {
    return (
      <InsertTemplateDialog
        templates={list.data ?? []}
        state={list.isError ? "failed" : list.isPending ? "loading" : "ready"}
        load={(item) => loadTemplateDoc(templates, item)}
        insert={insert}
        onClose={close}
      />
    );
  }
  if (mode === "detach") {
    return (
      <AlertDialog
        open
        onOpenChange={(o) => !o && close()}
        title={m.detachTitle}
        description={m.detachDescription}
        labels={{ cancel: m.cancel, confirm: m.detachConfirm }}
        onConfirm={() => void detach().finally(close)}
      />
    );
  }
  return null;
}

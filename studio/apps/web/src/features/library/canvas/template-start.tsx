// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { TemplateService } from "@plux/api";
import { Select } from "@plux/ui";
import { useQuery } from "@tanstack/react-query";
import type { ReactElement } from "react";
import { useApp } from "../../../lib/use-app.tsx";
import type { WholeWrite } from "../../editor/edit-session.ts";
import { canvasLibraryMessages } from "./canvas.messages.ts";

const BLANK = "";
/** plux.v1.Severity.SEVERITY_ERROR. */
const SEVERITY_ERROR = 3;

/** STU-055: the new-screen dialog's "Start from" choice: a blank screen or one of the page templates. */
export function TemplateStartField({
  value,
  onChange,
}: {
  readonly value: string;
  readonly onChange: (templateId: string) => void;
}): ReactElement | null {
  const m = canvasLibraryMessages(useLingui().i18n);
  const { session } = useApp();
  const list = useQuery({
    queryKey: ["library", "page-templates"],
    queryFn: async () =>
      (await createClient(TemplateService, session.transport).listLibraryTemplates({ page: { pageSize: 200 } })).items,
  });
  const pages = (list.data ?? []).filter((t) => t.templateKind === "page");
  if (pages.length === 0) {
    return null;
  }
  return (
    <Select
      label={m.fromTemplate}
      value={value}
      options={[{ value: BLANK, label: m.blank }, ...pages.map((t) => ({ value: t.id, label: t.name }))]}
      onValueChange={onChange}
    />
  );
}

/** Creates a screen from a template on the server, in the editor session; failures read like any other write. */
export async function instantiateScreen(
  transport: Parameters<typeof createClient>[1],
  input: {
    readonly templateId: string;
    readonly appId: string;
    readonly pluginId: string;
    readonly editorSessionId: string;
    readonly key: string;
  },
): Promise<WholeWrite> {
  try {
    const res = await createClient(TemplateService, transport).instantiate({
      templateId: input.templateId,
      appId: input.appId,
      pluginId: input.pluginId,
      session: input.editorSessionId,
      arguments: new TextEncoder().encode("{}"),
      key: input.key,
    });
    return res.diagnostics.some((d) => d.severity >= SEVERITY_ERROR) ? { ok: false, reason: "unknown" } : { ok: true };
  } catch {
    return { ok: false, reason: "unknown" };
  }
}

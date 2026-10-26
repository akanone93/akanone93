// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { AppService } from "@plux/api";
import { Button, Dialog } from "@plux/ui";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useMemo } from "react";
import { useApp } from "../../lib/use-app.tsx";
import { appCanvasMessages } from "./app-canvas.messages.ts";

/**
 * The trash of one app (A44, GOV-031): what was deleted in the last 30 days, restorable with its
 * old IDs so that links return. Permanent deletion stays with administrators and is not offered here.
 */
export function TrashDialog({
  appId,
  onClose,
  onRestored,
}: {
  readonly appId: string;
  readonly onClose: () => void;
  readonly onRestored: () => void;
}): ReactNode {
  const m = appCanvasMessages(useLingui().i18n);
  const { session } = useApp();
  const queryClient = useQueryClient();
  const apps = useMemo(() => createClient(AppService, session.transport), [session.transport]);
  const trash = useQuery({
    queryKey: ["trash", appId],
    queryFn: async () => (await apps.listTrash({ appId })).items,
  });
  const restore = useMutation({
    mutationFn: async (id: string) => apps.restoreFromTrash({ id }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["trash", appId] });
      await queryClient.invalidateQueries({ queryKey: ["project-documents", appId] });
      onRestored();
    },
  });
  const items = trash.data ?? [];
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()} title={m.trash} labels={{ close: m.close }}>
      {trash.isSuccess && items.length === 0 && <p className="text-body text-secondary">{m.trashEmpty}</p>}
      {trash.isError && (
        <p role="alert" className="text-body">
          {m.failed}
        </p>
      )}
      <ul className="mt-2 flex flex-col gap-2">
        {items.map((item) => (
          <li key={item.id} className="flex items-center justify-between gap-2">
            <span className="text-body text-primary">{m.trashItem(item.name, item.kind)}</span>
            <Button
              size="sm"
              variant="secondary"
              aria-label={m.trashRestore(item.name)}
              disabled={restore.isPending}
              onClick={() => restore.mutate(item.id)}
            >
              {m.restore}
            </Button>
          </li>
        ))}
      </ul>
      {restore.isError && (
        <p role="alert" className="mt-2 text-body">
          {m.failed}
        </p>
      )}
    </Dialog>
  );
}

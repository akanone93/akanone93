// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { AlertDialog } from "@plux/ui";
import type { ReactElement } from "react";
import type { GhostTarget } from "./ghosts.ts";
import { pluginCanvasMessages } from "./plugin-canvas.messages.ts";

/** D21: the confirmation before a ghost screen opens on its own plugin's canvas (STU-053). */
export function GhostLeaveDialog(props: {
  readonly ghost: GhostTarget | null;
  readonly onCancel: () => void;
  readonly onOpen: (ghost: GhostTarget) => void;
}): ReactElement {
  const m = pluginCanvasMessages(useLingui().i18n);
  const { ghost, onCancel, onOpen } = props;
  return (
    <AlertDialog
      open={ghost !== null}
      onOpenChange={(o) => !o && onCancel()}
      title={m.ghostLeaveTitle}
      description={m.ghostLeaveBody(ghost?.title ?? "")}
      labels={{ cancel: m.ghostStay, confirm: m.ghostOpen }}
      onConfirm={() => {
        if (ghost !== null) {
          onOpen(ghost);
        }
      }}
    />
  );
}

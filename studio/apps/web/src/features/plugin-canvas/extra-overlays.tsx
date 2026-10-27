// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Document } from "@plux/api";
import type { Problem } from "@plux/validator";
import { useParams } from "@tanstack/react-router";
import { useCanEdit, useEditSession } from "../editor/index.ts";
import { HistoryOverlay } from "../history/index.ts";
import { LocalisationHost } from "../localisation/index.ts";
import type { OverlayId } from "../overlay/index.ts";
import { StateDesignerOverlay, type StateTarget } from "../state-designer/index.ts";
import { uuidv7 } from "./screens.ts";

export interface ExtraOverlaysProps {
  readonly open: OverlayId | null;
  readonly onClose: () => void;
  readonly documents: readonly Document[] | undefined;
  readonly appDocuments: readonly Document[] | undefined;
  readonly target: StateTarget;
  readonly validatePage: ((file: string, data: string) => Promise<readonly Problem[]>) | undefined;
}

/** The localisation table V4, the state designer V5 and the history V9 over the plugin canvas (STU-063). */
export function ExtraOverlays(props: ExtraOverlaysProps) {
  const session = useEditSession();
  const editable = useCanEdit();
  const { plugin } = useParams({ strict: false }) as { plugin?: string };
  return (
    <>
      <LocalisationHost
        open={props.open === "l10n"}
        onClose={props.onClose}
        documents={props.documents}
        appDocuments={props.appDocuments}
      />
      <HistoryOverlay open={props.open === "history"} onClose={props.onClose} />
      <StateDesignerOverlay
        open={props.open === "state"}
        onClose={props.onClose}
        session={session}
        editable={editable}
        pluginPath={props.target.pluginPath}
        pluginId={plugin ?? ""}
        documents={props.target.loaded}
        validatePage={props.validatePage}
        newId={uuidv7}
      />
    </>
  );
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useParams } from "@tanstack/react-router";
import { usePublishedLoader } from "../json-editor/use-published.ts";
import type { OverlayId } from "../overlay/index.ts";
import type { Validation } from "../problems/use-validation.ts";
import { DataSourceOverlay } from "./data-source-overlay.tsx";
import { DbSchemaOverlay } from "./db-schema-overlay.tsx";
import { ThemeOverlay } from "./theme-overlay.tsx";
import type { DesignerDocument } from "./use-document.ts";

export interface DesignersHostProps {
  /** The overlay that is open, if any. */
  readonly open: OverlayId | null;
  readonly onClose: () => void;
  /** The plugin's documents and the app's; none until they load. */
  readonly documents: readonly DesignerDocument[] | undefined;
  readonly appDocuments: readonly DesignerDocument[] | undefined;
  readonly validation: Validation;
}

/** The data source (V6), DB schema (V7) and theme (V8) overlays of the plugin canvas, wired to the validator worker and the edit pipeline. */
export function DesignersHost(props: DesignersHostProps) {
  const { plugin } = useParams({ strict: false }) as { plugin?: string };
  const loadPublished = usePublishedLoader(plugin ?? "");
  const done = props.validation.state === "done" ? props.validation : undefined;
  return (
    <>
      <DataSourceOverlay
        open={props.open === "data"}
        onClose={props.onClose}
        documents={props.documents}
        validatePage={done?.validatePage}
        tool={done?.tool}
      />
      <DbSchemaOverlay
        open={props.open === "db"}
        onClose={props.onClose}
        documents={props.documents}
        validatePage={done?.validatePage}
        tool={done?.tool}
        loadPublished={loadPublished}
      />
      <ThemeOverlay
        open={props.open === "theme"}
        onClose={props.onClose}
        documents={props.appDocuments}
        validatePage={done?.validatePage}
      />
    </>
  );
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { AppProjectService } from "@plux/api";
import { useApp } from "../../lib/use-app.tsx";
import { blankProject } from "./blank-project.ts";
import { blankOptions, type Draft } from "./draft.ts";
import { packArchive } from "./plux-archive.ts";

/** What the server made. */
export interface CreatedApp {
  readonly id: string;
  readonly key: string;
  readonly name: string;
  readonly documents: number;
  /** Validation errors an imported project still has. */
  readonly errors: number;
}

export type CreateStage = "preparing" | "creating";

/**
 * Creates the app a draft describes with the project RPCs (ADR-0102): a blank app is built here as a project and
 * imported as a `.plux` archive, a template is a sample app the server imports, and an archive is imported as it
 * is. Each is one request, which the server validates in full and either creates whole or refuses.
 */
export function useCreateApp(organizationId: string) {
  const { session } = useApp();
  return async (draft: Draft, stage: (stage: CreateStage) => void): Promise<CreatedApp> => {
    const client = createClient(AppProjectService, session.transport);
    const key = draft.key;
    const name = draft.name.trim();
    stage("preparing");
    if (draft.source === "template") {
      stage("creating");
      const res = await client.importSampleApp({ organizationId, sampleId: draft.sampleId, key, name });
      return createdOf(res.app, res.documents, 0);
    }
    const archive =
      draft.source === "import"
        ? (draft.archive?.bytes ?? new Uint8Array())
        : await packArchive(
            { key, name, description: draft.description.trim(), tags: [] },
            blankProject(blankOptions(draft)),
          );
    stage("creating");
    const res = await client.importApp({ organizationId, archive: new Uint8Array(archive), key, name });
    return createdOf(res.app, res.documents, res.errorDiagnostics);
  };
}

function createdOf(
  app: { readonly id: string; readonly key: string; readonly name: string } | undefined,
  documents: number,
  errors: number,
): CreatedApp {
  return { id: app?.id ?? "", key: app?.key ?? "", name: app?.name ?? "", documents, errors };
}

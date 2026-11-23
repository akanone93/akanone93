// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { useParams } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { PageFrame } from "../../components/page.tsx";
import { EditorProvider } from "../editor/editor-provider.tsx";
import { assetsMessages } from "../panels/assets.messages.ts";
import { usePluginDocuments } from "../plugin-canvas/use-plugin-documents.ts";
import { AppAssets } from "./app-assets.tsx";

/**
 * The app's assets as a page of the app menu (#203): the same panel as the canvases' Assets tabs — every image,
 * animation and font of the app, where each is used, uploads and deletes under the app's lock — so a file needs no
 * trip to a canvas before Design › Typography or a picker can use it.
 */
export function AssetsPage(): ReactElement {
  const m = assetsMessages(useLingui().i18n);
  const params = useParams({ strict: false }) as { app?: string };
  const appId = params.app ?? "";
  const docs = usePluginDocuments(appId, "");
  return (
    <PageFrame
      id="assets-title"
      icon="image"
      title={m.tab}
      caption={m.pageCaption}
      loading={docs.isPending}
      failed={docs.isError}
      error={docs.error}
      labels={{ loading: m.pageLoading, loadFailed: m.pageFailed }}
    >
      <EditorProvider appId={appId} pluginId="" documents={docs.data}>
        <AppAssets appId={appId} documents={docs.data} />
      </EditorProvider>
    </PageFrame>
  );
}

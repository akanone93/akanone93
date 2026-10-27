// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { type ReactNode, useCallback, useMemo } from "react";
import { DeviceBay } from "../devices/index.ts";
import { nodePathOf, resolveNodePath } from "../devices/inspect-path.ts";
import type { DocState } from "../editor/edit-session.ts";
import { uploadAsset } from "../panels/upload-asset.ts";
import { ProblemsContent, ProblemsDrawer } from "../problems/problems-panel.tsx";
import type { Validation } from "../problems/use-validation.ts";
import type { Node } from "./plugin-model.ts";
import type { Selection } from "./select/selection.ts";
import type { SelectionStore } from "./select/store.ts";
import type { NodeIndex } from "./select/tree.ts";
import type { useAssetsTab } from "./use-assets-tab.ts";

interface BridgeInput {
  readonly screens: readonly { readonly id: string; readonly root: Node | undefined }[];
  readonly store: SelectionStore;
  readonly index: NodeIndex;
  readonly selection: Selection;
  readonly local: Readonly<Record<string, DocState>>;
  readonly assets: ReturnType<typeof useAssetsTab>;
}

/**
 * What the canvas gives the device bay (STU-131, STU-134, NFR-030): the document path of the selected node, the
 * selection a device tap makes, the screenshot store over the asset upload, and the count of accepted autosaves.
 */
function useDeviceBridge({ screens, store, index, selection, local, assets }: BridgeInput) {
  const only = selection.ids.length === 1 ? selection.ids[0] : undefined;
  const selectedPath = only === undefined ? undefined : nodePathOf(screens, only);
  const accepted = useMemo(() => Object.values(local).reduce((n, d) => n + Number(d.revision), 0), [local]);
  const inspect = useCallback(
    (path: string) => {
      const hit = resolveNodePath(screens, path);
      if (hit !== undefined) {
        store.set({ ids: [hit.node], anchor: hit.node, scope: index.parent.get(hit.node) ?? null });
      }
    },
    [screens, store, index],
  );
  const screenshotStore = useMemo(
    () => ({
      upload: (job: { name: string; file: Blob }) =>
        uploadAsset(
          assets.client,
          { appId: assets.appId, session: assets.session, ...job },
          { current: null },
          () => {},
        ),
    }),
    [assets],
  );
  return { selectedPath, accepted, inspect, screenshotStore, only };
}

/** The device bay of the canvas page, wired to its selection, documents and assets. */
export function DeviceBayHost({
  appId,
  validation,
  screenOf,
  entryPage,
  ...input
}: BridgeInput & {
  readonly appId: string | undefined;
  readonly validation: Validation;
  readonly screenOf: ReadonlyMap<string, string>;
  readonly entryPage: string | undefined;
}): ReactNode {
  const bridge = useDeviceBridge(input);
  const selectedScreen = bridge.only === undefined ? undefined : screenOf.get(bridge.only);
  return (
    <DeviceBay
      appId={appId ?? ""}
      problems={<ProblemsContent validation={validation} />}
      collapsed={<ProblemsDrawer validation={validation} />}
      selectedPath={bridge.selectedPath}
      onInspect={bridge.inspect}
      accepted={bridge.accepted}
      store={bridge.screenshotStore}
      route={selectedScreen ?? entryPage ?? ""}
    />
  );
}

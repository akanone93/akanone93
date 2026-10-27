// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { create } from "@bufbuild/protobuf";
import { FieldMaskSchema } from "@bufbuild/protobuf/wkt";
import { useMutation, useQuery } from "@connectrpc/connect-query";
import { PreferencesSchema, PreferencesService } from "@plux/api";
import { useCallback } from "react";
import { type PreviewState, readPreview, writePreview } from "./switches.ts";

/**
 * The preview switches of one app, stored per user in the existing preferences (STU-058). The server
 * replaces the whole `preview` map for that mask, so the other apps' entries are sent along unchanged.
 */
export function usePreviewSwitches(appId: string): { state: PreviewState; set: (next: PreviewState) => Promise<void> } {
  const prefs = useQuery(PreferencesService.method.getPreferences, {});
  const update = useMutation(PreferencesService.method.updatePreferences);
  const state = readPreview(prefs.data?.preferences?.preview[appId]);
  const stored = prefs.data?.preferences?.preview;
  const { mutateAsync } = update;
  const { refetch } = prefs;
  const set = useCallback(
    async (next: PreviewState) => {
      await mutateAsync({
        preferences: create(PreferencesSchema, { preview: { ...stored, [appId]: writePreview(next) } }),
        updateMask: create(FieldMaskSchema, { paths: ["preview"] }),
      });
      await refetch();
    },
    [appId, stored, mutateAsync, refetch],
  );
  return { state, set };
}

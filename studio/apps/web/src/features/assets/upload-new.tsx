// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { AssetService, PluginService } from "@plux/api";
import { FileDrop } from "@plux/ui";
import { useQueryClient } from "@tanstack/react-query";
import { type ReactElement, useState } from "react";
import { withReason } from "../../components/refusal.tsx";
import { useApp } from "../../lib/use-app.tsx";
import { assetsMessages } from "../panels/assets.messages.ts";
import { assetNameOf, uploadUnderAppLock } from "../panels/upload-asset.ts";

/**
 * "Upload a new file" beside an asset picker (build log #203): the file becomes an app asset, as from the Assets
 * page, under the app's lock taken for the upload alone, and the picker chooses it at once. The server checks and
 * scans the file; a refusal shows its reason.
 */
export function UploadNewAsset({
  appId,
  accept,
  disabled = false,
  onUploaded,
}: {
  readonly appId: string;
  /** The file types offered, as the file input's `accept`. */
  readonly accept: string;
  readonly disabled?: boolean;
  /** Called with the new asset's identifier. */
  readonly onUploaded: (assetId: string) => void;
}): ReactElement {
  const m = assetsMessages(useLingui().i18n);
  const { session } = useApp();
  const queryClient = useQueryClient();
  const [uploading, setUploading] = useState(false);
  const [failure, setFailure] = useState<unknown>(null);
  const upload = async (file: File) => {
    setUploading(true);
    setFailure(null);
    const name = assetNameOf(file);
    try {
      const assets = createClient(AssetService, session.transport);
      await uploadUnderAppLock({ ...assets, ...createClient(PluginService, session.transport) }, appId, name, file);
      const list = (await assets.listAssets({ appId })).assets;
      queryClient.setQueryData(["assets", appId], list);
      const made = list.find((a) => a.name === name);
      if (made !== undefined) {
        onUploaded(made.id);
      }
    } catch (error) {
      setFailure(error);
    } finally {
      setUploading(false);
    }
  };
  return (
    <div className="flex flex-col gap-1.5">
      <FileDrop
        variant="button"
        label={uploading ? m.uploadingNew : m.uploadNew}
        accept={accept}
        disabled={disabled || uploading}
        onFiles={(files) => {
          const file = files[0];
          if (file !== undefined) {
            void upload(file);
          }
        }}
      />
      {failure !== null && (
        <p role="alert" className="text-caption text-danger">
          {withReason(m.uploadNewFailed, failure)}
        </p>
      )}
    </div>
  );
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { type Asset, AssetService, PluginService } from "@plux/api";
import { FileDrop } from "@plux/ui";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { type ReactElement, useId, useState } from "react";
import { withReason } from "../../components/refusal.tsx";
import { objectUrl } from "../../lib/object-url.ts";
import { useApp } from "../../lib/use-app.tsx";
import { Choice } from "../new-app/choice.tsx";
import { assetNameOf, uploadUnderAppLock } from "../panels/upload-asset.ts";
import { shellSettingsMessages } from "./shell-settings.messages.ts";

/** The first URL of an asset's processed forms, or undefined while it has none. */
function urlOf(asset: Asset): string | undefined {
  const url = asset.variants.map((v) => v.artifact?.urls[0]).find((u) => u !== undefined && u !== "");
  return url === undefined ? undefined : objectUrl(url);
}

/**
 * The icon of the app (GEN-004): the default icon, or one of the app's PNG assets, each with a preview. The server
 * checks that the image is square and from 1024 to 4096 pixels a side when the settings are saved; the picker only
 * offers PNG files and shows their size.
 */
export function IconPicker({
  appId,
  value,
  disabled,
  onChange,
}: {
  readonly appId: string;
  /** The asset identifier of the chosen icon; empty for the default icon. */
  readonly value: string;
  readonly disabled: boolean;
  readonly onChange: (assetId: string) => void;
}): ReactElement {
  const m = shellSettingsMessages(useLingui().i18n);
  const { session } = useApp();
  const name = useId();
  const assets = useQuery({
    queryKey: ["shell-icon-assets", appId],
    queryFn: async () => (await createClient(AssetService, session.transport).listAssets({ appId })).assets,
  });
  const images = (assets.data ?? []).filter((a) => a.mediaType === "image/png");
  const queryClient = useQueryClient();
  const [uploading, setUploading] = useState(false);
  const [failure, setFailure] = useState<unknown>(null);
  // An icon uploaded here becomes an app asset (the same as from the Assets panel) and is chosen at once.
  const upload = async (file: File) => {
    setUploading(true);
    setFailure(null);
    const name = assetNameOf(file);
    try {
      const client = {
        ...createClient(AssetService, session.transport),
        ...createClient(PluginService, session.transport),
      };
      await uploadUnderAppLock(client, appId, name, file);
      const list = (await createClient(AssetService, session.transport).listAssets({ appId })).assets;
      queryClient.setQueryData(["shell-icon-assets", appId], list);
      void queryClient.invalidateQueries({ queryKey: ["assets", appId] });
      const made = list.find((a) => a.name === name);
      if (made !== undefined) {
        onChange(made.id);
      }
    } catch (error) {
      setFailure(error);
    } finally {
      setUploading(false);
    }
  };
  return (
    <fieldset className="flex flex-col gap-2">
      <legend className="text-label font-medium text-foreground">{m.fieldIcon}</legend>
      <p className="text-caption text-muted-foreground">{m.iconHint}</p>
      <div className="grid grid-cols-2 gap-2">
        <Choice
          type="radio"
          name={name}
          checked={value === ""}
          disabled={disabled}
          label={m.iconDefault}
          onChange={() => onChange("")}
        />
        {images.map((asset) => {
          const url = urlOf(asset);
          return (
            <Choice
              key={asset.id}
              type="radio"
              name={name}
              checked={value === asset.id}
              disabled={disabled}
              label={asset.name}
              {...(asset.width > 0 && { hint: m.iconSize(asset.width, asset.height) })}
              {...(url !== undefined && {
                leading: <img src={url} alt={m.iconPreview(asset.name)} className="size-10 rounded-md object-cover" />,
              })}
              onChange={() => onChange(asset.id)}
            />
          );
        })}
      </div>
      {assets.isError && (
        <p role="alert" className="text-body text-danger">
          {m.iconLoadFailed}
        </p>
      )}
      {assets.isSuccess && images.length === 0 && <p className="text-body text-muted-foreground">{m.iconNoAssets}</p>}
      <FileDrop
        variant="button"
        label={uploading ? m.iconUploading : m.iconUpload}
        accept="image/png"
        disabled={disabled || uploading}
        onFiles={(files) => {
          const file = files[0];
          if (file !== undefined) {
            void upload(file);
          }
        }}
      />
      {failure !== null && (
        <p role="alert" className="text-body text-danger">
          {withReason(m.iconUploadFailed, failure)}
        </p>
      )}
    </fieldset>
  );
}

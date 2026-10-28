// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { type Asset, AssetService } from "@plux/api";
import { useQuery } from "@tanstack/react-query";
import { type ReactElement, useId } from "react";
import { useApp } from "../../lib/use-app.tsx";
import { Choice } from "../new-app/choice.tsx";
import { shellSettingsMessages } from "./shell-settings.messages.ts";

/** The first URL of an asset's processed forms, or undefined while it has none. */
function urlOf(asset: Asset): string | undefined {
  return asset.variants.map((v) => v.artifact?.urls[0]).find((u) => u !== undefined && u !== "");
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
  return (
    <fieldset className="flex flex-col gap-2">
      <legend className="text-label font-medium text-primary">{m.fieldIcon}</legend>
      <p className="text-caption text-secondary">{m.iconHint}</p>
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
      {assets.isSuccess && images.length === 0 && <p className="text-body text-secondary">{m.iconNoAssets}</p>}
    </fieldset>
  );
}

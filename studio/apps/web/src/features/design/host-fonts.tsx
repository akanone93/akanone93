// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { AppService, AssetService } from "@plux/api";
import { Button, Field, Input, Select } from "@plux/ui";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { type ReactElement, useMemo, useState } from "react";
import { useApp } from "../../lib/use-app.tsx";
import { usePermissions } from "../security/use-security.ts";
import { designMessages } from "./design.messages.ts";
import { useDesign } from "./scope.tsx";

const FONT_TYPES: ReadonlySet<string> = new Set(["font/ttf", "font/otf"]);

/**
 * The font files the host app bundles (STU-006): each is a family name and the uploaded font asset that holds the
 * same file, so the canvas lays text out with the device's file. The list is the app's `hostFonts`; saving
 * replaces it whole.
 */
export function HostFonts(): ReactElement {
  const m = designMessages(useLingui().i18n);
  const { app } = useDesign();
  const { session } = useApp();
  const queryClient = useQueryClient();
  const apps = useMemo(() => createClient(AppService, session.transport), [session.transport]);
  const assets = useMemo(() => createClient(AssetService, session.transport), [session.transport]);
  const can = usePermissions().has("app.manage");
  const stored = useQuery({ queryKey: ["app", app], queryFn: async () => (await apps.getApp({ id: app })).app });
  const listed = useQuery({
    queryKey: ["assets", app],
    queryFn: async () => (await assets.listAssets({ appId: app })).assets,
  });
  const [family, setFamily] = useState("");
  const [asset, setAsset] = useState("");
  const [failed, setFailed] = useState(false);
  const fonts = stored.data?.hostFonts ?? [];
  const files = (listed.data ?? []).filter((a) => FONT_TYPES.has(a.mediaType));
  const nameOf = (id: string): string => files.find((a) => a.id === id)?.name ?? id;
  const save = async (next: readonly { family: string; assetId: string }[]): Promise<void> => {
    const current = stored.data;
    if (current === undefined) {
      return;
    }
    try {
      await apps.updateApp({
        id: app,
        name: current.name,
        defaultPluginKey: current.defaultPluginKey,
        setHostFonts: true,
        hostFonts: [...next],
      });
      setFailed(false);
      await queryClient.invalidateQueries({ queryKey: ["app", app] });
    } catch {
      setFailed(true);
    }
  };
  return (
    <section className="flex flex-col gap-3" aria-labelledby="design-host-fonts">
      <h2 id="design-host-fonts" className="text-section">
        {m.hostFontsTitle}
      </h2>
      <p className="text-body text-secondary">{m.hostFontsCaption}</p>
      {!can && <p className="text-caption text-secondary">{m.hostFontsNoPermission}</p>}
      {stored.isSuccess && fonts.length === 0 && <p className="text-body text-secondary">{m.hostFontsNone}</p>}
      <ul className="flex flex-col gap-1">
        {fonts.map((f) => (
          <li key={`${f.family}:${f.assetId}`} className="flex items-center gap-3 text-body">
            <span className="w-48">{f.family}</span>
            <span className="font-mono text-secondary">{nameOf(f.assetId)}</span>
            <Button
              variant="ghost"
              disabled={!can}
              aria-label={m.hostFontRemove(f.family)}
              onClick={() => void save(fonts.filter((x) => x !== f))}
            >
              {"×"}
            </Button>
          </li>
        ))}
      </ul>
      {listed.isSuccess && files.length === 0 ? (
        <p className="text-body text-secondary">{m.hostFontsNoAssets}</p>
      ) : (
        <form
          className="flex items-end gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            const name = family.trim();
            if (name === "" || asset === "") {
              return;
            }
            void save([...fonts, { family: name, assetId: asset }]).then(() => setFamily(""));
          }}
        >
          <Field label={m.hostFontFamily}>
            <Input value={family} disabled={!can} onChange={(e) => setFamily(e.target.value)} />
          </Field>
          <Field label={m.hostFontAsset}>
            <Select
              label={m.hostFontAsset}
              disabled={!can}
              options={files.map((a) => ({ value: a.id, label: a.name }))}
              onValueChange={setAsset}
            />
          </Field>
          <Button type="submit" disabled={!can || family.trim() === "" || asset === ""}>
            {m.hostFontAdd}
          </Button>
        </form>
      )}
      {failed && (
        <p role="alert" className="text-body text-danger">
          {m.hostFontsSaveFailed}
        </p>
      )}
    </section>
  );
}

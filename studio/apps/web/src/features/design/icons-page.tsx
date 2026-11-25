// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { type Asset, AssetService } from "@plux/api";
import { FileDrop, Icon } from "@plux/ui";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { type ReactElement, useMemo, useRef, useState } from "react";
import { PageEmpty } from "../../components/page.tsx";
import { withReason } from "../../components/refusal.tsx";
import { isSvg, previewUrlOf } from "../../lib/asset-forms.ts";
import { useApp } from "../../lib/use-app.tsx";
import { assetsMessages } from "../panels/assets.messages.ts";
import { apart } from "../panels/assets-tab.tsx";
import { uploadAsset, uploadFailure } from "../panels/upload-asset.ts";
import { assetFileLimit } from "../plugin-canvas/asset-uses.ts";
import { underAppLock } from "../plugin-canvas/use-assets-tab.ts";
import { designMessages } from "./design.messages.ts";
import { groupRows, ICONS_GROUP } from "./design-model.ts";
import { RemoveToken, show } from "./parts.tsx";
import { Frame, useDesign } from "./scope.tsx";

/** The folder the Icons page uploads custom icons to. */
const ICONS_FOLDER = "icons";

/** The asset name of an uploaded icon: its file name in lower case, safe characters only, under `icons/`. */
export function iconNameOf(file: { readonly name: string }): string {
  return `${ICONS_FOLDER}/${file.name.toLowerCase().replaceAll(/[^a-z0-9._-]/g, "-")}`;
}

/** Whether a file is offered as an SVG, by its type or its name; the server tells the type from the bytes (SRV-060). */
const looksSvg = (f: File): boolean => f.type === "image/svg+xml" || /\.svg$/i.test(f.name);

/** The app's SVG assets by folder, folders and icons by name: `icons/brand/star.svg` is in `icons/brand`. */
export function iconSets(assets: readonly Asset[]): (readonly [string, readonly Asset[]])[] {
  const sets = new Map<string, Asset[]>();
  for (const a of assets.filter(isSvg)) {
    const cut = a.name.lastIndexOf("/");
    const set = cut < 0 ? "" : a.name.slice(0, cut);
    sets.set(set, [...(sets.get(set) ?? []), a]);
  }
  return [...sets]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([set, list]) => [set, list.sort((x, y) => x.name.localeCompare(y.name))] as const);
}

/** The asset client of the page, its writes under the app lock the Design area holds (SRV-040). */
function useIconClient() {
  const { session } = useApp();
  const ctx = useDesign();
  const lock = useRef(ctx.session?.lock);
  lock.current = ctx.session?.lock;
  return useMemo(
    () => underAppLock(createClient(AssetService, session.transport), () => lock.current),
    [session.transport],
  );
}

/** Uploads SVG files as custom icons under `icons/`; a file that is not an SVG or is too large is refused first. */
function UploadIcons(props: { readonly client: ReturnType<typeof useIconClient> }): ReactElement {
  const { i18n } = useLingui();
  const m = designMessages(i18n);
  const am = assetsMessages(i18n);
  const ctx = useDesign();
  const queries = useQueryClient();
  const [busy, setBusy] = useState(0);
  const [notes, setNotes] = useState<readonly string[]>([]);
  const pick = async (files: readonly File[]) => {
    const max = assetFileLimit();
    const refused = files.flatMap((f) => {
      if (!looksSvg(f)) {
        return [m.notSvg(f.name)];
      }
      return f.size > max ? [am.tooBig(f.name, ...apart(f.size, max))] : [];
    });
    const sent = files.filter((f) => looksSvg(f) && f.size <= max);
    setNotes(refused);
    setBusy(sent.length);
    for (const file of sent) {
      try {
        await uploadAsset(
          props.client,
          { appId: ctx.app, name: iconNameOf(file), session: "", file },
          { current: null },
          () => undefined,
        );
      } catch (err) {
        const failure = uploadFailure(err);
        const note = failure === null ? withReason(am.failed(file.name), err) : am.failure(failure);
        setNotes((list) => [...list, note]);
      }
      setBusy((n) => n - 1);
    }
    await queries.invalidateQueries({ queryKey: ["assets", ctx.app] });
  };
  return (
    <div className="flex flex-col gap-2">
      <FileDrop
        label={m.uploadIcons}
        hint={m.uploadIconsHint}
        accept=".svg,image/svg+xml"
        multiple
        disabled={!ctx.canEdit || busy > 0}
        onFiles={(files) => void pick(files)}
      />
      {busy > 0 && (
        <p role="status" className="text-caption text-muted-foreground">
          {m.uploadingIcons(busy)}
        </p>
      )}
      {notes.map((note) => (
        <p key={note} role="alert" className="flex items-center gap-1 text-caption text-danger">
          <Icon name="warning" size="pill" />
          {note}
        </p>
      ))}
    </div>
  );
}

/** One custom icon: its picture as the browser draws it, its file name, and whether devices can draw it yet. */
function IconCard({ asset }: { readonly asset: Asset }): ReactElement {
  const m = designMessages(useLingui().i18n);
  const url = previewUrlOf(asset);
  const file = asset.name.slice(asset.name.lastIndexOf("/") + 1);
  const problem = asset.diagnostics[0]?.message;
  return (
    <li className="icon-card" title={asset.name}>
      <span className="icon-card-preview" aria-hidden="true">
        {url === undefined ? <Icon name="shapes" /> : <img src={url} alt="" loading="lazy" />}
      </span>
      <span className="icon-card-name font-mono" dir="ltr">
        {file}
      </span>
      {asset.processing === "pending" && <span className="text-caption text-muted-foreground">{m.iconPending}</span>}
      {asset.processing === "failed" && (
        <span className="text-caption text-danger" title={problem}>
          {m.iconFailed}
        </span>
      )}
    </li>
  );
}

/** The app's own icons: its SVG assets by folder, and the upload that adds to them (ADR-0032, THM-005). */
function CustomIcons(): ReactElement {
  const m = designMessages(useLingui().i18n);
  const ctx = useDesign();
  const client = useIconClient();
  const listed = useQuery({
    queryKey: ["assets", ctx.app],
    enabled: ctx.app !== "",
    queryFn: async () => (await client.listAssets({ appId: ctx.app })).assets,
  });
  const sets = iconSets(listed.data ?? []);
  return (
    <section aria-labelledby="design-icons-custom" className="design-card">
      <h2 id="design-icons-custom" className="text-label font-medium">
        {m.customIconsTitle}
      </h2>
      <p className="text-caption text-muted-foreground">{m.customIconsNote}</p>
      <UploadIcons client={client} />
      {listed.isSuccess && sets.length === 0 && <PageEmpty icon="shapes" text={m.noCustomIcons} />}
      {listed.isError && (
        <p role="alert" className="text-caption text-danger">
          {withReason(m.iconsUnavailable, listed.error)}
        </p>
      )}
      {sets.map(([set, icons]) => (
        <section key={set} aria-label={set === "" ? m.iconsAtRoot : set} className="flex flex-col gap-2">
          <h3 className="font-mono text-caption text-muted-foreground" dir="ltr">
            {set === "" ? m.iconsAtRoot : `${set}/`}
          </h3>
          <ul className="icon-grid">
            {icons.map((a) => (
              <IconCard key={a.id} asset={a} />
            ))}
          </ul>
        </section>
      ))}
    </section>
  );
}

/** The two built-in sets, named in an icon property and sent subset with the plugin (THM-005, CMP-032). */
function BuiltInSets(): ReactElement {
  const m = designMessages(useLingui().i18n);
  return (
    <section aria-labelledby="design-icons-builtin" className="design-card">
      <h2 id="design-icons-builtin" className="text-label font-medium">
        {m.builtInIconsTitle}
      </h2>
      <p className="text-caption text-muted-foreground">{m.builtInIconsNote}</p>
    </section>
  );
}

/**
 * The `icons` tokens earlier versions of the page saved, which nothing reads (build log row 25): listed only when a
 * theme still has them, each removable.
 */
function UnreadTokens(): ReactElement | null {
  const m = designMessages(useLingui().i18n);
  const ctx = useDesign();
  const rows = groupRows(ctx.rows, ICONS_GROUP);
  if (rows.length === 0) {
    return null;
  }
  return (
    <section aria-labelledby="design-icons-tokens" className="design-card">
      <h2 id="design-icons-tokens" className="text-label font-medium">
        {m.unreadIconTokens}
      </h2>
      <p className="text-caption text-muted-foreground">{m.unreadIconTokensNote}</p>
      <ul>
        {rows.map((r) => (
          <li key={r.path.join(".")} className="flex items-center gap-2 text-body">
            <span className="font-mono">{r.path.join(".")}</span>
            <span className="text-muted-foreground">{show(r.light)}</span>
            <RemoveToken path={[...ICONS_GROUP, ...r.path]} />
          </li>
        ))}
      </ul>
    </section>
  );
}

/**
 * A18: the app's icons (STU-112, THM-005): the built-in Material Symbols and Cupertino sets, and the app's own icons,
 * SVG assets shown with an `Image` (ADR-0032; build log rows 25 and 39).
 */
export function IconsPage(): ReactElement {
  const m = designMessages(useLingui().i18n);
  return (
    <Frame id="design-icons" title={m.iconsTitle} caption={m.iconsCaption}>
      <div className="flex max-w-4xl flex-col gap-4">
        <CustomIcons />
        <BuiltInSets />
        <UnreadTokens />
      </div>
    </Frame>
  );
}

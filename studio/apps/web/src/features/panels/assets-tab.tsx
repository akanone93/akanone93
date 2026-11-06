// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Client } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import type { Asset, AssetService } from "@plux/api";
import { AlertDialog, Button, FileDrop, Icon, type IconName, Meter } from "@plux/ui";
import { Button as IconButton } from "@plux/ui/components/button";
import { InputGroup, InputGroupAddon, InputGroupInput } from "@plux/ui/components/input-group";
import { type ReactElement, useCallback, useEffect, useRef, useState } from "react";
import { type AssetUse, assetBudget, assetFileLimit } from "../plugin-canvas/asset-uses.ts";
import { AssetDetails } from "./asset-details.tsx";
import { assetsMessages } from "./assets.messages.ts";
import { type UploadFailure, type UploadState, uploadAsset, uploadFailure } from "./upload-asset.ts";

type AssetClient = Pick<Client<typeof AssetService>, "listAssets" | "deleteAsset"> & Parameters<typeof uploadAsset>[0];

interface Pending {
  readonly key: number;
  readonly file: File;
  readonly name: string;
  readonly state: { current: UploadState | null };
  received: number;
  failed: boolean;
  failure: UploadFailure | null;
}

const MB = 1024 * 1024;

/** A file size as people read it: kilobytes below a megabyte, megabytes with a decimal below ten. */
function kb(n: number): string {
  if (n < MB) {
    return `${Math.max(1, Math.round(n / 1024))} KB`;
  }
  return `${(n / MB).toFixed(n < 10 * MB ? 1 : 0)} MB`;
}

/** Two sizes that must read as different: exact bytes when the rounded sizes would look the same. */
function apart(a: number, b: number): [string, string] {
  return kb(a) === kb(b) ? [`${a.toLocaleString("en")} B`, `${b.toLocaleString("en")} B`] : [kb(a), kb(b)];
}

type AssetKind = "image" | "vector" | "font" | "animation" | "other";
const assetKinds: readonly AssetKind[] = ["image", "vector", "font", "animation", "other"];
const kindIcons: Readonly<Record<AssetKind, IconName>> = {
  image: "image",
  vector: "shapes",
  font: "file-type",
  animation: "film",
  other: "file",
};

/** The kind of an asset from its sniffed media type and its name. */
export function assetKind(a: Pick<Asset, "mediaType" | "name">): AssetKind {
  const t = a.mediaType.toLowerCase();
  if (t === "image/svg+xml") {
    return "vector";
  }
  if (t.startsWith("image/")) {
    return "image";
  }
  if (t.startsWith("font/") || /\.(ttf|otf|woff2?)$/i.test(a.name)) {
    return "font";
  }
  if (/lottie|rive/.test(t) || /\.(riv|lottie)$/i.test(a.name)) {
    return "animation";
  }
  return "other";
}

/** The drag data type of an image dragged from the Assets tab: its asset id; dropped on a screen it becomes an `Image`. */
export const ASSET_DRAG_TYPE = "application/x-plux-asset";

/** The first URL of an asset's processed forms, for a raster image's thumbnail. */
function thumbOf(a: Asset): string | undefined {
  return a.variants.map((v) => v.artifact?.urls[0]).find((u) => u !== undefined && u !== "");
}

/**
 * The Assets tab: the app's files, upload with progress through the chunked RPCs (resumable after a
 * failed chunk) and delete with confirmation (S2.6, SRV-060). `client` is AssetService; `session`
 * is the editor session that holds the lock.
 */
/** One asset of the grid: open its details, drag it onto a screen, delete it; how many widgets use it (PC-36). */
function AssetCard(p: {
  readonly a: Asset;
  readonly uses: number | undefined;
  readonly onOpen: () => void;
  readonly onDelete: () => void;
}) {
  const m = assetsMessages(useLingui().i18n);
  const { a } = p;
  const k = assetKind(a);
  const thumb = k === "image" ? thumbOf(a) : undefined;
  return (
    <li
      className="asset-card"
      title={a.name}
      draggable={k === "image"}
      onDragStart={(e) => {
        e.dataTransfer.setData(ASSET_DRAG_TYPE, a.id);
        e.dataTransfer.effectAllowed = "copy";
      }}
    >
      <button type="button" className="asset-open" aria-label={m.open(a.name)} onClick={p.onOpen}>
        <span className="asset-thumb" data-kind={k} aria-hidden="true">
          {thumb === undefined ? <Icon name={kindIcons[k]} /> : <img src={thumb} alt="" loading="lazy" />}
        </span>
        <span className="asset-name font-mono" dir="ltr">
          {a.name}
        </span>
        <span className="asset-meta" dir="ltr">
          {a.mediaType.split("/")[1] ?? a.mediaType} · {kb(Number(a.size))}
        </span>
        {p.uses !== undefined && (
          <span className="asset-uses" data-unused={p.uses === 0 || undefined}>
            {p.uses === 0 ? m.unused : m.uses(p.uses)}
          </span>
        )}
      </button>
      <IconButton
        size="icon-xs"
        variant="secondary"
        className="asset-delete"
        aria-label={m.delete(a.name)}
        onClick={p.onDelete}
      >
        <Icon name="trash" size="pill" />
      </IconButton>
    </li>
  );
}

/** What the plugin's used assets weigh against `plugin.assetBytes`, the limit publish enforces (AST-003). */
function AssetBudget(p: { readonly assets: readonly Asset[]; readonly used: ReadonlySet<string> }) {
  const m = assetsMessages(useLingui().i18n);
  const budget = assetBudget(
    p.assets.map((a) => ({ id: a.id, size: Number(a.size) })),
    p.used,
  );
  const text = m.budgetText(kb(budget.total), kb(budget.limit));
  return (
    <div className="asset-budget" data-state={budget.state}>
      <span className="asset-budget-head">
        <span>{m.budget}</span>
        <span className="tabular-nums">{text}</span>
      </span>
      <Meter value={budget.total} max={budget.limit} label={m.budget} valueText={text} layout="bar" />
      {budget.state !== "within" && (
        <p className="asset-budget-note" role="status">
          {budget.state === "over" ? m.budgetOver : m.budgetNear}
        </p>
      )}
    </div>
  );
}

export function AssetsTab({
  client,
  appId,
  session,
  onChanged,
  uses,
  onSelectUses,
  onReplaceUses,
}: {
  readonly client: AssetClient;
  readonly appId: string;
  readonly session: string;
  /** Called after the list changed (an upload or a delete), for the canvas and the pickers to see it. */
  readonly onChanged?: () => void;
  /** The widgets of the plugin's pages that show each asset, by asset id (PC-36). */
  readonly uses?: ReadonlyMap<string, readonly AssetUse[]>;
  /** Selects widgets on the canvas. */
  readonly onSelectUses?: (nodes: readonly string[]) => void;
  /** Points the plugin's uses of one asset at another, after a replacement was uploaded. */
  readonly onReplaceUses?: (from: string, to: string) => void;
}): ReactElement {
  const m = assetsMessages(useLingui().i18n);
  const [assets, setAssets] = useState<readonly Asset[] | null>(null);
  const [loadFailed, setLoadFailed] = useState(false);
  const [pending, setPending] = useState<readonly Pending[]>([]);
  const [toDelete, setToDelete] = useState<Asset | null>(null);
  const [open, setOpen] = useState<Asset | null>(null);
  const [refused, setRefused] = useState<readonly string[]>([]);
  const [replacing, setReplacing] = useState(false);
  const [note, setNote] = useState<string | undefined>();
  const keys = useRef(0);

  const refresh = useCallback(async () => {
    try {
      setAssets((await client.listAssets({ appId })).assets);
      setLoadFailed(false);
      onChanged?.();
    } catch {
      setLoadFailed(true);
    }
  }, [client, appId, onChanged]);
  useEffect(() => {
    void refresh();
  }, [refresh]);

  const patch = (key: number, change: Partial<Pending>) =>
    setPending((list) => list.map((p) => (p.key === key ? { ...p, ...change } : p)));

  const run = async (p: Pending) => {
    patch(p.key, { failed: false, failure: null });
    try {
      await uploadAsset(client, { appId, name: p.name, session, file: p.file }, p.state, (received) =>
        patch(p.key, { received }),
      );
      setPending((list) => list.filter((x) => x.key !== p.key));
      await refresh();
    } catch (err) {
      const failure = uploadFailure(err);
      // These failures end the server's upload session: a resume must begin afresh.
      if (failure === "notFound" || failure === "hashMismatch" || failure === "sizeMismatch") {
        p.state.current = null;
      }
      patch(p.key, { failed: true, failure });
    }
  };

  const nameOf = (file: File) => `images/${file.name.toLowerCase().replaceAll(/[^a-z0-9._-]/g, "-")}`;
  // A file above asset.fileSize is refused here, before its bytes are sent (AST-003).
  const fits = (file: File) => file.size <= assetFileLimit();
  const pick = (files: readonly File[]) => {
    setRefused(files.filter((f) => !fits(f)).map((f) => m.tooBig(f.name, ...apart(f.size, assetFileLimit()))));
    for (const file of files.filter(fits)) {
      keys.current += 1;
      const p: Pending = {
        key: keys.current,
        file,
        name: nameOf(file),
        state: { current: null },
        received: 0,
        failed: false,
        failure: null,
      };
      setPending((list) => [...list, p]);
      void run(p);
    }
  };

  const remove = async (a: Asset) => {
    await client.deleteAsset({ id: a.id, session });
    setOpen((o) => (o?.id === a.id ? null : o));
    await refresh();
  };

  // Replace (PC-36): the new file is uploaded, then the plugin's uses of the old one point at it; the old file stays.
  const replace = async (old: Asset, file: File) => {
    if (!fits(file)) {
      setNote(m.tooBig(file.name, ...apart(file.size, assetFileLimit())));
      return;
    }
    setReplacing(true);
    try {
      const made = await uploadAsset(
        client,
        { appId, name: nameOf(file), session, file },
        { current: null },
        () => undefined,
      );
      if (made !== undefined) {
        onReplaceUses?.(old.id, made.id);
        setNote(m.replaced(made.name));
      }
      await refresh();
    } catch (err) {
      const failure = uploadFailure(err);
      setNote(failure === null ? m.failed(file.name) : m.failure(failure));
    } finally {
      setReplacing(false);
    }
  };
  const usesOf = (id: string) => uses?.get(id) ?? [];

  const [query, setQuery] = useState("");
  const [kind, setKind] = useState<AssetKind | "all">("all");
  const count = (k: AssetKind | "all") => (assets ?? []).filter((a) => k === "all" || assetKind(a) === k).length;
  const shown = (assets ?? []).filter(
    (a) => (kind === "all" || assetKind(a) === kind) && a.name.toLowerCase().includes(query.trim().toLowerCase()),
  );
  return (
    <section aria-label={m.tab} className="flex flex-col gap-3">
      <FileDrop label={m.upload} hint={m.dropHint} multiple onFiles={pick} />
      {refused.map((r) => (
        <p key={r} role="alert" className="asset-refused">
          <Icon name="warning" size="pill" />
          {r}
        </p>
      ))}
      {assets !== null && uses !== undefined && <AssetBudget assets={assets} used={new Set(uses.keys())} />}
      {open !== null && (
        <AssetDetails
          asset={open}
          thumb={assetKind(open) === "image" ? thumbOf(open) : undefined}
          size={kb(Number(open.size))}
          uses={usesOf(open.id).length}
          canReplace={onReplaceUses !== undefined}
          busy={replacing}
          note={note}
          onClose={() => {
            setOpen(null);
            setNote(undefined);
          }}
          onSelectUses={() => onSelectUses?.(usesOf(open.id).map((u) => u.node))}
          onReplace={(file) => void replace(open, file)}
        />
      )}
      {pending.length > 0 && (
        <ul className="flex flex-col gap-2">
          {pending.map((p) => (
            <li key={p.key} className="asset-upload">
              <span className="truncate font-mono text-xs" dir="ltr">
                {p.name}
              </span>
              {p.failed ? (
                <>
                  <span role="alert" className="flex items-center gap-1.5 text-xs text-destructive">
                    <Icon name="warning" size="pill" label={m.failed(p.name)} />
                    {m.failed(p.name)}
                  </span>
                  {p.failure !== null && <span className="text-xs text-muted-foreground">{m.failure(p.failure)}</span>}
                  <span className="flex gap-1.5">
                    <Button size="sm" variant="secondary" onClick={() => void run(p)}>
                      <Icon name="arrow-clockwise" size="pill" /> {m.resume}
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => setPending((list) => list.filter((x) => x.key !== p.key))}
                    >
                      {m.dismiss}
                    </Button>
                  </span>
                </>
              ) : (
                <Meter
                  value={p.received}
                  max={p.file.size}
                  label={m.uploading(p.name)}
                  valueText={m.progress(kb(p.received), kb(p.file.size))}
                />
              )}
            </li>
          ))}
        </ul>
      )}
      {assets !== null && assets.length > 0 && (
        <>
          <InputGroup>
            <InputGroupAddon>
              <Icon name="magnifying-glass" />
            </InputGroupAddon>
            <InputGroupInput
              aria-label={m.search}
              placeholder={m.search}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
          </InputGroup>
          {/* biome-ignore lint/a11y/useSemanticElements: a labelled group of filter toggles, not a form's fieldset */}
          <div role="group" aria-label={m.kinds} className="gallery-kinds">
            {(["all", ...assetKinds] as const)
              .filter((k) => k === "all" || count(k) > 0)
              .map((k) => (
                <button
                  key={k}
                  type="button"
                  className="gallery-kind"
                  aria-pressed={kind === k}
                  onClick={() => setKind(k)}
                >
                  {m.kind(k)}
                  <span className="tabular-nums opacity-70">{count(k)}</span>
                </button>
              ))}
          </div>
        </>
      )}
      {loadFailed && <p role="alert">{m.loadFailed}</p>}
      {assets === null && !loadFailed && <p className="dock-note">{m.loading}</p>}
      {assets?.length === 0 && pending.length === 0 && <p className="dock-note">{m.empty}</p>}
      {assets !== null && assets.length > 0 && shown.length === 0 && <p className="dock-note">{m.noMatch}</p>}
      {shown.length > 0 && (
        <ul aria-label={m.list} className="asset-grid">
          {shown.map((a) => (
            <AssetCard
              key={a.id}
              a={a}
              uses={uses === undefined ? undefined : usesOf(a.id).length}
              onOpen={() => {
                setOpen(a);
                setNote(undefined);
              }}
              onDelete={() => setToDelete(a)}
            />
          ))}
        </ul>
      )}
      <AlertDialog
        open={toDelete !== null}
        onOpenChange={(open) => {
          if (!open) {
            setToDelete(null);
          }
        }}
        title={m.deleteTitle}
        description={
          toDelete === null
            ? ""
            : usesOf(toDelete.id).length > 0
              ? m.deleteUsed(toDelete.name, usesOf(toDelete.id).length)
              : m.deleteBody(toDelete.name)
        }
        labels={{ cancel: m.cancel, confirm: m.confirm }}
        onConfirm={() => {
          if (toDelete !== null) {
            void remove(toDelete);
          }
        }}
      />
    </section>
  );
}

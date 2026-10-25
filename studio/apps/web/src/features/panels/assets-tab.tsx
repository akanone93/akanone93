// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Client } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import type { Asset, AssetService } from "@plux/api";
import { AlertDialog, Button, Icon, Meter } from "@plux/ui";
import { type ReactElement, useCallback, useEffect, useRef, useState } from "react";
import { assetsMessages } from "./assets.messages.ts";
import { type UploadState, uploadAsset } from "./upload-asset.ts";

type AssetClient = Pick<Client<typeof AssetService>, "listAssets" | "deleteAsset"> & Parameters<typeof uploadAsset>[0];

interface Pending {
  readonly key: number;
  readonly file: File;
  readonly name: string;
  readonly state: { current: UploadState | null };
  received: number;
  failed: boolean;
}

function kb(n: number): string {
  return `${Math.max(1, Math.round(n / 1024))} KB`;
}

/**
 * The Assets tab: the app's files, upload with progress through the chunked RPCs (resumable after a
 * failed chunk) and delete with confirmation (S2.6, SRV-060). `client` is AssetService; `session`
 * is the editor session that holds the lock.
 */
export function AssetsTab({
  client,
  appId,
  session,
}: {
  readonly client: AssetClient;
  readonly appId: string;
  readonly session: string;
}): ReactElement {
  const m = assetsMessages(useLingui().i18n);
  const [assets, setAssets] = useState<readonly Asset[] | null>(null);
  const [loadFailed, setLoadFailed] = useState(false);
  const [pending, setPending] = useState<readonly Pending[]>([]);
  const [toDelete, setToDelete] = useState<Asset | null>(null);
  const keys = useRef(0);

  const refresh = useCallback(async () => {
    try {
      setAssets((await client.listAssets({ appId })).assets);
      setLoadFailed(false);
    } catch {
      setLoadFailed(true);
    }
  }, [client, appId]);
  useEffect(() => {
    void refresh();
  }, [refresh]);

  const patch = (key: number, change: Partial<Pending>) =>
    setPending((list) => list.map((p) => (p.key === key ? { ...p, ...change } : p)));

  const run = async (p: Pending) => {
    patch(p.key, { failed: false });
    try {
      await uploadAsset(client, { appId, name: p.name, session, file: p.file }, p.state, (received) =>
        patch(p.key, { received }),
      );
      setPending((list) => list.filter((x) => x.key !== p.key));
      await refresh();
    } catch {
      patch(p.key, { failed: true });
    }
  };

  const pick = (files: FileList | null) => {
    for (const file of Array.from(files ?? [])) {
      keys.current += 1;
      const p: Pending = {
        key: keys.current,
        file,
        name: `images/${file.name.toLowerCase().replaceAll(/[^a-z0-9._-]/g, "-")}`,
        state: { current: null },
        received: 0,
        failed: false,
      };
      setPending((list) => [...list, p]);
      void run(p);
    }
  };

  const remove = async (a: Asset) => {
    await client.deleteAsset({ id: a.id, session });
    await refresh();
  };

  return (
    <section aria-label={m.tab} className="flex flex-col gap-3">
      <div>
        <label className="inline-flex">
          <span className="sr-only">{m.upload}</span>
          <input
            type="file"
            multiple
            aria-label={m.upload}
            className="text-body"
            onChange={(e) => {
              pick(e.currentTarget.files);
              e.currentTarget.value = "";
            }}
          />
        </label>
      </div>
      {pending.length > 0 && (
        <ul className="flex flex-col gap-2">
          {pending.map((p) => (
            <li key={p.key} className="flex items-center gap-2 text-body">
              <span className="font-mono text-data" dir="ltr">
                {p.name}
              </span>
              {p.failed ? (
                <>
                  <Icon name="warning" size="pill" label={m.failed(p.name)} />
                  <span role="alert">{m.failed(p.name)}</span>
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
      {loadFailed && <p role="alert">{m.loadFailed}</p>}
      {assets === null && !loadFailed && <p className="text-body text-secondary">{m.loading}</p>}
      {assets?.length === 0 && pending.length === 0 && <p className="text-body text-secondary">{m.empty}</p>}
      {assets !== null && assets.length > 0 && (
        <ul className="flex flex-col gap-1">
          {assets.map((a) => (
            <li key={a.id} className="flex items-center justify-between gap-2 text-body">
              <span className="font-mono text-data" dir="ltr">
                {a.name}
              </span>
              <span className="font-mono text-caption text-secondary" dir="ltr">
                {a.mediaType} {kb(Number(a.size))}
              </span>
              <Button size="sm" variant="ghost" aria-label={m.delete(a.name)} onClick={() => setToDelete(a)}>
                <Icon name="trash" size="pill" />
              </Button>
            </li>
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
        description={toDelete === null ? "" : m.deleteBody(toDelete.name)}
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

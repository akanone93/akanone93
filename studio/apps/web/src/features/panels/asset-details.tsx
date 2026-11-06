// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { Asset } from "@plux/api";
import { Button, Icon } from "@plux/ui";
import { type ReactElement, useRef } from "react";
import { assetsMessages } from "./assets.messages.ts";
import "./asset-details.css";

/**
 * An asset's details in the Assets tab (PC-36, AST-001): its preview, type, pixels, bytes, content hash, processing and
 * variants, the server's findings, and its uses in this plugin — select them on the canvas, or replace the file they
 * show.
 */
export function AssetDetails(p: {
  readonly asset: Asset;
  readonly thumb: string | undefined;
  readonly size: string;
  readonly uses: number;
  readonly canReplace: boolean;
  readonly busy: boolean;
  readonly note: string | undefined;
  readonly onClose: () => void;
  readonly onSelectUses: () => void;
  readonly onReplace: (file: File) => void;
}): ReactElement {
  const m = assetsMessages(useLingui().i18n);
  const input = useRef<HTMLInputElement>(null);
  const a = p.asset;
  const variants = a.variants
    .map((v) => `${v.kind}${v.density > 0 ? ` @${v.density}x` : ""}${v.width > 0 ? ` ${v.width}×${v.height}` : ""}`)
    .join(" · ");
  return (
    <section className="asset-details" aria-label={m.details(a.name)}>
      <header className="asset-details-head">
        <span className="truncate font-mono text-xs" dir="ltr">
          {a.name}
        </span>
        <button type="button" className="asset-details-close" aria-label={m.close} onClick={p.onClose}>
          <Icon name="x" size="pill" />
        </button>
      </header>
      <span className="asset-details-preview" aria-hidden="true">
        {p.thumb === undefined ? <Icon name="image" /> : <img src={p.thumb} alt="" />}
      </span>
      <dl className="asset-details-facts">
        <dt>{a.mediaType}</dt>
        <dd>{p.size}</dd>
        {a.width > 0 && (
          <>
            <dt>{m.pixels(a.width, a.height)}</dt>
            <dd>{m.processing(a.processing)}</dd>
          </>
        )}
        {a.sha256 !== "" && (
          <>
            <dt>{m.hash}</dt>
            <dd className="font-mono" title={a.sha256} dir="ltr">
              {a.sha256.slice(0, 12)}
            </dd>
          </>
        )}
        {variants !== "" && (
          <>
            <dt>{m.variants}</dt>
            <dd dir="ltr">{variants}</dd>
          </>
        )}
      </dl>
      {a.diagnostics.length > 0 && (
        <ul className="asset-details-findings">
          {a.diagnostics.map((d) => (
            <li key={`${d.code}:${d.message}`}>
              <Icon name="warning" size="pill" />
              {d.message}
            </li>
          ))}
        </ul>
      )}
      <div className="asset-details-actions">
        <span className="asset-uses" data-unused={p.uses === 0 || undefined}>
          {p.uses === 0 ? m.unused : m.uses(p.uses)}
        </span>
        {p.uses > 0 && (
          <Button size="sm" variant="secondary" onClick={p.onSelectUses}>
            <Icon name="crosshair" size="pill" />
            {m.selectUses}
          </Button>
        )}
        {p.canReplace && (
          <>
            <Button size="sm" variant="secondary" disabled={p.busy} onClick={() => input.current?.click()}>
              <Icon name="arrow-clockwise" size="pill" />
              {m.replace}
            </Button>
            <input
              ref={input}
              type="file"
              hidden
              aria-label={m.replace}
              onChange={(e) => {
                const file = e.target.files?.[0];
                if (file !== undefined) {
                  p.onReplace(file);
                }
                e.target.value = "";
              }}
            />
          </>
        )}
      </div>
      {p.canReplace && <p className="asset-details-hint">{p.note ?? m.replaceHint}</p>}
    </section>
  );
}

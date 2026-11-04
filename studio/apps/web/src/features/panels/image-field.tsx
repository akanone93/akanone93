// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Input } from "@plux/ui/components/input";
import { Popover, PopoverContent, PopoverTrigger } from "@plux/ui/components/popover";
import { type ReactElement, useState } from "react";
import { useAppImages } from "../plugin-canvas/screen-images.ts";
import { styleMessages } from "./style.messages.ts";
import "./image-field.css";

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/** An `ImageSource` value's asset id or web address. */
export function sourceOf(value: unknown): { readonly asset?: string; readonly url?: string } {
  if (!isObj(value)) {
    return {};
  }
  const asset = isObj(value["asset"]) ? value["asset"]["$asset"] : undefined;
  return typeof asset === "string" ? { asset } : typeof value["url"] === "string" ? { url: value["url"] } : {};
}

/** The `ImageSource` of an app asset, keeping the value's ThumbHash. */
export function assetSource(value: unknown, asset: string): Obj {
  const thumb = isObj(value) && typeof value["thumbHash"] === "string" ? { thumbHash: value["thumbHash"] } : {};
  return { asset: { $asset: asset }, ...thumb };
}

/**
 * An `ImageSource` property (AST-002): a field with the image's preview and name that opens a picker of the app's
 * uploaded images, or of a web address the device loads.
 */
export function ImageSourceField({
  name,
  value,
  readOnly = false,
  onChange,
  onEnd,
}: {
  readonly name: string;
  readonly value: unknown;
  readonly readOnly?: boolean;
  readonly onChange: (value: unknown) => void;
  readonly onEnd: () => void;
}): ReactElement {
  const m = styleMessages(useLingui().i18n);
  const images = useAppImages();
  const current = sourceOf(value);
  const [kind, setKind] = useState<"asset" | "url">(current.url === undefined ? "asset" : "url");
  const chosen = current.asset === undefined ? undefined : images?.find((i) => i.id === current.asset);
  const label = chosen?.name ?? current.asset ?? (current.url === undefined ? m.none : m.imageWeb);
  const pick = (next: unknown) => {
    onChange(next);
    onEnd();
  };
  return (
    <Popover>
      <PopoverTrigger
        disabled={readOnly}
        render={
          <button type="button" className="color-field" aria-label={`${m.pick(name)}: ${label}`}>
            {chosen === undefined ? null : <img className="image-thumb" src={chosen.url} alt="" />}
            <span className="min-w-0 flex-1 truncate text-start" dir="ltr">
              {chosen?.name.replace(/^images\//, "") ?? current.url ?? label}
            </span>
          </button>
        }
      />
      <PopoverContent align="start" className="color-popover icon-popover">
        <div className="icon-sets">
          {(["asset", "url"] as const).map((k) => (
            <button key={k} type="button" aria-pressed={k === kind} className="icon-set" onClick={() => setKind(k)}>
              {m.imageSource(k)}
            </button>
          ))}
        </div>
        {kind === "asset" ? (
          <ImageGrid images={images} current={current.asset} onPick={(id) => pick(assetSource(value, id))} />
        ) : (
          <div className="flex flex-col gap-1.5">
            <Input
              aria-label={m.imageSource("url")}
              placeholder="https://"
              className="h-8 font-mono"
              dir="ltr"
              defaultValue={current.url ?? ""}
              onBlur={(e) => {
                const url = e.target.value.trim();
                if (url !== "" && url !== current.url) {
                  pick({ url });
                }
              }}
            />
            <p className="color-popover-title">{m.imageUrlNote}</p>
          </div>
        )}
        <button type="button" className="color-clear" onClick={() => pick(undefined)}>
          {m.clear}
        </button>
      </PopoverContent>
    </Popover>
  );
}

function ImageGrid({
  images,
  current,
  onPick,
}: {
  readonly images: readonly { readonly id: string; readonly name: string; readonly url: string }[] | null;
  readonly current: string | undefined;
  readonly onPick: (id: string) => void;
}): ReactElement | null {
  const m = styleMessages(useLingui().i18n);
  if (images === null) {
    return null;
  }
  if (images.length === 0) {
    return <p className="color-popover-title">{m.imageEmpty}</p>;
  }
  return (
    <div className="image-grid">
      {images.map((i) => (
        <button
          key={i.id}
          type="button"
          className="image-chip"
          title={i.name}
          aria-label={i.name}
          aria-pressed={current === i.id}
          onClick={() => onPick(i.id)}
        >
          <img src={i.url} alt="" />
        </button>
      ))}
    </div>
  );
}

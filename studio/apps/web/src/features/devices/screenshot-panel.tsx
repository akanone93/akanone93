// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Field, Input, Select } from "@plux/ui";
import { useMutation } from "@tanstack/react-query";
import { type ReactNode, useEffect, useMemo, useState } from "react";
import type { DeviceLook } from "../preview/preview-bar.tsx";
import { compareMessages } from "./compare.messages.ts";
import { CompareView, type DrawnLook } from "./compare-view.tsx";
import { devicesMessages } from "./devices.messages.ts";
import { type ScreenshotStore, type ScreenshotUse, storeScreenshot } from "./screenshots.ts";

/** A captured screenshot as the device event carries it. */
export interface Capture {
  readonly png: Uint8Array<ArrayBuffer>;
  readonly platformViewsNotCaptured: boolean;
  /** What the device drew it under; absent from a runtime that does not say. */
  readonly look?: DeviceLook | undefined;
}

/** What a device drew a screenshot under, from the event: logical pixels, bars, mode and text scale (PC-38). */
export function lookOf(e: {
  readonly width: number;
  readonly height: number;
  readonly pixelRatio: number;
  readonly dark: boolean;
  readonly textScale: number;
  readonly paddingTop: number;
  readonly paddingBottom: number;
}): DeviceLook | undefined {
  if (e.width <= 0 || e.height <= 0 || e.pixelRatio <= 0 || e.textScale <= 0) {
    return undefined;
  }
  return {
    width: Math.round(e.width / e.pixelRatio),
    height: Math.round(e.height / e.pixelRatio),
    density: e.pixelRatio,
    top: Math.round(e.paddingTop),
    bottom: Math.round(e.paddingBottom),
    dark: e.dark,
    textScale: e.textScale,
  };
}

/** Hands the image to an img element as an object URL, released when it is replaced. */
function useObjectUrl(png: Uint8Array<ArrayBuffer>): string {
  const url = useMemo(() => URL.createObjectURL(new Blob([png], { type: "image/png" })), [png]);
  useEffect(() => () => URL.revokeObjectURL(url), [url]);
  return url;
}

/**
 * The latest screenshot of a paired device and what to keep it as (STU-134, WGT-031, DEV-033): a screen
 * thumbnail, a native slot preview or translation context, stored through the asset upload.
 */
export function ScreenshotPanel({
  capture,
  store,
  defaultTarget = "",
  canvas,
  canvasLook,
  onMatch,
}: {
  readonly capture: Capture;
  readonly store: ScreenshotStore;
  readonly defaultTarget?: string;
  /** The canvas's drawing of the screen the device shows, to compare with (PC-38); absent outside a canvas. */
  readonly canvas?: (() => Promise<Uint8Array | null>) | undefined;
  /** What the canvas draws under now, to say whether it matches the device. */
  readonly canvasLook?: DrawnLook | undefined;
  /** Sets the canvas to the device's conditions (its size, bars, mode and text scale). */
  readonly onMatch?: ((device: DeviceLook) => void) | undefined;
}): ReactNode {
  const m = devicesMessages(useLingui().i18n);
  const url = useObjectUrl(capture.png);
  const [use, setUse] = useState<ScreenshotUse>("thumbnail");
  const [target, setTarget] = useState(defaultTarget);
  const save = useMutation({ mutationFn: () => storeScreenshot(store, use, target, capture.png) });
  const cm = compareMessages(useLingui().i18n);
  const [comparing, setComparing] = useState(false);
  return (
    <section className="flex flex-wrap items-end gap-3">
      <img src={url} alt={m.screenshotAlt} className="max-h-40 rounded-md" />
      <Select
        label={m.screenshotUse}
        value={use}
        onValueChange={(v) => setUse(v as ScreenshotUse)}
        options={[
          { value: "thumbnail", label: m.useThumbnail },
          { value: "slotPreview", label: m.useSlotPreview },
          { value: "translationContext", label: m.useTranslationContext },
        ]}
      />
      <Field label={m.screenshotTarget}>
        <Input value={target} onChange={(e) => setTarget(e.target.value)} />
      </Field>
      <Button size="sm" disabled={target.trim() === ""} loading={save.isPending} onClick={() => save.mutate()}>
        {m.screenshotSave}
      </Button>
      {canvas !== undefined && (
        <Button
          variant={comparing ? "primary" : "secondary"}
          aria-pressed={comparing}
          onClick={() => setComparing(!comparing)}
        >
          {cm.open}
        </Button>
      )}
      {capture.platformViewsNotCaptured && <p className="text-caption text-muted-foreground">{m.platformViews}</p>}
      {comparing && canvas !== undefined && (
        <CompareView
          device={url}
          canvas={canvas}
          deviceLook={capture.look}
          canvasLook={canvasLook}
          onMatch={
            onMatch === undefined || capture.look === undefined ? undefined : () => onMatch(capture.look as DeviceLook)
          }
        />
      )}
      {save.isSuccess && (
        <p role="status" className="text-body text-muted-foreground">
          {m.screenshotSaved}
        </p>
      )}
      {save.isError && (
        <p role="alert" className="text-body">
          {m.failed}
        </p>
      )}
    </section>
  );
}

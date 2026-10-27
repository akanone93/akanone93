// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Field, Input, Select } from "@plux/ui";
import { useMutation } from "@tanstack/react-query";
import { type ReactNode, useEffect, useMemo, useState } from "react";
import { devicesMessages } from "./devices.messages.ts";
import { type ScreenshotStore, type ScreenshotUse, storeScreenshot } from "./screenshots.ts";

/** A captured screenshot as the device event carries it. */
export interface Capture {
  readonly png: Uint8Array<ArrayBuffer>;
  readonly platformViewsNotCaptured: boolean;
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
}: {
  readonly capture: Capture;
  readonly store: ScreenshotStore;
  readonly defaultTarget?: string;
}): ReactNode {
  const m = devicesMessages(useLingui().i18n);
  const url = useObjectUrl(capture.png);
  const [use, setUse] = useState<ScreenshotUse>("thumbnail");
  const [target, setTarget] = useState(defaultTarget);
  const save = useMutation({ mutationFn: () => storeScreenshot(store, use, target, capture.png) });
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
      {capture.platformViewsNotCaptured && <p className="text-caption text-secondary">{m.platformViews}</p>}
      {save.isSuccess && (
        <p role="status" className="text-body text-secondary">
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

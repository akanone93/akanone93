// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Dialog, Field, Input } from "@plux/ui";
import { type ReactElement, useState } from "react";
import {
  type CustomFrameRefusal,
  customFrame,
  customFrameLimits,
  customFrameRefusal,
  type DeviceFrame,
  encodeCustom,
} from "./frames.ts";
import { previewMessages } from "./preview.messages.ts";

/** D40: a custom device frame from width, height and density (STU-057). */
export function CustomFrameDialog({
  open,
  onClose,
  onUse,
}: {
  readonly open: boolean;
  readonly onClose: () => void;
  readonly onUse: (frame: DeviceFrame) => void;
}): ReactElement {
  const m = previewMessages(useLingui().i18n);
  const [width, setWidth] = useState("360");
  const [height, setHeight] = useState("800");
  const [density, setDensity] = useState("2");
  const [error, setError] = useState<CustomFrameRefusal | "invalid" | undefined>(undefined);
  const submit = () => {
    const input = { width: Number(width), height: Number(height), density: Number(density) };
    const made = customFrame(encodeCustom(input), input);
    if (made === "invalid") {
      setError(customFrameRefusal(input) ?? "invalid");
      return;
    }
    setError(undefined);
    onUse(made);
  };
  const { maxPixels, maxDensity } = customFrameLimits();
  const sizeError = error === "size" ? m.frameTooLarge(maxPixels) : error === "invalid" ? m.invalidFrame : undefined;
  const densityError = error === "density" ? m.densityOutOfRange(maxDensity) : undefined;
  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()} title={m.customFrame} labels={{ close: m.close }}>
      <div className="mt-4 flex flex-col gap-3">
        <Field label={m.width} {...(sizeError !== undefined && { error: sizeError })}>
          <Input inputMode="numeric" value={width} onChange={(e) => setWidth(e.target.value)} />
        </Field>
        <Field label={m.height}>
          <Input inputMode="numeric" value={height} onChange={(e) => setHeight(e.target.value)} />
        </Field>
        <Field label={m.density} {...(densityError !== undefined && { error: densityError })}>
          <Input inputMode="decimal" value={density} onChange={(e) => setDensity(e.target.value)} />
        </Field>
        <div className="flex justify-end">
          <Button onClick={submit}>{m.customFrameAdd}</Button>
        </div>
      </div>
    </Dialog>
  );
}

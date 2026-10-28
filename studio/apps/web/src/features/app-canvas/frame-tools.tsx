// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Button, Dialog, Field, Input } from "@plux/ui";
import { type ReactNode, useEffect, useState } from "react";
import type { appCanvasMessages } from "./app-canvas.messages.ts";
import type { Frame, FrameChange } from "./app-canvas.model.ts";

type Messages = ReturnType<typeof appCanvasMessages>;

/** D20: one button per frame that opens its rename and resize dialog; read-only viewers get none. */
export function FrameTools({
  frames,
  messages: m,
  onChange,
}: {
  readonly frames: readonly Frame[];
  readonly messages: Messages;
  readonly onChange: (id: string, change: FrameChange) => void;
}): ReactNode {
  const [editing, setEditing] = useState<Frame | undefined>();
  const [label, setLabel] = useState("");
  const [width, setWidth] = useState("");
  const [height, setHeight] = useState("");
  useEffect(() => {
    if (editing !== undefined) {
      setLabel(editing.label);
      setWidth(String(editing.width));
      setHeight(String(editing.height));
    }
  }, [editing]);
  return (
    <>
      {frames.map((f) => (
        <Button key={f.id} onClick={() => setEditing(f)}>
          {m.editFrame(f.label)}
        </Button>
      ))}
      {editing === undefined ? null : (
        <Dialog
          open
          onOpenChange={(o) => !o && setEditing(undefined)}
          title={m.editFrame(editing.label)}
          size="sm"
          labels={{ close: m.close }}
        >
          <Field label={m.frameLabel}>
            <Input value={label} onChange={(e) => setLabel(e.target.value)} />
          </Field>
          <Field label={m.frameWidth}>
            <Input type="number" value={width} onChange={(e) => setWidth(e.target.value)} />
          </Field>
          <Field label={m.frameHeight}>
            <Input type="number" value={height} onChange={(e) => setHeight(e.target.value)} />
          </Field>
          <div className="flex justify-end gap-2">
            <Button onClick={() => setEditing(undefined)}>{m.cancel}</Button>
            <Button
              onClick={() => {
                onChange(editing.id, { label, width: Number(width), height: Number(height) });
                setEditing(undefined);
              }}
            >
              {m.save}
            </Button>
          </div>
        </Dialog>
      )}
    </>
  );
}

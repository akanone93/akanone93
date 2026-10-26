// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Dialog, TypeTag } from "@plux/ui";
import { useState } from "react";
import { linkMessages } from "./links.messages.ts";
import { type ApplyMode, type Binding, bindingOf } from "./links.ts";

export interface TriggerPickerProps {
  readonly open: boolean;
  /** The widget node, to tell which events are already bound. */
  readonly node: unknown;
  /** The events the widget type declares (descriptor order). */
  readonly events: readonly string[];
  /** Title of the screen the link was dropped on. */
  readonly target: string;
  readonly onConfirm: (event: string, mode: ApplyMode) => void;
  readonly onCancel: () => void;
}

/** E7: which event of the widget starts the link; limited to the events its type declares (STU-047). */
export function TriggerPicker(props: TriggerPickerProps) {
  const m = linkMessages(useLingui().i18n);
  const [chosen, setChosen] = useState<string | undefined>(undefined);
  const [mode, setMode] = useState<ApplyMode>("replace");
  const event = chosen ?? props.events[0];
  const binding: Binding = event === undefined ? "free" : bindingOf(props.node, event);
  return (
    <Dialog
      open={props.open}
      onOpenChange={(open) => {
        if (!open) {
          props.onCancel();
        }
      }}
      title={m.pickerTitle}
      description={m.pickerDescription(props.target)}
      size="sm"
      labels={{ close: m.close }}
    >
      <fieldset className="flex flex-col gap-1">
        <legend className="sr-only">{m.pickerEvents}</legend>
        {props.events.map((name) => (
          <label key={name} className="flex items-center gap-2 text-body">
            <input type="radio" name="trigger" value={name} checked={event === name} onChange={() => setChosen(name)} />
            <TypeTag>{name}</TypeTag>
          </label>
        ))}
      </fieldset>
      {binding !== "free" && event !== undefined && (
        <fieldset className="flex flex-col gap-1">
          <legend className="text-body text-secondary">{m.bound(event)}</legend>
          {(["replace", "append"] as const).map((value) => (
            <label key={value} className="flex items-center gap-2 text-body">
              <input type="radio" name="mode" value={value} checked={mode === value} onChange={() => setMode(value)} />
              {value === "replace" ? m.replace : m.append}
            </label>
          ))}
        </fieldset>
      )}
      <div className="flex justify-end gap-2">
        <Button variant="ghost" onClick={props.onCancel}>
          {m.pickerCancel}
        </Button>
        <Button
          disabled={event === undefined}
          onClick={() => {
            if (event !== undefined) {
              props.onConfirm(event, binding === "free" ? "replace" : mode);
            }
          }}
        >
          {m.pickerConfirm}
        </Button>
      </div>
    </Dialog>
  );
}

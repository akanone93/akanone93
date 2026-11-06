// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Icon, Select } from "@plux/ui";
import { Input } from "@plux/ui/components/input";
import { type ReactElement, useState } from "react";
import type { RouteOption } from "../plugin-canvas/links.ts";
import {
  addStepRecipe,
  handlersOf,
  NEEDS,
  type QuickInput,
  type QuickKind,
  quickStep,
  ready,
  removeStepRecipe,
  snackbarOf,
} from "../plugin-canvas/quick-actions.ts";
import { type QuickActionMessages, quickActionMessages } from "./quick-actions.messages.ts";
import "./quick-actions-panel.css";

/**
 * Quick actions (PC-27): an event of the selected widget gets a common action in one step — open a dialog or a sheet,
 * show a message, open a link, go back, set state, submit a form — replacing what it runs or after it; and every step
 * its events run, each removable.
 */

const KINDS: readonly QuickKind[] = ["dialog", "sheet", "message", "link", "back", "state", "submit"];
const KIND_ICONS: Readonly<Record<QuickKind, Parameters<typeof Icon>[0]["name"]>> = {
  page: "arrow-right",
  dialog: "frame",
  sheet: "panel-bottom",
  message: "bell",
  link: "globe",
  back: "arrow-left",
  state: "variable",
  submit: "paper-plane-tilt",
};

function Fields(p: {
  readonly kind: QuickKind;
  readonly input: QuickInput;
  readonly routes: readonly RouteOption[];
  readonly forms: readonly string[];
  readonly m: QuickActionMessages;
  readonly set: (key: keyof QuickInput, value: string) => void;
}) {
  const pages = p.routes.filter((r) => r.kind === (p.kind === "dialog" ? "dialog" : "bottomSheet"));
  if ((p.kind === "dialog" || p.kind === "sheet") && pages.length === 0) {
    return <p className="lay-note">{p.m.noPages(p.kind)}</p>;
  }
  const keys = [...NEEDS[p.kind], ...(p.kind === "message" ? (["actionLabel"] as const) : [])];
  return (
    <>
      {keys.map((k) =>
        k === "route" || k === "form" ? (
          <Select
            key={k}
            label={p.m.field(k)}
            placeholder={p.m.field(k)}
            options={
              k === "route"
                ? pages.map((r) => ({ value: r.route, label: r.title }))
                : p.forms.map((f) => ({ value: f, label: f }))
            }
            value={String(p.input[k] ?? "")}
            onValueChange={(v) => p.set(k, v)}
          />
        ) : (
          <Input
            key={k}
            aria-label={p.m.field(k)}
            placeholder={p.m.field(k)}
            className="quick-input"
            value={String(p.input[k] ?? "")}
            onChange={(e) => p.set(k, e.target.value)}
          />
        ),
      )}
    </>
  );
}

export function QuickActions(p: {
  readonly nodeId: string;
  readonly node: unknown;
  readonly events: readonly string[];
  readonly routes: readonly RouteOption[];
  readonly forms?: readonly string[] | undefined;
  readonly canEdit: boolean;
  readonly apply: (recipe: (root: unknown) => void) => void;
  readonly newId: () => string;
}): ReactElement {
  const m = quickActionMessages(useLingui().i18n);
  const [event, setEvent] = useState<string | undefined>();
  const [kind, setKind] = useState<QuickKind>("message");
  const [input, setInput] = useState<QuickInput>({});
  const [mode, setMode] = useState<"replace" | "append">("append");
  const on = event !== undefined && p.events.includes(event) ? event : p.events[0];
  const steps = handlersOf(p.node);
  const bound = steps.some((s) => s.event === on);
  const write = () => {
    if (on !== undefined && ready(kind, input)) {
      const value = kind === "state" ? parseValue(String(input.value ?? "")) : input.value;
      p.apply(addStepRecipe(p.nodeId, on, quickStep(kind, { ...input, value }, p.newId()), bound ? mode : "replace"));
      setInput({});
    }
  };
  return (
    <div className="quick">
      <section className="flex flex-col gap-2" aria-label={m.title}>
        <h3 className="text-label font-medium">{m.title}</h3>
        <Select
          label={m.event}
          value={on ?? ""}
          options={p.events.map((e) => ({ value: e, label: e }))}
          disabled={!p.canEdit}
          onValueChange={setEvent}
        />
        <div className="quick-kinds" role="radiogroup" aria-label={m.title}>
          {KINDS.map((k) => (
            // biome-ignore lint/a11y/useSemanticElements: a tile of a segmented choice
            <button
              key={k}
              type="button"
              role="radio"
              aria-checked={kind === k}
              className="quick-kind"
              disabled={!p.canEdit}
              onClick={() => {
                setKind(k);
                setInput({});
              }}
            >
              <Icon name={KIND_ICONS[k]} size="pill" />
              <span>{m.kind(k)}</span>
            </button>
          ))}
        </div>
        <Fields
          kind={kind}
          input={input}
          routes={p.routes}
          forms={p.forms ?? []}
          m={m}
          set={(k, v) => setInput((prev) => ({ ...prev, [k]: v }))}
        />
        {bound && (
          <Select
            label={m.append}
            value={mode}
            options={[
              { value: "append", label: m.append },
              { value: "replace", label: m.replace },
            ]}
            onValueChange={(v) => setMode(v === "replace" ? "replace" : "append")}
          />
        )}
        <Button disabled={!p.canEdit || on === undefined || !ready(kind, input)} onClick={write}>
          {m.add}
        </Button>
        {snackbarOf(p.node) !== undefined && <p className="lay-note">{m.snackShown}</p>}
      </section>
      <section className="flex flex-col gap-2" aria-label={m.handlers}>
        <h3 className="text-label font-medium">{m.handlers}</h3>
        {steps.length === 0 ? (
          <p className="text-body text-muted-foreground">{m.none}</p>
        ) : (
          <ul className="quick-steps">
            {steps.map((s) => (
              <li key={`${s.event}:${s.index}`} className="quick-step">
                <span className="quick-step-event">{s.event}</span>
                <span className="quick-step-action">{s.action}</span>
                {s.detail !== undefined && <span className="quick-step-detail">{s.detail}</span>}
                {p.canEdit && (
                  <button
                    type="button"
                    className="form-icon"
                    aria-label={m.removeStep(s.action)}
                    onClick={() => p.apply(removeStepRecipe(p.nodeId, s.event, s.index))}
                  >
                    <Icon name="x" size="pill" />
                  </button>
                )}
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}

/** A state value typed as text: JSON when it parses (a number, true, a list), else the text. */
function parseValue(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

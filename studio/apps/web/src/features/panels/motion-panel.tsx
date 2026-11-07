// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Icon, Select } from "@plux/ui";
import { Input } from "@plux/ui/components/input";
import { Switch } from "@plux/ui/components/switch";
import { createContext, type ReactElement, useContext } from "react";
import { animationOf, CURVES, DEFAULT_MS, TRANSITION_KINDS } from "../plugin-canvas/motion.ts";
import { FoldCard } from "./fold.tsx";
import { type MotionMessages, motionMessages } from "./motion-panel.messages.ts";

/**
 * The motion panel of the inspector (PC-29): a widget's enter and exit transitions, each played on the canvas; whether
 * its values animate when they change, with duration, curve and delay; its hero tag and the widgets it flies to; and
 * what it does when the device asks to reduce motion.
 */

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);
const num = (v: unknown): number | undefined => (typeof v === "number" && Number.isFinite(v) ? v : undefined);

/** Plays a widget's transition on the canvas; null outside a canvas. */
export const MotionPreviewContext = createContext<((id: string, phase: "enter" | "exit") => void) | null>(null);

function Ms(p: {
  readonly label: string;
  readonly value: number | undefined;
  readonly ro: boolean;
  readonly set: (v: number | undefined) => void;
}) {
  return (
    <Input
      type="number"
      min={0}
      max={10000}
      step={50}
      aria-label={p.label}
      placeholder={p.label}
      className="motion-ms"
      value={p.value ?? ""}
      readOnly={p.ro}
      onChange={(e) =>
        p.set(e.target.value === "" ? undefined : Math.min(10000, Math.max(0, Math.round(Number(e.target.value)))))
      }
    />
  );
}

function CurveSelect(p: {
  readonly m: MotionMessages;
  readonly value: unknown;
  readonly ro: boolean;
  readonly set: (v: string) => void;
}) {
  return (
    <Select
      label={p.m.curveLabel}
      value={typeof p.value === "string" ? p.value : "fastOutSlowIn"}
      disabled={p.ro}
      options={CURVES.map((c) => ({ value: c, label: p.m.curve(c) }))}
      onValueChange={p.set}
    />
  );
}

/** One transition: its kind, duration and curve, and the button that plays it. */
function TransitionRow(p: {
  readonly id: string;
  readonly phase: "enter" | "exit";
  readonly value: unknown;
  readonly m: MotionMessages;
  readonly ro: boolean;
  readonly set: (v: unknown) => void;
}) {
  const play = useContext(MotionPreviewContext);
  const t = isObj(p.value) ? p.value : undefined;
  const put = (k: string, v: unknown) => p.set({ ...(t ?? { kind: "fade" }), [k]: v });
  return (
    <div className="motion-block">
      <div className="motion-head">
        <span className="lay-label">{p.phase === "enter" ? p.m.enter : p.m.exit}</span>
        {t !== undefined && play !== null && (
          <button type="button" className="motion-play" onClick={() => play(p.id, p.phase)}>
            <Icon name="play" size="pill" />
            {p.m.play(p.phase)}
          </button>
        )}
      </div>
      <Select
        label={p.phase === "enter" ? p.m.enter : p.m.exit}
        value={typeof t?.["kind"] === "string" ? t["kind"] : "none"}
        disabled={p.ro}
        options={["none", ...TRANSITION_KINDS].map((k) => ({ value: k, label: p.m.kind(k) }))}
        onValueChange={(k) => p.set(k === "none" ? undefined : { ...(t ?? {}), kind: k })}
      />
      {t !== undefined && (
        <div className="motion-pair">
          <Ms
            label={p.m.duration}
            value={num(t["durationMs"]) ?? DEFAULT_MS}
            ro={p.ro}
            set={(v) => put("durationMs", v)}
          />
          <CurveSelect m={p.m} value={t["curve"]} ro={p.ro} set={(c) => put("curve", c)} />
        </div>
      )}
    </div>
  );
}

export function MotionPanel(p: {
  readonly id: string;
  readonly node: Obj;
  readonly canEdit: boolean;
  /** How many widgets of other screens share its hero tag. */
  readonly heroCount: number;
  readonly set: (key: string, value: unknown) => void;
}): ReactElement {
  const m = motionMessages(useLingui().i18n);
  const a = animationOf(p.node);
  const ro = !p.canEdit;
  const implicit = a["durationMs"] !== undefined;
  const hero = typeof a["hero"] === "string" ? a["hero"] : "";
  return (
    <FoldCard id="motion" icon="sparkle" title={m.title} bodyClassName="motion-panel">
      <TransitionRow id={p.id} phase="enter" value={a["enter"]} m={m} ro={ro} set={(v) => p.set("enter", v)} />
      <TransitionRow id={p.id} phase="exit" value={a["exit"]} m={m} ro={ro} set={(v) => p.set("exit", v)} />
      <div className="motion-block">
        <div className="lay-switch">
          <span>{m.animate}</span>
          <Switch
            aria-label={m.animate}
            checked={implicit}
            disabled={ro}
            onCheckedChange={(on) => p.set("durationMs", on ? DEFAULT_MS : undefined)}
          />
        </div>
        {implicit && (
          <div className="motion-pair">
            <Ms
              label={m.duration}
              value={num(a["durationMs"])}
              ro={ro}
              set={(v) => p.set("durationMs", v ?? DEFAULT_MS)}
            />
            <Ms label={m.delay} value={num(a["delayMs"])} ro={ro} set={(v) => p.set("delayMs", v)} />
            <CurveSelect m={m} value={a["curve"]} ro={ro} set={(c) => p.set("curve", c)} />
          </div>
        )}
      </div>
      <div className="motion-block">
        <span className="lay-label">{m.hero}</span>
        <Input
          key={hero}
          aria-label={m.hero}
          className="motion-hero"
          defaultValue={hero}
          readOnly={ro}
          onBlur={(e) => e.target.value.trim() !== hero && p.set("hero", e.target.value.trim() || undefined)}
        />
        {hero !== "" && <p className="lay-note">{m.heroMatches(p.heroCount)}</p>}
      </div>
      {Object.keys(a).length > 0 && (
        <div className="motion-block">
          <span className="lay-label">{m.reduce}</span>
          <Select
            label={m.reduce}
            value={typeof a["reduceMotion"] === "string" ? a["reduceMotion"] : "default"}
            disabled={ro}
            options={["default", "skip", "shorten", "ignore"].map((k) => ({ value: k, label: m.reduceKind(k) }))}
            onValueChange={(v) => p.set("reduceMotion", v === "default" ? undefined : v)}
          />
        </div>
      )}
    </FoldCard>
  );
}

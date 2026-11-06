// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { widget } from "@plux/schema";
import { Button, Icon, Select } from "@plux/ui";
import { Input } from "@plux/ui/components/input";
import { Switch } from "@plux/ui/components/switch";
import { type ReactElement, useEffect, useRef, useState } from "react";
import { type TimelineMessages, timelineMessages } from "./timelines.messages.ts";
import { playedMs, type Timeline, trackValue } from "./timelines.ts";
import "./timeline-strip.css";

/**
 * The timeline strip over the canvas (PC-30, ANI-002, ANI-006): play, pause and scrub a page's timeline while the
 * canvas draws the page at the playhead; a row per track with its keyframes; a keyframe added at the playhead for a
 * property of the selected widget; the timeline's duration, repeats, reverse, autoplay and scroll driver.
 */

type Obj = Record<string, unknown>;

/** The props of a widget a track can animate: numbers, colours and offsets. */
const ANIMATABLE: ReadonlySet<string> = new Set(["double", "int", "color", "Offset"]);
export const animatableProps = (type: string | undefined): string[] =>
  (widget(type ?? "")?.props ?? []).filter((p) => ANIMATABLE.has(p.type.replace(/\?$/, ""))).map((p) => p.name);

const seconds = (ms: number) => `${(ms / 1000).toFixed(2)} s`;

export interface TimelineStripProps {
  readonly timeline: Timeline;
  readonly ms: number;
  readonly setMs: (ms: number) => void;
  readonly canEdit: boolean;
  readonly nameOf: (id: string) => string;
  /** The selected widget, for a new keyframe. */
  readonly selected: { readonly id: string; readonly type?: string; readonly props: Obj } | undefined;
  /** The page's scroll views, a timeline can be driven by. */
  readonly scrollViews: readonly { readonly id: string; readonly name: string }[];
  readonly set: (key: string, value: unknown) => void;
  readonly putKeyframe: (node: string, prop: string, value: unknown) => void;
  readonly removeKeyframe: (node: string | undefined, prop: string, atMs: number) => void;
  readonly onClose: () => void;
}

function usePlayer(timeline: Timeline, setMs: (ms: number) => void) {
  const [playing, setPlaying] = useState(false);
  const start = useRef(0);
  useEffect(() => {
    if (!playing) {
      return undefined;
    }
    let frame = 0;
    start.current = performance.now();
    const tick = () => {
      const at = playedMs(timeline, performance.now() - start.current);
      if (at === undefined) {
        setMs(timeline.reverse === true && (timeline.repeat ?? 0) % 2 === 1 ? 0 : timeline.durationMs);
        setPlaying(false);
        return;
      }
      setMs(at);
      frame = window.setTimeout(() => requestAnimationFrame(tick), 33);
    };
    requestAnimationFrame(tick);
    return () => window.clearTimeout(frame);
  }, [playing, timeline, setMs]);
  return { playing, setPlaying };
}

function Tracks(p: TimelineStripProps & { readonly m: TimelineMessages }) {
  const { timeline, m } = p;
  if (timeline.tracks.length === 0) {
    return <p className="tl-empty">{m.noTracks}</p>;
  }
  return (
    <div className="tl-tracks">
      {timeline.tracks.map((t) => (
        <div key={`${t.node ?? "route"}.${t.prop}`} className="tl-track">
          <span className="tl-track-name" title={`${t.node ?? ""}.${t.prop}`}>
            {t.node === undefined ? "route" : p.nameOf(t.node)}
            <span className="tl-track-prop">.{t.prop}</span>
          </span>
          <div className="tl-lane">
            <span className="tl-head" style={{ insetInlineStart: `${(p.ms / timeline.durationMs) * 100}%` }} />
            {t.keyframes.map((k) => (
              <button
                key={k.atMs}
                type="button"
                className="tl-key"
                data-at={Math.abs(k.atMs - p.ms) < 1 || undefined}
                style={{ insetInlineStart: `${(k.atMs / timeline.durationMs) * 100}%` }}
                title={`${seconds(k.atMs)} · ${JSON.stringify(k.value)} — ${m.keyframeHint}`}
                aria-label={`${t.prop} ${seconds(k.atMs)}`}
                onClick={(e) => (e.shiftKey && p.canEdit ? p.removeKeyframe(t.node, t.prop, k.atMs) : p.setMs(k.atMs))}
              />
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}

function AddKeyframe(p: TimelineStripProps & { readonly m: TimelineMessages }) {
  const props = animatableProps(p.selected?.type);
  const [prop, setProp] = useState<string | undefined>();
  const chosen = prop !== undefined && props.includes(prop) ? prop : props[0];
  const track = p.timeline.tracks.find((t) => t.node === p.selected?.id && t.prop === chosen);
  const now = track === undefined ? p.selected?.props[chosen ?? ""] : trackValue(track, p.ms);
  const [text, setText] = useState("");
  useEffect(() => setText(now === undefined ? "" : JSON.stringify(now)), [now]);
  if (p.selected === undefined || chosen === undefined || !p.canEdit) {
    return null;
  }
  const value = (() => {
    try {
      return JSON.parse(text) as unknown;
    } catch {
      return text;
    }
  })();
  return (
    <div className="tl-add">
      <Select
        label={p.m.prop}
        value={chosen}
        options={props.map((x) => ({ value: x, label: x }))}
        onValueChange={setProp}
      />
      <Input aria-label={p.m.value} className="tl-input" value={text} onChange={(e) => setText(e.target.value)} />
      <Button size="sm" onClick={() => p.selected !== undefined && p.putKeyframe(p.selected.id, chosen, value)}>
        {p.m.addKeyframe(seconds(Math.round(p.ms)))}
      </Button>
    </div>
  );
}

function Options(p: TimelineStripProps & { readonly m: TimelineMessages }) {
  const { timeline: t, m } = p;
  const ro = !p.canEdit;
  return (
    <div className="tl-options">
      <div className="tl-option">
        <span>{m.duration}</span>
        <Input
          type="number"
          min={1}
          aria-label={m.duration}
          className="tl-input"
          key={t.durationMs}
          defaultValue={t.durationMs}
          readOnly={ro}
          onBlur={(e) => p.set("durationMs", Math.max(1, Math.round(Number(e.target.value) || t.durationMs)))}
        />
      </div>
      <div className="tl-option">
        <span>{m.repeat}</span>
        <Input
          type="number"
          min={0}
          aria-label={m.repeat}
          className="tl-input"
          key={t.repeat ?? 0}
          defaultValue={t.repeat ?? 0}
          readOnly={ro}
          onBlur={(e) => p.set("repeat", Number(e.target.value) > 0 ? Math.round(Number(e.target.value)) : undefined)}
        />
      </div>
      {(["repeatForever", "reverse", "autoplay"] as const).map((k) => (
        <div key={k} className="tl-option">
          <span>{k === "repeatForever" ? m.forever : k === "reverse" ? m.reverse : m.autoplay}</span>
          <Switch checked={t[k] === true} disabled={ro} onCheckedChange={(on) => p.set(k, on)} />
        </div>
      ))}
      <div className="tl-option">
        <span>{m.drivenBy}</span>
        <Select
          label={m.drivenBy}
          value={t.driver?.node ?? ""}
          disabled={ro}
          options={[{ value: "", label: m.noDriver }, ...p.scrollViews.map((v) => ({ value: v.id, label: v.name }))]}
          onValueChange={(v) =>
            p.set("driver", v === "" ? undefined : { kind: "scroll", node: v, extent: t.driver?.extent ?? 400 })
          }
        />
      </div>
      {t.driver !== undefined && (
        <div className="tl-option">
          <span>{m.extent}</span>
          <Input
            type="number"
            min={1}
            aria-label={m.extent}
            className="tl-input"
            key={t.driver.extent}
            defaultValue={t.driver.extent}
            readOnly={ro}
            onBlur={(e) => p.set("driver", { ...t.driver, extent: Math.max(1, Number(e.target.value) || 400) })}
          />
        </div>
      )}
    </div>
  );
}

export function TimelineStrip(p: TimelineStripProps): ReactElement {
  const m = timelineMessages(useLingui().i18n);
  const { playing, setPlaying } = usePlayer(p.timeline, p.setMs);
  const [more, setMore] = useState(false);
  return (
    <section className="tl-strip" aria-label={m.strip(p.timeline.name)}>
      <div className="tl-bar">
        <Button
          size="sm"
          aria-label={playing ? m.pause : m.play}
          disabled={p.timeline.driver !== undefined}
          onClick={() => setPlaying(!playing)}
        >
          <Icon name={playing ? "pause" : "play"} />
        </Button>
        <strong className="tl-name">{p.timeline.name}</strong>
        <input
          type="range"
          aria-label={m.scrub}
          className="tl-scrub"
          min={0}
          max={p.timeline.durationMs}
          step={10}
          value={Math.round(p.ms)}
          onChange={(e) => {
            setPlaying(false);
            p.setMs(Number(e.target.value));
          }}
        />
        <span className="tl-time">
          {seconds(p.ms)} / {seconds(p.timeline.durationMs)}
        </span>
        <Button size="sm" variant="ghost" aria-label={m.duration} aria-pressed={more} onClick={() => setMore(!more)}>
          <Icon name="sliders" />
        </Button>
        <Button size="sm" variant="ghost" aria-label={m.close} onClick={p.onClose}>
          <Icon name="x" />
        </Button>
      </div>
      {p.timeline.driver !== undefined && <p className="tl-empty">{m.driven}</p>}
      {more && <Options {...p} m={m} />}
      <Tracks {...p} m={m} />
      <AddKeyframe {...p} m={m} />
    </section>
  );
}

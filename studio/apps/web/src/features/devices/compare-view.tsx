// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Icon } from "@plux/ui";
import { type ReactElement, useEffect, useState } from "react";
import { compareMessages } from "./compare.messages.ts";
import "./compare-view.css";

/**
 * The canvas and the device side by side (PC-38, DEV-033, STU-005): the device's screenshot next to the canvas's drawing
 * of the same screen, or one over the other with the canvas's opacity to slide, or their difference, where only what
 * does not match lights up — the round trip that shows whether the canvas draws what the device shows.
 */

export type CompareMode = "side" | "overlay" | "difference";

function useCanvasUrl(load: () => Promise<Uint8Array | null>): string | null | undefined {
  const [url, setUrl] = useState<string | null | undefined>();
  useEffect(() => {
    let made: string | undefined;
    let live = true;
    void load()
      .then((png) => {
        if (!live) {
          return;
        }
        made =
          png === null
            ? undefined
            : URL.createObjectURL(new Blob([png as Uint8Array<ArrayBuffer>], { type: "image/png" }));
        setUrl(made ?? null);
      })
      .catch(() => live && setUrl(null));
    return () => {
      live = false;
      if (made !== undefined) {
        URL.revokeObjectURL(made);
      }
    };
  }, [load]);
  return url;
}

/** What a screen is drawn under, as the comparison states it. */
export interface DrawnLook {
  readonly width: number;
  readonly height: number;
  readonly top: number;
  readonly bottom: number;
  readonly dark: boolean;
  readonly textScale: number;
}

/** Whether the canvas draws under the device's conditions: its size, bars, mode and text scale. */
export function sameLook(device: DrawnLook, canvas: DrawnLook): boolean {
  return (
    device.width === canvas.width &&
    device.height === canvas.height &&
    device.top === canvas.top &&
    device.bottom === canvas.bottom &&
    device.dark === canvas.dark &&
    Math.abs(device.textScale - canvas.textScale) < 0.01
  );
}

/** The two looks, and the switch that sets the canvas to the device's when they differ. */
function Looks(p: {
  readonly device: DrawnLook;
  readonly canvas: DrawnLook | undefined;
  readonly onMatch: (() => void) | undefined;
}): ReactElement {
  const m = compareMessages(useLingui().i18n);
  const said = (l: DrawnLook) => m.look(l.width, l.height, l.dark, Math.round(l.textScale * 100) / 100);
  const differs = p.canvas !== undefined && !sameLook(p.device, p.canvas);
  return (
    <div className="compare-looks">
      <dl>
        <div>
          <dt>{m.deviceLook}</dt>
          <dd>{said(p.device)}</dd>
        </div>
        {p.canvas !== undefined && (
          <div>
            <dt>{m.canvasLook}</dt>
            <dd>{said(p.canvas)}</dd>
          </div>
        )}
      </dl>
      {differs && (
        <p className="compare-note" role="status">
          <Icon name="info" size="pill" />
          {m.differs}
        </p>
      )}
      {differs && p.onMatch !== undefined && (
        <Button size="sm" onClick={p.onMatch}>
          {m.match}
        </Button>
      )}
    </div>
  );
}

export function CompareView(p: {
  /** The device's screenshot, as an object URL. */
  readonly device: string;
  /** The canvas's drawing of the screen the device shows, as PNG bytes; null when it has not drawn it. */
  readonly canvas: () => Promise<Uint8Array | null>;
  /** What the device drew its screenshot under; absent from a runtime that does not say. */
  readonly deviceLook?: DrawnLook | undefined;
  /** What the canvas draws under now. */
  readonly canvasLook?: DrawnLook | undefined;
  /** Sets the canvas to the device's conditions. */
  readonly onMatch?: (() => void) | undefined;
}): ReactElement {
  const m = compareMessages(useLingui().i18n);
  const canvas = useCanvasUrl(p.canvas);
  const [mode, setMode] = useState<CompareMode>("side");
  const [opacity, setOpacity] = useState(50);
  const modes: readonly { readonly id: CompareMode; readonly label: string }[] = [
    { id: "side", label: m.side },
    { id: "overlay", label: m.overlay },
    { id: "difference", label: m.difference },
  ];
  return (
    <section className="compare" aria-label={m.title}>
      {p.deviceLook !== undefined && <Looks device={p.deviceLook} canvas={p.canvasLook} onMatch={p.onMatch} />}
      {/* biome-ignore lint/a11y/useSemanticElements: a labelled radio group of comparison modes, not a form's fieldset */}
      <div role="radiogroup" aria-label={m.mode} className="compare-modes">
        {modes.map((x) => (
          // biome-ignore lint/a11y/useSemanticElements: a chip of a segmented choice
          <button
            key={x.id}
            type="button"
            role="radio"
            aria-checked={mode === x.id}
            className="compare-mode"
            onClick={() => setMode(x.id)}
          >
            {x.label}
          </button>
        ))}
      </div>
      {canvas === null && (
        <p className="compare-note">
          <Icon name="info" size="pill" />
          {m.noCanvas}
        </p>
      )}
      {canvas !== null && canvas !== undefined && (
        <div className="compare-stage" data-mode={mode}>
          <figure className="compare-shot">
            <img src={p.device} alt={m.deviceAlt} />
            {mode === "side" && <figcaption>{m.device}</figcaption>}
          </figure>
          <figure
            className="compare-shot"
            data-canvas=""
            style={mode === "overlay" ? { opacity: opacity / 100 } : undefined}
          >
            <img src={canvas} alt={m.canvasAlt} />
            {mode === "side" && <figcaption>{m.canvas}</figcaption>}
          </figure>
        </div>
      )}
      {mode === "overlay" && canvas !== null && canvas !== undefined && (
        <label className="compare-opacity">
          {m.opacity}
          <input type="range" min={0} max={100} value={opacity} onChange={(e) => setOpacity(Number(e.target.value))} />
        </label>
      )}
      {mode === "difference" && <p className="compare-note">{m.differenceHint}</p>}
    </section>
  );
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Icon } from "@plux/ui";
import {
  animate,
  domAnimation,
  LazyMotion,
  type MotionStyle,
  type MotionValue,
  m,
  useMotionValue,
  useReducedMotion,
  useSpring,
  useTransform,
} from "motion/react";
import { type CSSProperties, useEffect } from "react";
import { type AppEntry, healthOf, monogramOf, toneOf } from "../apps/apps-model.ts";
import { homeMessages } from "./home.messages.ts";
import { productionKey } from "./org-pulse.ts";

/** How high each layer stands above the floor at full height, in px, from the bottom layer up. */
const HEIGHTS = [0, 64, 128, 192] as const;
/** The pointer parallax at the edge of the window, in degrees. */
const PARALLAX = 6;

/** One layer of the stack: an app as a tile lying in the plane, lifted to its height as the stack builds. */
function Layer({
  entry,
  index,
  top,
  build,
  locale,
}: {
  entry: AppEntry;
  index: number;
  top: boolean;
  build: MotionValue<number>;
  locale: string;
}) {
  const copy = homeMessages(useLingui().i18n);
  const height = HEIGHTS[Math.min(index, HEIGHTS.length - 1)] ?? 0;
  // Each layer starts rising a little after the one below it, so the stack builds from the floor up.
  const z = useTransform(build, (b) => Math.min(1, Math.max(0, b * HEIGHTS.length - index * 0.6)) * height);
  const { app, summary } = entry;
  const devices = new Intl.NumberFormat(locale, { notation: "compact" }).format(summary?.activeDevices ?? 0n);
  const live = summary?.releases.find((r) => r.environmentKey === productionKey && r.liveSequence > 0n);
  const health = summary === undefined ? undefined : healthOf(summary);
  const rate =
    health === undefined || health.kind === "none"
      ? undefined
      : new Intl.NumberFormat(locale, { style: "percent", maximumFractionDigits: 1 }).format(health.rate);
  return (
    <m.div className="hs-layer" data-tone={toneOf(app)} data-top={top} style={{ z, "--i": index } as MotionStyle}>
      <div className="hs-tile">
        <span className="hs-icon">{monogramOf(app)}</span>
        <span className="hs-title">
          <span className="hs-name">{app.name}</span>
          <span className="hs-key">{app.key}</span>
        </span>
        {top && (
          <span className="hs-bars">
            {[0.42, 0.58, 0.5, 0.74, 0.66, 0.88, 1].map((h, k) => (
              // biome-ignore lint/suspicious/noArrayIndexKey: a fixed decorative series
              <i key={k} style={{ "--h": h } as CSSProperties} />
            ))}
          </span>
        )}
        <span className="hs-devices">
          <Icon name="device-mobile" size="pill" />
          {copy.stackDevices(devices)}
        </span>
      </div>
      {top && live !== undefined && (
        <span className="hs-chip hs-chip-live">
          <i className="hs-dot" />
          {copy.stackLive(Number(live.liveSequence))}
        </span>
      )}
      {top && rate !== undefined && (
        <span className="hs-chip hs-chip-health">
          <Icon name="check-circle" size="pill" />
          {copy.stackCrashFree(rate)}
        </span>
      )}
    </m.div>
  );
}

/** A dashed outline where the first app will stand, while the organisation has none. */
function Ghost({ index }: { index: number }) {
  return <div className="hs-layer hs-ghost" style={{ "--i": index } as CSSProperties} />;
}

/**
 * The home's picture (U16.3): the organisation's busiest apps stacked as layers in the shape of Plux's mark, the
 * busiest on top. The stack builds from the floor when the page opens and follows the pointer a little. Decorative:
 * the picture has one accessible name and the same facts stand in the pulse and on the cards.
 */
export function AppStack({ entries }: { entries: readonly AppEntry[] }) {
  const { i18n } = useLingui();
  const copy = homeMessages(i18n);
  const reduced = useReducedMotion() === true;
  const build = useMotionValue(reduced ? 1 : 0);
  // The busiest app is the top layer: the list arrives busiest first and is drawn from the floor up.
  const layers = [...entries].reverse();

  useEffect(() => {
    if (reduced) {
      build.set(1);
      return;
    }
    const controls = animate(build, 1, { duration: 1.8, ease: [0.16, 1, 0.3, 1], delay: 0.3 });
    return () => controls.stop();
  }, [reduced, build]);

  const pointerX = useMotionValue(0);
  const pointerY = useMotionValue(0);
  const smoothX = useSpring(pointerX, { stiffness: 50, damping: 20 });
  const smoothY = useSpring(pointerY, { stiffness: 50, damping: 20 });
  const rotateY = useTransform(smoothX, (v) => v * PARALLAX);
  const rotateX = useTransform(smoothY, (v) => -v * PARALLAX * 0.6);

  useEffect(() => {
    if (reduced || !window.matchMedia("(pointer: fine)").matches) {
      return;
    }
    const move = (event: PointerEvent) => {
      pointerX.set((event.clientX / window.innerWidth) * 2 - 1);
      pointerY.set((event.clientY / window.innerHeight) * 2 - 1);
    };
    window.addEventListener("pointermove", move, { passive: true });
    return () => window.removeEventListener("pointermove", move);
  }, [reduced, pointerX, pointerY]);

  return (
    <LazyMotion features={domAnimation} strict>
      <div className="hs-scene" role="img" aria-label={entries.length === 0 ? copy.stackEmptyLabel : copy.stackLabel}>
        <div aria-hidden="true" className="hs-glow" />
        <m.div className="hs-view" style={{ rotateX, rotateY }}>
          <div className="hs-plane">
            <div aria-hidden="true" className="hs-floor" />
            {layers.length === 0
              ? [0, 1, 2].map((i) => <Ghost key={i} index={i} />)
              : layers.map((entry, i) => (
                  <Layer
                    key={entry.app.id}
                    entry={entry}
                    index={i}
                    top={i === layers.length - 1}
                    build={build}
                    locale={i18n.locale}
                  />
                ))}
          </div>
        </m.div>
      </div>
    </LazyMotion>
  );
}

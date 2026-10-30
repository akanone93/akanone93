// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import {
  animate,
  domAnimation,
  LazyMotion,
  type MotionValue,
  m,
  useMotionValue,
  useMotionValueEvent,
  useReducedMotion,
  useSpring,
  useTransform,
} from "motion/react";
import { type CSSProperties, type ReactNode, useEffect, useState } from "react";
import { authMessages } from "./auth.messages.ts";
import { type SampleScreen, sampleScreens } from "./sample-screens.tsx";
import "./screen-ring.css";

/** The ring's radius in px: the same value as `--ring-radius` in screen-ring.css. */
const RADIUS = 360;
/**
 * The front screen never stops: it arrives `DRIFT` degrees right of facing the viewer, glides
 * slowly through facing to `DRIFT` degrees left over `DRIFT_S` seconds, then the ring turns on to
 * the next screen in `TURN_S` seconds, always in the same direction. Both curves are symmetric, so
 * the way in mirrors the way out.
 */
const DRIFT = 16;
const DRIFT_S = 7;
const TURN_S = 3;
/**
 * Everything a screen shows follows its angle from facing, the same way on both sides: it is
 * fully seen within `DRIFT` degrees and fades out over the next `FADE` degrees, while the ring
 * moves; its layers stand fully lifted within `LIFT_FULL` degrees and lie flat at `DRIFT + FADE`.
 */
const FADE = 13;
const LIFT_FULL = 5;
/**
 * How high each layer of the front screen stands above it, in px, from the lowest layer up. All
 * layers lift by the same factor at every frame, so a higher layer moves faster and the ratio of
 * their heights holds: they stay stacked on the screen.
 */
const HEIGHTS = [34, 92, 156, 230] as const;
/** The glow behind the screen at full visibility. */
const GLOW = 0.32;
/** Pointer parallax in degrees at the edge of the window, and the view's slight look from above. */
const PARALLAX = 5;
const LOOK_DOWN = 4;

function clamp01(v: number): number {
  return Math.min(1, Math.max(0, v));
}

/** An angle in degrees folded into -180..180. */
function fold(degrees: number): number {
  return ((((degrees + 180) % 360) + 360) % 360) - 180;
}

function smooth(t: number): number {
  return t * t * (3 - 2 * t);
}

/**
 * One cycle of the ring, from one screen's arrival to the next one's, as a single curve: steady
 * through the glide (the first `glide` of the time covers `share` of the angle), then the speed
 * rises and falls back as a smooth bump through the turn. The speed is the same at both ends of
 * the turn and of the cycle, so glide, turn and the next glide join without a hitch.
 */
function cycleCurve(glide: number, share: number): (t: number) => number {
  const steady = share / glide;
  const bump = (2 * (1 - steady)) / (1 - glide);
  const span = 1 - glide;
  return (t) => {
    if (t <= glide) {
      return steady * t;
    }
    const u = t - glide;
    return steady * t + bump * (u / 2 - (span / (4 * Math.PI)) * Math.sin((2 * Math.PI * u) / span));
  };
}

/** One layer of a screen, lifted by the screen's lift factor to its own height. */
function Lifted({
  lift,
  opacity,
  height,
  children,
}: {
  lift: MotionValue<number>;
  opacity: MotionValue<number>;
  height: number;
  children: ReactNode;
}) {
  const z = useTransform(lift, (f) => f * height);
  return (
    <m.div className="sr-layer" style={{ z, opacity }}>
      {children}
    </m.div>
  );
}

/**
 * One screen on the ring. Its opacity and the lift of its layers are functions of its angle from
 * facing, not timers, so they move with the ring and mirror on the way in and out. Opacity is set
 * on each plane, not on the slot: opacity on a 3D container would flatten its layers. The screen's
 * own live moment (CSS) plays while it is the front one.
 */
function Slot({
  screen,
  index,
  step,
  angle,
  front,
}: {
  screen: SampleScreen;
  index: number;
  step: number;
  angle: MotionValue<number>;
  front: boolean;
}) {
  const style = { "--slot": index, "--slots": Math.round(360 / step) } as CSSProperties;
  const away = useTransform(angle, (a) => Math.abs(fold(a + index * step)));
  const opacity = useTransform(away, (d) => clamp01(1 - (d - DRIFT) / FADE));
  const lift = useTransform(away, (d) => smooth(clamp01((DRIFT + FADE - d) / (DRIFT + FADE - LIFT_FULL))));
  const glow = useTransform(opacity, (o) => o * GLOW);
  const height = (k: number) => HEIGHTS[Math.min(k, HEIGHTS.length - 1)] ?? 0;
  return (
    <div className="sr-slot" style={style} data-app={screen.app} data-front={front}>
      <m.div className="sr-glow" style={{ opacity: glow }} />
      <m.div className="sr-edge" data-edge="2" style={{ opacity }} />
      <m.div className="sr-edge" data-edge="1" style={{ opacity }} />
      <m.div className="sr-screen" style={{ opacity }}>
        {screen.base}
      </m.div>
      {screen.layers.map((layer, k) => (
        <Lifted
          key={layer !== null && typeof layer === "object" && "key" in layer ? String(layer.key) : k}
          lift={lift}
          opacity={opacity}
          height={height(k)}
        >
          {layer}
        </Lifted>
      ))}
    </div>
  );
}

/**
 * The sign-in picture (U16.2): sample apps built with Plux stand on a ring above a quiet technical
 * floor and turn on its vertical axis, one screen at a time. The front screen lifts its layers and
 * plays its moment, and the caption under it names the Plux feature it shows. Decorative: the
 * picture has one accessible name; the caption is plain text.
 */
export function ScreenRing() {
  const { i18n } = useLingui();
  const copy = authMessages(i18n);
  const reduced = useReducedMotion() === true;
  const list = sampleScreens(copy, i18n.locale);
  const count = list.length;
  const step = 360 / count;
  /** The ring's turn in degrees: screen k faces the viewer at -k * step. */
  const angle = useMotionValue(reduced ? 0 : step - DRIFT);
  const [front, setFront] = useState(0);

  useMotionValueEvent(angle, "change", (a) => {
    const nearest = (((Math.round(-a / step) % count) + count) % count) as number;
    setFront((current) => (current === nearest ? current : nearest));
  });

  useEffect(() => {
    if (reduced) {
      angle.set(0);
      return;
    }
    let alive = true;
    let running: { stop: () => void } | undefined;
    const run = async (target: number, duration: number, ease: (t: number) => number) => {
      const controls = animate(angle, target, { duration, ease });
      running = controls;
      await controls;
    };
    const cycle = DRIFT_S + TURN_S;
    const glide = DRIFT_S / cycle;
    const curve = cycleCurve(glide, (2 * DRIFT) / step);
    // The first arrival is the end of a cycle: the turn part of the curve, rescaled to 0..1.
    const atTurn = curve(glide);
    const arrival = (t: number) => (curve(glide + t * (1 - glide)) - atTurn) / (1 - atTurn);
    void (async () => {
      angle.set(step - DRIFT);
      await run(DRIFT, TURN_S, arrival);
      for (let k = 0; alive; k++) {
        const started = performance.now();
        await run(-(k + 1) * step + DRIFT, cycle, curve);
        // A cycle that ends at once means animations are switched off (Motion's skipAnimations):
        // stop rather than turn the ring in a tight loop.
        if (performance.now() - started < cycle * 500) {
          return;
        }
      }
    })();
    return () => {
      alive = false;
      running?.stop();
    };
  }, [reduced, angle, step]);

  const pointerX = useMotionValue(0);
  const pointerY = useMotionValue(0);
  const smoothX = useSpring(pointerX, { stiffness: 50, damping: 20 });
  const smoothY = useSpring(pointerY, { stiffness: 50, damping: 20 });
  const tiltY = useTransform(smoothX, (v) => v * PARALLAX);
  const tiltX = useTransform(smoothY, (v) => LOOK_DOWN - v * PARALLAX * 0.6);

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
      <div className="sr-scene">
        <div aria-hidden="true" className="sr-grid" />
        <div role="img" aria-label={copy.sceneLabel} className="sr-stage">
          <m.div className="sr-view" style={{ rotateX: tiltX, rotateY: tiltY }}>
            <m.div className="sr-ring" style={{ z: -RADIUS, rotateY: angle }}>
              {list.map((screen, i) => (
                <Slot key={screen.key} screen={screen} index={i} step={step} angle={angle} front={i === front} />
              ))}
            </m.div>
          </m.div>
        </div>
        <div className="sr-stories">
          {list.map((screen, i) => (
            <div
              key={screen.key}
              className="sr-story"
              data-app={screen.app}
              data-front={i === front}
              aria-hidden={i !== front}
            >
              <span className="sr-tag">{screen.story.tag}</span>
              <span className="sr-story-title">{screen.story.title}</span>
              <span className="sr-story-body">{screen.story.body}</span>
            </div>
          ))}
        </div>
      </div>
    </LazyMotion>
  );
}

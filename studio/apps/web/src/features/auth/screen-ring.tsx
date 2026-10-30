// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Icon, type IconName } from "@plux/ui";
import {
  animate,
  domAnimation,
  LazyMotion,
  type MotionValue,
  m,
  useMotionValue,
  useReducedMotion,
  useSpring,
  useTransform,
} from "motion/react";
import { type CSSProperties, type ReactNode, useEffect } from "react";
import { authMessages } from "./auth.messages.ts";
import "./screen-ring.css";

type Messages = ReturnType<typeof authMessages>;

/** One sample screen: its layers from the screen surface up, and the feature chip beside it. */
interface SampleScreen {
  readonly key: string;
  readonly layers: readonly ReactNode[];
  readonly chip: { readonly icon: IconName; readonly text: string };
}

/** The ring's radius in px: the same value as `--ring-radius` in screen-ring.css. */
const RADIUS = 360;
/**
 * The front screen never stops: it arrives `DRIFT` degrees right of facing the viewer, glides
 * slowly through facing to `DRIFT` degrees left over `DRIFT_S` seconds, then the ring turns on to
 * the next screen in `TURN_S` seconds, always in the same direction. Both curves are symmetric, so
 * the way in mirrors the way out.
 */
const DRIFT = 12;
const DRIFT_S = 4.6;
const TURN_S = 1.3;
const driftEase = [0.37, 0, 0.63, 1] as const;
const turnEase = [0.65, 0, 0.35, 1] as const;
/**
 * Everything a screen shows follows its angle from facing, the same way on both sides: it is
 * fully seen within `DRIFT` degrees and fades out over the next `FADE` degrees, while the ring
 * moves; its layers stand fully lifted within `LIFT_FULL` degrees and lie flat at `DRIFT + FADE`.
 */
const FADE = 8;
const LIFT_FULL = 4;
/**
 * How high each layer of the front screen stands above it, in px, from the lowest layer up; the
 * chip floats highest. All layers lift by the same factor at every frame, so a higher layer moves
 * faster and the ratio of their heights holds: they stay stacked on the screen.
 */
const HEIGHTS = [30, 84, 146, 220] as const;
const CHIP_HEIGHT = 290;
/** The glow behind the screen at full visibility. */
const GLOW = 0.28;
/** Pointer parallax in degrees at the edge of the window, and the view's slight look from above. */
const PARALLAX = 6;
const LOOK_DOWN = 14;

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

function money(locale: string, value: number, digits = 2): string {
  return new Intl.NumberFormat(locale, {
    style: "currency",
    currency: "EUR",
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  }).format(value);
}

function screens(m$: Messages, locale: string): SampleScreen[] {
  const percent = new Intl.NumberFormat(locale, { style: "percent" }).format(0.25);
  return [
    {
      key: "loan",
      layers: [
        <div key="h" className="sr-header">
          <span className="sr-title">{m$.loanTitle}</span>
          <span className="sr-caption">{m$.loanBorrow}</span>
          <span className="sr-figure">{money(locale, 12000, 0)}</span>
        </div>,
        <div key="c" className="sr-card sr-at-middle">
          <div className="sr-track">
            <div className="sr-track-fill" />
            <div className="sr-thumb" />
          </div>
          <div className="sr-row">
            <span className="sr-caption">{m$.loanMonthly}</span>
            <span className="sr-amount">{money(locale, 342.18)}</span>
          </div>
        </div>,
        <div key="a" className="sr-action">
          {m$.loanApply}
        </div>,
      ],
      chip: { icon: "check-circle", text: m$.chipSigned("1.4") },
    },
    {
      key: "shop",
      layers: [
        <div key="h" className="sr-header">
          <span className="sr-title">{m$.shopTitle}</span>
        </div>,
        <div key="p" className="sr-product">
          <div className="sr-product-image" />
          <div className="sr-row">
            <span className="sr-label">{m$.shopProduct}</span>
            <span className="sr-amount">{money(locale, 59, 0)}</span>
          </div>
          <div className="sr-swatches">
            <span data-swatch="1" />
            <span data-swatch="2" />
            <span data-swatch="3" />
          </div>
        </div>,
        <div key="a" className="sr-action">
          {m$.shopAdd}
        </div>,
      ],
      chip: { icon: "lightning", text: m$.chipInstant },
    },
    {
      key: "bank",
      layers: [
        <div key="h" className="sr-header">
          <span className="sr-caption">{m$.bankBalance}</span>
          <span className="sr-figure">{money(locale, 8240.5)}</span>
        </div>,
        <div key="k" className="sr-bank-card" />,
        <div key="b" className="sr-pair">
          <span>{m$.bankSend}</span>
          <span>{m$.bankRequest}</span>
        </div>,
        <div key="l" className="sr-card sr-at-bottom">
          <span className="sr-caption">{m$.bankToday}</span>
          <div className="sr-row">
            <span className="sr-label">{m$.bankCoffee}</span>
            <span className="sr-amount">{money(locale, -4.2)}</span>
          </div>
        </div>,
      ],
      chip: { icon: "rocket-launch", text: m$.chipRollout(percent) },
    },
    {
      key: "onboard",
      layers: [
        <div key="o" className="sr-orb" />,
        <div key="t" className="sr-welcome">
          <span className="sr-title sr-title-large">{m$.onboardTitle}</span>
          <span className="sr-caption">{m$.onboardBody}</span>
          <div className="sr-dots">
            <span data-active="true" />
            <span />
            <span />
          </div>
        </div>,
        <div key="a" className="sr-action">
          {m$.onboardStart}
        </div>,
      ],
      chip: { icon: "flask", text: m$.chipExperiment },
    },
    {
      key: "delivery",
      layers: [
        <div key="h" className="sr-header">
          <span className="sr-title">{m$.deliveryTitle}</span>
          <span className="sr-figure">{m$.deliveryEta("12")}</span>
        </div>,
        <div key="m" className="sr-map">
          <div className="sr-route" />
          <div className="sr-pin" />
        </div>,
        <div key="c" className="sr-card sr-at-bottom">
          <div className="sr-progress">
            <div />
          </div>
          <span className="sr-caption">{m$.deliveryNear}</span>
        </div>,
        <div key="a" className="sr-action">
          {m$.deliveryTrack}
        </div>,
      ],
      chip: { icon: "wifi-slash", text: m$.chipOffline },
    },
  ];
}

/** One layer of a screen, lifted by the screen's lift factor to its own height. */
function Lifted({
  lift,
  opacity,
  height,
  className,
  children,
}: {
  lift: MotionValue<number>;
  opacity: MotionValue<number>;
  height: number;
  className: string;
  children: ReactNode;
}) {
  const z = useTransform(lift, (f) => f * height);
  return (
    <m.div className={className} style={{ z, opacity }}>
      {children}
    </m.div>
  );
}

/**
 * One screen on the ring. Its opacity and the lift of its layers are functions of its angle from
 * facing, not timers, so they move with the ring and mirror on the way in and out. Opacity is set
 * on each plane, not on the slot: opacity on a 3D container would flatten its layers.
 */
function Slot({
  screen,
  index,
  step,
  angle,
}: {
  screen: SampleScreen;
  index: number;
  step: number;
  angle: MotionValue<number>;
}) {
  const style = { "--slot": index, "--slots": Math.round(360 / step) } as CSSProperties;
  const away = useTransform(angle, (a) => Math.abs(fold(a + index * step)));
  const opacity = useTransform(away, (d) => clamp01(1 - (d - DRIFT) / FADE));
  const lift = useTransform(away, (d) => smooth(clamp01((DRIFT + FADE - d) / (DRIFT + FADE - LIFT_FULL))));
  const glow = useTransform(opacity, (o) => o * GLOW);
  const height = (k: number) => HEIGHTS[Math.min(k, HEIGHTS.length - 1)] ?? 0;
  return (
    <div className="sr-slot" style={style}>
      <m.div className="sr-glow" style={{ opacity: glow }} />
      <m.div className="sr-edge" data-edge="2" style={{ opacity }} />
      <m.div className="sr-edge" data-edge="1" style={{ opacity }} />
      <m.div className="sr-screen" style={{ opacity }} />
      {screen.layers.map((layer, k) => (
        <Lifted
          key={layer !== null && typeof layer === "object" && "key" in layer ? String(layer.key) : k}
          className="sr-layer"
          lift={lift}
          opacity={opacity}
          height={height(k)}
        >
          {layer}
        </Lifted>
      ))}
      <Lifted className="sr-chip" lift={lift} opacity={opacity} height={CHIP_HEIGHT}>
        <Icon name={screen.chip.icon} size="pill" />
        {screen.chip.text}
      </Lifted>
    </div>
  );
}

/**
 * The sign-in picture (U16.2): sample app screens built with Plux stand on a ring that turns on
 * its vertical axis, one screen at a time. The screen at the front lifts its layers off the
 * surface one by one, the way Plux sends a screen to a device, and a chip names a feature behind
 * it. Decorative: the region has one accessible name and no controls.
 */
export function ScreenRing() {
  const { i18n } = useLingui();
  const m$ = authMessages(i18n);
  const reduced = useReducedMotion() === true;
  const list = screens(m$, i18n.locale);
  const count = list.length;
  const step = 360 / count;
  /** The ring's turn in degrees: screen k faces the viewer at -k * step. */
  const angle = useMotionValue(reduced ? 0 : DRIFT + FADE + 24);

  useEffect(() => {
    if (reduced) {
      angle.set(0);
      return;
    }
    let alive = true;
    let running: { stop: () => void } | undefined;
    const run = async (target: number, duration: number, ease: readonly [number, number, number, number]) => {
      const controls = animate(angle, target, { duration, ease });
      running = controls;
      await controls;
    };
    void (async () => {
      let k = 0;
      await run(DRIFT, TURN_S, turnEase);
      while (alive) {
        const started = performance.now();
        await run(-k * step - DRIFT, DRIFT_S, driftEase);
        // A glide that ends at once means animations are switched off (Motion's skipAnimations):
        // stop rather than turn the ring in a tight loop.
        if (!alive || performance.now() - started < DRIFT_S * 500) {
          return;
        }
        k += 1;
        await run(-k * step + DRIFT, TURN_S, turnEase);
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
      <div role="img" aria-label={m$.sceneLabel} className="sr-stage">
        <m.div className="sr-view" style={{ rotateX: tiltX, rotateY: tiltY }}>
          <m.div className="sr-ring" style={{ z: -RADIUS, rotateY: angle }}>
            {list.map((screen, i) => (
              <Slot key={screen.key} screen={screen} index={i} step={step} angle={angle} />
            ))}
          </m.div>
        </m.div>
      </div>
    </LazyMotion>
  );
}

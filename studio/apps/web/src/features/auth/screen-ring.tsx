// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Icon, type IconName } from "@plux/ui";
import {
  domAnimation,
  LazyMotion,
  type MotionStyle,
  m,
  useMotionValue,
  useReducedMotion,
  useSpring,
  useTransform,
} from "motion/react";
import { type ReactNode, useEffect, useState } from "react";
import { authMessages } from "./auth.messages.ts";
import "./screen-ring.css";

type Messages = ReturnType<typeof authMessages>;

/** One sample screen: its layers from the screen surface up, and the feature chip beside it. */
interface SampleScreen {
  readonly key: string;
  readonly layers: readonly ReactNode[];
  readonly chip: { readonly icon: IconName; readonly text: string };
}

/** How long a screen stays at the front before the ring turns on. */
const PAUSE_MS = 3600;
/** The yaw of the whole ring, so the front screen is seen a little from the side and its layers show their depth. */
const YAW = -16;
/**
 * How high each layer of the front screen stands above it, in px, from the lowest layer up; the
 * chip floats highest. Every layer moves on the same curve over the same time, so a higher layer
 * moves faster and the ratio of their heights holds at every frame: they stay stacked on the screen.
 */
const HEIGHTS = [22, 58, 104, 158] as const;
const CHIP_HEIGHT = 210;
/** Pointer parallax in degrees at the edge of the window. */
const PARALLAX = 7;

const turn = { type: "spring", stiffness: 42, damping: 14, mass: 1 } as const;
/** The layers rise once the screen has settled at the front, and sink as it turns away. */
const rise = { duration: 1.2, ease: [0.16, 1, 0.3, 1], delay: 0.55 } as const;
const sink = { duration: 0.5, ease: [0.4, 0, 1, 1] } as const;
/** Only the front screen is seen: the others fade out as they turn away and in as they arrive. */
const show = { duration: 0.45, ease: "easeOut", delay: 0.1 } as const;
const hide = { duration: 0.3, ease: "easeIn" } as const;

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

/** The distance of a slot from the front, around the ring. */
function distance(slot: number, front: number, count: number): number {
  const d = (((slot - front) % count) + count) % count;
  return Math.min(d, count - d);
}

function Slot({ screen, index, count, front }: { screen: SampleScreen; index: number; count: number; front: number }) {
  const reduced = useReducedMotion() === true;
  const isFront = distance(index, front, count) === 0;
  const style = { "--slot": index, "--slots": count } as MotionStyle;
  const height = (k: number) => HEIGHTS[Math.min(k, HEIGHTS.length - 1)] ?? 0;
  return (
    <m.div
      className="sr-slot"
      style={style}
      data-front={isFront}
      initial={false}
      animate={{ opacity: isFront ? 1 : 0 }}
      transition={reduced ? { duration: 0 } : isFront ? show : hide}
    >
      <div className="sr-screen" />
      {screen.layers.map((layer, k) => (
        <m.div
          key={layer !== null && typeof layer === "object" && "key" in layer ? String(layer.key) : k}
          className="sr-layer"
          initial={false}
          animate={{ z: isFront ? height(k) : 0 }}
          transition={reduced ? { duration: 0 } : isFront ? rise : sink}
        >
          {layer}
        </m.div>
      ))}
      <m.div
        className="sr-chip"
        initial={false}
        animate={{ z: isFront ? CHIP_HEIGHT : 0, opacity: isFront ? 1 : 0 }}
        transition={reduced ? { duration: 0 } : isFront ? rise : sink}
      >
        <Icon name={screen.chip.icon} size="pill" />
        {screen.chip.text}
      </m.div>
    </m.div>
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
  const [step, setStep] = useState(0);

  useEffect(() => {
    if (reduced) {
      return;
    }
    const timer = window.setInterval(() => {
      if (document.visibilityState === "visible") {
        setStep((s) => s + 1);
      }
    }, PAUSE_MS);
    return () => window.clearInterval(timer);
  }, [reduced]);

  const pointerX = useMotionValue(0);
  const pointerY = useMotionValue(0);
  const smoothX = useSpring(pointerX, { stiffness: 50, damping: 20 });
  const smoothY = useSpring(pointerY, { stiffness: 50, damping: 20 });
  const tiltY = useTransform(smoothX, (v) => v * PARALLAX);
  const tiltX = useTransform(smoothY, (v) => 6 - v * PARALLAX * 0.6);

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

  const count = list.length;
  const front = step % count;
  return (
    <LazyMotion features={domAnimation} strict>
      <div role="img" aria-label={m$.sceneLabel} className="sr-stage">
        <m.div className="sr-view" style={{ rotateX: tiltX, rotateY: tiltY }}>
          <m.div
            className="sr-ring"
            initial={reduced ? false : { rotateY: YAW + 120, opacity: 0, scale: 0.9 }}
            animate={{ rotateY: YAW - step * (360 / count), opacity: 1, scale: 1 }}
            transition={reduced ? { duration: 0 } : { rotateY: turn, opacity: { duration: 0.8 }, scale: turn }}
          >
            {list.map((screen, i) => (
              <Slot key={screen.key} screen={screen} index={i} count={count} front={front} />
            ))}
          </m.div>
        </m.div>
      </div>
    </LazyMotion>
  );
}

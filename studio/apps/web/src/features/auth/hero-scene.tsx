// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Icon } from "@plux/ui";
import { domAnimation, LazyMotion, m, useMotionValue, useReducedMotion, useSpring, useTransform } from "motion/react";
import { type ReactNode, useEffect, useMemo } from "react";
import { authMessages } from "./auth.messages.ts";
import "./hero-scene.css";
import { Liquid } from "./liquid.tsx";

/** The isometric view: the plane leans back and turns, as in an exploded drawing. */
const TILT_X = 56;
const TILT_Z = -38;
/** How far the layers stand apart: wide while the visitor looks, close while they type. */
const GAP_APART = 30;
const GAP_TOGETHER = 7;
/** Pointer parallax in degrees at the edge of the window. */
const PARALLAX = 5;

const settle = { type: "spring", stiffness: 90, damping: 18, mass: 1 } as const;

function Layer({ index, gap, children }: { index: number; gap: number; children: ReactNode }) {
  const reduced = useReducedMotion() === true;
  return (
    <m.div
      className="hero-layer"
      initial={reduced ? false : { z: index * GAP_APART + 360, opacity: 0 }}
      animate={{ z: index * gap, opacity: 1 }}
      transition={{ ...settle, delay: reduced ? 0 : 0.35 + index * 0.11 }}
    >
      {children}
    </m.div>
  );
}

/**
 * The sign-in picture (U16.2): a sample app screen taken apart into the layers Plux sends to a
 * device. The layers float apart while the visitor looks and come together while they type in
 * the form (`together`), so the picture answers what they do. Decorative: one accessible name.
 */
export function HeroScene({ together }: { together: boolean }) {
  const { i18n } = useLingui();
  const copy = authMessages(i18n);
  const reduced = useReducedMotion() === true;
  const gap = together ? GAP_TOGETHER : GAP_APART;

  const pointerX = useMotionValue(0);
  const pointerY = useMotionValue(0);
  const smoothX = useSpring(pointerX, { stiffness: 60, damping: 20 });
  const smoothY = useSpring(pointerY, { stiffness: 60, damping: 20 });
  const rotateZ = useTransform(smoothX, (v) => TILT_Z + v * PARALLAX);
  const rotateX = useTransform(smoothY, (v) => TILT_X - v * PARALLAX);

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

  const money = useMemo(
    () => new Intl.NumberFormat(i18n.locale, { style: "currency", currency: "EUR", maximumFractionDigits: 0 }),
    [i18n.locale],
  );
  const cents = useMemo(
    () => new Intl.NumberFormat(i18n.locale, { style: "currency", currency: "EUR" }),
    [i18n.locale],
  );

  return (
    <LazyMotion features={domAnimation} strict>
      <div role="img" aria-label={copy.sceneLabel} className="hero-viewport">
        <Liquid />
        <m.div
          className="hero-plane"
          style={{ rotateX, rotateZ }}
          initial={reduced ? false : { opacity: 0, scale: 0.9 }}
          animate={reduced ? { opacity: 1 } : { opacity: 1, scale: 1, y: [0, -12, 0] }}
          transition={{
            opacity: { duration: 0.8 },
            scale: { ...settle },
            y: { duration: 8, ease: "easeInOut", repeat: Number.POSITIVE_INFINITY },
          }}
        >
          <div className="hero-layer">
            <div className="hero-ground" />
          </div>
          <Layer index={0} gap={gap}>
            <div className="hero-device" />
          </Layer>
          <Layer index={1} gap={gap}>
            <div className="hero-screen" />
          </Layer>
          <Layer index={2} gap={gap}>
            <div className="hero-header">
              <span className="hero-title">{copy.sceneTitle}</span>
              <span className="hero-caption">{copy.sceneBorrow}</span>
              <span className="hero-amount">{money.format(12000)}</span>
            </div>
          </Layer>
          <Layer index={3} gap={gap}>
            <div className="hero-card">
              <div className="hero-track">
                <div className="hero-track-fill" />
                <div className="hero-thumb" />
              </div>
              <div className="hero-row">
                <span className="hero-caption">{copy.sceneMonthly}</span>
                <span className="hero-monthly">{cents.format(342.18)}</span>
              </div>
            </div>
          </Layer>
          <Layer index={4} gap={gap}>
            <div className="hero-action">{copy.sceneApply}</div>
          </Layer>
          <Layer index={5} gap={gap}>
            <div className="hero-badge">
              <Icon name="check-circle" size="pill" />
              {copy.sceneSigned("1.4")}
            </div>
          </Layer>
        </m.div>
      </div>
    </LazyMotion>
  );
}

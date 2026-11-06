// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** The words of the motion panel (PC-29, ANI-001–ANI-004, ANI-007). */
export function motionMessages(i18n: I18n) {
  return {
    title: i18n._({ id: "motion.title", message: "Motion", comment: "Heading of a widget's animations." }),
    enter: i18n._({ id: "motion.enter", message: "Enter", comment: "The transition a widget plays when it appears." }),
    exit: i18n._({ id: "motion.exit", message: "Exit", comment: "The transition a widget plays when it goes." }),
    kind: (kind: string) =>
      i18n._({
        id: "motion.kind",
        message:
          "{kind, select, none {None} fade {Fade} scale {Scale} slideUp {Slide up} slideDown {Slide down} slideLeft {Slide left} slideRight {Slide right} other {Transition}}",
        values: { kind },
        comment: "An enter or exit transition of a widget (ANI-003).",
      }),
    curve: (curve: string) =>
      i18n._({
        id: "motion.curve",
        message:
          "{curve, select, linear {Linear} easeIn {Ease in} easeOut {Ease out} easeInOut {Ease in and out} fastOutSlowIn {Standard} decelerate {Decelerate} bounceIn {Bounce in} bounceOut {Bounce out} elasticOut {Elastic} overshoot {Overshoot} other {Curve}}",
        values: { curve },
        comment: "An animation curve, as Flutter's Curves name them; Standard is Material's fastOutSlowIn.",
      }),
    curveLabel: i18n._({ id: "motion.curveLabel", message: "Curve", comment: "Label of an animation's curve." }),
    duration: i18n._({
      id: "motion.duration",
      message: "Duration (ms)",
      comment: "Label of an animation's duration in milliseconds.",
    }),
    delay: i18n._({ id: "motion.delay", message: "Delay (ms)", comment: "Label of an implicit animation's delay." }),
    play: (phase: string) =>
      i18n._({
        id: "motion.play",
        message: "{phase, select, enter {Play enter} other {Play exit}}",
        values: { phase },
        comment: "Button that plays the widget's transition on the canvas.",
      }),
    animate: i18n._({
      id: "motion.animate",
      message: "Animate changes",
      comment: "Switch: the widget's numbers and colours animate when their bound value changes (ANI-001).",
    }),
    hero: i18n._({
      id: "motion.hero",
      message: "Hero tag",
      comment: "Label of the tag of a shared-element transition (ANI-004).",
    }),
    heroMatches: (n: number) =>
      i18n._({
        id: "motion.heroMatches",
        message:
          "{n, plural, =0 {No other widget has this tag yet} one {Flies to # widget, outlined on the canvas} other {Flies to # widgets, outlined on the canvas}}",
        values: { n },
        comment: "How many widgets of other screens share the hero tag.",
      }),
    reduce: i18n._({
      id: "motion.reduce",
      message: "Reduce motion",
      comment: "What the animation does when the device asks to reduce motion (ANI-007).",
    }),
    reduceKind: (kind: string) =>
      i18n._({
        id: "motion.reduceKind",
        message: "{kind, select, skip {Skip} shorten {Shorten} ignore {Play as declared} other {Default}}",
        values: { kind },
        comment: "Skip jumps to the end, Shorten plays a quarter of the duration, Play as declared keeps the motion.",
      }),
  };
}

export type MotionMessages = ReturnType<typeof motionMessages>;

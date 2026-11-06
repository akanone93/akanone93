// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** The words of the timeline strip and the page's timelines (PC-30, ANI-002, ANI-006). */
export function timelineMessages(i18n: I18n) {
  return {
    timelines: i18n._({
      id: "timeline.timelines",
      message: "Timelines",
      comment: "Section of the page settings: the page's animation timelines.",
    }),
    add: i18n._({
      id: "timeline.add",
      message: "Add a timeline",
      comment: "Button that adds an animation timeline to the page.",
    }),
    open: (name: string) =>
      i18n._({
        id: "timeline.open",
        message: "Edit {name} on the canvas",
        values: { name },
        comment: "Button that opens a timeline's strip over the canvas.",
      }),
    remove: (name: string) =>
      i18n._({
        id: "timeline.remove",
        message: "Remove {name}",
        values: { name },
        comment: "Button that removes a timeline.",
      }),
    strip: (name: string) =>
      i18n._({
        id: "timeline.strip",
        message: "Timeline {name}",
        values: { name },
        comment: "Accessible name of the timeline strip over the canvas.",
      }),
    meta: (seconds: string, tracks: number) =>
      i18n._({
        id: "timeline.meta",
        message: "{seconds} s · {tracks, plural, one {# track} other {# tracks}}",
        values: { seconds, tracks },
        comment: "A timeline's length and how many tracks it has.",
      }),
    play: i18n._({ id: "timeline.play", message: "Play", comment: "Plays the timeline on the canvas." }),
    pause: i18n._({ id: "timeline.pause", message: "Pause", comment: "Pauses the timeline." }),
    close: i18n._({ id: "timeline.close", message: "Close the timeline", comment: "Closes the timeline strip." }),
    scrub: i18n._({
      id: "timeline.scrub",
      message: "Playhead",
      comment: "Slider that moves through the timeline; the canvas shows the page there.",
    }),
    duration: i18n._({ id: "timeline.duration", message: "Duration (ms)", comment: "Label of the timeline's length." }),
    repeat: i18n._({
      id: "timeline.repeat",
      message: "Repeats",
      comment: "How many times the timeline plays again after the first.",
    }),
    forever: i18n._({ id: "timeline.forever", message: "Loop", comment: "Switch: the timeline repeats forever." }),
    reverse: i18n._({
      id: "timeline.reverse",
      message: "Reverse on repeat",
      comment: "Switch: each repeat plays backwards.",
    }),
    autoplay: i18n._({
      id: "timeline.autoplay",
      message: "Play when the page shows",
      comment: "Switch: the timeline plays on page enter.",
    }),
    drivenBy: i18n._({
      id: "timeline.drivenBy",
      message: "Driven by scroll",
      comment: "Select of the scroll view whose offset moves the timeline (ANI-006).",
    }),
    noDriver: i18n._({
      id: "timeline.noDriver",
      message: "Time",
      comment: "Option: the timeline runs in time, not by scroll.",
    }),
    extent: i18n._({
      id: "timeline.extent",
      message: "Scroll extent (px)",
      comment: "Pixels of scrolling that span the whole timeline.",
    }),
    driven: i18n._({
      id: "timeline.driven",
      message: "Scroll the view in Interact mode to move it.",
      comment: "Note on a timeline driven by a scroll view.",
    }),
    keyframeHint: i18n._({
      id: "timeline.keyframeHint",
      message: "Click to go to this keyframe; Shift+click to remove it",
      comment: "Tooltip of a keyframe on the timeline.",
    }),
    noTracks: i18n._({
      id: "timeline.noTracks",
      message: "Select a widget, pick a property and add a keyframe at the playhead.",
      comment: "Shown on an empty timeline.",
    }),
    prop: i18n._({
      id: "timeline.prop",
      message: "Property",
      comment: "Select of the selected widget's animatable properties.",
    }),
    value: i18n._({ id: "timeline.value", message: "Value", comment: "The value of a new keyframe." }),
    addKeyframe: (at: string) =>
      i18n._({
        id: "timeline.addKeyframe",
        message: "Add keyframe at {at}",
        values: { at },
        comment: "Button; {at} is the playhead, e.g. 0.40 s.",
      }),
  };
}

export type TimelineMessages = ReturnType<typeof timelineMessages>;

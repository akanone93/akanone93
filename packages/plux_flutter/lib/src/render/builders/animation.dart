// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The interaction and animation widgets of the catalogue catch-up
/// (ADR-0071, WGT-003): `Dismissible`, `Hero`, `Timeline`, the implicit `Animated*` widgets
/// and the timeline-driven `*Transition` widgets. All of them follow the
/// platform's reduce-motion setting (ANI-007): an implicit animation then
/// takes no time, and a timeline follows its own reduction.
///
/// These are P5 widgets, so the generator writes no builder and no
/// `WidgetIds` entry for them; their permanent IDs are declared here, as
/// for `FormScope`.
library;

import 'package:flutter/gestures.dart' show DragStartBehavior;
import 'package:flutter/material.dart';
import 'package:plux_flutter/src/animation/curves.dart';
import 'package:plux_flutter/src/animation/registry.dart';
import 'package:plux_flutter/src/animation/timeline_spec.dart';
import 'package:plux_flutter/src/render/decoders.dart';
import 'package:plux_flutter/src/render/decoding.dart';
import 'package:plux_flutter/src/render/generated/render.g.dart';
import 'package:plux_flutter/src/render/node_context.dart';
import 'package:plux_flutter/src/render/scope.dart';

/// Permanent widget ID of `Dismissible`.
const int dismissibleWidgetId = 133;

/// Permanent widget ID of `Hero`.
const int heroWidgetId = 137;

/// Permanent widget ID of `Timeline`.
const int timelineWidgetId = 138;

/// Permanent widget ID of `AnimatedContainer`.
const int animatedContainerWidgetId = 139;

/// Permanent widget ID of `AnimatedOpacity`.
const int animatedOpacityWidgetId = 140;

/// Permanent widget ID of `AnimatedAlign`.
const int animatedAlignWidgetId = 141;

/// Permanent widget ID of `AnimatedPadding`.
const int animatedPaddingWidgetId = 142;

/// Permanent widget ID of `AnimatedPositioned`.
const int animatedPositionedWidgetId = 143;

/// Permanent widget ID of `AnimatedScale`.
const int animatedScaleWidgetId = 144;

/// Permanent widget ID of `AnimatedRotation`.
const int animatedRotationWidgetId = 145;

/// Permanent widget ID of `AnimatedSlide`.
const int animatedSlideWidgetId = 146;

/// Permanent widget ID of `AnimatedSwitcher`.
const int animatedSwitcherWidgetId = 147;

/// Permanent widget ID of `AnimatedCrossFade`.
const int animatedCrossFadeWidgetId = 148;

/// Permanent widget ID of `AnimatedSize`.
const int animatedSizeWidgetId = 149;

/// Permanent widget ID of `FadeTransition`.
const int fadeTransitionWidgetId = 150;

/// Permanent widget ID of `SlideTransition`.
const int slideTransitionWidgetId = 151;

/// Permanent widget ID of `ScaleTransition`.
const int scaleTransitionWidgetId = 152;

/// Permanent widget ID of `RotationTransition`.
const int rotationTransitionWidgetId = 153;

/// The builders of this group, by permanent widget ID.
const Map<int, NodeBuilder> animationBuilders = {
  dismissibleWidgetId: _dismissible,
  heroWidgetId: _hero,
  timelineWidgetId: _timeline,
  animatedContainerWidgetId: _animatedContainer,
  animatedOpacityWidgetId: _animatedOpacity,
  animatedAlignWidgetId: _animatedAlign,
  animatedPaddingWidgetId: _animatedPadding,
  animatedPositionedWidgetId: _animatedPositioned,
  animatedScaleWidgetId: _animatedScale,
  animatedRotationWidgetId: _animatedRotation,
  animatedSlideWidgetId: _animatedSlide,
  animatedSwitcherWidgetId: _animatedSwitcher,
  animatedCrossFadeWidgetId: _animatedCrossFade,
  animatedSizeWidgetId: _animatedSize,
  fadeTransitionWidgetId: _fadeTransition,
  slideTransitionWidgetId: _slideTransition,
  scaleTransitionWidgetId: _scaleTransition,
  rotationTransitionWidgetId: _rotationTransition,
};

// ── Shared reads ───────────────────────────────────────────────────────────

const List<String> _curveNames = [
  'linear',
  'easeIn',
  'easeOut',
  'easeInOut',
  'fastOutSlowIn',
  'decelerate',
  'bounceIn',
  'bounceOut',
  'elasticOut',
  'overshoot',
];

/// The curve in prop [id]; [fallback] when absent.
Curve _curve(NodeContext c, int id, [Curve fallback = Curves.linear]) {
  final name = c.decode(id, nameCurve);
  final at = name == null ? -1 : _curveNames.indexOf(name);
  return at < 0 ? fallback : animationCurves[at];
}

/// The duration in prop [id]; zero under reduce motion (ANI-007), or one
/// microsecond where [measured]: Flutter's `AnimatedSize` asserts on a zero
/// duration.
Duration _duration(NodeContext c, int id, {bool measured = false}) {
  if (MediaQuery.maybeDisableAnimationsOf(c.context) ?? false) {
    return measured ? const Duration(microseconds: 1) : Duration.zero;
  }
  return c.decode(id, asDuration) ?? c.missing('duration');
}

/// An optional duration; negligible under reduce motion (one microsecond,
/// as for [_duration] with `measured`).
Duration? _optionalDuration(NodeContext c, int id) {
  final d = c.decode(id, asDuration);
  if (d == null) return null;
  return (MediaQuery.maybeDisableAnimationsOf(c.context) ?? false)
      ? const Duration(microseconds: 1)
      : d;
}

VoidCallback? _onEnd(NodeContext c) => c.handles(1) ? () => c.fire(1) : null;

Alignment? _plainAlignment(NodeContext c, int id) {
  final a = c.decode(id, decodeAlignment);
  return switch (a) {
    Alignment() => a,
    AlignmentDirectional() => a.resolve(c.textDirection),
    _ => null,
  };
}

// ── Interaction ────────────────────────────────────────────────────────────

Widget _dismissible(NodeContext c) => _KeyedDismissible(
  direction: c.decode(1, decodeDismissDirection) ?? DismissDirection.horizontal,
  resize: c.decode(2, asDuration) ?? const Duration(milliseconds: 300),
  movement: c.decode(3, asDuration) ?? const Duration(milliseconds: 200),
  crossAxisEndOffset: c.decode(4, asDouble) ?? 0.0,
  dragStartBehavior:
      c.decode(5, decodeDragStartBehavior) ?? DragStartBehavior.start,
  behavior: c.decode(6, decodeHitTestBehavior) ?? HitTestBehavior.opaque,
  onDismissed: c.handles(1) ? (d) => c.fire(1, d.index + 1) : null,
  background: c.slot(2),
  secondaryBackground: c.slot(3),
  child: c.slot(1) ?? c.missing('child'),
);

/// Owns the key Flutter's `Dismissible` requires, so it survives rebuilds.
final class _KeyedDismissible extends StatefulWidget {
  const _KeyedDismissible({
    required this.direction,
    required this.resize,
    required this.movement,
    required this.crossAxisEndOffset,
    required this.dragStartBehavior,
    required this.behavior,
    required this.onDismissed,
    required this.background,
    required this.secondaryBackground,
    required this.child,
  });

  final DismissDirection direction;
  final Duration resize;
  final Duration movement;
  final double crossAxisEndOffset;
  final DragStartBehavior dragStartBehavior;
  final HitTestBehavior behavior;
  final void Function(DismissDirection)? onDismissed;
  final Widget? background;
  final Widget? secondaryBackground;
  final Widget child;

  @override
  State<_KeyedDismissible> createState() => _KeyedDismissibleState();
}

final class _KeyedDismissibleState extends State<_KeyedDismissible> {
  final Key _key = UniqueKey();

  @override
  Widget build(BuildContext context) {
    final reduce = MediaQuery.maybeDisableAnimationsOf(context) ?? false;
    final w = widget;
    final background = w.background;
    return Dismissible(
      key: _key,
      direction: w.direction,
      resizeDuration: reduce ? Duration.zero : w.resize,
      movementDuration: reduce ? Duration.zero : w.movement,
      crossAxisEndOffset: w.crossAxisEndOffset,
      dragStartBehavior: w.dragStartBehavior,
      behavior: w.behavior,
      onDismissed: w.onDismissed,
      background: background,
      secondaryBackground: background == null ? null : w.secondaryBackground,
      child: w.child,
    );
  }
}

// ── Hero and timelines ─────────────────────────────────────────────────────

Widget _hero(NodeContext c) {
  final child = c.slot(1) ?? c.missing('child');
  if (MediaQuery.maybeDisableAnimationsOf(c.context) ?? false) return child;
  return Hero(
    tag: c.decode(1, asString) ?? c.missing('tag'),
    transitionOnUserGestures: c.decode(2, asBool) ?? false,
    curve: _curve(c, 3, Curves.fastOutSlowIn),
    reverseCurve: c.decode(4, nameCurve) == null ? null : _curve(c, 4),
    child: child,
  );
}

Widget _timeline(NodeContext c) => _TimelinePlayer(
  name: c.decode(1, asString) ?? c.missing('timeline'),
  autoplay: c.decode(2, asBool) ?? true,
  repeat: c.decode(3, asBool) ?? false,
  child: c.slot(1) ?? c.missing('child'),
);

/// The page's run of timeline [name], or null where there is none.
TimelineRun? _runOf(BuildContext context, String name) =>
    RenderScope.of(context).animations?.run(name);

/// Starts the named timeline when shown and, with [repeat], again each time
/// it has played to its end (ANI-002).
final class _TimelinePlayer extends StatefulWidget {
  const _TimelinePlayer({
    required this.name,
    required this.autoplay,
    required this.repeat,
    required this.child,
  });

  final String name;
  final bool autoplay;
  final bool repeat;
  final Widget child;

  @override
  State<_TimelinePlayer> createState() => _TimelinePlayerState();
}

final class _TimelinePlayerState extends State<_TimelinePlayer> {
  TimelineRun? _run;
  bool _wasPlaying = false;

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    final run = _runOf(context, widget.name);
    if (identical(run, _run)) return;
    _run?.removeListener(_changed);
    _run = run;
    run?.addListener(_changed);
    if (widget.autoplay && run != null && !run.active) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (mounted && !run.active) run.start();
      });
    }
  }

  void _changed() {
    final run = _run;
    if (run == null || !widget.repeat) return;
    final playing = run.playing;
    final ended = _wasPlaying && !playing && run.position >= run.end;
    _wasPlaying = playing;
    if (ended && !(MediaQuery.maybeDisableAnimationsOf(context) ?? false)) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (mounted) run.start();
      });
    }
  }

  @override
  void dispose() {
    _run?.removeListener(_changed);
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => widget.child;
}

// ── Implicit animations ────────────────────────────────────────────────────

Widget _animatedContainer(NodeContext c) => AnimatedContainer(
  alignment: c.decode(1, decodeAlignment),
  padding: c.decode(2, decodeEdgeInsets),
  color: c.decode(3, asColor),
  decoration: c.decode(4, decodeBoxDecoration),
  foregroundDecoration: c.decode(5, decodeBoxDecoration),
  width: c.decode(6, asDouble),
  height: c.decode(7, asDouble),
  constraints: c.decode(8, decodeBoxConstraints),
  margin: c.decode(9, decodeEdgeInsets),
  transformAlignment: c.decode(10, decodeAlignment),
  clipBehavior: c.decode(11, decodeClip) ?? Clip.none,
  curve: _curve(c, 12),
  duration: _duration(c, 13),
  onEnd: _onEnd(c),
  child: c.slot(1),
);

Widget _animatedOpacity(NodeContext c) => AnimatedOpacity(
  opacity: (c.decode(1, asDouble) ?? c.missing('opacity')).clamp(0.0, 1.0),
  alwaysIncludeSemantics: c.decode(2, asBool) ?? false,
  curve: _curve(c, 3),
  duration: _duration(c, 4),
  onEnd: _onEnd(c),
  child: c.slot(1),
);

Widget _animatedAlign(NodeContext c) => AnimatedAlign(
  alignment: c.decode(1, decodeAlignment) ?? c.missing('alignment'),
  heightFactor: c.decode(2, asDouble),
  widthFactor: c.decode(3, asDouble),
  curve: _curve(c, 4),
  duration: _duration(c, 5),
  onEnd: _onEnd(c),
  child: c.slot(1),
);

Widget _animatedPadding(NodeContext c) => AnimatedPadding(
  padding: c.decode(1, decodeEdgeInsets) ?? c.missing('padding'),
  curve: _curve(c, 2),
  duration: _duration(c, 3),
  onEnd: _onEnd(c),
  child: c.slot(1),
);

Widget _animatedPositioned(NodeContext c) => AnimatedPositioned(
  left: c.decode(1, asDouble),
  top: c.decode(2, asDouble),
  right: c.decode(3, asDouble),
  bottom: c.decode(4, asDouble),
  width: c.decode(5, asDouble),
  height: c.decode(6, asDouble),
  curve: _curve(c, 7),
  duration: _duration(c, 8),
  onEnd: _onEnd(c),
  child: c.slot(1) ?? c.missing('child'),
);

Widget _animatedScale(NodeContext c) => AnimatedScale(
  scale: c.decode(1, asDouble) ?? c.missing('scale'),
  alignment: _plainAlignment(c, 2) ?? Alignment.center,
  filterQuality: c.decode(3, decodeFilterQuality),
  curve: _curve(c, 4),
  duration: _duration(c, 5),
  onEnd: _onEnd(c),
  child: c.slot(1),
);

Widget _animatedRotation(NodeContext c) => AnimatedRotation(
  turns: c.decode(1, asDouble) ?? c.missing('turns'),
  alignment: _plainAlignment(c, 2) ?? Alignment.center,
  filterQuality: c.decode(3, decodeFilterQuality),
  curve: _curve(c, 4),
  duration: _duration(c, 5),
  onEnd: _onEnd(c),
  child: c.slot(1),
);

Widget _animatedSlide(NodeContext c) => AnimatedSlide(
  offset: c.decode(1, decodeOffset) ?? c.missing('offset'),
  curve: _curve(c, 2),
  duration: _duration(c, 3),
  onEnd: _onEnd(c),
  child: c.slot(1),
);

Widget _animatedSwitcher(NodeContext c) => AnimatedSwitcher(
  duration: _duration(c, 1),
  reverseDuration: _optionalDuration(c, 2),
  switchInCurve: _curve(c, 3),
  switchOutCurve: _curve(c, 4),
  child: c.slot(1),
);

Widget _animatedCrossFade(NodeContext c) => AnimatedCrossFade(
  firstChild: c.slot(1) ?? c.missing('firstChild'),
  secondChild: c.slot(2) ?? c.missing('secondChild'),
  crossFadeState:
      c.decode(1, decodeCrossFadeState) ?? c.missing('crossFadeState'),
  firstCurve: _curve(c, 2),
  secondCurve: _curve(c, 3),
  sizeCurve: _curve(c, 4),
  alignment: c.decode(5, decodeAlignment) ?? AlignmentDirectional.topCenter,
  duration: _duration(c, 6, measured: true),
  reverseDuration: _optionalDuration(c, 7),
  excludeBottomFocus: c.decode(8, asBool) ?? true,
  clipBehavior: c.decode(9, decodeClip) ?? Clip.hardEdge,
  onEnd: _onEnd(c),
);

Widget _animatedSize(NodeContext c) => AnimatedSize(
  alignment: c.decode(1, decodeAlignment) ?? Alignment.center,
  curve: _curve(c, 2),
  duration: _duration(c, 3, measured: true),
  reverseDuration: _optionalDuration(c, 4),
  clipBehavior: c.decode(5, decodeClip) ?? Clip.hardEdge,
  onEnd: _onEnd(c),
  child: c.slot(1),
);

// ── Timeline-driven transitions ────────────────────────────────────────────

/// Builds [build] from the value of one track of a page timeline, rebuilding
/// as the timeline moves. Without a running timeline the track's start value
/// is used; a timeline or track the page does not have shows the child
/// untouched.
final class _TrackValue extends StatelessWidget {
  const _TrackValue({
    required this.timeline,
    required this.track,
    required this.child,
    required this.builder,
  });

  final String timeline;
  final int track;
  final Widget child;
  final Widget Function(double value, Widget child) builder;

  @override
  Widget build(BuildContext context) {
    final run = _runOf(context, timeline);
    if (run == null || track < 0 || track >= run.spec.tracks.length) {
      return child;
    }
    final TrackSpec spec = run.spec.tracks[track];
    if (spec.keyframes.isEmpty) return child;
    return AnimatedBuilder(
      animation: run,
      child: child,
      builder: (context, child) {
        final v = run.active ? run.valueOf(spec) : spec.keyframes.first.value;
        return builder(v is num ? v.toDouble() : 0.0, child!);
      },
    );
  }
}

Widget _driven(
  NodeContext c,
  Widget Function(double value, Widget child) build,
) => _TrackValue(
  timeline: c.decode(1, asString) ?? c.missing('timeline'),
  track: c.decode(2, asInt) ?? 0,
  builder: build,
  child: c.slot(1) ?? c.missing('child'),
);

Widget _fadeTransition(NodeContext c) {
  final semantics = c.decode(3, asBool) ?? false;
  return _driven(
    c,
    (v, child) => FadeTransition(
      opacity: AlwaysStoppedAnimation(v.clamp(0.0, 1.0)),
      alwaysIncludeSemantics: semantics,
      child: child,
    ),
  );
}

Widget _slideTransition(NodeContext c) {
  final vertical =
      (c.decode(3, decodeAxis) ?? Axis.horizontal) == Axis.vertical;
  final hits = c.decode(4, asBool) ?? true;
  final direction = c.decode(5, decodeTextDirection);
  return _driven(
    c,
    (v, child) => SlideTransition(
      position: AlwaysStoppedAnimation(vertical ? Offset(0, v) : Offset(v, 0)),
      transformHitTests: hits,
      textDirection: direction,
      child: child,
    ),
  );
}

Widget _scaleTransition(NodeContext c) {
  final alignment = _plainAlignment(c, 3) ?? Alignment.center;
  final quality = c.decode(4, decodeFilterQuality);
  return _driven(
    c,
    (v, child) => ScaleTransition(
      scale: AlwaysStoppedAnimation(v),
      alignment: alignment,
      filterQuality: quality,
      child: child,
    ),
  );
}

Widget _rotationTransition(NodeContext c) {
  final alignment = _plainAlignment(c, 3) ?? Alignment.center;
  final quality = c.decode(4, decodeFilterQuality);
  return _driven(
    c,
    (v, child) => RotationTransition(
      turns: AlwaysStoppedAnimation(v),
      alignment: alignment,
      filterQuality: quality,
      child: child,
    ),
  );
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// What a page contributes to a structural session replay (ANL-050,
/// ADR-0101): a [ReplayCapture] around the page keeps the position of every
/// node marked by a [ReplayNodeMarker] and turns a touch into the node it
/// hit. It reads rectangles after a frame and never a node's content: no
/// text, no image, no value.
library;

import 'dart:async';

import 'package:flutter/widgets.dart';
import 'package:plux_flutter/src/replay/replay_recorder.dart';

/// The node markers of one page and the snapshot of their rectangles.
final class ReplayHost {
  /// Creates a host that reports to [recorder].
  ReplayHost(this.recorder);

  /// The recorder.
  final ReplayRecorder recorder;

  /// The least time between two layout snapshots.
  static const Duration spacing = Duration(milliseconds: 250);

  final Map<String, (String, BuildContext)> _nodes = {};
  RenderBox? Function() _root = () => null;
  bool _scheduled = false;
  DateTime _last = DateTime.fromMillisecondsSinceEpoch(0);
  Timer? _timer;

  /// Tells the host which render box the rectangles are relative to.
  void bindRoot(RenderBox? Function() root) => _root = root;

  /// A node of widget type [widget] at [path] was built in [context].
  void attach(String path, String widget, BuildContext context) {
    _nodes[path] = (widget, context);
    _schedule();
  }

  /// The node at [path] built in [context] was removed.
  void detach(String path, BuildContext context) {
    if (_nodes[path]?.$2 == context) {
      _nodes.remove(path);
      _schedule();
    }
  }

  void _schedule() {
    if (_scheduled || !recorder.recording) return;
    _scheduled = true;
    WidgetsBinding.instance.addPostFrameCallback((_) {
      _scheduled = false;
      final wait = spacing - DateTime.now().difference(_last);
      if (wait <= Duration.zero) {
        snapshot();
      } else {
        _timer?.cancel();
        _timer = Timer(wait, snapshot);
      }
    });
  }

  /// Records the rectangles of the nodes now.
  void snapshot() {
    _last = DateTime.now();
    if (!recorder.recording) return;
    final nodes = <ReplayNode>[];
    for (final e in _rects().entries) {
      nodes.add(
        ReplayNode(
          path: e.key,
          widget: _nodes[e.key]!.$1,
          x: e.value.left,
          y: e.value.top,
          width: e.value.width,
          height: e.value.height,
        ),
      );
    }
    nodes.sort((a, b) => a.path.compareTo(b.path));
    recorder.layout(nodes);
  }

  Map<String, Rect> _rects() {
    final root = _root();
    final out = <String, Rect>{};
    if (root == null || !root.attached) return out;
    for (final e in _nodes.entries) {
      final context = e.value.$2;
      if (!context.mounted) continue;
      final box = context.findRenderObject();
      if (box is! RenderBox || !box.attached || !box.hasSize) continue;
      out[e.key] = box.localToGlobal(Offset.zero, ancestor: root) & box.size;
    }
    return out;
  }

  /// Records the touch at [global] on the smallest node that holds it.
  void touched(Offset global) {
    if (!recorder.recording) return;
    final root = _root();
    if (root == null) return;
    final point = root.globalToLocal(global);
    String? best;
    Rect? bestRect;
    for (final e in _rects().entries) {
      final r = e.value;
      if (!r.contains(point)) continue;
      if (bestRect == null ||
          r.width * r.height <= bestRect.width * bestRect.height) {
        best = e.key;
        bestRect = r;
      }
    }
    if (best == null || bestRect == null) return;
    recorder.tap(
      best,
      bestRect.width == 0 ? 0 : (point.dx - bestRect.left) / bestRect.width,
      bestRect.height == 0 ? 0 : (point.dy - bestRect.top) / bestRect.height,
    );
  }

  /// Stops the pending snapshot.
  void dispose() => _timer?.cancel();
}

/// Provides the [ReplayHost] to the nodes below a [ReplayCapture].
final class ReplayScope extends InheritedWidget {
  /// Creates the scope.
  const ReplayScope({required this.host, required super.child, super.key});

  /// The host.
  final ReplayHost host;

  /// The host above [context], or null when the page is not recorded.
  static ReplayHost? maybeOf(BuildContext context) =>
      context.getInheritedWidgetOfExactType<ReplayScope>()?.host;

  @override
  bool updateShouldNotify(ReplayScope old) => host != old.host;
}

/// Wraps a page that is recorded: it listens to touches without taking part
/// in them, and takes the first layout snapshot after the first frame.
final class ReplayCapture extends StatefulWidget {
  /// Creates the capture around [child].
  const ReplayCapture({required this.recorder, required this.child, super.key});

  /// The recorder.
  final ReplayRecorder recorder;

  /// The page.
  final Widget child;

  @override
  State<ReplayCapture> createState() => _ReplayCaptureState();
}

final class _ReplayCaptureState extends State<ReplayCapture> {
  late final ReplayHost _host = ReplayHost(widget.recorder);

  @override
  void initState() {
    super.initState();
    _host.bindRoot(() {
      final box = context.findRenderObject();
      return box is RenderBox ? box : null;
    });
  }

  @override
  void dispose() {
    _host.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => ReplayScope(
    host: _host,
    child: Listener(
      behavior: HitTestBehavior.translucent,
      onPointerUp: (e) => _host.touched(e.position),
      child: widget.child,
    ),
  );
}

/// Marks a node so that its rectangle is part of a layout snapshot. It adds
/// no render object and reads nothing of the node.
final class ReplayNodeMarker extends StatefulWidget {
  /// Creates the marker for the node at [path] of widget type [widget].
  const ReplayNodeMarker({
    required this.host,
    required this.path,
    required this.widget,
    required this.child,
    super.key,
  });

  /// The host.
  final ReplayHost host;

  /// The node's path.
  final String path;

  /// The widget type name.
  final String widget;

  /// The node's widget.
  final Widget child;

  @override
  State<ReplayNodeMarker> createState() => _ReplayNodeMarkerState();
}

final class _ReplayNodeMarkerState extends State<ReplayNodeMarker> {
  @override
  void initState() {
    super.initState();
    widget.host.attach(widget.path, widget.widget, context);
  }

  @override
  void dispose() {
    widget.host.detach(widget.path, context);
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => widget.child;
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// What the device tools of a dev session know about the nodes on screen
/// (`DEV-030`, `DEV-034`): each node's widget is wrapped in a marker that
/// carries its identity, and a [DevNodeHost] keeps the markers, the rebuild
/// counts and the state of the tools. Only builds with `kPluxDevtools`
/// create any of it; a node outside a [DevNodeScope] is not wrapped.
library;

import 'dart:collection';

import 'package:flutter/rendering.dart';
import 'package:flutter/widgets.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';

/// The status names of `DevCommandStatus` in the proto3 JSON mapping.
const String devStatusOk = 'DEV_COMMAND_STATUS_OK';

/// `DEV_COMMAND_STATUS_FAILED`.
const String devStatusFailed = 'DEV_COMMAND_STATUS_FAILED';

/// `DEV_COMMAND_STATUS_REFUSED`.
const String devStatusRefused = 'DEV_COMMAND_STATUS_REFUSED';

/// The `CommandResultEvent` of a command that failed with [status].
Map<String, Object?> devFailure(
  String status,
  String message, [
  PluxErrorCode? code,
]) => {
  'status': status,
  'error': {
    if (code != null) 'code': code.id,
    if (code != null) 'reason': code.reason,
    'message': message,
  },
};

/// The object of command [name] in [command], or null when it is not one.
Map<String, Object?>? devCommandBody(
  Map<String, Object?> command,
  String name,
) {
  final body = command[name];
  return body is Map<String, Object?> ? body : null;
}

/// Who a node is on screen.
@immutable
final class DevNodeInfo {
  /// Creates the identity of a node.
  const DevNodeInfo({
    required this.section,
    required this.index,
    required this.path,
    required this.widget,
    this.interactive = false,
  });

  /// The ID of the page or component section holding the node, as the
  /// canonical text of its UUID.
  final String section;

  /// The node's index in the section's node array.
  final int index;

  /// The node's path for reports: the scope's path and the index.
  final String path;

  /// The widget type the node builds, such as `Button`; empty for a
  /// component or a native slot.
  final String widget;

  /// Whether the node takes taps, so its area is held to the minimum
  /// target size (`A11Y-004`).
  final bool interactive;

  @override
  bool operator ==(Object other) =>
      other is DevNodeInfo &&
      other.section == section &&
      other.index == index &&
      other.path == path &&
      other.widget == widget &&
      other.interactive == interactive;

  @override
  int get hashCode => Object.hash(section, index, path, widget, interactive);
}

/// Maps a node to its location in the document, from the dev bundle's
/// source map: [section] is a section ID as in [DevNodeInfo.section].
/// Null when the map has no entry for the node.
typedef DevSourceMapLookup = String? Function(String section, int index);

/// One frame of the frame chart: the time its build and raster phases took.
@immutable
final class DevFrame {
  /// Creates a frame.
  const DevFrame(this.build, this.raster);

  /// The build phase on the UI thread.
  final Duration build;

  /// The raster phase.
  final Duration raster;

  /// Build and raster together.
  Duration get total => build + raster;
}

/// The overlays of `DEV-034`.
enum DevOverlayKind {
  /// The build and raster time of the last frames.
  frameChart('DEV_OVERLAY_FRAME_CHART', 1),

  /// How often each node has built.
  rebuildCounters('DEV_OVERLAY_REBUILD_COUNTERS', 2),

  /// The interactive nodes smaller than the minimum target.
  touchTargets('DEV_OVERLAY_TOUCH_TARGETS', 3),

  /// The text with too little contrast against its background.
  contrast('DEV_OVERLAY_CONTRAST', 4);

  const DevOverlayKind(this.protoName, this.protoNumber);

  /// The name in the proto3 JSON mapping of `DevOverlay`.
  final String protoName;

  /// The number of `DevOverlay`.
  final int protoNumber;

  /// The overlay for a proto3 JSON enum value (its name or its number), or
  /// null for an unspecified or unknown one.
  static DevOverlayKind? fromProto(Object? value) {
    for (final k in values) {
      if (value == k.protoName || value == k.protoNumber) return k;
    }
    return null;
  }
}

/// The state of the device tools of one runtime: the markers of the nodes
/// on screen, the rebuild counts, the selection and the overlays.
final class DevNodeHost extends ChangeNotifier {
  /// Creates the host. [sourceMap] maps nodes to document paths.
  DevNodeHost({this.sourceMap});

  /// The most frames the frame chart keeps.
  static const int frameCapacity = 120;

  /// Maps nodes to their location in the document; null before a dev
  /// bundle with a source map has been applied.
  DevSourceMapLookup? sourceMap;

  /// Receives the node a tap selected while [inspecting].
  void Function(DevNodeInfo info)? onInspect;

  final Set<RenderDevNode> _markers = {};
  final Map<String, int> _rebuilds = {};
  final Queue<DevFrame> _frames = Queue();
  final Set<DevOverlayKind> _overlays = {};
  bool _inspecting = false;
  String? _selected;

  /// The markers of the nodes on screen.
  Iterable<RenderDevNode> get markers => _markers;

  /// Whether a tap on the page selects the node under it instead of
  /// reaching the app.
  bool get inspecting => _inspecting;
  set inspecting(bool value) {
    if (value == _inspecting) return;
    _inspecting = value;
    notifyListeners();
  }

  /// The path of the node outlined on screen, or null.
  String? get selected => _selected;
  set selected(String? value) {
    if (value == _selected) return;
    _selected = value;
    notifyListeners();
  }

  /// The overlays switched on.
  Set<DevOverlayKind> get overlays => Set.unmodifiable(_overlays);

  /// Switches [overlay] on or off.
  void setOverlay(DevOverlayKind overlay, {required bool enabled}) {
    if (enabled ? _overlays.add(overlay) : _overlays.remove(overlay)) {
      notifyListeners();
    }
  }

  /// The last [frameCapacity] frames, oldest first.
  List<DevFrame> get frames => List.unmodifiable(_frames);

  /// Records a frame for the frame chart.
  void recordFrame(DevFrame frame) {
    _frames.addLast(frame);
    while (_frames.length > frameCapacity) {
      _frames.removeFirst();
    }
    if (_overlays.contains(DevOverlayKind.frameChart)) notifyListeners();
  }

  /// Counts one build of the node at [path]. It does not notify: it runs
  /// while a build is in progress.
  void built(String path) => _rebuilds[path] = (_rebuilds[path] ?? 0) + 1;

  /// How often the node at [path] has built.
  int rebuilds(String path) => _rebuilds[path] ?? 0;

  /// The location of [info] in the document, or its runtime path when no
  /// source map knows the node.
  String documentPath(DevNodeInfo info) =>
      sourceMap?.call(info.section, info.index) ?? info.path;

  /// The marker of the node at [path], given as a runtime path or as a
  /// document path; null when no such node is on screen.
  RenderDevNode? find(String path) {
    for (final m in _markers) {
      if (m.info.path == path || documentPath(m.info) == path) return m;
    }
    return null;
  }

  void _attach(RenderDevNode m) => _markers.add(m);

  void _detach(RenderDevNode m) => _markers.remove(m);
}

/// Provides the [DevNodeHost] to the nodes below it.
final class DevNodeScope extends InheritedWidget {
  /// Provides [host].
  const DevNodeScope({super.key, required this.host, required super.child});

  /// The host.
  final DevNodeHost host;

  /// The host above [context], or null; it does not rebuild [context].
  static DevNodeHost? maybeOf(BuildContext context) =>
      context.getInheritedWidgetOfExactType<DevNodeScope>()?.host;

  @override
  bool updateShouldNotify(DevNodeScope old) => !identical(old.host, host);
}

/// Wraps a node's widget so the tools can find it on screen.
final class DevNodeMarker extends SingleChildRenderObjectWidget {
  /// Creates the marker.
  const DevNodeMarker({
    super.key,
    required this.info,
    required this.host,
    super.child,
  });

  /// The node.
  final DevNodeInfo info;

  /// The host that keeps the marker.
  final DevNodeHost host;

  @override
  RenderDevNode createRenderObject(BuildContext context) =>
      RenderDevNode(info, host);

  @override
  void updateRenderObject(BuildContext context, RenderDevNode renderObject) {
    renderObject.info = info;
  }
}

/// The render object of a [DevNodeMarker]: it lays out and paints as its
/// child does and registers with the host while attached.
final class RenderDevNode extends RenderProxyBox {
  /// Creates the render object.
  RenderDevNode(this.info, this._host);

  /// The node.
  DevNodeInfo info;

  final DevNodeHost _host;

  @override
  void attach(PipelineOwner owner) {
    super.attach(owner);
    _host._attach(this);
  }

  @override
  void detach() {
    _host._detach(this);
    super.detach();
  }
}

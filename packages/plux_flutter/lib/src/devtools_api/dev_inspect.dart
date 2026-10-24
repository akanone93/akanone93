// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Inspecting nodes on the device (`DEV-030`): with inspect mode on, a tap
/// selects the nearest Plux node under the finger and reports its place in
/// the document; `inspectSelect` outlines a node the developer picked in
/// Studio. Only builds with `kPluxDevtools` create it.
library;

import 'dart:typed_data';

import 'package:flutter/rendering.dart';
import 'package:flutter/widgets.dart';
import 'package:plux_flutter/src/bundle/fbs/bundle_fbs_generated.dart' as fbs;
import 'package:plux_flutter/src/devtools_api/dev_nodes.dart';
import 'package:plux_flutter/src/devtools_api/dev_session.dart';

/// Builds the node lookup of the dev bundle's source map section
/// (`SectionKind.sourceMap`), which names each node's location in the
/// document by pointer.
DevSourceMapLookup sourceMapLookup(Uint8List sourceMap) {
  final nodes = fbs.SourceMap(sourceMap).nodes ?? const <fbs.Location>[];
  final pointers = <String, String>{};
  for (final l in nodes) {
    final section = l.section;
    final pointer = l.pointer;
    if (section == null || pointer == null) continue;
    pointers['${_uuid(section.hi, section.lo)}#${l.index}'] = pointer;
  }
  return (section, index) => pointers['$section#$index'];
}

/// A section ID in the canonical text of its UUID.
String _uuid(int hi, int lo) {
  String hex(int v) =>
      ((v >> 32) & 0xffffffff).toRadixString(16).padLeft(8, '0') +
      (v & 0xffffffff).toRadixString(16).padLeft(8, '0');
  final s = hex(hi) + hex(lo);
  return '${s.substring(0, 8)}-${s.substring(8, 12)}-${s.substring(12, 16)}-'
      '${s.substring(16, 20)}-${s.substring(20)}';
}

/// The section ID of the page or component section whose ID bytes are
/// [id], as [DevNodeInfo.section] holds it.
String devSectionId(Uint8List id) {
  final v = ByteData.sublistView(id);
  return _uuid(v.getUint64(0), v.getUint64(8));
}

/// Carries out `inspectSelect` and reports the nodes tapped in inspect mode
/// as `inspect` events.
final class DevInspectHandler implements DevCommandHandler {
  /// Creates the handler. It takes over [DevNodeHost.onInspect] of [host].
  DevInspectHandler({required this.host, required this.sink}) {
    host.onInspect = _report;
  }

  /// The nodes on screen.
  final DevNodeHost host;

  /// Receives the `inspect` events.
  final DevEventSink sink;

  void _report(DevNodeInfo info) {
    host.selected = info.path;
    sink.emit({
      'inspect': {'nodePath': host.documentPath(info), 'widget': info.widget},
    });
  }

  @override
  Future<Map<String, Object?>> handle(Map<String, Object?> command) async {
    final body = devCommandBody(command, 'inspectSelect');
    if (body == null) {
      return devFailure(devStatusFailed, 'inspectSelect is not an object');
    }
    final path = body['nodePath'];
    if (path is! String) {
      return devFailure(devStatusFailed, 'inspectSelect needs a nodePath');
    }
    if (path.isEmpty) {
      host.selected = null;
      return {'status': devStatusOk};
    }
    final marker = host.find(path);
    if (marker == null) {
      return devFailure(devStatusFailed, 'no node $path is on screen');
    }
    host.selected = marker.info.path;
    return {'status': devStatusOk};
  }
}

/// The nearest node marker under [globalPosition] in the render tree of
/// [root], or null.
RenderDevNode? devNodeAt(RenderBox root, Offset globalPosition) {
  final result = BoxHitTestResult();
  root.hitTest(result, position: root.globalToLocal(globalPosition));
  for (final entry in result.path) {
    final target = entry.target;
    if (target is RenderDevNode) return target;
  }
  return null;
}

/// Catches taps over the app while inspect mode is on and selects the node
/// under the finger in the app below, found through [appKey].
final class DevInspectBarrier extends StatelessWidget {
  /// Creates the barrier.
  const DevInspectBarrier({
    super.key,
    required this.host,
    required this.appKey,
  });

  /// The nodes on screen.
  final DevNodeHost host;

  /// The key of the app the barrier lies over.
  final GlobalKey appKey;

  @override
  Widget build(BuildContext context) => GestureDetector(
    behavior: HitTestBehavior.opaque,
    onTapUp: (details) {
      final root = appKey.currentContext?.findRenderObject();
      if (root is! RenderBox) return;
      final marker = devNodeAt(root, details.globalPosition);
      if (marker != null) host.onInspect?.call(marker.info);
    },
    child: const SizedBox.expand(),
  );
}

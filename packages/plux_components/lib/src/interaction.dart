// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The Layer 1 interaction widgets of this package (WGT-020, ADR-0071):
/// `Draggable`, `DragTarget` and `InteractiveViewer`.
library;

import 'package:flutter/material.dart';
import 'package:plux_components/src/ids.dart';
import 'package:plux_flutter/plux_flutter.dart';

/// The builders of this group, by permanent widget ID.
final Map<int, NodeBuilder> interactionBuilders = {
  PluxComponentIds.draggable: _draggable,
  PluxComponentIds.dragTarget: _dragTarget,
  PluxComponentIds.interactiveViewer: _interactiveViewer,
};

Alignment? _plainAlignment(NodeContext c, int id) {
  final a = c.decode(id, decodeAlignment);
  return switch (a) {
    Alignment() => a,
    AlignmentDirectional() => a.resolve(c.textDirection),
    _ => null,
  };
}

Widget _draggable(NodeContext c) => Draggable<Object>(
  data: c.prop(1),
  axis: c.decode(2, decodeAxis),
  feedbackOffset: c.decode(3, decodeOffset) ?? Offset.zero,
  maxSimultaneousDrags: c.decode(4, asInt),
  ignoringFeedbackSemantics: c.decode(5, asBool) ?? true,
  ignoringFeedbackPointer: c.decode(6, asBool) ?? true,
  rootOverlay: c.decode(7, asBool) ?? false,
  hitTestBehavior:
      c.decode(8, decodeHitTestBehavior) ?? HitTestBehavior.deferToChild,
  onDragStarted: c.handles(1) ? () => c.fire(1) : null,
  onDragEnd: c.handles(2) ? (d) => c.fire(2, d.wasAccepted) : null,
  onDragCompleted: c.handles(3) ? () => c.fire(3) : null,
  onDraggableCanceled: c.handles(4) ? (_, _) => c.fire(4) : null,
  feedback: keepScope(c.context, c.slot(2) ?? c.missing('feedback')),
  childWhenDragging: c.slot(3),
  child: c.slot(1) ?? c.missing('child'),
);

Widget _dragTarget(NodeContext c) {
  final accepts = c.prop(1);
  final tint = c.decode(2, asColor);
  final child = c.slot(1) ?? c.missing('child');
  return DragTarget<Object>(
    hitTestBehavior:
        c.decode(3, decodeHitTestBehavior) ?? HitTestBehavior.translucent,
    onWillAcceptWithDetails: (d) => accepts == null || d.data == accepts,
    onAcceptWithDetails: c.handles(1) ? (d) => c.fire(1, d.data) : null,
    onLeave: c.handles(2) ? (_) => c.fire(2) : null,
    builder: (context, candidates, rejected) {
      if (candidates.isEmpty) return child;
      final color =
          tint ?? Theme.of(context).colorScheme.primary.withValues(alpha: 0.16);
      return DecoratedBox(
        position: DecorationPosition.foreground,
        decoration: BoxDecoration(color: color),
        child: child,
      );
    },
  );
}

Widget _interactiveViewer(NodeContext c) => InteractiveViewer(
  clipBehavior: c.decode(1, decodeClip) ?? Clip.hardEdge,
  panAxis: c.decode(2, decodePanAxis) ?? PanAxis.free,
  boundaryMargin: c.decode(3, decodeEdgeInsetsResolved) ?? EdgeInsets.zero,
  constrained: c.decode(4, asBool) ?? true,
  maxScale: c.decode(5, asDouble) ?? 2.5,
  minScale: c.decode(6, asDouble) ?? 0.8,
  interactionEndFrictionCoefficient: c.decode(7, asDouble) ?? 0.0000135,
  panEnabled: c.decode(8, asBool) ?? true,
  scaleEnabled: c.decode(9, asBool) ?? true,
  scaleFactor: c.decode(10, asDouble) ?? 200.0,
  alignment: _plainAlignment(c, 11),
  trackpadScrollCausesScale: c.decode(12, asBool) ?? false,
  onInteractionStart: c.handles(1) ? (_) => c.fire(1) : null,
  onInteractionEnd: c.handles(2) ? (_) => c.fire(2) : null,
  child: c.slot(1) ?? c.missing('child'),
);

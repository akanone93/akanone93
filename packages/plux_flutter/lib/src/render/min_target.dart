// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The widgets whose descriptor sets `minTarget` (A11Y-004): the renderer
/// holds their interactive area to at least [minTargetSize] logical pixels
/// square. The set [minTargetTypes] is generated from the descriptors.
library;

import 'package:flutter/foundation.dart'
    show TargetPlatform, defaultTargetPlatform;
import 'package:flutter/rendering.dart';
import 'package:flutter/widgets.dart';
import 'package:plux_flutter/src/schema/registry.g.dart';

/// The smallest interactive area in logical pixels (A11Y-004): 44 on iOS,
/// 48 on every other platform.
double get minTargetSize =>
    defaultTargetPlatform == TargetPlatform.iOS ? 44 : 48;

/// The permanent IDs of the widgets in [minTargetTypes].
final Set<int> minTargetWidgets = {
  for (final d in widgetDescriptors)
    if (minTargetTypes.contains(d.type)) d.id,
};

/// The types of the icon-only buttons: their own tap node has no text to
/// read, so a semantics label on the node names that tap node (A11Y-001).
const Set<String> iconOnlyButtonTypes = {'FloatingActionButton', 'IconButton'};

/// The permanent IDs of the widgets in [iconOnlyButtonTypes].
final Set<int> iconOnlyButtonWidgets = {
  for (final d in widgetDescriptors)
    if (iconOnlyButtonTypes.contains(d.type)) d.id,
};

/// A segmented [control] whose segments keep their look and size but whose
/// semantics reach [minTargetSize] in height (A11Y-004): the control's own
/// segment nodes are 28 points high, so they are hidden from the
/// accessibility tree and one node per segment of the full height of this
/// widget takes their place. Each node reads the segment's own label and
/// state and selects it on a tap; it draws nothing and takes no pointer.
final class MinTargetSegments extends StatelessWidget {
  /// Creates the wrapper.
  const MinTargetSegments({
    required this.control,
    required this.segments,
    required this.selected,
    required this.onSelect,
    super.key,
  });

  /// The control, drawn as it is.
  final Widget control;

  /// The segment widgets in order, as they are given to [control].
  final List<Widget> segments;

  /// The index of the selected segment, or null.
  final int? selected;

  /// Selects the segment at an index; null when the control is disabled.
  final void Function(int index)? onSelect;

  @override
  Widget build(BuildContext context) {
    final select = onSelect;
    return ConstrainedBox(
      constraints: BoxConstraints(minHeight: minTargetSize),
      child: Stack(
        alignment: Alignment.center,
        children: [
          ExcludeSemantics(child: control),
          Positioned.fill(
            child: _PointerTransparent(
              child: Row(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  for (var i = 0; i < segments.length; i++)
                    Expanded(
                      child: Semantics(
                        container: true,
                        button: true,
                        selected: i == selected,
                        inMutuallyExclusiveGroup: true,
                        onTap: select == null ? null : () => select(i),
                        child: Center(
                          child: Opacity(
                            opacity: 0,
                            alwaysIncludeSemantics: true,
                            child: segments[i],
                          ),
                        ),
                      ),
                    ),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }
}

/// Takes no pointer events but keeps its subtree's semantics and does not
/// block assistive actions: [IgnorePointer] with `ignoringSemantics: false`,
/// which is deprecated, without its deprecated parameter.
final class _PointerTransparent extends SingleChildRenderObjectWidget {
  const _PointerTransparent({required super.child});

  @override
  RenderObject createRenderObject(BuildContext context) =>
      _RenderPointerTransparent();
}

final class _RenderPointerTransparent extends RenderProxyBox {
  @override
  bool hitTest(BoxHitTestResult result, {required Offset position}) => false;
}

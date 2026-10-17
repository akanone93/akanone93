// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// How a node's semantics, test ID and text direction wrap its widget
/// (A11Y-001, A11Y-004, A11Y-006, I18N-005).
library;

import 'package:flutter/semantics.dart' show OrdinalSortKey;
import 'package:flutter/widgets.dart';
import 'package:plux_flutter/src/bundle/fbs/bundle_fbs_generated.dart' as fbs;
import 'package:plux_flutter/src/render/min_target.dart';

/// Wraps the box widget [w] of a node: the minimum touch target when
/// [minTarget], the node's own [direction] over the inherited one, then its
/// semantics [sem] and [testId]. [text] resolves a semantics string. With
/// [namesTapNode] the label merges into the widget's own tap node, so a
/// screen reader reads it on the control instead of on a node above it.
Widget wrapSemantics(
  Widget w, {
  required fbs.Semantics? sem,
  required fbs.TextDirectionOverride direction,
  required String? testId,
  required String? Function(fbs.Value?) text,
  bool minTarget = false,
  bool namesTapNode = false,
}) {
  var out = w;
  if (minTarget) {
    out = ConstrainedBox(
      constraints: const BoxConstraints(
        minWidth: minTargetSize,
        minHeight: minTargetSize,
      ),
      child: out,
    );
  }
  final dir = switch (direction) {
    fbs.TextDirectionOverride.Ltr => TextDirection.ltr,
    fbs.TextDirectionOverride.Rtl => TextDirection.rtl,
    _ => null,
  };
  if (dir != null) out = Directionality(textDirection: dir, child: out);
  if (sem == null && testId == null) return out;
  if (sem != null && (sem.exclude || sem.decorative)) {
    out = ExcludeSemantics(child: out);
  }
  // A group reads as one unit: its descendants merge into one node.
  if (sem != null && sem.group) out = MergeSemantics(child: out);
  final level = sem != null && sem.headingLevel > 0 ? sem.headingLevel : null;
  final named = Semantics(
    identifier: testId,
    label: text(sem?.label),
    hint: text(sem?.hint),
    value: text(sem?.value),
    header: sem != null && (sem.header || level != null) ? true : null,
    headingLevel: level,
    button: sem != null && sem.button ? true : null,
    liveRegion: sem != null && sem.liveRegion ? true : null,
    sortKey: sem != null && sem.focusOrder >= 0
        ? OrdinalSortKey(sem.focusOrder)
        : null,
    child: out,
  );
  return namesTapNode && sem?.label != null
      ? MergeSemantics(child: named)
      : named;
}

/// Wraps the sliver [w] of a node: a box `Semantics` around a sliver
/// breaks the viewport, so the semantics go in `SliverSemantics`.
Widget wrapSliverSemantics(
  Widget w, {
  required fbs.Semantics? sem,
  required String? testId,
  required String? Function(fbs.Value?) text,
}) {
  if (sem == null && testId == null) return w;
  final level = sem != null && sem.headingLevel > 0 ? sem.headingLevel : null;
  return SliverSemantics(
    identifier: testId,
    label: text(sem?.label),
    hint: text(sem?.hint),
    value: text(sem?.value),
    header: sem != null && (sem.header || level != null) ? true : null,
    headingLevel: level,
    button: sem != null && sem.button ? true : null,
    liveRegion: sem != null && sem.liveRegion ? true : null,
    container: sem != null && sem.group,
    excludeSemantics: sem != null && (sem.exclude || sem.decorative),
    sortKey: sem != null && sem.focusOrder >= 0
        ? OrdinalSortKey(sem.focusOrder)
        : null,
    sliver: w,
  );
}

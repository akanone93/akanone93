// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The features this runtime supports, for `required_features` (BND-008):
/// `pxl.v1`, `pxl.regex.v1` and `pxl.phone.v1` (0.3.0), `navigation.guards.v1`
/// (route guards, ADR-0040), `data.v1` (the data layer, ADR-0048) with `data.streams.v1`, `data.outbox.v1` and
/// `data.transfers.v1`, the state
/// engine's `state.write.v1`, `state.computed.v1` and `state.persistence.v1`,
/// forms' `forms.v1` (ADR-0047) and `forms.sensitive.v1`, `security.secureui.v1`, the device actions' `device.v1`,
/// confidential bundles' `security.confidential.v1` (SEC-053, ADR-0055),
/// animation's `anim.v1`, `anim.timelines.v1` and `anim.transitions.v1`
/// (ANI-001–ANI-006, NAV-010), the local database's `db.v1` (ADR-0049), the
/// experiment variants' `experiments.variant.v1` (ABT-002), the
/// device-function section `section.wasm.v1` when the host registers a
/// function engine (FN-050, FN-051), and
/// registry revisions — `widget.<Type>.v<n>`,
/// `type.<Name>.v<n>`, `enum.<Name>.v<n>` — up to the revision this
/// runtime's generated registry knows, for widgets it can build.
library;

import 'package:plux_flutter/src/schema/registry.g.dart';
import 'package:plux_flutter/src/security/content_key.dart';

/// Answers whether this runtime supports a required feature.
final class RuntimeFeatures {
  /// Creates the answer for a runtime that builds the widgets [canBuild]
  /// accepts; by default, every widget of the registry.
  RuntimeFeatures({
    bool Function(String type)? canBuild,
    this.deviceFunctions = false,
  }) : _canBuild = canBuild ?? ((_) => true);

  final bool Function(String type) _canBuild;

  /// Whether the host registered a function engine (`plux_functions`), so
  /// this runtime can run the device-placed functions of a bundle's wasm
  /// section (FN-050, FN-051).
  final bool deviceFunctions;

  static final Map<String, int> _widgets = {
    for (final w in widgetDescriptors) w.type: w.revision,
  };
  static final Map<String, int> _types = {
    for (final t in valueTypeDescriptors) t.name: t.revision,
  };
  static final Map<String, int> _enums = {
    for (final e in enumDescriptors) e.name: e.revision,
  };

  /// The PXL features this runtime evaluates.
  static const Set<String> pxl = {'pxl.v1', 'pxl.regex.v1', 'pxl.phone.v1'};

  /// The navigation features this runtime honours: a bundle with a guarded
  /// page requires them, so a runtime without guards never opens it.
  static const Set<String> navigation = {'navigation.guards.v1'};

  /// The action engine features this runtime runs, first in 0.3.0:
  /// triggers, concurrency policies and detached runs, retries, flows and
  /// R1's control actions (ACT-002–ACT-006, ACT-061).
  static const Set<String> actions = {
    'actions.triggers.v1',
    'actions.concurrency.v1',
    'actions.retry.v1',
    'actions.flows.v1',
    'actions.control.v1',
  };

  /// The data layer (ADR-0048): a bundle declaring a REST or GraphQL
  /// source requires it.
  static const Set<String> data = {'data.v1'};

  /// The data layer's streams, offline outbox and file transfers
  /// (DAT-012, DAT-020, DAT-031): a bundle declaring a WebSocket, SSE or
  /// subscription source, an offline-capable mutation or a transfer
  /// requires them.
  static const Set<String> dataIo = {
    'data.streams.v1',
    'data.outbox.v1',
    'data.transfers.v1',
  };

  /// The state engine's features (STA-*): writes, computed entries and
  /// session, persisted and secure entries.
  static const Set<String> state = {
    'state.write.v1',
    'state.computed.v1',
    'state.persistence.v1',
  };

  /// Forms (STA-020, ADR-0047): a bundle declaring a form or running a
  /// form action requires `forms.v1`, and one tagging a form field
  /// `sensitive` requires `forms.sensitive.v1` (SCH-012, first in 0.4.0).
  static const Set<String> forms = {'forms.v1', 'forms.sensitive.v1'};

  /// The feedback and device actions (SEC-080): a bundle running
  /// `showSnackbar`, `haptic`, `openUrl`, `pickImage` or another of them
  /// requires it.
  static const Set<String> device = {'device.v1'};

  /// Animation (ANI-*): implicit, enter and exit and hero animation,
  /// timelines, and custom route transitions.
  static const Set<String> animation = {
    'anim.v1',
    'anim.timelines.v1',
    'anim.transitions.v1',
  };

  /// The local database (DB-001, ADR-0049): a bundle declaring a collection
  /// or running a database action requires it.
  static const Set<String> db = {'db.v1'};

  /// Confidential bundles (SEC-053): a release whose bundles arrive encrypted
  /// requires it, so a runtime that cannot decrypt them never downloads one.
  static const Set<String> security = {confidentialFeature};

  /// The secure UI (SEC-090–SEC-094, ADR-0059): a bundle with a secure
  /// widget or page, or an app that turns on the inactivity lock or screen
  /// protection, requires it, first in 0.4.0.
  static const Set<String> secureUi = {'security.secureui.v1'};

  /// Experiment variants (ABT-001, ABT-002, ADR-0079): a bundle with a
  /// `Variant` node requires it.
  static const Set<String> experiments = {'experiments.variant.v1'};

  /// Device-placed functions (FN-051): a bundle whose plugin calls one
  /// carries the linked module in a wasm section and requires it. Without
  /// an engine the bundle is refused before download, as for any feature
  /// this runtime lacks (BND-008).
  static const Set<String> functions = {'section.wasm.v1'};

  /// Whether [feature] is supported.
  bool supports(String feature) {
    if (functions.contains(feature)) return deviceFunctions;
    if (forms.contains(feature) ||
        security.contains(feature) ||
        secureUi.contains(feature) ||
        device.contains(feature) ||
        animation.contains(feature) ||
        db.contains(feature) ||
        experiments.contains(feature) ||
        pxl.contains(feature) ||
        navigation.contains(feature) ||
        actions.contains(feature) ||
        data.contains(feature) ||
        dataIo.contains(feature) ||
        state.contains(feature)) {
      return true;
    }
    final m = RegExp(
      r'^(widget|type|enum)\.([A-Za-z][A-Za-z0-9]*)\.v([1-9]\d*)$',
    ).firstMatch(feature);
    if (m == null) return false;
    final revision = int.parse(m[3]!);
    final known = switch (m[1]) {
      'widget' => _widgets[m[2]],
      'type' => _types[m[2]],
      _ => _enums[m[2]],
    };
    if (known == null || revision > known) return false;
    return m[1] != 'widget' || _canBuild(m[2]!);
  }
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// What a widget builder reads its node through (ADR-0031).
library;

import 'dart:typed_data';

import 'package:flutter/widgets.dart';
import 'package:http/http.dart' as http;
import 'package:plux_flutter/src/render/decoding.dart';

/// Builds the widget of one node.
typedef NodeBuilder = Widget Function(NodeContext c);

/// A node as its builder sees it: props by permanent ID after override
/// layers and bindings, children and slot fills as lazily built widgets,
/// item templates, and event wiring. Builders are generated from the
/// descriptors, or written by hand against the same generated decoders.
abstract interface class NodeContext implements Decoding {
  /// The build context of the node.
  BuildContext get context;

  /// The value of prop [id] in PXL form, or null when absent. A binding
  /// that fails is reported and reads as absent.
  Object? prop(int id);

  /// Prop [id] decoded by [decoder]; null when absent, and null after a
  /// report (PLX-4002) when present but not decodable.
  T? decode<T>(int id, T? Function(Decoding d, Object? v) decoder);

  /// Fails the node: a required prop, field or slot has no value.
  Never missing(String name);

  /// The node filling slot [id], built lazily; null when the slot is empty.
  Widget? slot(int id);

  /// A slot whose Flutter parameter must be a [PreferredSizeWidget], such as
  /// an app bar; the size is the built widget's own.
  PreferredSizeWidget? preferredSizeSlot(int id);

  /// The nodes filling list slot [id].
  List<Widget> slotList(int id);

  /// Whether slot [id] is filled.
  bool hasSlot(int id);

  /// The children, built lazily.
  List<Widget> children();

  /// Builds item [index] of item-template slot [slot], with `item` and
  /// `index` in scope for its bindings (WGT-012).
  Widget item(int slot, Object? item, int index);

  /// The renderer's pinned HTTP client for fetching [url], when [url] is
  /// HTTPS on a domain the plugin declares (SEC-080, AST-002); else null,
  /// reported (PLX-6030).
  http.Client? remoteClient(String url);

  /// A remote image at [url] under the same rule as [remoteClient]; null,
  /// reported (PLX-6030), when blocked.
  ImageProvider<Object>? remoteImage(String url);

  /// The bytes at [url] fetched through the pinned client under the same
  /// rule as [remoteClient], at most the asset size limit; null, reported,
  /// when blocked or the fetch failed.
  Future<Uint8List?> remoteBytes(String url);

  /// Where the platform player may read the media at [url] from: the
  /// address itself, or with `pinnedMediaOnly` (SEC-041) a temporary file
  /// the pinned client downloaded, at most the asset size limit. Null,
  /// reported, when the domain is not declared (PLX-6030) or the download
  /// failed.
  Future<Uri?> mediaSource(String url);

  /// Null when the plugin may use device API [api]; else the standard
  /// refusal to return instead of opening the camera, files or location,
  /// reported (PLX-5400) (SEC-080, X1.1).
  Widget? deviceRefusal(String api);

  /// Reports that an image of the node failed to load.
  void imageFailed(Object error);

  /// Whether the node handles event [id].
  bool handles(int id);

  /// Fires event [id] with [payload]. Actions arrive in P5: in P3 this
  /// reports PLX-4010 in debug builds and does nothing else (ADR-0031).
  void fire(int id, [Object? payload]);

  /// An input that keeps its value locally (ADR-0031): [build] receives the
  /// current value, starting at [initial] and following it when it
  /// changes, and a callback that updates it and fires event [event], or
  /// null when the node does not handle [event].
  Widget controlled<T>(
    int event,
    T initial,
    Widget Function(T value, ValueChanged<T>? onChanged) build,
  );

  /// A text input whose text lives in a controller the node keeps
  /// (ADR-0031): it starts at [initial] and follows it when it changes,
  /// while what the user types stays local.
  Widget text(
    String initial,
    Widget Function(TextEditingController controller) build,
  );
}

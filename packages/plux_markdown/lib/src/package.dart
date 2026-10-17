// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The widget package: builders for `MarkdownView` and `TermsViewer`. The
/// IDs are those of the descriptors in `schema/widgets/layer2`; a test
/// compares them.
library;

import 'package:flutter/widgets.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_markdown/src/markdown_view.dart';
import 'package:plux_markdown/src/terms_viewer.dart';

/// The widget IDs of the package's descriptors.
abstract final class PluxMarkdownIds {
  /// `TermsViewer`.
  static const int termsViewer = 191;

  /// `MarkdownView`.
  static const int markdownView = 192;
}

/// The Markdown widgets of Plux pages, to register in
/// `PluxConfig.widgetPackages`.
final class PluxMarkdown implements PluxWidgetPackage {
  /// Creates the package.
  const PluxMarkdown();

  @override
  String get name => 'plux_markdown';

  @override
  Map<int, NodeBuilder> get builders => {
    PluxMarkdownIds.termsViewer: _terms,
    PluxMarkdownIds.markdownView: _markdown,
  };
}

/// An address becomes a remote image, which the domain rule decides
/// (SEC-080, AST-002); anything else is an asset of the release.
MarkdownImageResolver _images(NodeContext c) =>
    (source) =>
        source.contains('://') ? c.remoteImage(source) : c.image(asset: source);

Widget _markdown(NodeContext c) => PluxMarkdownView(
  source: c.decode(1, asString) ?? c.missing('MarkdownView.source'),
  onLink: c.handles(1) ? (url) => c.fire(1, url) : null,
  imageResolver: _images(c),
);

Widget _terms(NodeContext c) => PluxTermsViewer(
  source: c.decode(1, asString) ?? c.missing('TermsViewer.source'),
  version: c.decode(2, asString) ?? c.missing('TermsViewer.version'),
  acceptLabel: c.decode(3, asString) ?? 'Accept',
  scrollHint: c.decode(4, asString) ?? 'Scroll to the end to accept',
  onAccepted: c.handles(1) ? (version) => c.fire(1, version) : null,
  onLink: c.handles(2) ? (url) => c.fire(2, url) : null,
  imageResolver: _images(c),
);

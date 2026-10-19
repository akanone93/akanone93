// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// `TermsViewer`: Markdown terms that scroll, with an accept button that
/// enables once the end was reached.
library;

import 'package:flutter/material.dart';
import 'package:plux_markdown/src/markdown_view.dart';

/// Terms in Markdown, scrolled to the end before they can be accepted.
final class PluxTermsViewer extends StatefulWidget {
  /// Creates the viewer.
  const PluxTermsViewer({
    required this.source,
    required this.version,
    this.acceptLabel = 'Accept',
    this.scrollHint = 'Scroll to the end to accept',
    this.onAccepted,
    this.onLink,
    this.imageResolver,
    super.key,
  });

  /// The terms as Markdown.
  final String source;

  /// The version of the terms, passed to [onAccepted].
  final String version;

  /// The accept button's label.
  final String acceptLabel;

  /// Shown, and spoken, while the end has not been reached.
  final String scrollHint;

  /// Called with [version] when the user accepts.
  final ValueChanged<String>? onAccepted;

  /// Called with the address of a tapped link.
  final ValueChanged<String>? onLink;

  /// Resolves image sources to bundle assets.
  final MarkdownImageResolver? imageResolver;

  @override
  State<PluxTermsViewer> createState() => _PluxTermsViewerState();
}

final class _PluxTermsViewerState extends State<PluxTermsViewer> {
  bool _atEnd = false;

  @override
  void didUpdateWidget(PluxTermsViewer old) {
    super.didUpdateWidget(old);
    if (old.source != widget.source || old.version != widget.version) {
      _atEnd = false;
    }
  }

  bool _metrics(ScrollMetrics m) {
    // Once the end was reached it stays reached.
    if (!_atEnd && m.extentAfter <= 1) setState(() => _atEnd = true);
    return false;
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return LayoutBuilder(
      builder: (context, constraints) {
        final height = constraints.hasBoundedHeight
            ? constraints.maxHeight
            : MediaQuery.sizeOf(context).height * 0.6;
        return SizedBox(
          height: height,
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Expanded(
                child: NotificationListener<ScrollMetricsNotification>(
                  onNotification: (n) => _metrics(n.metrics),
                  child: NotificationListener<ScrollNotification>(
                    onNotification: (n) => _metrics(n.metrics),
                    child: Scrollbar(
                      child: SingleChildScrollView(
                        padding: const EdgeInsets.symmetric(vertical: 8),
                        child: PluxMarkdownView(
                          source: widget.source,
                          onLink: widget.onLink,
                          imageResolver: widget.imageResolver,
                        ),
                      ),
                    ),
                  ),
                ),
              ),
              if (!_atEnd)
                Padding(
                  padding: const EdgeInsets.only(top: 8),
                  child: Semantics(
                    liveRegion: true,
                    child: Text(
                      widget.scrollHint,
                      style: theme.textTheme.bodySmall,
                    ),
                  ),
                ),
              Padding(
                padding: const EdgeInsets.only(top: 8),
                child: FilledButton(
                  onPressed: _atEnd && widget.onAccepted != null
                      ? () => widget.onAccepted!(widget.version)
                      : null,
                  child: Text(widget.acceptLabel),
                ),
              ),
            ],
          ),
        );
      },
    );
  }
}

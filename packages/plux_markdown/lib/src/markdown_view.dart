// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// `MarkdownView`: Markdown parsed with the `markdown` package into a tree
/// and drawn with Flutter text spans (ADR-0071). There is no HTML engine:
/// raw HTML in the source is shown as text. Links are reported to the page,
/// never opened here, so they go through the `openUrl` action; images are
/// bundle assets the host resolves.
library;

import 'dart:convert';

import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart';
import 'package:markdown/markdown.dart' as md;

/// Resolves the source of a Markdown image to a bundle asset; null shows the
/// image's alt text instead.
typedef MarkdownImageResolver = ImageProvider<Object>? Function(String source);

/// Parses [source] as CommonMark. HTML is not encoded: raw HTML stays in the
/// tree as plain text nodes, and is drawn as text.
List<md.Node> parseMarkdown(String source) => md.Document(
  extensionSet: md.ExtensionSet.commonMark,
  encodeHtml: false,
).parseLines(const LineSplitter().convert(source));

/// Formatted text from Markdown: headings (announced as headers),
/// paragraphs, emphasis, lists, block quotes, code and links.
final class PluxMarkdownView extends StatefulWidget {
  /// Creates the view of [source].
  const PluxMarkdownView({
    required this.source,
    this.onLink,
    this.imageResolver,
    super.key,
  });

  /// The Markdown text.
  final String source;

  /// Called with the address of a tapped link. Links look and act as links
  /// only when this is set.
  final ValueChanged<String>? onLink;

  /// Resolves image sources to bundle assets.
  final MarkdownImageResolver? imageResolver;

  @override
  State<PluxMarkdownView> createState() => _PluxMarkdownViewState();
}

final class _PluxMarkdownViewState extends State<PluxMarkdownView> {
  late List<md.Node> _nodes = parseMarkdown(widget.source);
  final List<TapGestureRecognizer> _recognizers = [];

  @override
  void didUpdateWidget(PluxMarkdownView old) {
    super.didUpdateWidget(old);
    if (old.source != widget.source) _nodes = parseMarkdown(widget.source);
  }

  @override
  void dispose() {
    _release();
    super.dispose();
  }

  void _release() {
    for (final r in _recognizers) {
      r.dispose();
    }
    _recognizers.clear();
  }

  @override
  Widget build(BuildContext context) {
    _release();
    final builder = _Builder(
      Theme.of(context),
      widget.onLink,
      widget.imageResolver,
      _recognizers,
    );
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      mainAxisSize: MainAxisSize.min,
      children: builder.blocks(_nodes),
    );
  }
}

const _inlineTags = {'em', 'strong', 'code', 'a', 'img', 'br', 'del'};

bool _isInline(md.Node n) =>
    n is md.Text || (n is md.Element && _inlineTags.contains(n.tag));

final class _Builder {
  _Builder(this.theme, this.onLink, this.images, this.recognizers);

  final ThemeData theme;
  final ValueChanged<String>? onLink;
  final MarkdownImageResolver? images;
  final List<TapGestureRecognizer> recognizers;

  TextStyle get _body => theme.textTheme.bodyMedium ?? const TextStyle();

  static const _gap = 12.0;

  List<Widget> blocks(List<md.Node> nodes) {
    final out = <Widget>[];
    var run = <md.Node>[];
    void flush() {
      if (run.isEmpty) return;
      out.add(_text(run, _body));
      run = [];
    }

    for (final n in nodes) {
      if (_isInline(n)) {
        run.add(n);
        continue;
      }
      flush();
      if (n is md.Element) out.add(_block(n));
    }
    flush();
    return [
      for (var i = 0; i < out.length; i++)
        Padding(
          padding: EdgeInsets.only(top: i == 0 ? 0 : _gap),
          child: out[i],
        ),
    ];
  }

  Widget _text(List<md.Node> nodes, TextStyle style) =>
      Text.rich(TextSpan(style: style, children: inline(nodes, style)));

  Widget _block(md.Element e) {
    switch (e.tag) {
      case 'h1' || 'h2' || 'h3' || 'h4' || 'h5' || 'h6':
        final t = theme.textTheme;
        final style =
            switch (e.tag) {
              'h1' => t.headlineMedium,
              'h2' => t.headlineSmall,
              'h3' => t.titleLarge,
              'h4' => t.titleMedium,
              'h5' => t.titleSmall,
              _ => t.labelLarge,
            } ??
            _body;
        return Semantics(
          header: true,
          child: _text(e.children ?? const [], style),
        );
      case 'p':
        return _text(e.children ?? const [], _body);
      case 'ul' || 'ol':
        return _list(e);
      case 'blockquote':
        return DecoratedBox(
          decoration: BoxDecoration(
            border: BorderDirectional(
              start: BorderSide(color: theme.colorScheme.outline, width: 3),
            ),
          ),
          child: Padding(
            padding: const EdgeInsetsDirectional.only(start: 12),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              mainAxisSize: MainAxisSize.min,
              children: blocks(e.children ?? const []),
            ),
          ),
        );
      case 'pre':
        return _codeBlock(e.textContent);
      case 'hr':
        return const Divider(height: 1);
      default:
        // An unknown block shows its text, never markup.
        return _text(e.children ?? const [], _body);
    }
  }

  Widget _codeBlock(String code) {
    final text = code.endsWith('\n')
        ? code.substring(0, code.length - 1)
        : code;
    return DecoratedBox(
      decoration: BoxDecoration(
        color: theme.colorScheme.surfaceContainerHighest,
        borderRadius: BorderRadius.circular(8),
      ),
      child: SingleChildScrollView(
        scrollDirection: Axis.horizontal,
        padding: const EdgeInsets.all(12),
        child: Text(
          text,
          textDirection: TextDirection.ltr,
          style: _body.copyWith(fontFamily: 'monospace'),
        ),
      ),
    );
  }

  Widget _list(md.Element e) {
    final ordered = e.tag == 'ol';
    final start = int.tryParse(e.attributes['start'] ?? '') ?? 1;
    final items = [
      for (final c in e.children ?? const <md.Node>[])
        if (c is md.Element && c.tag == 'li') c,
    ];
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      mainAxisSize: MainAxisSize.min,
      children: [
        for (var i = 0; i < items.length; i++)
          Padding(
            padding: EdgeInsets.only(top: i == 0 ? 0 : 4),
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                SizedBox(
                  width: 28,
                  child: Text(
                    ordered ? '${start + i}.' : '•',
                    style: _body,
                    textAlign: TextAlign.end,
                  ),
                ),
                const SizedBox(width: 8),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    mainAxisSize: MainAxisSize.min,
                    children: blocks(items[i].children ?? const []),
                  ),
                ),
              ],
            ),
          ),
      ],
    );
  }

  /// The spans of [nodes]; [link] is the recognizer of the link they are in,
  /// which every text leaf carries, as a span's recognizer does not reach
  /// its children.
  List<InlineSpan> inline(
    List<md.Node> nodes,
    TextStyle style, [
    TapGestureRecognizer? link,
  ]) => [for (final n in nodes) ..._span(n, style, link)];

  List<InlineSpan> _span(
    md.Node n,
    TextStyle style,
    TapGestureRecognizer? link,
  ) {
    if (n is md.Text) return [TextSpan(text: n.text, recognizer: link)];
    if (n is! md.Element) return const [];
    final kids = n.children ?? const <md.Node>[];
    switch (n.tag) {
      case 'em':
        return [
          TextSpan(
            style: const TextStyle(fontStyle: FontStyle.italic),
            children: inline(kids, style, link),
          ),
        ];
      case 'strong':
        return [
          TextSpan(
            style: const TextStyle(fontWeight: FontWeight.w700),
            children: inline(kids, style, link),
          ),
        ];
      case 'del':
        return [
          TextSpan(
            style: const TextStyle(decoration: TextDecoration.lineThrough),
            children: inline(kids, style, link),
          ),
        ];
      case 'code':
        return [
          TextSpan(
            text: n.textContent,
            recognizer: link,
            style: TextStyle(
              fontFamily: 'monospace',
              backgroundColor: theme.colorScheme.surfaceContainerHighest,
            ),
          ),
        ];
      case 'br':
        return [const TextSpan(text: '\n')];
      case 'a':
        final href = n.attributes['href'] ?? '';
        final callback = onLink;
        if (callback == null || href.isEmpty) {
          return inline(kids, style, link);
        }
        final recognizer = TapGestureRecognizer()..onTap = () => callback(href);
        recognizers.add(recognizer);
        return [
          TextSpan(
            style: TextStyle(
              color: theme.colorScheme.primary,
              decoration: TextDecoration.underline,
            ),
            children: inline(kids, style, recognizer),
          ),
        ];
      case 'img':
        return [_image(n)];
      default:
        return inline(kids, style, link);
    }
  }

  InlineSpan _image(md.Element e) {
    final alt = e.attributes['alt'] ?? '';
    final src = e.attributes['src'] ?? '';
    final image = images?.call(src);
    if (image == null) return TextSpan(text: alt);
    return WidgetSpan(
      alignment: PlaceholderAlignment.middle,
      child: Image(
        image: image,
        semanticLabel: alt.isEmpty ? null : alt,
        excludeFromSemantics: alt.isEmpty,
        errorBuilder: (_, _, _) => Text(alt),
      ),
    );
  }
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Markdown widgets for Plux pages: `MarkdownView` and `TermsViewer`
/// (ADR-0071). Register [PluxMarkdown] in `PluxConfig.widgetPackages`.
library;

export 'src/markdown_view.dart'
    show MarkdownImageResolver, PluxMarkdownView, parseMarkdown;
export 'src/package.dart' show PluxMarkdown, PluxMarkdownIds;
export 'src/terms_viewer.dart' show PluxTermsViewer;

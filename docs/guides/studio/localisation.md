<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Localisation

Studio shows every translation key of an app by locale and lets a reviewer change the status of
many messages at once. The concepts and the command line are in the
[localisation guide](../localisation.md) and the [translation workflow](../translation-workflow.md).

## See what is missing

Open **Localisation**. The **Heatmap** shows, for each plugin, how many of the keys it uses a locale
still lacks ("3 of 40 missing").

## Review in the grid

1. The **Grid** lists keys with the **Source** text, the **Translation** for the **Locale** you
   choose, the **Status**, the **Context** and the screens a key appears on. Filter by plugin,
   status (*Missing*, *New*, *Needs review*, *Reviewed*, *Approved*) or search keys and texts.
2. Select rows (or **Select all shown keys**) and choose **Approve**, **Mark as needs review** or
   **Copy from source**. Studio states how many messages change and asks you to confirm.
   Approving needs the `translations.approve` permission.
3. To change a translation, open the **Localisation table** from the plugin canvas. Choose the
   **Keys shown** (used by this plugin, used by the selection, or all keys of the app) and the
   **Locale to edit**. Keys are lower-case words joined by dots or hyphens, such as `home.title`;
   **Add key** needs the draft lock and the default locale's lock, and another editor holding the
   lock makes the table read-only.
4. A literal text on the canvas can be moved to a translation key (**Move to a translation key**, or the quick fix
   of the "literal text" warning). Editing a literal text in place (double-click it on the canvas) does the same on
   its own when you may edit the keys and the default locale: the new key is named after the page and the words.
   An extraction is one change across three documents (the key in `translations/keys.json`, its message in the
   default locale, and the reference on the page), and it applies as a whole or not at all: the page refers to the
   key only once the server has stored the key and its message. If the server refuses either, the other is rolled
   back, a message says why, and the text keeps its literal value; a page never points at a key that does not exist.
5. To use a key that already exists, select the text and open **Properties › Content**. The button at the end of the
   text's row (**Data** for a `Text`) switches it between a literal, a **Translation key** and a PXL expression;
   choose **Translation key** and pick the key. The list holds every key of the app, including one added on this
   canvas a moment ago, and the canvas then shows the key's message in the previewed locale. A key's message is
   one text per locale: it cannot point at another key.

## Add a locale, import and export

- **Add a locale** takes a code such as `de` or `pt-BR`.
- **Import translations** reads XLIFF, ARB, JSON or CSV into the locale you choose. The file is read
  first, so a file that is too large, not valid or empty is refused before anything changes.
- **Export translations** downloads XLIFF, ARB, JSON or CSV for translators.

Preview any locale on the canvas with the **Locale** switch, including right-to-left languages and
the pseudo-locales. Fallbacks and missing keys on devices: [Locale fallbacks](../../runbooks/locale-fallbacks.md);
a refused import: [Translation import](../../runbooks/translation-import.md).

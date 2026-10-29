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
4. A literal text on the canvas can be moved to a translation key (**Move to a translation key**).

## Add a locale, import and export

- **Add a locale** takes a code such as `de` or `pt-BR`.
- **Import translations** reads XLIFF, ARB, JSON or CSV into the locale you choose. The file is read
  first, so a file that is too large, not valid or empty is refused before anything changes.
- **Export translations** downloads XLIFF, ARB, JSON or CSV for translators.

Preview any locale on the canvas with the **Locale** switch, including right-to-left languages and
the pseudo-locales. Fallbacks and missing keys on devices: [Locale fallbacks](../../runbooks/locale-fallbacks.md);
a refused import: [Translation import](../../runbooks/translation-import.md).

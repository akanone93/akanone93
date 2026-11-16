<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Design System

The **Design** area of an app has five tabs: **Tokens**, **Themes**, **Typography**, **Icons** and
**Component library**. Pages use tokens, never fixed values, so a change here restyles every screen
and every theme ([theming reference](../../reference/theming.md)).

## Edit tokens

1. Open **Design tokens**. Each token has a **Light**, **Dark** and **High contrast** value. Studio keeps them where
   the device reads them ([theming reference](../../reference/theming.md) §3): the dark value in the token's
   `$extensions.dev.plux.dark`, the high-contrast value as the token at `contrast.high.<path>`.
2. **Edit {token}** changes a value, **Remove {token}** deletes one with its high-contrast and brand values, and
   **Add a token** takes a path, a **Type** and a **Value**. A colour has a colour picker beside its code; the code
   also takes an alpha (`#00000066`) or an alias. A value is stored as its type keeps it (`1.5` is a number
   for a `number` token, `Inter, Noto Sans` a list for a `fontFamily`), and `{color.primary}` makes the token an
   alias of another; the device follows aliases, and publishing reports one that names no token or loops.
3. **Import tokens** reads a DTCG JSON file, such as a Figma variables export: drop it or choose it, or paste its
   content. Studio compares it with the theme and reports how many tokens are **New**, **Changed** and
   **Unchanged**, and which are **Skipped** and why (name not allowed, type missing or unknown, value or dark value
   does not fit the type, nested too deeply); **Add to the theme** applies them, and is off when nothing would change.
4. **Export tokens** downloads the theme's tokens as the same DTCG file, to edit elsewhere and import back.

## Themes and brand overlays

**Themes** shows the colours per mode. **Create a brand overlay** replaces chosen tokens for one
brand, so one app serves several brands; the overlay is the group `brands.<brand>`, and the canvas's **Brand**
preview shows it as the device does.

## Typography and fonts

**Typography** sets one font family list per script (Latin, Cyrillic, Arabic, Devanagari and
others) with fallbacks, so no text shows missing glyphs. Type the families separated by commas; the list is saved
when you leave the field. Each list is the token `font.script.<code>` (`font.script.Ethi` for Ethiopic), whose
families the device and the canvas add to every text style's fallbacks. **Host app fonts** lists the fonts your
host app bundles: enter the **Font family**, pick a **Font file** from your uploaded assets (upload
a TrueType or OpenType file in the **Assets** panel first) and **Add font**. The canvas draws these
fonts, so measurements match the device. Changing the list needs permission to manage the app.

## Icons

**Icons** lists the icon sets and the icons each provides to the app, by name. **Add icon** takes an
icon name and its source, an asset or a code point.

## Reuse components and templates

The **Library** (organisation level) holds **Components** and **Templates**. Save a selection as a
template or extract a component from the canvas; browse by kind, visibility (*Private*,
*Organisation*, *Public*), author, category and tag. A component's page shows its **Versions** and
**Instances across apps**, so you see what a change touches before you publish a new version.
Saving a template records what it uses (design tokens, assets and components) as references, never
content. A property bound to a page-state entry (for example `page.amount`) becomes a template
parameter of the entry's type, filled with the entry's default; the second step of the dialog lists
them. Page state that cannot become a parameter, such as a reference inside a larger expression or
the state path of an action, stays in the template and is listed as flagged, so you can check it
after inserting. Apps **adopt** a version when they choose to. **Import** and **Export** move library items as files.

Studio's own look is separate: the [engineering handbook](../../engineering/studio-ui-language.md)
describes it.

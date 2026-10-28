<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Design System

The **Design** area of an app has five tabs: **Tokens**, **Themes**, **Typography**, **Icons** and
**Component library**. Pages use tokens, never fixed values, so a change here restyles every screen
and every theme ([theming reference](../../reference/theming.md)).

## Edit tokens

1. Open **Design tokens**. Each token has a **Light**, **Dark** and **High contrast** value.
2. **Edit {token}** changes a value, **Remove {token}** deletes one, and **Add token** takes a path,
   a **Type** and a **Value**.
3. **Import tokens** reads a DTCG JSON file, such as a Figma variables export. Studio reports how
   many tokens can be added and why others are skipped (name not allowed, type missing or unknown,
   value does not fit the type, nested too deeply); **Add to the theme** applies them.

## Themes and brand overlays

**Themes** shows the colours per mode. **Create a brand overlay** replaces chosen tokens for one
brand, so one app serves several brands.

## Typography and fonts

**Typography** sets one font family list per script (Latin, Cyrillic, Arabic, Devanagari and
others) with fallbacks, so no text shows missing glyphs. **Host app fonts** lists the fonts your
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
Apps **adopt** a version when they choose to. **Import** and **Export** move library items as files.

Studio's own look is separate: the [engineering handbook](../../engineering/studio-ui-language.md)
describes it.

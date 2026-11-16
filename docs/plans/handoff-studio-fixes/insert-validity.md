<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Widgets as the Insert pane creates them (build log #71)

Every Layer 1 widget built as **Insert** builds it (descriptor defaults, first values, starter content of #62),
placed in a screen's body Column and compiled with the conformance fixture. These are the errors each one has the
moment it is inserted, before the person touches it (the fixture's old runtime pin, PLX-1119, is left out).
30 of 149 widgets are invalid as inserted, down from 88 before
the starting values of #71. Most of those left lack only a semantics label (PLX-1401, PLX-1402), which Studio leaves
for the person to write: a placeholder label would pass the check and still tell a screen-reader user nothing. An
Image needs its picture and a FormScope a declared form; the buttons' dark-mode contrast comes from the fixture's
theme. The check is reproduced by building every widget with `buildNode` and compiling each on the fixture's result
page.

| Widget | Errors |
|---|---|
| `Autocomplete` | PLX-1401 has no semantics label and no text to announce |
| `BackButton` | PLX-1401 has no semantics label and no text to announce |
| `CalendarDatePicker` | PLX-1401 has no semantics label and no text to announce |
| `Checkbox` | PLX-1401 has no semantics label and no text to announce |
| `CircleAvatar` | PLX-1402 has no semanticLabel, no semantics.label and is not decorative |
| `CloseButton` | PLX-1401 has no semantics label and no text to announce |
| `CupertinoContextMenu` | PLX-1107 needs prop "actions" |
| `CupertinoDatePicker` | PLX-1401 has no semantics label and no text to announce |
| `CupertinoPicker` | PLX-1401 has no semantics label and no text to announce |
| `CupertinoSlider` | PLX-1401 has no semantics label and no text to announce |
| `CupertinoSwitch` | PLX-1401 has no semantics label and no text to announce |
| `CupertinoTextField` | PLX-1401 has no semantics label and no text to announce |
| `DropdownMenu` | PLX-1401 has no semantics label and no text to announce |
| `ElevatedButton` | PLX-1403 text has a contrast of dark 3.37:1, below 4.5:1 |
| `FilledButton` | PLX-1403 text has a contrast of dark 3.37:1, below 4.5:1 |
| `FormScope` | PLX-1240 FormScope.form must be the literal name of a form the page or component declares |
| `GestureDetector` | PLX-1401 has no semantics label and no text to announce |
| `Icon` | PLX-1402 has no semanticLabel, no semantics.label and is not decorative |
| `Image` | PLX-1107 needs prop "source"<br>PLX-1402 has no semanticLabel, no semantics.label and is not decorative |
| `InkWell` | PLX-1401 has no semantics label and no text to announce |
| `Match` | PLX-1111 needs slot "branches" |
| `Radio` | PLX-1401 has no semantics label and no text to announce |
| `RangeSlider` | PLX-1401 has no semantics label and no text to announce |
| `SearchAnchor` | PLX-1401 has no semantics label and no text to announce |
| `SearchBar` | PLX-1401 has no semantics label and no text to announce |
| `SegmentedButton` | PLX-1401 has no semantics label and no text to announce |
| `Slider` | PLX-1401 has no semantics label and no text to announce |
| `Switch` | PLX-1401 has no semantics label and no text to announce |
| `TextField` | PLX-1401 has no semantics label and no text to announce |
| `TextFormField` | PLX-1401 has no semantics label and no text to announce |

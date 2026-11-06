<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Canvas and Editing

Studio has two canvases. The **app canvas** shows the plugins of an app and how they link; the
**plugin canvas** shows the screens of one plugin and lets you edit them. Both draw with Plux
Canvas, and you can drive both from the keyboard.

## Create an organisation, an app and its first plugin

1. An installation administrator chooses **New organisation** in the top bar (or **Create
   organisation** on the page shown to a person who belongs to no organisation). Give it a name and
   a key. The key is 1 to 64 lower-case letters, digits and hyphens, starting and ending with a
   letter or digit; Studio suggests one from the name, and it cannot be changed later. Studio
   switches to the new organisation. If the server refuses (for example a quota), the dialog says
   why and nothing changes.
2. On the organisation home, people who hold the `app.manage` permission see **Create app**, on the
   list of apps and on the empty state. Give the app a name and a key as above. Studio opens the
   new app's canvas. People without the permission see the apps, but no button.
3. The new app has no plugins. Choose **New plugin** on its canvas to add the first one.

## Arrange the plugins of an app

1. Open the app: its card, or its detail card, on the organisation home. The **App canvas** fills the
   window: a dotted surface with a card per plugin and arrows for the links between them. A card
   shows the plugin's initial on a coloured tile, its name and key, its newest version (**v3**, or
   **Draft** before the first publish), how many screens, versions and links it has, and its status
   along the foot (for example **Valid**, **2 errors** or **Unpublished changes**); a **+N** pill
   counts the rest, which the detail panel lists; the line under the name adds the owning team. A
   **native** target (a screen of the host app) is a dashed outline with a **Native** chip. The
   canvas opens at 100 %, centred beside the dock. Panels float over the canvas as frosted glass, so
   the plugins stay in view beneath them:
   - top left, the way back to the organisation and the app's name and number of plugins;
   - top centre, the tools (**Select** and **Hand**), **Frame selection**, **Arrange**, **Trash** and
     **New plugin**;
   - top right, what is selected, whether your changes are saved, the validation errors over every
     plugin, and the light and dark switch;
   - on the start side, the **App canvas** dock: **Plugins** lists every plugin with its mark, key,
     owner, version and state — search it, or show only **Unpublished**, **With errors** or
     **Archived** ones; choose a row to select the plugin and bring it into view, double-click to
     open it. **Frames** lists the frames to rename or resize them, and **Overview** counts the
     plugins, screens, frames and problems and names the entry plugin. Fold the dock away with its
     arrow;
   - on the end side, the plugin's **detail panel**;
   - bottom left, **Zoom out**, the zoom (choose it to go back to 100%), **Zoom in** and **Zoom to
     fit**.
2. Move around as in a design tool. Scroll, or drag with two fingers, to pan; hold **Ctrl** (or
   **Cmd**) and scroll, or pinch, to zoom about the pointer. Drag with the **Hand** tool (**H**),
   with **Space** held, or with the middle mouse button to pan. **V** goes back to **Select**;
   **+** and **-** zoom, **Shift+1** fits everything, **Ctrl+0** resets the zoom and **Escape**
   clears the selection.
3. Hover a plugin, or move keyboard focus to it, to see its detail panel: the description, a
   mini-map of its screens and the links among them, its screens and versions, its bundle size,
   how many links go out and in, the functions it uses, and who published it last and when. Select
   a plugin to add its actions: **Open plugin**, **Plugin settings**, **Duplicate plugin**, **Make
   entry plugin**, **Move to…**, **Archive plugin** (or **Restore plugin**) and **Delete plugin**.
   Right-click a plugin for the same actions in its context menu.
4. Choose **New plugin**, give it a name and a key, and save.
5. Drag a plugin to move it, or use **Move to…** with X and Y positions. Near another plugin's edge or
   centre it snaps into line, and a guide shows what it lines up with; a plugin you drag stays on
   top of the ones it passes over. Drag on the background to select every plugin the rectangle
   meets. Frame a group with **Frame selection**, then rename or
   resize the frame from the **Plugins** panel. **Arrange** tidies the layout.
6. **Plugin settings** changes the description, tags and owning team.
7. **Delete plugin** moves a plugin to the **Trash** for 30 days; restoring it brings its links back.
   Deleting or deprecating a plugin of an app with a production release asks you to type its key.
   If other plugins link into it, **Redirect incoming links** to a new target screen first.

The canvas follows the light, dark and contrast themes.

Positions are read-only while someone else is editing the app.

## Edit a plugin

1. Double-click a plugin. The **Plugin canvas** shows its screens in navigation order on the same
   dotted surface as the app canvas, and zooms, pans and draws the same way; it opens at 100 %. The
   top bar has three parts: on the left the way back to the app canvas and the app over the plugin's
   name; in the centre **New screen**, the current screen (choose another to bring it into view),
   **Screen menu**, **Undo** and **Redo**, **Preview**, **History**, **Pair a device**, **Send to
   device** and **Publish**; on the right what is selected, whether your changes are saved, the error
   and warning counts (choose them to open **Problems**) and the light and dark switch. The zoom is at
   the bottom start, as on the app canvas. On the left, the **Plugin editor** dock names the plugin
   and opens **Insert**, **Layers**, **Assets** and **Data** from its coloured rail; on the right is
   the **Inspector**.
2. Work on the canvas as in a design tool. **Select** (V) picks and moves, **Hand** (H) pans; Space held, the middle
   mouse button, the wheel or two fingers pan with either tool, and Ctrl or Cmd with the wheel (a pinch) zooms.
   Point at a screen and the widget a click selects is outlined with its name. A click selects the top-level widget
   under the pointer; Ctrl-click (Cmd-click) selects the innermost; Shift-click adds or removes; double-click goes one
   level in at the pointer, Esc one level out (from a top-level widget to its screen, then nothing); Enter selects the
   first child and Tab the next sibling. The selection is outlined with its name, its size and, where it has a width
   and a height, eight handles that resize it. Drag a selected widget to move it: in a column, row or wrap a line
   shows where it lands among its siblings or in another container (outlined in green); in a stack it moves to where
   you drop it (always inside its stack), with guides and distances to its siblings; the arrow keys nudge by 1
   (Shift: 10). Click a screen's
   name above its frame to select the screen, and drag the name — or a selected screen's body — to move it with every
   selected screen; it snaps to the others and stays where you drop it (Undo puts it back). Double-click the name to
   rename the screen. Drag on the empty canvas, or across a screen that is not selected, to select the screens or the
   widgets the rectangle meets (Alt: only those inside it). Right-click a widget, a screen or the empty canvas for its
   actions: copy, cut, paste, duplicate, wrap, unwrap, select the parent or the first child, zoom to the selection
   (Shift+2) and delete (Delete or Backspace); a screen's rename, duplicate, move and delete; a new screen and fit all
   (Shift+1). The current screen in the top bar follows the selection.
3. Insert a widget from **Insert**: search it, open **Categories** to show one kind (**Layout**,
   **Content**, **Inputs**, **Interaction**, **Navigation**, **Motion**, **Platform**, **More**), or
   use **Recently used**; point at a tile to read what the widget does, then choose it or drag it
   onto a screen. **Layers** shows each screen over its widgets with their icons and what they show:
   choose a caret to fold a row, **Expand all** or **Collapse all**, **Filter layers** by name, type
   or text, and drag a layer to move it. Double-click a layer's name (or press F2) to rename it; the
   eye hides it on the canvas (Ctrl+Shift+H) and the lock keeps it from being selected, moved or edited
   (Ctrl+Shift+L). Names, hiding and locks are Studio's own: the device still shows the widget. The
   card at the top of **Layers** lists the selection, with **Show on canvas** and **Clear selection**.
   The fold button in each dock's header folds it; the button beside the folded dock (or a tab on its
   rail) opens it again, on both canvases and both sides. **Assets** takes files dropped on its zone (or chosen
   by clicking it) and shows them as cards you can search and filter by kind; **Data** holds the
   plugin's **State** and **Data sources**. Select a widget and edit it in the **Inspector** on the right: its header
   names the widget, its screen and its id, and its rail on the outer edge holds **Properties** (open at first),
   **Editors** (JSON, translations, theme, state, data sources, database, action graph, Rust), **Actions**, **JSON**,
   **Code**, **Localisation**, **Accessibility**, **Performance** and **Docs**. **Page** holds the settings of the
   page the selection is on: its title, description, kind, route name (a new name takes this plugin's links with it;
   links from other plugins block it), key, whether it is the plugin's entry, a dialog's result type, its typed
   parameters with defaults and design-time mocks, its state and data sources (each opens its editor), its lifecycle
   handlers, its transition, timeline and guards, and whether it blocks screenshots or needs an assurance level. A
   text field there changes the page when you leave it or press Enter; Escape takes the change back. **Properties** lists one property per
   row — its name, a dot when it is set, and its editor — in the sections **Content**, **Layout**, **Appearance**,
   **Behaviour** and **Other**; filter them by name, show **Set only**, pick the **Size class** an edit applies to
   (**M** or **E** marks an override), and point at a name to reset it. A text switches between a literal, a
   translation key and a PXL expression from the button at its end; short choices are segmented, an alignment is a
   3×3 grid of points, a padding is four sides in one row, and a decoration, a text style or a constraint opens its
   fields under its name. With nothing selected the **Inspector** shows the **Plugin overview**. Each editor opens over
   the canvas; **Close** or Escape returns. **Wrap selection in a column**, **Unwrap selection**,
   **Duplicate selection** and **Create experiment from selection** are on the selection's menu.
4. Manage screens with **New screen**, **Duplicate screen**, **Rename screen**, **Delete screen** and
   **Move to plugin**. The entry screen and a screen another screen navigates to cannot be deleted
   until you choose another entry or remove the link.
5. Under **Preview**, use the preview switches to see the screen on a **Device frame** (or a **Custom frame**), in
   **Light** or **Dark**, in any **Locale**, at a **Text scale**, in **Material** or **Cupertino**
   style, and in the **Success**, **Loading**, **Empty** or **Error** data state. Every screen is drawn at the chosen
   device's size, in **Portrait** or **Landscape** (**Orientation**), with the device's status bar, camera cutout
   (a punch hole, a notch or an island) and home indicator, gesture handle or navigation buttons; set **Device
   chrome** to **Hidden** to draw the screens without them (their space stays, as on the device). The **Device
   matrix** shows several frames at once, each with its own chrome. A right-to-left locale and the text scale change
   the screens only; Studio's own panels keep your Studio language and size.
6. The **Developer console** at the bottom lists validation errors, warnings and information under
   **Problems** as you type; the counts also appear on the plugin card. Open it to stream the paired
   devices' **Logs**, **Process flow**, **State**, **Network**, **Functions**, **Sync**,
   **Performance** and **Console**, with a source device, a level filter, search, **Pause** and
   **Export trace**; **On the device** opens a screen there, **Highlight selection on device** and
   **Capture screenshot** work on it. Drag the console's top edge, or use the arrow keys on it, to
   change its height.

Each screen's name is shown above its device frame, with its route name and kind (**Page**, **Dialog**, **Bottom
sheet** or **Full-screen dialog**) after it and **Entry** on the plugin's entry screen; drag the screen by either. A
button or a field shows its text inside its box. A page's app bar reaches under the status bar and keeps its title
below it, a `SafeArea` keeps its content clear of the status bar, the cutout and the home indicator, and the status
bar's icons turn light over a dark app bar or header, as the device draws them. A page's `Scaffold` is laid out at the
device's size with its parts where the device puts them: the app bar at the top, the navigation bar and persistent
footer buttons at the bottom above the home indicator, the floating action button at its location and the bottom sheet
above the bottom bar; a scrolling body keeps all its content, past the frame. Select a page to add what its scaffold
lacks from the chips on its frame — **App bar**, **Navigation bar**, **Action button** and **Drawer** — each as one
undo step; **Show drawer** draws the drawer open over its scrim, and selecting anything inside a drawer opens it too.
Showing a drawer changes only the canvas, never the page. **New screen** asks for its **Kind**: a **Page**, a
**Dialog**, a **Bottom sheet** or a **Full-screen dialog**, each starting from a fitting layout. A dialog page is drawn
as the device presents it — over the page that opens it, dimmed by a scrim, on Material 3's dialog card — and a bottom
sheet page at the bottom of that page on its sheet; a full-screen dialog is drawn as a page. A link to a dialog or
sheet page opens it as one (`openDialog`, `openBottomSheet`), and its arrow comes from the page that opens it. Choose
**Interact** in the tool bar (or press I) to use the screens as on a device: the wheel scrolls a screen's list or
scroll view inside its frame, a click switches a tab, a navigation destination or a segment (a choice bound to page
state switches everything bound to it, as a tab bar switches its view), the app bar's menu button opens the drawer and
its scrim closes it, back and close go to the page that opens the screen, and a button that opens a page brings that
page into view. Actions that call networks or functions are not run; the canvas names them — they run on a device.
Nothing you do there changes the plugin; V or Escape returns to editing. A selected widget shows its spacing: the
padding around its child striped in magenta and the gaps between its children in violet, each with its size. A widget
with an error or a warning has a red or amber dot on its corner, and its first problem shows under its size when it is
selected. Double-click down to a text to edit it in place: type, then press Enter (Shift+Enter breaks the line) or click
away to save, Escape to cancel. A text you write this way becomes a translation key named after the page and its
words, with your text as the default locale's message, when you may edit the app's keys and messages; a text already
bound to a key changes its message in the default locale. A selected widget that pads its child shows a short magenta
bar on each side of the child, and a row or column one in each gap between its children: drag a bar to change that
padding (with Alt both sides of the axis, with Shift all four) or the gap, live and as one undo step. A value near one
of the theme's spacing tokens snaps to it and is saved as that token (`space.md`), so the design keeps to the theme;
resizing a widget snaps its width and height to a dimension token near them the same way. An app bar shows the button it implies on the
device: the menu with a drawer, back on a page another opens, close on a full-screen dialog. Screens are drawn in the app theme's colours: switch **Theme** to **Dark**, or choose a **Brand** under
**Preview**, and they recolour. A colour property opens a picker with the theme's colours, its colour tokens and a
custom colour with its opacity; a decoration has its own editor for fill, gradient, corners (or a circle), border and
a shadow by Material elevation. Texts take Material 3's type scale, or the theme's `typography` tokens where it has
them; a text style opens an editor with the type-scale role (display to label) or a custom size, weight, letter
spacing, line height, colour, italic and line, and a text with **maxLines** ends in an ellipsis; a rich text draws each
span in its own style. Screens are laid out edge to edge, as on the device: a column, row, padding, size, alignment or
container follows its properties, and a page body that does not scroll is as high as the device. Icons are drawn
from Material Symbols and the Cupertino icons in their colour and size; an icon property opens a picker of every icon
of both sets, searched by name. The canvas draws Material Symbols at their default fill and weight. Images of the
app's uploaded assets are drawn in their fit and corners, and avatars in their image or colour with their initials;
drag an image from **Assets** onto a screen to add it, or pick one for an image property. A web image is loaded on the
device; the canvas shows its placeholder. Buttons, chips, checkboxes, radios, switches, sliders and progress
indicators look as Material 3 draws them, their labels and icons in the component's colours; a control without an
action is drawn disabled, as it is on the device until you add one. Text fields show their decoration — underline,
filled or outlined, the label resting or floating over a value, hint, prefix, suffix, icons, and the helper or error
text, the error in red; search bars are drawn as Material 3's. A page's app bar is its own, with its title, leading
button and actions; navigation bars, rails and tab bars show their selected destination or tab; list tiles, cards and
dividers are drawn as on the device. Cupertino widgets — buttons, switches, sliders, text fields, activity indicators,
navigation and tab bars, segmented controls — are drawn with iOS's colours; choose **Cupertino** as the platform style
under **Preview** and a control marked adaptive is drawn the iOS way. Layer 2 components show their content — money
amounts, card numbers, totals, list items — on their surfaces, as on the device. Charts are drawn from their data;
maps, scanners, videos, audio players, QR codes, animations and native slots show a preview of what the device draws.
The canvas previews with the same layout code the device runs; the device decides. Check anything
that matters with [a paired device](security-and-admin.md#pair-a-device).

## Link screens

Select a widget, open **Actions**, choose the **Event**, the **Target screen** (in this plugin or
another) and **Add link**. Or drag the link handle on the widget to a screen; the keyboard
equivalent is Enter on the handle, then choose a screen. If the event already does something you
choose **Replace** or **Add after**. Open the **Action graph** for anything a link cannot express:
add built-in, host, function and flow steps and connect their ports. The **State designer** edits
state entries (single value, list or map; in memory, for the session, saved on the device or in
secure storage; sensitive or not), and the **JSON editor** edits the document itself with a
comparison against the last published version. **Apply** writes only a valid document.

## Undo, locks and conflicts

- Every edit goes through one pipeline. `Mod+Z` and `Mod+Shift+Z` undo and redo, including after a
  restore, which is one undo step.
- One person edits a plugin at a time. Others can look around, select and copy; **Request edit**
  asks the holder to hand over, and **Take over** works once the holder has stopped editing. Your
  work up to a takeover is kept as a snapshot.
- Changes made while you are offline show as waiting. If the server's version moved, a conflict dialog shows
  where the two versions differ: **Keep mine**, **Take the server's** or **Copy mine as JSON**.
  Autosave is paused until you choose.
- **History** lists snapshots (edit, lock taken over, published, restored). Pick two to compare;
  **Restore** returns the plugin's documents to a snapshot.

See also [Concepts](../concepts.md), [Routing](../routing.md) and [Actions](../actions.md).

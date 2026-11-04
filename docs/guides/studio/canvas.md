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
2. Insert a widget from **Insert**: search it, open **Categories** to show one kind (**Layout**,
   **Content**, **Inputs**, **Interaction**, **Navigation**, **Motion**, **Platform**, **More**), or
   use **Recently used**; point at a tile to read what the widget does, then choose it or drag it
   onto a screen. **Layers** shows each screen over its widgets with their icons and what they show:
   choose a caret to fold a row, **Expand all** or **Collapse all**, **Filter layers** by name, type
   or text, and drag a layer to move it. Double-click a layer's name (or press F2) to rename it; the
   eye hides it on the canvas (Ctrl+Shift+H) and the lock keeps it from being selected, moved or edited
   (Ctrl+Shift+L). Names, hiding and locks are Studio's own: the device still shows the widget. The
   card at the top of **Layers** lists the selection, with **Show on canvas** and **Clear selection**.
   The fold button in each dock's header folds it; the button beside the folded dock (or a tab on its
   rail) opens it again, on both canvases and both sides. In
   the **Inspector**, **Editors** and **Details** fold separately. **Assets** takes files dropped on its zone (or chosen
   by clicking it) and shows them as cards you can search and filter by kind; **Data** holds the
   plugin's **State** and **Data sources**. Select a widget and edit it in the **Inspector**: it
   names the widget and its screen, opens the **Editors** (JSON, translations, state, data sources,
   database, theme, action graph, Rust), and holds the tabs **Properties** (with the **Size class**
   the edit applies to, and sections: **Content**, **Layout**, **Appearance**, **Behaviour**,
   **Other**), **Actions**, **JSON**, **Code**, **Localisation**, **Accessibility**, **Performance**
   and **Docs**; with nothing selected it shows the **Plugin overview**. Each editor opens over the
   canvas; **Close** or Escape returns. **Wrap selection in a column**, **Unwrap selection**,
   **Duplicate selection** and **Create experiment from selection** are on the selection's menu.
3. Manage screens with **New screen**, **Duplicate screen**, **Rename screen**, **Delete screen** and
   **Move to plugin**. The entry screen and a screen another screen navigates to cannot be deleted
   until you choose another entry or remove the link.
4. Under **Preview**, use the preview switches to see the screen on a **Device frame** (or a **Custom frame**), in
   **Light** or **Dark**, in any **Locale**, at a **Text scale**, in **Material** or **Cupertino**
   style, and in the **Success**, **Loading**, **Empty** or **Error** data state. The **Device matrix**
   shows several frames at once.
5. The **Developer console** at the bottom lists validation errors, warnings and information under
   **Problems** as you type; the counts also appear on the plugin card. Open it to stream the paired
   devices' **Logs**, **Process flow**, **State**, **Network**, **Functions**, **Sync**,
   **Performance** and **Console**, with a source device, a level filter, search, **Pause** and
   **Export trace**; **On the device** opens a screen there, **Highlight selection on device** and
   **Capture screenshot** work on it. Drag the console's top edge, or use the arrow keys on it, to
   change its height.

Each screen's name is shown above its device frame, and a button or a field shows its text inside its
box. Screens are drawn in the app theme's colours: switch **Theme** to **Dark**, or choose a **Brand** under
**Preview**, and they recolour. A colour property opens a picker with the theme's colours, its colour tokens and a
custom colour with its opacity; a decoration has its own editor for fill, gradient, corners (or a circle), border and
a shadow by Material elevation. Texts take Material 3's type scale, or the theme's `typography` tokens where it has
them; a text style opens an editor with the type-scale role (display to label) or a custom size, weight, letter
spacing, line height, colour, italic and line, and a text with **maxLines** ends in an ellipsis; a rich text draws each
span in its own style. Screens are laid out edge to edge, as on the device: a column, row, padding, size, alignment or
container follows its properties, and a page body that does not scroll is as high as the device. Icons are drawn
from Material Symbols and the Cupertino icons in their colour and size; an icon property opens a picker of every icon
of both sets, searched by name. The canvas draws Material Symbols at their default fill and weight. The canvas previews with the same layout code the device runs; the device decides. Check anything
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

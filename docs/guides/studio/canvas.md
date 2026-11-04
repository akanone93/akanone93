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
   shows the plugin's initial on a coloured tile, its name, its key and number of screens, and its
   status (for example **Valid**, **2 errors** or **Unpublished changes**); a **+N** pill counts the
   rest, which the detail panel lists. Panels float over the canvas as frosted glass, so the
   plugins stay in view beneath them:
   - top left, the way back to the organisation and the app's name and number of plugins;
   - top centre, the tools (**Select** and **Hand**), **Frame selection**, **Arrange**, **Trash** and
     **New plugin**;
   - on the start side, **Plugins**: every plugin and frame as a list; choose a row to select the
     plugin and bring it into view, double-click to open it, or fold the panel away;
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

1. Double-click a plugin. The **Plugin canvas** shows its screens in
   navigation order, with panels for **Insert**, **Layers**, **Screens**, **Selection** and **Assets**
   on the left.
2. Insert a widget from **Widgets**, **Components** or a template, select it, and edit its
   properties. **Wrap selection in a column**, **Unwrap selection**, **Duplicate selection** and
   **Create experiment from selection** are on the selection's menu.
3. Manage screens with **New screen**, **Duplicate screen**, **Rename screen**, **Delete screen** and
   **Move to plugin**. The entry screen and a screen another screen navigates to cannot be deleted
   until you choose another entry or remove the link.
4. Use the preview switches to see the screen on a **Device frame** (or a **Custom frame**), in
   **Light** or **Dark**, in any **Locale**, at a **Text scale**, in **Material** or **Cupertino**
   style, and in the **Success**, **Loading**, **Empty** or **Error** data state. The **Device matrix**
   shows several frames at once.
5. The **Problems** drawer lists validation errors, warnings and information as you type; the
   counts also appear on the plugin card.

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

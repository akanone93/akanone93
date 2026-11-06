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
with an error or a warning has a red or amber dot on its corner — every page is checked in full as it loads and again
shortly after each edit — and its problems (each with its code) show under its
size when it is selected; a warning about a timeline — an expensive animation, say — is pinned to the widget it
animates. Under each screen's corner its cost shows: its widgets against the page's node budget (amber above the count
the compiler reports, red over the limit), the timelines that run at once when there are more than the compiler
accepts, and its errors and warnings; the **Performance** tab lists every screen this way. Double-click down to a text to edit it in place: type, then press Enter (Shift+Enter breaks the line) or click
away to save, Escape to cancel. A text you write this way becomes a translation key named after the page and its
words, with your text as the default locale's message, when you may edit the app's keys and messages; a text already
bound to a key changes its message in the default locale. A selected widget that pads its child shows a short magenta
bar on each side of the child, and a row or column one in each gap between its children: drag a bar to change that
padding (with Alt both sides of the axis, with Shift all four) or the gap, live and as one undo step. A value near one
of the theme's spacing tokens snaps to it and is saved as that token (`space.md`), so the design keeps to the theme;
resizing a widget snaps its width and height to a dimension token near them the same way. A bar of quick actions stands
above the selected widget: **Wrap in** a container (a column, a row, a container, a padding, a card…), **Convert to** a
related widget (a column to a row, a filled button to an outlined one; the props both have and the children are kept),
**Add after** it a text, a button, an icon, a space, a divider, an image or a box, **Align in row or column** to line up
the children of the row or column it is in, **Duplicate**, **Add an action** (opens **Actions**) and **Delete**. Drag a widget anywhere: among its siblings, into
another row, column or stack, into an empty padding, centre, card, box or any other widget that holds one child, or
onto another screen, where it lands at the drop as one undo step across both pages. An image dragged from **Assets**
onto an image or an avatar replaces its picture; dropped anywhere else it adds an image. An app bar shows the button it implies on the
device: the menu with a drawer, back on a page another opens, close on a full-screen dialog. Screens are drawn in the app theme's colours: switch **Theme** to **Dark**, or choose a **Brand** under
**Preview**, and they recolour. A colour property opens a picker with the theme's colours, its colour tokens and a
custom colour with its opacity; a decoration has its own editor for fill, gradient, corners (or a circle), border and
a shadow by Material elevation. A property set per interaction state — a button's background, content colour,
outline, shape, elevation or padding — shows a chip for each state (**Default**, **Pressed**, **Hovered**,
**Focused**, **Disabled**, **Selected**, **Error**, **Dragged**), marked in its colour when it is set: pick one to edit
it, and the canvas draws the selected widget in that state, with Material 3's state layer over it, until you pick
**Default** again or select something else. A state left unset uses **Default**, as on the device. A shape has its
kind (rounded, pill, circle, bevelled or smooth rectangle), its corner radius and its outline. A number offers the
theme's dimension tokens beside it — radii first for a radius, spacing first for a gap or a padding — and a number bound
to a token shows the token with its pixels; **Unlink** keeps the pixels instead. **Copy style** (Ctrl+Alt+C,
Cmd+Option+C on a Mac) and **Paste style** (Ctrl+Alt+V) in a widget's menu carry its colours, decoration, borders,
shape, elevation and text style to other widgets, all in one undo step: each takes the properties it has of the same
type, so a button's style lands on buttons and a container's colour on boxes. A selected row, column, flex or wrap
shows **Auto layout** above its properties: its **Direction** (a row becomes a column keeping its settings), a grid of
nine points that aligns its children along and across it, **Distribute** (packed, space between, around or evenly),
**Stretch children**, the **Gap** (and a wrap's **Run gap**) and its **Size** (hug its children or fill its parent). A
widget in a row or column is sized **Hug** (by its content), **Fill** (it takes the free space, in an `Expanded`, with
its **Share** of it) or **Fixed** (its own width or height, or a `SizedBox` around it); a widget in a stack is pinned to
its edges from a box of four bars, with **Set the width** and **Set the height** — pinning puts it in a `Positioned`,
and taking off the last pin takes it out. A box's **Constraints** edit their least and most width and height (an empty
most is unbounded). Pick **Medium** or **Expanded** above the properties to edit what the widget does on wider windows:
a value set there overrides the smaller classes, and the widget lists the classes it overrides — click one to edit it,
or clear it. **Reset layout** clears the layout properties at the class you edit. A widget bound to an expression is
drawn with the value it would have on the device at design time: a page parameter's mock (else its default), the
default of the page's, plugin's or app's state, or a data source's mock in the state chosen under **Preview** — one
**Data state** for all sources, or one for each source (**Data: orders**). A bound widget has a small teal badge on its
corner (with a count when several of its properties are bound); select it to see each binding, its expression and the
value drawn, and the inspector shows **The canvas shows** under a bound field. A value only the device knows (a
computed state, a flag, an event) is drawn as the widget's default and marked with a dash. A list, grid or page view
with an item template is drawn as the device builds it: one copy of the template for each of its items (up to 24),
each with `item` and `index` bound to that item, from a list written in the page or the data source the items are
bound to. Click any copy to select the template. A list follows its data's state — a loading or failed source shows
the list's **loading** or **error** view, an empty one its **empty** view (a default view when it has none), and a
list that `hasMore` ends in a progress row. Select the list to see its **Items** and pick the state drawn under
**Show** (**From data**, **Items**, **Empty**, **Loading**, **Error**, **Loading more**); **Add an empty view**, **Add a
loading view** and **Add an error view** give the list a view for that state from a template — an error view's **Try
again** reloads the list's source — which you then design like any widget. Conditions are drawn the way the device
would decide them now: an `If` draws its **then** or **else** branch, a `Match` the branch of its value (or
**otherwise**), a `ForEach` one copy of its template per item, and a widget whose **Visible when** is false is left
out. The canvas works out comparisons (`page.count > 0`, `params.tab == "paid"`), negations, `isEmpty` and
`isNotEmpty`, joined by `&&` or `||`, from the same design-time values as bindings; anything else draws the first
branch and keeps the widget. Select a widget to edit its **Visible when** under **Conditions**, see what it is now
(**Now true**, **Now false**, or **Only the device can work this out**), and pick what the canvas draws — **From
data**, a branch, **Shown** or **Hidden** — to design a branch the data does not show. A page's forms are edited under **Page › Forms**: **Add a form**, name it
(bindings read it as `page.<form>`), and give it fields — a name, a type, an initial value and checks (**Required**,
**Length**, **Range**, **Pattern**, **Email**, **Phone**, **IBAN**, **Date range**, **Decimals**, **Custom rule**,
**Server check**), each with its options and message. Select a text field, a checkbox, a switch or a slider and pick
a field under **Form field** to bind it in one step: its value reads the field, a change writes it, leaving it marks it
touched, and a text field shows the field's error under it. Preview a form **Pristine**, with its **Errors** (the
checks run on the initial values, as the device would), **Valid** (filled with values every check accepts) or
**Submitting**; widgets bound to `page.<form>.status` or `.valid` follow. A widget whose events run actions has an
amber badge on its corner (with their count), and the selected one lists each event, its action and what it acts on.
Under **Actions › Quick actions**, pick the event and a common action — **Open a dialog**, **Open a sheet**, **Show a
message** (with an optional button), **Open a link**, **Go back**, **Set state** or **Submit a form** — and **Add
action**: it replaces what the event runs, or runs after it. **What its events run** lists every step, each with a
button to remove it. A selected widget that shows a message draws it at the bottom of its screen as the device's
snackbar shows it, above the navigation bar, with the action button lifted over it. An arrow's lane names the
parameters its link passes (`Orders → Order (orderId: item.id)`); pick a lane to give the page it opens each parameter
the page declares — an expression such as `item.id`, a number, `true` or `false`, or `"text"` in quotes — with the
required ones it misses named, and choose the transition the page **Opens with**. **Arrange by flow** (in the canvas
menu and the command palette) lays the screens out in columns, a column per step from the entry screen, the screens
no link reaches last. Every widget has **Motion** in its properties: an **Enter** and an **Exit** transition (fade,
scale, or a slide in any direction) with its duration and curve — **Play enter** and **Play exit** play it on the
canvas, as the device would — **Animate changes** (its numbers and colours animate when their bound value changes,
with a duration, a delay and a curve), a **Hero tag** (widgets of other screens with the same tag are outlined while
it is selected: it flies to them when one opens the other), and what it does when the device asks to **Reduce
motion**. A page's timelines are under **Page › Timelines**: **Add a timeline**, or open one on the canvas. Its strip
over the canvas plays and pauses it and scrubs it with the **Playhead** — the canvas draws the page at that moment — and
lists a row per track with its keyframes: click one to go to it, Shift+click to remove it. Select a widget, pick a
property and a value, and **Add keyframe** at the playhead; a new track starts from the widget's value. Its settings set
the duration, the repeats, **Loop**, **Reverse on repeat**, **Play when the page shows**, and **Driven by scroll**: a
scroll view whose offset moves the timeline over the **Scroll extent** — scroll it in **Interact** mode to see it move.
**Accessibility view** in the tool bar marks on every screen the order focus moves through its interactive widgets
(numbered; an explicit **Focus order** comes first, then reading order), each touch target smaller than the platform's
minimum (48 × 48 on Android, 44 × 44 on iOS, outlined with its size) and each text whose colour does not stand out
enough from what is behind it (4.5:1, 3:1 for large text, with its ratio), worked out from the theme's colours. The
inspector's **Accessibility** tab edits a widget's label and hint, **Header** with its **Heading level**, **Button**,
**Live region**, **Group**, **Decorative**, **Exclude semantics** and **Focus order**, and lists its accessibility
problems.

The **Locale** switch previews the screens in the app's own locales (and the pseudo-locales in development). Each
text takes its message along the fallback chain the device uses (`de-AT`, then `de`, then the app's default locale), a
plural, `select`, `number`, `date` or `time` message is formatted with its arguments' design-time values, with Latin
digits as on the device, and a node's **localeOverrides** for the locale replace its props (the language's first,
the exact locale's over them). **Localisation view** in the tool bar marks every text whose key has no message in the
locale (the device shows the default locale's), every text written in the page rather than as a key in an app with
more than one locale, and every text that does not fit its box in this locale (a cut line included). The inspector's
**Localisation** tab shows the selected widget's messages in the locale and whether each is translated, a plural
message's **Plural sample** (from the data, or 0, 1, 2, 5 or 21) the canvas draws it with, and the props it sets
**Only in** this locale, added and removed in place.

Each screen is drawn in the window size class of its frame, as the device draws it (Material 3: **Compact** below
600 dp, **Medium** to 839 dp, **Expanded** from 840 dp): a widget's **responsive** props for medium apply from 600 dp and
those for expanded over them from 840 dp, a **Responsive** shows the variant of the size class (else the next narrower
one), and a widget whose **Visible when** or an **If** reads `device.sizeClass` shows what that size class shows — in
the device matrix too, where each frame is drawn in its own. The inspector's **Properties** edits the layer of the
frame's size class. **Size classes view** in the tool bar tags each screen with its size class and width, and marks
the widgets a size-class layer changes there and the widgets shown only in some size classes; on a foldable it also
marks the controls and images that cross the fold, which the frame draws as a crease down the middle (across it in
landscape). On a page's navigation bar, **Make navigation adaptive** in the selection's toolbar keeps the bar on
compact screens and puts a navigation rail with the same destinations, selection and handler beside the body from
600 dp, extended from 840 dp.

The selection's **Theme** card lists each colour and size the widget sets. A value bound to a design token shows the
token, its value and how many widgets on the plugin's screens use it (choose it to select them all); change the value
there and every widget using the token changes on every screen at once, as do edits in the theme editors — the canvas
draws the theme as it is being edited, before it is saved. A value written in the widget shows **Not a token**, with
**Use** the theme's token of that same value or **Make a token**, which adds it to the theme under a name you can change
(`space.18`, `color.custom1`) and binds the widget to it.

Texts take Material 3's type scale, or the theme's `typography` tokens where it has
them; a text style opens an editor with the type-scale role (display to label) or a custom size, weight, letter
spacing, line height, colour, italic and line, and a text with **maxLines** ends in an ellipsis; a rich text draws each
span in its own style. Screens are laid out edge to edge, as on the device: a column, row, padding, size, alignment or
container follows its properties, and a page body that does not scroll is as high as the device. Icons are drawn
from Material Symbols and the Cupertino icons in their colour and size; an icon property opens a picker of every icon
of both sets, searched by name. The canvas draws Material Symbols at their default fill and weight. Images of the
app's uploaded assets are drawn in their fit and corners, and avatars in their image or colour with their initials;
drag an image from **Assets** onto a screen to add it, or pick one for an image property. A web image is loaded on the
device; the canvas shows its placeholder. Buttons, chips, checkboxes, radios, switches, sliders and progress
indicators look as Material 3 draws them, their labels and icons in the component's colours, and a button in its own
style where it sets one; a control without an
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

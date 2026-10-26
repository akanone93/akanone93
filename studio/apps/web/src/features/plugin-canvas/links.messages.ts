// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of link creation: the Actions tab, the trigger picker and the link handle (F6, E7, STU-047). */
export function linkMessages(i18n: I18n) {
  return {
    tab: i18n._({
      id: "links.tab",
      message: "Actions",
      comment: "Label of the properties tab that lists and creates the selected widget's navigation links.",
    }),
    nothing: i18n._({
      id: "links.nothing",
      message: "Select a widget to add a link.",
      comment: "Shown in the Actions tab when no widget is selected.",
    }),
    noEvents: i18n._({
      id: "links.noEvents",
      message: "This widget has no events a link can start from.",
      comment: "Shown in the Actions tab when the selected widget type declares no events.",
    }),
    heading: i18n._({
      id: "links.heading",
      message: "Navigate",
      comment: "Heading of the section of the Actions tab that creates a navigation link.",
    }),
    event: i18n._({
      id: "links.event",
      message: "Event",
      comment: "Label of the choice of the widget event that starts the navigation, for example onPressed.",
    }),
    target: i18n._({
      id: "links.target",
      message: "Target screen",
      comment: "Label of the choice of the screen the navigation goes to.",
    }),
    thisPlugin: i18n._({
      id: "links.thisPlugin",
      message: "This plugin",
      comment: "Marks screens of the plugin being edited in the list of navigation targets.",
    }),
    otherPlugin: i18n._({
      id: "links.otherPlugin",
      message: "Other plugin",
      comment: "Marks screens of another plugin of the app in the list of navigation targets.",
    }),
    targetOption: (title: string, scope: string) =>
      i18n._({
        id: "links.targetOption",
        message: "{title} ({scope})",
        values: { title, scope },
        comment: "A screen in the list of targets; title is the screen's title and scope says which plugin it is in.",
      }),
    noTarget: i18n._({
      id: "links.noTarget",
      message: "No screens to link to.",
      comment: "Shown when the app has no screen a link could target.",
    }),
    add: i18n._({
      id: "links.add",
      message: "Add link",
      comment: "Button that writes the navigation link to the document.",
    }),
    existing: i18n._({
      id: "links.existing",
      message: "Links from this widget",
      comment: "Heading of the list of navigation links the selected widget already has.",
    }),
    none: i18n._({
      id: "links.none",
      message: "This widget has no links yet.",
      comment: "Shown in the Actions tab when the selected widget navigates nowhere.",
    }),
    linkRow: (event: string, route: string) =>
      i18n._({
        id: "links.linkRow",
        message: "{event} goes to {route}",
        values: { event, route },
        comment: "One existing link; event is the widget event and route the target screen's route name.",
      }),
    missing: (route: string) =>
      i18n._({
        id: "links.missing",
        message: "No screen has the route {route}.",
        values: { route },
        comment: "Problem shown for a link whose target screen does not exist; route is the route name.",
      }),
    redirect: i18n._({
      id: "links.redirect",
      message: "Redirect to the chosen screen",
      comment: "Fix button that points a broken link at the screen chosen in the target list.",
    }),
    remove: i18n._({
      id: "links.remove",
      message: "Remove link",
      comment: "Button that deletes a navigation link from the widget.",
    }),
    bound: (event: string) =>
      i18n._({
        id: "links.bound",
        message: "{event} already does something. Replace its steps or add the navigation after them.",
        values: { event },
        comment: "Shown when the chosen event already has steps; event is the event name.",
      }),
    replace: i18n._({
      id: "links.replace",
      message: "Replace",
      comment: "Choice that replaces the steps of an event that already has some with the navigation.",
    }),
    append: i18n._({
      id: "links.append",
      message: "Add after",
      comment: "Choice that adds the navigation after the steps an event already has.",
    }),
    pickerTitle: i18n._({
      id: "links.picker.title",
      message: "Which event starts the link?",
      comment: "Title of the dialog that asks which widget event should navigate after a link was dragged.",
    }),
    pickerDescription: (target: string) =>
      i18n._({
        id: "links.picker.description",
        message: "The widget goes to {target} when the event you pick happens.",
        values: { target },
        comment: "Description of the trigger picker; target is the title of the screen the link was dropped on.",
      }),
    pickerEvents: i18n._({
      id: "links.picker.events",
      message: "Events",
      comment: "Accessible name of the list of events in the trigger picker.",
    }),
    pickerConfirm: i18n._({
      id: "links.picker.confirm",
      message: "Create link",
      comment: "Button of the trigger picker that creates the link.",
    }),
    pickerCancel: i18n._({
      id: "links.picker.cancel",
      message: "Cancel",
      comment: "Button of the trigger picker that discards the link.",
    }),
    close: i18n._({
      id: "links.picker.close",
      message: "Close",
      comment: "Accessible name of the close button of the trigger picker.",
    }),
    handle: i18n._({
      id: "links.handle",
      message: "Link handle: drag to a screen, or press Enter to choose a screen",
      comment: "Accessible name of the handle on the selected widget that starts a link by dragging or by keyboard.",
    }),
    handleChoose: i18n._({
      id: "links.handleChoose",
      message: "Link to screen",
      comment: "Accessible name of the list of screens the keyboard alternative of the link drag offers.",
    }),
    dragging: (target: string) =>
      i18n._({
        id: "links.dragging",
        message: "Linking to {target}",
        values: { target },
        comment: "Announced while a link is dragged over a screen; target is the screen's title.",
      }),
    created: (event: string, target: string) =>
      i18n._({
        id: "links.created",
        message: "{event} now goes to {target}",
        values: { event, target },
        comment: "Announced after a link was created; event is the widget event and target the screen's title.",
      }),
    cancelled: i18n._({
      id: "links.cancelled",
      message: "Link cancelled",
      comment: "Announced when a link drag ended without a target screen.",
    }),
  };
}

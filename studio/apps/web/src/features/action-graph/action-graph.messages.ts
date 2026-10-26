// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the action graph editor (S5.3, STU-065). */
export function actionGraphMessages(i18n: I18n) {
  return {
    title: i18n._({
      id: "actionGraph.title",
      message: "Action graph editor",
      comment: "Title of the full-screen overlay in which an event's action graph is edited.",
    }),
    open: i18n._({
      id: "actionGraph.open",
      message: "Action graph",
      comment: "Button in the right panel that opens the full-screen action graph editor for the selected widget.",
    }),
    close: i18n._({
      id: "actionGraph.close",
      message: "Close the action graph editor",
      comment: "Accessible name of the button that closes the full-screen action graph editor.",
    }),
    event: i18n._({
      id: "actionGraph.event",
      message: "Event",
      comment: "Label of the list from which the widget event to edit is chosen.",
    }),
    noSelection: i18n._({
      id: "actionGraph.noSelection",
      message: "Select a widget on the canvas to edit the action graph of its events.",
      comment: "Shown in the action graph editor when no widget is selected.",
    }),
    noEvents: i18n._({
      id: "actionGraph.noEvents",
      message: "This widget has no events.",
      comment: "Shown in the action graph editor when the selected widget declares no events.",
    }),
    unresolved: i18n._({
      id: "actionGraph.unresolved",
      message: "This event refers to an action graph that does not exist.",
      comment: "Shown when an event's $graph reference names no action graph document.",
    }),
    readOnly: i18n._({
      id: "actionGraph.readOnly",
      message: "The plugin is locked by someone else: the graph is read-only.",
      comment: "Shown when the editing lock is not held.",
    }),
    catalogue: i18n._({
      id: "actionGraph.catalogue",
      message: "Actions",
      comment: "Heading of the list of actions that can be added to the graph.",
    }),
    search: i18n._({
      id: "actionGraph.search",
      message: "Search actions",
      comment: "Label of the field that filters the list of actions.",
    }),
    groupBuiltin: i18n._({
      id: "actionGraph.groupBuiltin",
      message: "Built-in",
      comment: "Heading of the group of built-in actions in the action list.",
    }),
    groupCustom: i18n._({
      id: "actionGraph.groupCustom",
      message: "Host actions",
      comment: "Heading of the group of custom actions registered by the host app in the action list.",
    }),
    groupFunction: i18n._({
      id: "actionGraph.groupFunction",
      message: "Functions",
      comment: "Heading of the group of plugin functions in the action list.",
    }),
    groupFlow: i18n._({
      id: "actionGraph.groupFlow",
      message: "Flows",
      comment: "Heading of the group of the plugin's reusable action graphs in the action list.",
    }),
    nothingFound: i18n._({
      id: "actionGraph.nothingFound",
      message: "No action matches.",
      comment: "Shown when the search matches no action.",
    }),
    add: i18n._({
      id: "actionGraph.add",
      message: "Add",
      comment: "Button that adds the action to the graph.",
    }),
    addNamed: (name: string) =>
      i18n._({
        id: "actionGraph.addNamed",
        message: "Add {name} to the graph",
        values: { name },
        comment: "Accessible name of the button that adds an action to the graph; name is the action.",
      }),
    empty: i18n._({
      id: "actionGraph.empty",
      message: "This event has no steps yet. Add an action from the list.",
      comment: "Shown when the event's action graph has no steps.",
    }),
    autoLayout: i18n._({
      id: "actionGraph.autoLayout",
      message: "Arrange",
      comment: "Button that lays the steps out again automatically.",
    }),
    entry: i18n._({
      id: "actionGraph.entry",
      message: "Entry",
      comment: "Tag on the first step of a graph, where it starts.",
    }),
    makeEntry: i18n._({
      id: "actionGraph.makeEntry",
      message: "Make entry",
      comment: "Button that makes a step the first step of the graph.",
    }),
    removeStep: i18n._({
      id: "actionGraph.removeStep",
      message: "Remove step",
      comment: "Button that removes a step and the edges that lead to it.",
    }),
    removeNamed: (id: string) =>
      i18n._({
        id: "actionGraph.removeNamed",
        message: "Remove step {id}",
        values: { id },
        comment: "Accessible name of the button that removes a step; id is the step's identifier.",
      }),
    continuesTo: (port: string) =>
      i18n._({
        id: "actionGraph.continuesTo",
        message: "{port} goes to",
        values: { port },
        comment:
          "Label of the list that chooses the step a port continues with; port is the port's name such as next or onError.",
      }),
    nowhere: i18n._({
      id: "actionGraph.nowhere",
      message: "Nowhere",
      comment: "Option of the list of target steps that leaves the port unconnected.",
    }),
    literal: i18n._({
      id: "actionGraph.literal",
      message: "Value",
      comment: "Label of a step input written as a plain value.",
    }),
    pxl: i18n._({
      id: "actionGraph.pxl",
      message: "PXL expression",
      comment: "Label of the switch that writes a step input as a PXL expression.",
    }),
    pxlNamed: (name: string) =>
      i18n._({
        id: "actionGraph.pxlNamed",
        message: "{name}: PXL expression",
        values: { name },
        comment: "Accessible name of a PXL expression field of a step input; name is the input's name.",
      }),
    valueNamed: (name: string) =>
      i18n._({
        id: "actionGraph.valueNamed",
        message: "{name}",
        values: { name },
        comment: "Accessible name of the plain value field of a step input; name is the input's name.",
      }),
    required: i18n._({
      id: "actionGraph.required",
      message: "required",
      comment: "Tag on a step input that must be given.",
    }),
    outputType: (type: string) =>
      i18n._({
        id: "actionGraph.outputType",
        message: "Returns {type}",
        values: { type },
        comment: "Shows the type a step's result has on its output port; type is a type expression.",
      }),
    problems: i18n._({
      id: "actionGraph.problems",
      message: "Problems",
      comment: "Heading of the list of problems found in the graph.",
    }),
    noProblems: i18n._({
      id: "actionGraph.noProblems",
      message: "No problems found.",
      comment: "Shown when the validator finds no problem in the graph.",
    }),
    checking: i18n._({
      id: "actionGraph.checking",
      message: "Checking the graph...",
      comment: "Shown while the validator checks the graph.",
    }),
    unavailable: i18n._({
      id: "actionGraph.unavailable",
      message: "The validator is not available.",
      comment: "Shown when the validator worker cannot check the graph.",
    }),
    stepProblems: (count: number) =>
      i18n._({
        id: "actionGraph.stepProblems",
        message: "{count, plural, one {# problem} other {# problems}}",
        values: { count },
        comment: "Count of problems found in one step.",
      }),
    graphLabel: i18n._({
      id: "actionGraph.graphLabel",
      message: "Action graph",
      comment: "Accessible name of the area in which the steps are drawn.",
    }),
    nodeHint: i18n._({
      id: "actionGraph.nodeHint",
      message:
        "Press Enter or Space to select the step. Use the lists in the step to connect it and Delete to remove it.",
      comment: "Description read by screen readers for each step in the graph.",
    }),
    nodeHintFixed: i18n._({
      id: "actionGraph.nodeHintFixed",
      message: "Press Enter or Space to select the step.",
      comment: "Description read by screen readers for each step in a graph that cannot be edited.",
    }),
    edgeHint: i18n._({
      id: "actionGraph.edgeHint",
      message: "Press Enter or Space to select the connection. Press Delete to remove it.",
      comment: "Description read by screen readers for each connection in the graph.",
    }),
    moved: (direction: string, x: string, y: string) =>
      i18n._({
        id: "actionGraph.moved",
        message: "Moved selected step {direction}. New position, x: {x}, y: {y}",
        values: { direction, x, y },
        comment: "Announcement after a step is moved with the keyboard; direction is the direction moved.",
      }),
    controls: i18n._({
      id: "actionGraph.controls",
      message: "Graph view controls",
      comment: "Accessible name of the group of zoom controls.",
    }),
    zoomIn: i18n._({
      id: "actionGraph.zoomIn",
      message: "Zoom in",
      comment: "Accessible name of the button that zooms the graph in.",
    }),
    zoomOut: i18n._({
      id: "actionGraph.zoomOut",
      message: "Zoom out",
      comment: "Accessible name of the button that zooms the graph out.",
    }),
    fitView: i18n._({
      id: "actionGraph.fitView",
      message: "Fit the graph to the view",
      comment: "Accessible name of the button that fits the whole graph in view.",
    }),
    lockView: i18n._({
      id: "actionGraph.lockView",
      message: "Lock or unlock moving the view",
      comment: "Accessible name of the button that locks the view in place.",
    }),
    miniMap: i18n._({
      id: "actionGraph.miniMap",
      message: "Overview of the graph",
      comment: "Accessible name of the small overview map of the graph.",
    }),
    handle: i18n._({
      id: "actionGraph.handle",
      message: "Connection point",
      comment: "Accessible name of a connection point of a step.",
    }),
    incoming: i18n._({
      id: "actionGraph.incoming",
      message: "Incoming connection",
      comment: "Accessible name of the point at which connections arrive at a step.",
    }),
    outgoing: (port: string) =>
      i18n._({
        id: "actionGraph.outgoing",
        message: "{port} connection",
        values: { port },
        comment: "Accessible name of the point from which a step's port connects onwards; port is the port's name.",
      }),
    applied: i18n._({
      id: "actionGraph.applied",
      message: "Applied",
      comment: "Status shown after a change to the graph is made.",
    }),
  };
}

export type ActionGraphMessages = ReturnType<typeof actionGraphMessages>;

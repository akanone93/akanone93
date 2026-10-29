// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the plugin canvas's library actions (STU-055, STU-080, STU-083, STU-084). */
export function canvasLibraryMessages(i18n: I18n) {
  return {
    saveCommand: i18n._({
      id: "library.canvas.save.command",
      message: "Save selection as template",
      comment: "Command: save the selected nodes as a template.",
    }),
    insertCommand: i18n._({
      id: "library.canvas.insert.command",
      message: "Insert template",
      comment: "Command: insert a template into the plugin.",
    }),
    extractCommand: i18n._({
      id: "library.canvas.extract.command",
      message: "Extract selection to component",
      comment: "Command: turn the selected nodes into a component.",
    }),
    detachCommand: i18n._({
      id: "library.canvas.detach.command",
      message: "Detach component instance",
      comment: "Command: turn an instance into plain nodes.",
    }),
    saveTitle: i18n._({
      id: "library.canvas.save.title",
      message: "Save as template",
      comment: "Title of the save-as-template dialog.",
    }),
    stepDetails: i18n._({
      id: "library.canvas.save.step1",
      message: "Step 1 of 2: details",
      comment: "First step of saving a template.",
    }),
    stepExpose: i18n._({
      id: "library.canvas.save.step2",
      message: "Step 2 of 2: parameters and visibility",
      comment: "Second step of saving a template.",
    }),
    name: i18n._({ id: "library.canvas.name", message: "Name", comment: "Field label." }),
    key: i18n._({
      id: "library.canvas.key",
      message: "Key",
      comment: "Field label for the unique key of a library item.",
    }),
    description: i18n._({ id: "library.canvas.description", message: "Description", comment: "Field label." }),
    exposeHeading: i18n._({
      id: "library.canvas.expose",
      message: "Parameters to ask when the template is inserted",
      comment: "Heading of the exposable parameters.",
    }),
    exposeNone: i18n._({
      id: "library.canvas.expose.none",
      message: "The selection has no text to expose.",
      comment: "Shown when nothing can be exposed.",
    }),
    flaggedNote: i18n._({
      id: "library.canvas.flagged",
      message: "Other page-state references stay as they are; check them after inserting.",
      comment: "Explains that non-exposed references are only flagged.",
    }),
    convertedHeading: i18n._({
      id: "library.canvas.converted.heading",
      message: "Page state turned into parameters",
      comment: "Heading of the list of page-state references that became template parameters.",
    }),
    convertedItem: (reference: string, parameter: string) =>
      i18n._({
        id: "library.canvas.converted.item",
        message: "{reference} becomes the parameter {parameter}",
        comment: "One page-state reference converted to a template parameter; both values are identifiers.",
        values: { reference, parameter },
      }),
    flaggedHeading: i18n._({
      id: "library.canvas.flagged.heading",
      message: "Page state that cannot become a parameter",
      comment: "Heading of the list of page-state references that stay in the template and are flagged.",
    }),
    flaggedItem: (expression: string, reason: "expression" | "undeclared" | "action") => {
      const values = { expression };
      switch (reason) {
        case "action":
          return i18n._({
            id: "library.canvas.flagged.action",
            message: "Action state path {expression}",
            comment: "A page-state path in an action step; it cannot become a template parameter.",
            values,
          });
        case "undeclared":
          return i18n._({
            id: "library.canvas.flagged.undeclared",
            message: "{expression} names state the page does not declare",
            comment: "A page-state reference to an entry the page does not declare; it cannot become a parameter.",
            values,
          });
        default:
          return i18n._({
            id: "library.canvas.flagged.expression",
            message: "{expression} is part of a larger expression",
            comment: "A page-state reference inside a larger expression; it cannot become a template parameter.",
            values,
          });
      }
    },
    next: i18n._({ id: "library.canvas.next", message: "Next", comment: "Button: next step." }),
    back: i18n._({ id: "library.canvas.back", message: "Back", comment: "Button: previous step." }),
    save: i18n._({ id: "library.canvas.save", message: "Save template", comment: "Button: save the template." }),
    saveFailed: i18n._({
      id: "library.canvas.save.failed",
      message: "The item could not be saved. Check that the key is unused.",
      comment: "Error after a failed save.",
    }),
    insertTitle: i18n._({
      id: "library.canvas.insert.title",
      message: "Insert template",
      comment: "Title of the insert-template dialog.",
    }),
    insertPick: i18n._({
      id: "library.canvas.insert.pick",
      message: "Template",
      comment: "Label of the template picker.",
    }),
    insertNone: i18n._({
      id: "library.canvas.insert.none",
      message: "No templates yet.",
      comment: "Shown when the library has no templates.",
    }),
    insertLoading: i18n._({
      id: "library.canvas.insert.loading",
      message: "Loading templates…",
      comment: "Shown while templates load.",
    }),
    insertFailed: i18n._({
      id: "library.canvas.insert.failed",
      message: "The template could not be loaded.",
      comment: "Error loading the templates.",
    }),
    insertConfirm: i18n._({
      id: "library.canvas.insert.confirm",
      message: "Insert",
      comment: "Button: insert the template.",
    }),
    insertCopyNote: i18n._({
      id: "library.canvas.insert.note",
      message: "The template is copied. Later changes to it do not change this plugin.",
      comment: "Explains that templates are copied, not linked.",
    }),
    extractTitle: i18n._({
      id: "library.canvas.extract.title",
      message: "Extract to component",
      comment: "Title of the extract dialog.",
    }),
    extractConfirm: i18n._({
      id: "library.canvas.extract.confirm",
      message: "Extract",
      comment: "Button: extract the component.",
    }),
    detachTitle: i18n._({
      id: "library.canvas.detach.title",
      message: "Detach this instance?",
      comment: "Title of the detach confirmation.",
    }),
    detachDescription: i18n._({
      id: "library.canvas.detach.description",
      message:
        "The instance becomes plain nodes with new IDs and no longer follows the component. You can undo this in one step.",
      comment: "Detach confirmation text.",
    }),
    detachConfirm: i18n._({ id: "library.canvas.detach.confirm", message: "Detach", comment: "Button: detach." }),
    cancel: i18n._({ id: "library.canvas.cancel", message: "Cancel", comment: "Button: cancel." }),
    close: i18n._({ id: "library.canvas.close", message: "Close", comment: "Close button of the dialogs." }),
    fromTemplate: i18n._({
      id: "library.canvas.fromTemplate",
      message: "Start from",
      comment: "Label of the new-screen template picker.",
    }),
    blank: i18n._({
      id: "library.canvas.blank",
      message: "Blank screen",
      comment: "New-screen option without a template.",
    }),
    private: i18n._({ id: "library.canvas.private", message: "Private", comment: "Visibility." }),
    organization: i18n._({ id: "library.canvas.organization", message: "Organisation", comment: "Visibility." }),
    visibility: i18n._({ id: "library.canvas.visibility", message: "Visibility", comment: "Field label." }),
    overridesHeading: i18n._({
      id: "library.canvas.overrides.heading",
      message: "Component overrides",
      comment: "Heading of the Properties section that edits a component instance's overrides.",
    }),
    overridesNone: i18n._({
      id: "library.canvas.overrides.none",
      message: "This component exposes no props or slots.",
      comment: "Shown when a component instance has nothing to override.",
    }),
    overrideReset: (name: string) =>
      i18n._({
        id: "library.canvas.overrides.reset",
        message: "Reset {name}",
        comment: "Button: remove one override from a component instance.",
        values: { name },
      }),
    overrideSlot: (name: string) =>
      i18n._({
        id: "library.canvas.overrides.slot",
        message: "Slot {name} is overridden",
        comment: "Shown when an instance replaces the content of a slot.",
        values: { name },
      }),
  };
}

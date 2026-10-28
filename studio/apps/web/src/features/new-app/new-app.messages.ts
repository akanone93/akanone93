// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the new-app wizard (STU-024). */
export function newAppMessages(i18n: I18n) {
  return {
    title: i18n._({
      id: "newApp.title",
      message: "Create app",
      comment: "Title of the wizard that creates an app, and the label of its final button.",
    }),
    creating: i18n._({
      id: "newApp.creating",
      message: "Creating",
      comment: "Label of the final button of the new-app wizard while the app is being created.",
    }),
    stepsLabel: i18n._({
      id: "newApp.steps",
      message: "Steps",
      comment: "Accessible name of the list of steps of the new-app wizard.",
    }),
    stepOf: (current: number, total: number, name: string) =>
      i18n._({
        id: "newApp.step-of",
        message: "Step {current} of {total}: {name}",
        values: { current, total, name },
        comment: "Heading of the current step of the new-app wizard; name is the step's name.",
      }),
    stepSource: i18n._({
      id: "newApp.step.source",
      message: "Start from",
      comment: "Name of the wizard step where a person chooses blank, template or archive.",
    }),
    stepDetails: i18n._({
      id: "newApp.step.details",
      message: "Details",
      comment: "Name of the wizard step that asks for the app's name and key.",
    }),
    stepLocales: i18n._({
      id: "newApp.step.locales",
      message: "Languages",
      comment: "Name of the wizard step that asks for the app's languages.",
    }),
    stepTheme: i18n._({
      id: "newApp.step.theme",
      message: "Theme",
      comment: "Name of the wizard step that asks for the app's theme.",
    }),
    stepSecurity: i18n._({
      id: "newApp.step.security",
      message: "Security",
      comment: "Name of the wizard step that asks for the app's security profile.",
    }),
    stepEnvironments: i18n._({
      id: "newApp.step.environments",
      message: "Environments",
      comment: "Name of the wizard step that asks which environments the app has.",
    }),
    stepDone: i18n._({
      id: "newApp.step.done",
      message: "Done",
      comment: "Name of the last wizard step, shown once the app exists.",
    }),
    back: i18n._({ id: "newApp.back", message: "Back", comment: "Button that returns to the previous wizard step." }),
    next: i18n._({ id: "newApp.next", message: "Next", comment: "Button that moves to the next wizard step." }),
    sourceLegend: i18n._({
      id: "newApp.source.legend",
      message: "How do you want to start?",
      comment: "Legend of the choice between a blank app, a template and an archive.",
    }),
    sourceBlank: i18n._({
      id: "newApp.source.blank",
      message: "Blank app",
      comment: "Choice: start from nothing.",
    }),
    sourceBlankHint: i18n._({
      id: "newApp.source.blank-hint",
      message: "One plugin with an empty entry screen, ready to design.",
      comment: "Explains the blank app choice.",
    }),
    sourceTemplate: i18n._({
      id: "newApp.source.template",
      message: "From a template",
      comment: "Choice: copy a sample app the server provides.",
    }),
    sourceTemplateHint: i18n._({
      id: "newApp.source.template-hint",
      message: "Copy one of the sample apps this server provides.",
      comment: "Explains the template choice.",
    }),
    sourceImport: i18n._({
      id: "newApp.source.import",
      message: "From a .plux archive",
      comment: "Choice: import an app archive; .plux is the file extension and stays as it is.",
    }),
    sourceImportHint: i18n._({
      id: "newApp.source.import-hint",
      message: "Import an app exported from Studio. Every file is checked before the app is created.",
      comment: "Explains the archive choice.",
    }),
    sourceAi: i18n._({
      id: "newApp.source.ai",
      message: "From an AI prompt",
      comment: "Choice that is not available yet: describe the app and let an assistant draft it.",
    }),
    sourceAiHint: i18n._({
      id: "newApp.source.ai-hint",
      message: "Arrives in a later release.",
      comment: "Explains why the AI prompt choice is disabled.",
    }),
    templateLegend: i18n._({
      id: "newApp.template.legend",
      message: "Template",
      comment: "Legend of the list of sample apps to copy.",
    }),
    templatesLoading: i18n._({
      id: "newApp.template.loading",
      message: "Loading the templates",
      comment: "Shown while the list of sample apps loads.",
    }),
    templatesFailed: i18n._({
      id: "newApp.template.failed",
      message: "The templates could not be loaded.",
      comment: "Shown when the list of sample apps failed to load.",
    }),
    templatesEmpty: i18n._({
      id: "newApp.template.empty",
      message: "This server has no sample apps.",
      comment: "Shown when the server embeds no sample apps.",
    }),
    templateRequired: i18n._({
      id: "newApp.template.required",
      message: "Choose a template.",
      comment: "Error when Next is pressed without choosing a sample app.",
    }),
    archiveLabel: i18n._({
      id: "newApp.archive.label",
      message: "Archive file",
      comment: "Label of the file input for a .plux archive.",
    }),
    archiveHint: (size: string) =>
      i18n._({
        id: "newApp.archive.hint",
        message: "A .plux file, up to {size} by default. Your organisation's limit may differ; the server tells you.",
        values: { size },
        comment: "Hint under the archive file input; size is the default limit such as 8 MB.",
      }),
    archiveChosen: (name: string, size: string) =>
      i18n._({
        id: "newApp.archive.chosen",
        message: "{name} ({size})",
        values: { name, size },
        comment: "The archive file that was chosen: its name and size.",
      }),
    archiveRequired: i18n._({
      id: "newApp.archive.required",
      message: "Choose a .plux file.",
      comment: "Error when Next is pressed without choosing an archive.",
    }),
    archiveExtension: i18n._({
      id: "newApp.archive.extension",
      message: "The file name must end in .plux.",
      comment: "Error when the chosen file is not a .plux file.",
    }),
    archiveUnreadable: i18n._({
      id: "newApp.archive.unreadable",
      message: "The file is not a .plux archive.",
      comment: "Error when the chosen file cannot be read as a ZIP archive.",
    }),
    archiveTooBig: (size: string, max: string) =>
      i18n._({
        id: "newApp.archive.too-big",
        message: "The file is {size}; no organisation can import more than {max}.",
        values: { size, max },
        comment: "Error when the chosen file is larger than any limit allows; both values are sizes such as 12 MB.",
      }),
    archiveTooManyFiles: (count: number, max: number) =>
      i18n._({
        id: "newApp.archive.too-many-files",
        message: "The archive holds {count} files; no organisation can import more than {max}.",
        values: { count, max },
        comment: "Error when the archive has more files than any limit allows.",
      }),
    archiveTooLarge: (size: string, max: string) =>
      i18n._({
        id: "newApp.archive.too-large",
        message: "The archive expands to {size}; no organisation can import more than {max}.",
        values: { size, max },
        comment: "Error when the unzipped archive is larger than any limit allows; both values are sizes.",
      }),
    importKeepHint: i18n._({
      id: "newApp.details.import-hint",
      message: "Leave the name and key empty to keep those in the archive.",
      comment: "Hint on the details step when importing an archive.",
    }),
    description: i18n._({
      id: "newApp.details.description",
      message: "Description",
      comment: "Label of the optional description of a new app.",
    }),
    descriptionHint: i18n._({
      id: "newApp.details.description-hint",
      message: "Optional. It appears in the list of apps.",
      comment: "Hint under the description of a new app.",
    }),
    localesLegend: i18n._({
      id: "newApp.locales.legend",
      message: "Supported languages",
      comment: "Legend of the checkboxes that choose the languages of a new app.",
    }),
    localesDefault: i18n._({
      id: "newApp.locales.default",
      message: "Default language",
      comment: "Label of the select that chooses the default language of a new app.",
    }),
    localesHint: (max: number) =>
      i18n._({
        id: "newApp.locales.hint",
        message: "Choose up to {max} languages. The default language is always supported.",
        values: { max },
        comment: "Hint on the languages step; max is the largest number of languages an app may support.",
      }),
    localesRequired: i18n._({
      id: "newApp.locales.required",
      message: "Choose at least one language.",
      comment: "Error when no language is chosen.",
    }),
    localesTooMany: (max: number) =>
      i18n._({
        id: "newApp.locales.too-many",
        message: "Choose at most {max} languages.",
        values: { max },
        comment: "Error when more languages are chosen than an app may support.",
      }),
    language: (name: string, code: string) =>
      i18n._({
        id: "newApp.locales.language",
        message: "{name} ({code})",
        values: { name, code },
        comment: "A language in the list: its name in the reader's language and its BCP 47 code.",
      }),
    themeLegend: i18n._({
      id: "newApp.theme.legend",
      message: "Theme",
      comment: "Legend of the choice of a theme for a new app.",
    }),
    themeViolet: i18n._({ id: "newApp.theme.violet", message: "Violet", comment: "Name of a ready-made theme." }),
    themeBlue: i18n._({ id: "newApp.theme.blue", message: "Blue", comment: "Name of a ready-made theme." }),
    themeGreen: i18n._({ id: "newApp.theme.green", message: "Green", comment: "Name of a ready-made theme." }),
    themeSlate: i18n._({ id: "newApp.theme.slate", message: "Slate", comment: "Name of a ready-made theme." }),
    themeCustom: i18n._({
      id: "newApp.theme.custom",
      message: "Custom colour",
      comment: "Choice: enter a primary colour instead of a ready-made theme.",
    }),
    themeCustomLabel: i18n._({
      id: "newApp.theme.custom-label",
      message: "Primary colour",
      comment: "Label of the field for a custom primary colour.",
    }),
    themeCustomHint: (example: string) =>
      i18n._({
        id: "newApp.theme.custom-hint",
        message: "Six hexadecimal digits after a hash sign, such as {example}. It is used in light and dark mode.",
        values: { example },
        comment: "Hint under the custom primary colour; example is a colour such as the violet of the first theme.",
      }),
    themeInvalid: (example: string) =>
      i18n._({
        id: "newApp.theme.invalid",
        message: "Enter a colour such as {example}.",
        values: { example },
        comment: "Error for a custom colour that is not six hexadecimal digits after a hash sign.",
      }),
    themeSwatch: (name: string, colour: string) =>
      i18n._({
        id: "newApp.theme.swatch",
        message: "{name}, primary colour {colour}",
        values: { name, colour },
        comment: "Accessible description of a theme choice: its name and primary colour.",
      }),
    securityLegend: i18n._({
      id: "newApp.security.legend",
      message: "Security profile",
      comment: "Legend of the choice of a security profile.",
    }),
    securityStandard: i18n._({
      id: "newApp.security.standard",
      message: "Standard",
      comment: "Name of the standard security profile (MASVS level 1).",
    }),
    securityStandardHint: i18n._({
      id: "newApp.security.standard-hint",
      message:
        "Requests are bound to the device's hardware key and the server certificate is pinned. One approval publishes to production.",
      comment: "Explains the standard security profile.",
    }),
    securityStrict: i18n._({
      id: "newApp.security.strict",
      message: "Strict",
      comment: "Name of the strict security profile (MASVS level 2).",
    }),
    securityStrictHint: i18n._({
      id: "newApp.security.strict-hint",
      message: "Stronger device checks and encrypted local stores. Two people approve a publish to production.",
      comment: "Explains the strict security profile.",
    }),
    securityMaximum: i18n._({
      id: "newApp.security.maximum",
      message: "Maximum",
      comment: "Name of the maximum security profile (MASVS level 2 with resilience).",
    }),
    securityMaximumHint: i18n._({
      id: "newApp.security.maximum-hint",
      message:
        "Attestation fails closed, rooted devices are blocked and bundles are confidential. Approvals come from distinct teams.",
      comment: "Explains the maximum security profile.",
    }),
    securityLater: i18n._({
      id: "newApp.security.later",
      message: "You can change the profile later in the app's security settings.",
      comment: "Note under the security profiles.",
    }),
    environmentsLegend: i18n._({
      id: "newApp.environments.legend",
      message: "Environments",
      comment: "Legend of the checkboxes that choose the environments of a new app.",
    }),
    environmentDevelopment: i18n._({
      id: "newApp.environments.development",
      message: "Development",
      comment: "Name of the development environment.",
    }),
    environmentStaging: i18n._({
      id: "newApp.environments.staging",
      message: "Staging",
      comment: "Name of the staging environment.",
    }),
    environmentProduction: i18n._({
      id: "newApp.environments.production",
      message: "Production (always included)",
      comment: "Name of the production environment, which every app has.",
    }),
    environmentsHint: i18n._({
      id: "newApp.environments.hint",
      message: "Each environment has its own variables, secrets, signing keys and devices.",
      comment: "Explains environments on the last step of the wizard.",
    }),
    progressLabel: i18n._({
      id: "newApp.progress.label",
      message: "Progress of creating the app",
      comment: "Accessible name of the progress bar shown while the app is created.",
    }),
    progressPreparing: i18n._({
      id: "newApp.progress.preparing",
      message: "Preparing the project",
      comment: "Progress stage: Studio is building or reading the files to send.",
    }),
    progressCreating: i18n._({
      id: "newApp.progress.creating",
      message: "The server is checking every file and creating the app",
      comment: "Progress stage: the server is validating the archive and creating the app.",
    }),
    doneTitle: (name: string) =>
      i18n._({
        id: "newApp.done.title",
        message: "{name} was created",
        values: { name },
        comment: "Heading of the last wizard step; name is the new app.",
      }),
    doneDocuments: (count: number) =>
      i18n._({
        id: "newApp.done.documents",
        message: "{count, plural, one {# document written} other {# documents written}}",
        values: { count },
        comment: "How many documents the new app holds.",
      }),
    doneErrors: (count: number) =>
      i18n._({
        id: "newApp.done.errors",
        message: "{count, plural, one {# validation error to fix} other {# validation errors to fix}}",
        values: { count },
        comment: "How many validation errors the imported project still has.",
      }),
    doneOpen: i18n._({
      id: "newApp.done.open",
      message: "Open the app",
      comment: "Button that opens the new app's canvas.",
    }),
    doneClose: i18n._({
      id: "newApp.done.close",
      message: "Close",
      comment: "Button that closes the wizard after the app was created.",
    }),
    projectTitle: i18n._({
      id: "newApp.project.title",
      message: "Flutter project",
      comment: "Heading of the offer to download the generated host project.",
    }),
    projectCaption: i18n._({
      id: "newApp.project.caption",
      message:
        "The host project holds the app's name, identifiers, icons, the Plux runtime and build scripts. You can also download it later from the app's shell settings.",
      comment: "Explains the generated Flutter project that can be downloaded.",
    }),
    projectDownload: i18n._({
      id: "newApp.project.download",
      message: "Download Flutter project",
      comment: "Button that generates and downloads the host project.",
    }),
  };
}

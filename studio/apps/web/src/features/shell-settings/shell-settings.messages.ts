// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the native shell settings of an app and of the project download (GEN-004, GEN-001). */
export function shellSettingsMessages(i18n: I18n) {
  return {
    title: i18n._({
      id: "shell.title",
      message: "Native shell",
      comment: "Heading of the page with the settings that live in the app's native shell project.",
    }),
    caption: i18n._({
      id: "shell.caption",
      message:
        "These settings are compiled into the Flutter project that is built and published to the stores. Changing one needs a new store build; everything else updates over the air.",
      comment: "Caption under the heading of the native shell settings page.",
    }),
    loading: i18n._({
      id: "shell.loading",
      message: "Loading the shell settings",
      comment: "Shown while the shell settings load.",
    }),
    loadFailed: i18n._({
      id: "shell.load-failed",
      message: "The shell settings could not be loaded.",
      comment: "Shown when the shell settings failed to load.",
    }),
    statusRequired: i18n._({
      id: "shell.status.required",
      message: "Shell update required",
      comment: "Status word: the settings differ from those the project was generated from.",
    }),
    statusCurrent: i18n._({
      id: "shell.status.current",
      message: "Shell is up to date",
      comment: "Status word: the project matches the settings.",
    }),
    statusNever: i18n._({
      id: "shell.status.never",
      message: "No project generated yet",
      comment: "Status word: no host project was ever generated for this app.",
    }),
    generatedAt: (when: string) =>
      i18n._({
        id: "shell.generated-at",
        message: "The project was generated on {when}.",
        values: { when },
        comment: "Says when the host project was last generated; when is a date and time.",
      }),
    requiredCaption: i18n._({
      id: "shell.required-caption",
      message:
        "These changes need a new store build. Publishing continues meanwhile; only what needs the new shell waits.",
      comment: "Explains what a required shell update means.",
    }),
    changesTitle: i18n._({
      id: "shell.changes.title",
      message: "What changed",
      comment: "Heading of the table of settings that differ from the generated project.",
    }),
    changesTable: i18n._({
      id: "shell.changes.table",
      message: "Settings that changed since the project was generated",
      comment: "Accessible caption of the table of changed settings.",
    }),
    colSetting: i18n._({ id: "shell.col.setting", message: "Setting", comment: "Column head: the setting's name." }),
    colGenerated: i18n._({
      id: "shell.col.generated",
      message: "In the project",
      comment: "Column head: the value the project was generated from.",
    }),
    colCurrent: i18n._({
      id: "shell.col.current",
      message: "Now",
      comment: "Column head: the setting's current value.",
    }),
    colReason: i18n._({
      id: "shell.col.reason",
      message: "Why it needs a store build",
      comment: "Column head: the reason the change needs a new store build.",
    }),
    none: i18n._({ id: "shell.none", message: "None", comment: "Shown for a setting that has no value." }),
    on: i18n._({ id: "shell.on", message: "On", comment: "A switch setting that is on." }),
    off: i18n._({ id: "shell.off", message: "Off", comment: "A switch setting that is off." }),
    formTitle: i18n._({
      id: "shell.form.title",
      message: "Settings",
      comment: "Heading of the form that edits the shell settings.",
    }),
    fieldName: i18n._({
      id: "shell.field.name",
      message: "App name",
      comment: "Label: the name shown under the launcher icon and in the stores.",
    }),
    fieldPackage: i18n._({
      id: "shell.field.package",
      message: "Dart package name",
      comment: "Label: the project's Dart package name, lower_snake_case.",
    }),
    fieldApplicationId: i18n._({
      id: "shell.field.application-id",
      message: "Android application ID",
      comment: "Label: the Android application ID such as com.example.app.",
    }),
    fieldBundleId: i18n._({
      id: "shell.field.bundle-id",
      message: "iOS bundle ID",
      comment: "Label: the iOS bundle identifier such as com.example.app.",
    }),
    fieldAppVersion: i18n._({
      id: "shell.field.app-version",
      message: "Version",
      comment: "Label: the app version such as 1.0.0+1 (version, then build number).",
    }),
    fieldSplashColor: i18n._({
      id: "shell.field.splash-color",
      message: "Splash colour",
      comment: "Label: the background colour of the launch screen.",
    }),
    fieldRouter: i18n._({
      id: "shell.field.router",
      message: "Router",
      comment: "Label: the router adapter the app's navigation runs on.",
    }),
    routerNone: i18n._({ id: "shell.router.none", message: "None", comment: "Option: no router adapter." }),
    fieldDeviceApis: i18n._({
      id: "shell.field.device-apis",
      message: "Device permissions",
      comment: "Legend of the checkboxes for the device APIs whose permissions the native project declares.",
    }),
    deviceCamera: i18n._({ id: "shell.api.camera", message: "Camera", comment: "A device API." }),
    devicePhotos: i18n._({ id: "shell.api.photos", message: "Photos", comment: "A device API." }),
    deviceFiles: i18n._({ id: "shell.api.files", message: "Files", comment: "A device API." }),
    deviceLocation: i18n._({ id: "shell.api.location", message: "Location", comment: "A device API." }),
    deviceContacts: i18n._({ id: "shell.api.contacts", message: "Contacts", comment: "A device API." }),
    deviceBiometrics: i18n._({ id: "shell.api.biometrics", message: "Biometrics", comment: "A device API." }),
    deviceNotifications: i18n._({
      id: "shell.api.notifications",
      message: "Notifications",
      comment: "A device API.",
    }),
    deviceClipboard: i18n._({ id: "shell.api.clipboard", message: "Clipboard", comment: "A device API." }),
    deviceShare: i18n._({ id: "shell.api.share", message: "Share", comment: "A device API." }),
    deviceHaptics: i18n._({ id: "shell.api.haptics", message: "Haptics", comment: "A device API." }),
    fieldPush: i18n._({
      id: "shell.field.push",
      message: "Push notifications",
      comment: "Label of the switch that adds push notification support to the native project.",
    }),
    fieldDeepLinkHosts: i18n._({
      id: "shell.field.deep-link-hosts",
      message: "Deep-link hosts",
      comment: "Label: host names the app opens links for, separated by commas.",
    }),
    fieldDeepLinkSchemes: i18n._({
      id: "shell.field.deep-link-schemes",
      message: "Deep-link schemes",
      comment: "Label: URL schemes the app opens links for, separated by commas.",
    }),
    fieldPackages: i18n._({
      id: "shell.field.packages",
      message: "Optional packages",
      comment: "Label: optional packages added to the shell, separated by commas.",
    }),
    fieldEntryRoute: i18n._({
      id: "shell.field.entry-route",
      message: "Entry route",
      comment: "Label: the route the app opens first.",
    }),
    listHint: i18n._({
      id: "shell.list-hint",
      message: "Separate several values with commas.",
      comment: "Hint under a field that takes a comma-separated list.",
    }),
    save: i18n._({ id: "shell.save", message: "Save settings", comment: "Button that stores the shell settings." }),
    saving: i18n._({ id: "shell.saving", message: "Saving", comment: "Label of the save button while it works." }),
    saved: i18n._({
      id: "shell.saved",
      message: "The shell settings were saved.",
      comment: "Confirmation after the shell settings were stored.",
    }),
    saveFailed: i18n._({
      id: "shell.save-failed",
      message: "The settings could not be saved.",
      comment: "Shown when saving the shell settings failed without a reason from the server.",
    }),
    needsManage: i18n._({
      id: "shell.needs-manage",
      message: "Changing the shell settings needs permission to manage the app.",
      comment: "Explains why the form is read-only for a person without the app-manage permission.",
    }),
    projectTitle: i18n._({
      id: "shell.project.title",
      message: "Host project",
      comment: "Heading of the section that regenerates and downloads the Flutter host project.",
    }),
    projectCaption: (perHour: number) =>
      i18n._({
        id: "shell.project.caption",
        message:
          "Generates the Flutter project from the saved settings as a zip. An app may queue {perHour, plural, one {# project an hour} other {# projects an hour}} by default; your organisation's limit may differ.",
        values: { perHour },
        comment: "Explains project generation; perHour is the default limit of generation jobs per hour.",
      }),
    projectRegenerate: i18n._({
      id: "shell.project.regenerate",
      message: "Regenerate project",
      comment: "Button that generates the host project again from the current settings.",
    }),
    projectQueued: i18n._({
      id: "shell.project.queued",
      message: "Waiting for the project generator",
      comment: "Progress: the project job is queued.",
    }),
    projectRunning: i18n._({
      id: "shell.project.running",
      message: "Generating the project",
      comment: "Progress: the project is being generated.",
    }),
    projectDownloading: i18n._({
      id: "shell.project.downloading",
      message: "Downloading the project",
      comment: "Progress: the generated project is being downloaded.",
    }),
    projectDone: (filename: string) =>
      i18n._({
        id: "shell.project.done",
        message: "Downloaded {filename}",
        values: { filename },
        comment: "Shown after the project was downloaded; filename is the zip file.",
      }),
    projectFailed: (reason: string) =>
      i18n._({
        id: "shell.project.failed",
        message: "The project could not be generated: {reason}",
        values: { reason },
        comment: "Shown when project generation failed; reason is the server's explanation.",
      }),
    projectFailedPlain: i18n._({
      id: "shell.project.failed-plain",
      message: "The project could not be generated.",
      comment: "Shown when project generation failed without an explanation.",
    }),
    projectProgress: i18n._({
      id: "shell.project.progress",
      message: "Progress of generating the project",
      comment: "Accessible name of the progress bar shown while the project is generated.",
    }),
    projectPatch: i18n._({
      id: "shell.project.patch",
      message: "Download patch",
      comment: "Button that downloads only the files that changed since the project was last generated.",
    }),
    projectPatchCaption: i18n._({
      id: "shell.project.patch-caption",
      message:
        "The patch holds the files that were added or changed since the last generation, and plux-patch.json, which lists the changed, added and deleted files. Unzip it over your project and delete the listed files.",
      comment: "Explains the patch download; plux-patch.json is a file name and stays as it is.",
    }),
    fieldIcon: i18n._({
      id: "shell.field.icon",
      message: "App icon",
      comment: "Name of the setting for the launcher and store icon; also the legend of the icon picker.",
    }),
    iconDefault: i18n._({
      id: "shell.icon.default",
      message: "Default icon",
      comment: "Choice of the icon picker: the icon Plux draws when the app has none, and the value shown for it.",
    }),
    iconHint: i18n._({
      id: "shell.icon.hint",
      message: "A square PNG, 1024 to 4096 pixels a side, from the app's assets.",
      comment: "Explains which images work as the app icon.",
    }),
    iconNoAssets: i18n._({
      id: "shell.icon.no-assets",
      message: "The app has no PNG assets yet. Upload one in the Assets panel to use it as the icon.",
      comment: "Shown in the icon picker when the app has no PNG image to choose.",
    }),
    iconLoadFailed: i18n._({
      id: "shell.icon.load-failed",
      message: "The app's images could not be loaded.",
      comment: "Shown when the list of images for the icon picker failed to load.",
    }),
    iconSize: (width: number, height: number) =>
      i18n._({
        id: "shell.icon.size",
        message: "{width} × {height} pixels",
        values: { width, height },
        comment: "Size of an image in the icon picker, in pixels.",
      }),
    iconPreview: (name: string) =>
      i18n._({
        id: "shell.icon.preview",
        message: "Preview of {name}",
        values: { name },
        comment: "Accessible name of the preview of an image in the icon picker; name is the image's file name.",
      }),
    linkLabel: i18n._({
      id: "shell.link",
      message: "Native shell settings",
      comment: "Label of the link from the app's settings to the native shell settings page.",
    }),
  };
}

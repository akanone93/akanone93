// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the device loop: pairing, send to device and the device drawer (STU-060, STU-066, STU-130, DEV-002, DEV-011, DEV-020). */
export function devicesMessages(i18n: I18n) {
  return {
    close: i18n._({
      id: "devices.close",
      message: "Close",
      comment: "Close button of the device dialogs.",
    }),
    failed: i18n._({
      id: "devices.failed",
      message: "The request failed. Try again.",
      comment: "Shown when a device request fails.",
    }),
    pairTitle: i18n._({
      id: "devices.pairTitle",
      message: "Pair a device",
      comment: "Title of the dialog that shows the QR code for pairing a device.",
    }),
    pairHelp: i18n._({
      id: "devices.pairHelp",
      message: "Scan the code with the Plux Dev app, or type the short code in the app.",
      comment: "Instruction in the pairing dialog.",
    }),
    qrLabel: i18n._({
      id: "devices.qrLabel",
      message: "QR code of the pairing link",
      comment: "Accessible name of the QR code image.",
    }),
    link: i18n._({
      id: "devices.link",
      message: "Pairing link",
      comment: "Label of the pairing link.",
    }),
    code: i18n._({
      id: "devices.code",
      message: "Short code",
      comment: "Label of the short code a person can type on the device.",
    }),
    expiresIn: (time: string) =>
      i18n._({
        id: "devices.expiresIn",
        message: "Expires in {time}",
        values: { time },
        comment: "Countdown of the pairing token; time is minutes and seconds such as 1:59.",
      }),
    expired: i18n._({
      id: "devices.expired",
      message: "This code has expired.",
      comment: "Shown when the pairing token has expired.",
    }),
    newCode: i18n._({
      id: "devices.newCode",
      message: "New code",
      comment: "Button: request a new pairing token.",
    }),
    sendTitle: i18n._({
      id: "devices.sendTitle",
      message: "Send to device",
      comment: "Title of the dialog that opens a screen on a paired device.",
    }),
    device: i18n._({
      id: "devices.device",
      message: "Device",
      comment: "Label of the paired device picker.",
    }),
    noDevices: i18n._({
      id: "devices.noDevices",
      message: "No device is paired yet.",
      comment: "Shown in the send dialog when no device is paired.",
    }),
    route: i18n._({
      id: "devices.route",
      message: "Screen route",
      comment: "Label of the route to open on the device.",
    }),
    locale: i18n._({
      id: "devices.locale",
      message: "Language",
      comment: "Label of the locale to open the screen with.",
    }),
    theme: i18n._({
      id: "devices.theme",
      message: "Theme",
      comment: "Label of the theme picker.",
    }),
    themeLight: i18n._({
      id: "devices.themeLight",
      message: "Light",
      comment: "Theme option.",
    }),
    themeDark: i18n._({
      id: "devices.themeDark",
      message: "Dark",
      comment: "Theme option.",
    }),
    themeSystem: i18n._({
      id: "devices.themeSystem",
      message: "System",
      comment: "Theme option.",
    }),
    textScale: i18n._({
      id: "devices.textScale",
      message: "Text scale",
      comment: "Label of the text scale input.",
    }),
    send: i18n._({
      id: "devices.send",
      message: "Send",
      comment: "Button: send the screen to the device.",
    }),
    sent: i18n._({
      id: "devices.sent",
      message: "Sent to the device.",
      comment: "Shown after the command was sent.",
    }),
    deviceName: (name: string, connected: boolean) =>
      i18n._({
        id: "devices.deviceName",
        message: "{name} ({state, select, true {connected} other {offline}})",
        values: { name, state: String(connected) },
        comment: "A paired device in a picker; state says whether its socket is open.",
      }),
    drawer: i18n._({
      id: "devices.drawer",
      message: "Device drawer",
      comment: "Accessible name of the bottom drawer fed by paired devices.",
    }),
    source: i18n._({
      id: "devices.source",
      message: "Source device",
      comment: "Label of the picker that chooses which device feeds the drawer.",
    }),
    allDevices: i18n._({
      id: "devices.allDevices",
      message: "All devices",
      comment: "Option of the source picker: every paired device.",
    }),
    search: i18n._({
      id: "devices.search",
      message: "Search",
      comment: "Label of the drawer's search field.",
    }),
    level: i18n._({
      id: "devices.level",
      message: "Level",
      comment: "Label of the drawer's log level filter.",
    }),
    levelAll: i18n._({
      id: "devices.levelAll",
      message: "All levels",
      comment: "Option: no level filter.",
    }),
    levelWarning: i18n._({
      id: "devices.levelWarning",
      message: "Warnings and errors",
      comment: "Option: only warnings and errors.",
    }),
    levelError: i18n._({
      id: "devices.levelError",
      message: "Errors only",
      comment: "Option: only errors.",
    }),
    pause: i18n._({
      id: "devices.pause",
      message: "Pause",
      comment: "Button: freeze the drawer's list while events keep arriving.",
    }),
    resume: i18n._({
      id: "devices.resume",
      message: "Resume",
      comment: "Button: show the events that arrived while paused.",
    }),
    exportTrace: i18n._({
      id: "devices.exportTrace",
      message: "Export trace",
      comment: "Button: download the session's redacted trace file.",
    }),
    empty: i18n._({
      id: "devices.empty",
      message: "Nothing yet.",
      comment: "Shown in a drawer tab that has no events.",
    }),
    streamEnded: i18n._({
      id: "devices.streamEnded",
      message: "The device stream ended.",
      comment: "Shown when the dev session stream closed.",
    }),
    tabProblems: i18n._({
      id: "devices.tabProblems",
      message: "Problems",
      comment: "Drawer tab: validation findings.",
    }),
    tabLogs: i18n._({
      id: "devices.tabLogs",
      message: "Logs",
      comment: "Drawer tab: device logs.",
    }),
    tabFlow: i18n._({
      id: "devices.tabFlow",
      message: "Process flow",
      comment: "Drawer tab: action runs.",
    }),
    tabState: i18n._({
      id: "devices.tabState",
      message: "State",
      comment: "Drawer tab: state changes.",
    }),
    tabNetwork: i18n._({
      id: "devices.tabNetwork",
      message: "Network",
      comment: "Drawer tab: network calls.",
    }),
    tabFunctions: i18n._({
      id: "devices.tabFunctions",
      message: "Functions",
      comment: "Drawer tab: function calls.",
    }),
    tabSync: i18n._({
      id: "devices.tabSync",
      message: "Sync",
      comment: "Drawer tab: sync and hot apply.",
    }),
    tabPerformance: i18n._({
      id: "devices.tabPerformance",
      message: "Performance",
      comment: "Drawer tab: frame timing.",
    }),
    tabConsole: i18n._({
      id: "devices.tabConsole",
      message: "Console",
      comment: "Drawer tab: PXL against device state.",
    }),
    pausedCount: (n: number) =>
      i18n._({
        id: "devices.pausedCount",
        message: "{n, plural, one {# new event} other {# new events}}",
        values: { n },
        comment: "Shown while the drawer is paused: how many events arrived since.",
      }),
    bayToggle: i18n._({
      id: "devices.bayToggle",
      message: "Devices",
      comment: "Button that shows or hides the device drawer under the canvas.",
    }),
    pairAction: i18n._({
      id: "devices.pairAction",
      message: "Pair a device",
      comment: "Toolbar button and palette command that open the pairing dialog.",
    }),
    sendAction: i18n._({
      id: "devices.sendAction",
      message: "Send to device",
      comment: "Toolbar button and palette command that open the send-to-device dialog.",
    }),
    highlight: i18n._({
      id: "devices.highlight",
      message: "Highlight selection on device",
      comment: "Switch: the node selected in Studio is highlighted on the paired device (inspect mode).",
    }),
    highlighted: i18n._({
      id: "devices.highlighted",
      message: "Highlighting on the device.",
      comment: "Status shown while highlighting is on.",
    }),
    inspected: i18n._({
      id: "devices.inspected",
      message: "Selected from the device.",
      comment: "Announced when a tap on the device selected a node in Studio.",
    }),
    controlsTitle: i18n._({
      id: "devices.controlsTitle",
      message: "Device controls",
      comment: "Heading of the controls that change a paired device.",
    }),
    reload: i18n._({
      id: "devices.reload",
      message: "Reload screen",
      comment: "Button: opens the current route on the device again.",
    }),
    apply: i18n._({
      id: "devices.apply",
      message: "Apply",
      comment: "Button: applies the locale, theme, text scale and route to the device.",
    }),
    lineDebug: i18n._({
      id: "devices.lineDebug",
      message: "debug",
      comment: "Level label of a console line.",
    }),
    lineInfo: i18n._({
      id: "devices.lineInfo",
      message: "info",
      comment: "Level label of a console line.",
    }),
    lineWarning: i18n._({
      id: "devices.lineWarning",
      message: "warning",
      comment: "Level label of a console line.",
    }),
    lineError: i18n._({
      id: "devices.lineError",
      message: "error",
      comment: "Level label of a console line.",
    }),
    screenshotCapture: i18n._({
      id: "devices.screenshotCapture",
      message: "Capture screenshot",
      comment: "Button: asks the paired device for a screenshot.",
    }),
    screenshotAlt: i18n._({
      id: "devices.screenshotAlt",
      message: "Screenshot of the paired device",
      comment: "Alternative text of a captured device screenshot.",
    }),
    screenshotUse: i18n._({
      id: "devices.screenshotUse",
      message: "Keep it as",
      comment: "Label of the choice of what a screenshot is kept as.",
    }),
    useThumbnail: i18n._({
      id: "devices.useThumbnail",
      message: "Screen thumbnail",
      comment: "Screenshot use: the thumbnail of a screen.",
    }),
    useSlotPreview: i18n._({
      id: "devices.useSlotPreview",
      message: "Native slot preview",
      comment: "Screenshot use: the preview of a native widget slot on the canvas.",
    }),
    useTranslationContext: i18n._({
      id: "devices.useTranslationContext",
      message: "Translation context",
      comment: "Screenshot use: context shown to translators.",
    }),
    screenshotTarget: i18n._({
      id: "devices.screenshotTarget",
      message: "Screen or slot ID",
      comment: "Label of the field naming the screen or slot a screenshot belongs to.",
    }),
    screenshotSave: i18n._({
      id: "devices.screenshotSave",
      message: "Save screenshot",
      comment: "Button: stores the screenshot as an app asset.",
    }),
    screenshotSaved: i18n._({
      id: "devices.screenshotSaved",
      message: "Screenshot saved.",
      comment: "Shown after a screenshot was stored.",
    }),
    platformViews: i18n._({
      id: "devices.platformViews",
      message: "Native views on the screen are not part of the capture.",
      comment: "Warning on a screenshot taken while platform views were showing.",
    }),
    latency: (ms: number) =>
      i18n._({
        id: "devices.latency",
        message: "Autosave to device: {ms} ms",
        values: { ms },
        comment: "Measured time from the server accepting an autosave to the device applying it (NFR-030).",
      }),
  };
}

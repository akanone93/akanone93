// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the Devices and security area (STU-115, SEC-180). */
export function securityMessages(i18n: I18n) {
  return {
    devicesTitle: i18n._({ id: "security.devicesTitle", message: "Devices", comment: "Heading of the devices list." }),
    devicesCaption: i18n._({
      id: "security.devicesCaption",
      message: "Registered devices of this environment.",
      comment: "Caption under the devices heading.",
    }),
    search: i18n._({ id: "security.search", message: "Search devices", comment: "Label of the device search field." }),
    colId: i18n._({ id: "security.colId", message: "Device", comment: "Column heading." }),
    colPlatform: i18n._({ id: "security.colPlatform", message: "Platform", comment: "Column heading." }),
    colBuild: i18n._({ id: "security.colBuild", message: "App build", comment: "Column heading." }),
    colAssurance: i18n._({ id: "security.colAssurance", message: "Assurance", comment: "Column heading." }),
    colAttested: i18n._({ id: "security.colAttested", message: "Last attestation", comment: "Column heading." }),
    colState: i18n._({ id: "security.colState", message: "State", comment: "Column heading." }),
    stateActive: i18n._({
      id: "security.stateActive",
      message: "Active",
      comment: "State of a device that is not revoked.",
    }),
    stateRevoked: i18n._({ id: "security.stateRevoked", message: "Revoked", comment: "State of a revoked device." }),
    noDevices: i18n._({
      id: "security.noDevices",
      message: "No devices match.",
      comment: "Shown when the device list is empty.",
    }),
    loading: i18n._({ id: "security.loading", message: "Loading", comment: "Shown while a page loads." }),
    loadFailed: i18n._({
      id: "security.loadFailed",
      message: "This could not be loaded.",
      comment: "Shown when a request fails.",
    }),
    envMissing: i18n._({
      id: "security.envMissing",
      message: "This environment does not exist.",
      comment: "Shown when the environment of the URL is unknown.",
    }),
    deviceTitle: i18n._({ id: "security.deviceTitle", message: "Device", comment: "Heading of the device detail." }),
    osVersion: i18n._({ id: "security.osVersion", message: "OS version", comment: "Label." }),
    runtimeVersion: i18n._({ id: "security.runtimeVersion", message: "Runtime version", comment: "Label." }),
    keyStorage: i18n._({ id: "security.keyStorage", message: "Key storage", comment: "Label." }),
    keyId: i18n._({ id: "security.keyId", message: "Key thumbprint", comment: "Label of the DPoP key thumbprint." }),
    registered: i18n._({ id: "security.registered", message: "Registered", comment: "Label." }),
    lastSeen: i18n._({ id: "security.lastSeen", message: "Last seen", comment: "Label." }),
    provider: i18n._({ id: "security.provider", message: "Attestation provider", comment: "Label." }),
    never: i18n._({ id: "security.never", message: "Never", comment: "Shown when there is no date." }),
    revoke: i18n._({ id: "security.revoke", message: "Revoke device", comment: "Button that revokes a device." }),
    revokeDescription: i18n._({
      id: "security.revokeDescription",
      message: "The device loses access at once. The reason is kept in the audit log.",
      comment: "Description of the revoke dialog.",
    }),
    revokeNoPermission: i18n._({
      id: "security.revokeNoPermission",
      message: "You may not manage this app.",
      comment: "Tooltip of a disabled action.",
    }),
    back: i18n._({ id: "security.back", message: "Back to devices", comment: "Link." }),
    profileTitle: i18n._({
      id: "security.profileTitle",
      message: "Security profile",
      comment: "Heading of the security page.",
    }),
    profileCaption: i18n._({
      id: "security.profileCaption",
      message: "The profile and every setting in force for this environment.",
      comment: "Caption.",
    }),
    profileLabel: i18n._({ id: "security.profileLabel", message: "Profile", comment: "Label." }),
    profileVersion: i18n._({ id: "security.profileVersion", message: "Version", comment: "Label." }),
    effective: i18n._({
      id: "security.effective",
      message: "Effective configuration",
      comment: "Heading of the settings table.",
    }),
    colSetting: i18n._({ id: "security.colSetting", message: "Setting", comment: "Column heading." }),
    colValue: i18n._({ id: "security.colValue", message: "Value", comment: "Column heading." }),
    changeProfile: i18n._({ id: "security.changeProfile", message: "Change profile", comment: "Button." }),
    profileConfirm: i18n._({
      id: "security.profileConfirm",
      message: "Changing the profile resets the overrides of this environment. Type the environment key to confirm.",
      comment: "Description of the confirm dialog.",
    }),
    profileStandard: i18n._({ id: "security.profileStandard", message: "Standard", comment: "Profile name." }),
    profileStrict: i18n._({ id: "security.profileStrict", message: "Strict", comment: "Profile name." }),
    profileMaximum: i18n._({ id: "security.profileMaximum", message: "Maximum", comment: "Profile name." }),
    keysTitle: i18n._({ id: "security.keysTitle", message: "Signing keys", comment: "Heading." }),
    keysCaption: i18n._({
      id: "security.keysCaption",
      message: "Public keys that may sign for this environment. Keys are rotated in the offline ceremony.",
      comment: "Caption.",
    }),
    colKeyId: i18n._({ id: "security.colKeyId", message: "Key", comment: "Column heading." }),
    colRole: i18n._({ id: "security.colRole", message: "Role", comment: "Column heading." }),
    colAlgorithm: i18n._({ id: "security.colAlgorithm", message: "Algorithm", comment: "Column heading." }),
    colKind: i18n._({ id: "security.colKind", message: "Kind", comment: "Column heading." }),
    noKeys: i18n._({
      id: "security.noKeys",
      message: "No keys are published for this environment.",
      comment: "Empty state.",
    }),
    rootVersions: i18n._({
      id: "security.rootVersions",
      message: "Root versions served",
      comment: "Label of the rotation state.",
    }),
    ceremony: i18n._({ id: "security.ceremony", message: "Key ceremony runbook", comment: "Link text." }),
    pinsTitle: i18n._({ id: "security.pinsTitle", message: "Certificate pins", comment: "Heading." }),
    pinsCaption: i18n._({
      id: "security.pinsCaption",
      message: "Pins live in the root metadata made by the offline ceremony and cannot be edited here.",
      comment: "Caption.",
    }),
    noPins: i18n._({
      id: "security.noPins",
      message: "No pin keys are published for this environment.",
      comment: "Empty state.",
    }),
    capsTitle: i18n._({ id: "security.capsTitle", message: "Capability grants", comment: "Heading." }),
    capsCaption: i18n._({
      id: "security.capsCaption",
      message: "What plugins of this app may do (SEC-080).",
      comment: "Caption.",
    }),
    deviceApis: i18n._({ id: "security.deviceApis", message: "Device APIs", comment: "Label." }),
    networkDomains: i18n._({ id: "security.networkDomains", message: "Network domains", comment: "Label." }),
    functions: i18n._({ id: "security.functions", message: "Functions", comment: "Label." }),
    none: i18n._({ id: "security.none", message: "None", comment: "Shown for an empty list." }),
    editGrants: i18n._({ id: "security.editGrants", message: "Edit grants", comment: "Button." }),
    editGrantsDescription: i18n._({
      id: "security.editGrantsDescription",
      message: "One entry per line. Releases are published only when every plugin requests a subset.",
      comment: "Description of the dialog.",
    }),
    save: i18n._({ id: "security.save", message: "Save", comment: "Button." }),
    cancel: i18n._({ id: "security.cancel", message: "Cancel", comment: "Button." }),
    close: i18n._({ id: "security.close", message: "Close", comment: "Button." }),
    saveFailed: i18n._({
      id: "security.saveFailed",
      message: "The change could not be saved.",
      comment: "Shown when saving fails.",
    }),
    deviceOf: (id: string) =>
      i18n._({ id: "security.device-of", message: "Device {id}", values: { id }, comment: "Page title of a device." }),
  };
}

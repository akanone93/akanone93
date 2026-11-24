// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** The words of the Gateway area of an app (build log row 199). */
export function gatewayMessages(i18n: I18n) {
  return {
    loading: i18n._({ id: "gateway.loading", message: "Loading…", comment: "Shown while a Gateway page loads." }),
    loadFailed: i18n._({
      id: "gateway.loadFailed",
      message: "The gateway settings could not be loaded.",
      comment: "Shown when a Gateway page could not load.",
    }),
    saveFailed: i18n._({
      id: "gateway.saveFailed",
      message: "The change was not saved.",
      comment: "Shown when a change on a Gateway page was refused; the server's reason follows.",
    }),
    areaNav: i18n._({
      id: "gateway.areaNav",
      message: "Gateway pages",
      comment: "Accessible name of the tab strip of the Gateway area.",
    }),
    environment: i18n._({
      id: "gateway.environment",
      message: "Environment",
      comment: "Label of the choice of the app environment whose gateway the page shows.",
    }),
    noPermission: i18n._({
      id: "gateway.noPermission",
      message: "Changing the gateway needs the gateway.manage permission.",
      comment: "Tooltip on a disabled change button; gateway.manage is a permission name, not translated.",
    }),
    stepUpNote: i18n._({
      id: "gateway.stepUpNote",
      message: "Each change asks for your security key.",
      comment: "Caption under a Gateway page's title: every change needs a fresh security-key confirmation.",
    }),
    cancel: i18n._({ id: "gateway.cancel", message: "Cancel", comment: "Button that closes a Gateway dialog." }),
    save: i18n._({ id: "gateway.save", message: "Save", comment: "Button that saves a Gateway dialog." }),
    close: i18n._({ id: "gateway.close", message: "Close", comment: "Accessible name of a dialog's close button." }),
    edit: i18n._({ id: "gateway.edit", message: "Edit", comment: "Button that edits one row of a Gateway list." }),
    remove: i18n._({
      id: "gateway.remove",
      message: "Remove",
      comment: "Button that removes one row of a Gateway list.",
    }),
    removeConfirm: (name: string) =>
      i18n._({
        id: "gateway.removeConfirm",
        message: "Remove {name}? Devices lose what it gives them at once.",
        values: { name },
        comment: "Question before an upstream, a value or the identity provider is removed.",
      }),
    upstreamsTitle: i18n._({
      id: "gateway.upstreams.title",
      message: "Upstreams",
      comment: "Title of the page of gateway upstreams.",
    }),
    upstreamsCaption: i18n._({
      id: "gateway.upstreams.caption",
      message: "The APIs this environment's gateway may call for its devices, and how it signs in to each.",
      comment: "Caption of the page of gateway upstreams.",
    }),
    addUpstream: i18n._({
      id: "gateway.upstreams.add",
      message: "Add upstream",
      comment: "Button that adds a gateway upstream.",
    }),
    noUpstreams: i18n._({
      id: "gateway.upstreams.empty",
      message: "The gateway reaches no upstream in this environment.",
      comment: "Shown when an environment has no gateway upstream.",
    }),
    colName: i18n._({ id: "gateway.col.name", message: "Name", comment: "Column of a name." }),
    colBaseUrl: i18n._({
      id: "gateway.col.baseUrl",
      message: "Base URL",
      comment: "Column of an upstream's base URL.",
    }),
    colAuth: i18n._({
      id: "gateway.col.auth",
      message: "Sign-in",
      comment: "Column of how the gateway signs in to an upstream.",
    }),
    colMethods: i18n._({
      id: "gateway.col.methods",
      message: "Methods",
      comment: "Column of the HTTP methods an upstream allows.",
    }),
    colOperations: i18n._({
      id: "gateway.col.operations",
      message: "Operations",
      comment: "Column of the number of named operations of an upstream.",
    }),
    colActions: i18n._({ id: "gateway.col.actions", message: "Actions", comment: "Column of a row's buttons." }),
    authNone: i18n._({
      id: "gateway.auth.none",
      message: "None",
      comment: "The gateway sends no credentials to the upstream.",
    }),
    authApiKey: i18n._({
      id: "gateway.auth.apiKey",
      message: "API key",
      comment: "The gateway sends a key in a header.",
    }),
    authOAuth: i18n._({
      id: "gateway.auth.oauth",
      message: "OAuth client credentials",
      comment: "The gateway signs in with the OAuth 2.0 client credentials grant.",
    }),
    authMtls: i18n._({
      id: "gateway.auth.mtls",
      message: "Client certificate (mTLS)",
      comment: "The gateway signs in with a client certificate.",
    }),
    upstreamDialog: i18n._({
      id: "gateway.upstream.dialog",
      message: "Upstream",
      comment: "Title of the dialog that edits an upstream.",
    }),
    upstreamDialogDescription: i18n._({
      id: "gateway.upstream.dialogDescription",
      message: "Secrets are named, never written here: the gateway reads them from the environment's secrets.",
      comment: "Description of the upstream dialog.",
    }),
    fieldName: i18n._({
      id: "gateway.field.name",
      message: "Name",
      comment: "Field of an upstream's name, which data sources refer to.",
    }),
    fieldBaseUrl: i18n._({
      id: "gateway.field.baseUrl",
      message: "Base URL",
      comment: "Field of an upstream's base URL.",
    }),
    fieldAuth: i18n._({
      id: "gateway.field.auth",
      message: "Sign-in",
      comment: "Choice of how the gateway signs in to the upstream.",
    }),
    fieldHeader: i18n._({
      id: "gateway.field.header",
      message: "Header",
      comment: "Field of the header that carries an API key.",
    }),
    fieldSecretRef: i18n._({
      id: "gateway.field.secretRef",
      message: "Secret name",
      comment: "Field naming the environment secret that holds a credential.",
    }),
    fieldTokenUrl: i18n._({
      id: "gateway.field.tokenUrl",
      message: "Token URL",
      comment: "Field of an OAuth token endpoint.",
    }),
    fieldClientId: i18n._({
      id: "gateway.field.clientId",
      message: "Client ID",
      comment: "Field of an OAuth client ID.",
    }),
    fieldScopes: i18n._({
      id: "gateway.field.scopes",
      message: "Scopes, one per line",
      comment: "Field of OAuth scopes.",
    }),
    fieldMethods: i18n._({
      id: "gateway.field.methods",
      message: "Allowed methods",
      comment: "Group of the HTTP methods the gateway forwards.",
    }),
    fieldAssurance: i18n._({
      id: "gateway.field.assurance",
      message: "Lowest assurance level",
      comment: "Choice of the lowest device assurance level (AL0 to AL3) a call needs.",
    }),
    fieldTimeout: i18n._({
      id: "gateway.field.timeout",
      message: "Timeout in milliseconds (empty for the default)",
      comment: "Field of the time limit of one call to the upstream.",
    }),
    fieldAudience: i18n._({
      id: "gateway.field.audience",
      message: "Audience (empty for the base URL)",
      comment: "Field of the audience of the assertion the gateway sends to the upstream.",
    }),
    fieldPrivate: i18n._({
      id: "gateway.field.private",
      message: "Allow private and loopback addresses",
      comment: "Switch that lets an upstream resolve to a private network address.",
    }),
    privateWarning: i18n._({
      id: "gateway.field.privateWarning",
      message:
        "Unsafe: the gateway could then reach the server's own network. Leave it off unless the upstream is internal.",
      comment: "Warning beside the switch that allows private addresses.",
    }),
    fieldOperations: i18n._({
      id: "gateway.field.operations",
      message: "Operations (JSON list; empty allows any path)",
      comment: "Field of an upstream's named operations, written as a JSON list.",
    }),
    operationsInvalid: i18n._({
      id: "gateway.field.operationsInvalid",
      message: "The operations are not a valid JSON list of operations.",
      comment: "Shown when the operations field does not parse.",
    }),
    required: i18n._({
      id: "gateway.required",
      message: "Required.",
      comment: "Shown under a required field left empty.",
    }),
    configTitle: i18n._({
      id: "gateway.config.title",
      message: "Configuration values",
      comment: "Title of the page of an environment's configuration values.",
    }),
    configCaption: i18n._({
      id: "gateway.config.caption",
      message:
        "Values the gateway binds into requests (server-bound) or devices read (live). Sensitive values are sealed and never shown again.",
      comment: "Caption of the page of configuration values.",
    }),
    addValue: i18n._({
      id: "gateway.config.add",
      message: "Add value",
      comment: "Button that adds a configuration value.",
    }),
    noValues: i18n._({
      id: "gateway.config.empty",
      message: "This environment has no configuration values.",
      comment: "Shown when an environment has no configuration value.",
    }),
    colKey: i18n._({ id: "gateway.col.key", message: "Key", comment: "Column of a configuration value's key." }),
    colType: i18n._({ id: "gateway.col.type", message: "Type", comment: "Column of a configuration value's type." }),
    colTier: i18n._({
      id: "gateway.col.tier",
      message: "Reaches",
      comment: "Column saying whether the gateway or devices use a value.",
    }),
    colValue: i18n._({ id: "gateway.col.value", message: "Value", comment: "Column of a configuration value." }),
    colVersion: i18n._({
      id: "gateway.col.version",
      message: "Version",
      comment: "Column of a configuration value's version.",
    }),
    sealed: i18n._({
      id: "gateway.config.sealed",
      message: "Sealed",
      comment: "Shown instead of a sensitive value, which is never returned.",
    }),
    tierServer: i18n._({
      id: "gateway.tier.server",
      message: "Gateway (server-bound)",
      comment: "A value only the gateway reads.",
    }),
    tierLive: i18n._({ id: "gateway.tier.live", message: "Devices (live)", comment: "A value devices fetch." }),
    typeString: i18n._({ id: "gateway.type.string", message: "Text", comment: "A configuration value's type." }),
    typeInteger: i18n._({ id: "gateway.type.integer", message: "Integer", comment: "A configuration value's type." }),
    typeDecimal: i18n._({ id: "gateway.type.decimal", message: "Decimal", comment: "A configuration value's type." }),
    typeBoolean: i18n._({
      id: "gateway.type.boolean",
      message: "True or false",
      comment: "A configuration value's type.",
    }),
    valueDialog: i18n._({
      id: "gateway.value.dialog",
      message: "Configuration value",
      comment: "Title of the dialog that edits a value.",
    }),
    fieldKey: i18n._({
      id: "gateway.field.key",
      message: "Key (a letter, then letters, digits and underscores)",
      comment: "Field of a configuration value's key.",
    }),
    fieldType: i18n._({
      id: "gateway.field.type",
      message: "Type",
      comment: "Choice of a configuration value's type.",
    }),
    fieldTier: i18n._({
      id: "gateway.field.tier",
      message: "Read by",
      comment: "Choice of whether the gateway or devices read a value.",
    }),
    fieldSensitive: i18n._({
      id: "gateway.field.sensitive",
      message: "Sensitive: sealed at rest and never shown again",
      comment: "Switch marking a value as sensitive; only gateway values may be.",
    }),
    fieldStrict: i18n._({
      id: "gateway.field.strict",
      message: "Strict: a device with an older version of the live values is refused",
      comment: "Switch marking a live value as strict.",
    }),
    fieldValue: i18n._({
      id: "gateway.field.value",
      message: "Value",
      comment: "Field of a configuration value's content.",
    }),
    newSealedValue: i18n._({
      id: "gateway.field.newSealedValue",
      message: "Enter the value again: a sensitive value is never shown.",
      comment: "Placeholder of the value field when a sensitive value is edited.",
    }),
    identityTitle: i18n._({
      id: "gateway.identity.title",
      message: "User sign-in",
      comment: "Title of the page of the user identity provider.",
    }),
    identityCaption: i18n._({
      id: "gateway.identity.caption",
      message:
        "The identity provider whose user tokens this environment's devices may exchange, and the claims passed on to upstreams.",
      comment: "Caption of the page of the user identity provider.",
    }),
    noProvider: i18n._({
      id: "gateway.identity.none",
      message: "This environment trusts no user identity provider.",
      comment: "Shown when an environment has no user identity provider.",
    }),
    fieldIssuer: i18n._({
      id: "gateway.field.issuer",
      message: "Issuer",
      comment: "Field of the issuer (iss) the provider's tokens carry.",
    }),
    fieldJwks: i18n._({
      id: "gateway.field.jwks",
      message: "JWKS URL (HTTPS)",
      comment: "Field of where the provider publishes its signing keys.",
    }),
    fieldAudiences: i18n._({
      id: "gateway.field.audiences",
      message: "Audiences, one per line",
      comment: "Field of the accepted token audiences.",
    }),
    fieldClaims: i18n._({
      id: "gateway.field.claims",
      message: "Claims passed to upstreams, one per line (sub always is)",
      comment: "Field of the token claims the gateway forwards.",
    }),
    saveProvider: i18n._({
      id: "gateway.identity.save",
      message: "Save provider",
      comment: "Button that saves the identity provider.",
    }),
    removeProvider: i18n._({
      id: "gateway.identity.remove",
      message: "Remove provider",
      comment: "Button that removes the identity provider.",
    }),
  };
}

export type GatewayMessages = ReturnType<typeof gatewayMessages>;

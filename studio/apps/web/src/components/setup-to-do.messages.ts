// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** The server-side setup an imported or duplicated app still needs, by kind (build log row 178). */
export function setupToDoMessages(i18n: I18n) {
  return {
    title: i18n._({
      id: "setupToDo.title",
      message: "Still to set up on this server",
      comment: "Heading of the list of server-side setup an imported or duplicated app lacks.",
    }),
    intro: i18n._({
      id: "setupToDo.intro",
      message:
        "The archive carries the app's documents and assets. Its server-side setup is named here without any value: set each up by hand, with values from whoever owns them.",
      comment: "Explains the setup list after an import or a duplicate; secrets never travel in an archive.",
    }),
    environment: (name: string) =>
      i18n._({
        id: "setupToDo.environment",
        message: "Environment {name}",
        values: { name },
        comment: "Group heading: the setup of one environment.",
      }),
    createEnvironment: (name: string) =>
      i18n._({
        id: "setupToDo.createEnvironment",
        message: "Create the environment {name}",
        values: { name },
        comment: "A setup step.",
      }),
    createProductionEnvironment: (name: string) =>
      i18n._({
        id: "setupToDo.createProductionEnvironment",
        message: "Create the production environment {name}",
        values: { name },
        comment: "A setup step for an environment marked as production.",
      }),
    channel: (name: string) =>
      i18n._({
        id: "setupToDo.channel",
        message: "Create the channel {name}",
        values: { name },
        comment: "A setup step: a release channel.",
      }),
    variable: (name: string) =>
      i18n._({
        id: "setupToDo.variable",
        message: "Set the variable {name}",
        values: { name },
        comment: "A setup step.",
      }),
    secret: (name: string) =>
      i18n._({
        id: "setupToDo.secret",
        message: "Set the secret {name}",
        values: { name },
        comment: "A setup step; the value never travels.",
      }),
    config: (name: string, detail: string) =>
      i18n._({
        id: "setupToDo.config",
        message: "Set the configuration value {name} ({detail})",
        values: { name, detail },
        comment: "A setup step; detail is the tier and type, such as 'live integer'.",
      }),
    upstream: (name: string, detail: string) =>
      i18n._({
        id: "setupToDo.upstream",
        message: "Register the gateway upstream {name} ({detail})",
        values: { name, detail },
        comment: "A setup step; detail is how the upstream authenticates, such as 'mtls'.",
      }),
    identityProvider: i18n._({
      id: "setupToDo.identityProvider",
      message: "Set the identity provider",
      comment: "A setup step: the provider whose tokens the gateway accepts.",
    }),
    function: (name: string) =>
      i18n._({
        id: "setupToDo.function",
        message: "Deploy the function {name}",
        values: { name },
        comment: "A setup step.",
      }),
    schedule: (name: string, detail: string) =>
      i18n._({
        id: "setupToDo.schedule",
        message: "Create the schedule of {name} ({detail})",
        values: { name, detail },
        comment: "A setup step; name is function@alias, detail the cron expression and time zone.",
      }),
    hook: (name: string) =>
      i18n._({
        id: "setupToDo.hook",
        message: "Create the hook of {name}",
        values: { name },
        comment: "A setup step; name is function@alias.",
      }),
    unread: (kind: string) =>
      i18n._({
        id: "setupToDo.unread",
        message: "Check by hand: whoever exported the app could not read its {kind}",
        values: { kind },
        comment:
          "A setup step: a kind of setup the exporter had no permission to read; kind is one of the kind names below.",
      }),
    kindSecrets: i18n._({ id: "setupToDo.kind.secrets", message: "secrets", comment: "Kind name in the unread step." }),
    kindVariables: i18n._({
      id: "setupToDo.kind.variables",
      message: "variables",
      comment: "Kind name in the unread step.",
    }),
    kindChannels: i18n._({
      id: "setupToDo.kind.channels",
      message: "channels",
      comment: "Kind name in the unread step.",
    }),
    kindConfig: i18n._({
      id: "setupToDo.kind.config",
      message: "configuration values",
      comment: "Kind name in the unread step.",
    }),
    kindUpstreams: i18n._({
      id: "setupToDo.kind.upstreams",
      message: "gateway upstreams",
      comment: "Kind name in the unread step.",
    }),
    kindIdentityProvider: i18n._({
      id: "setupToDo.kind.identityProvider",
      message: "identity provider",
      comment: "Kind name in the unread step.",
    }),
    kindFunctions: i18n._({
      id: "setupToDo.kind.functions",
      message: "functions",
      comment: "Kind name in the unread step.",
    }),
    kindSchedules: i18n._({
      id: "setupToDo.kind.schedules",
      message: "schedules",
      comment: "Kind name in the unread step.",
    }),
    kindHooks: i18n._({ id: "setupToDo.kind.hooks", message: "hooks", comment: "Kind name in the unread step." }),
  };
}

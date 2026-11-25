// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { ReactElement } from "react";
import { setupToDoMessages } from "./setup-to-do.messages.ts";

/** One named piece of server-side setup, as the server lists it (`SetupItem`). */
export interface SetupStep {
  readonly kind: string;
  readonly environment: string;
  readonly name: string;
  readonly detail: string;
}

type Messages = ReturnType<typeof setupToDoMessages>;

/** The plural name of a kind of setup, for a kind the exporter could not read. */
function kindName(m: Messages, kind: string): string {
  const names: Record<string, string> = {
    secret: m.kindSecrets,
    variable: m.kindVariables,
    channel: m.kindChannels,
    config: m.kindConfig,
    upstream: m.kindUpstreams,
    identityProvider: m.kindIdentityProvider,
    function: m.kindFunctions,
    schedule: m.kindSchedules,
    hook: m.kindHooks,
  };
  return names[kind] ?? kind;
}

/** The sentence of one step; a kind this Studio does not know is shown as the server names it. */
export function stepText(m: Messages, s: SetupStep): string {
  switch (s.kind) {
    case "environment":
      return s.detail === "production" ? m.createProductionEnvironment(s.name) : m.createEnvironment(s.name);
    case "channel":
      return m.channel(s.name);
    case "variable":
      return m.variable(s.name);
    case "secret":
      return m.secret(s.name);
    case "config":
      return m.config(s.name, s.detail);
    case "upstream":
      return m.upstream(s.name, s.detail);
    case "identityProvider":
      return m.identityProvider;
    case "function":
      return m.function(s.name);
    case "schedule":
      return m.schedule(s.name, s.detail);
    case "hook":
      return m.hook(s.name);
    case "unread":
      return m.unread(kindName(m, s.name));
    default:
      return [s.kind, s.name, s.detail].filter((x) => x !== "").join(" ");
  }
}

/**
 * The server-side setup an imported or duplicated app lacks, by environment (build log row 178): the archive names it
 * without values, so each piece is set up by hand. Nothing is shown when nothing is missing.
 */
export function SetupToDo({ steps }: { readonly steps: readonly SetupStep[] }): ReactElement | null {
  const m = setupToDoMessages(useLingui().i18n);
  if (steps.length === 0) {
    return null;
  }
  const environments = [...new Set(steps.map((s) => s.environment))];
  return (
    <section aria-labelledby="setup-to-do" className="flex flex-col gap-2">
      <h3 id="setup-to-do" className="text-section">
        {m.title}
      </h3>
      <p className="text-body text-muted-foreground">{m.intro}</p>
      {environments.map((env) => (
        <div key={env} className="flex flex-col gap-1">
          <h4 className="text-caption font-medium">{m.environment(env)}</h4>
          <ul className="flex list-disc flex-col gap-0.5 ps-5 text-body">
            {steps
              .filter((s) => s.environment === env)
              .map((s) => (
                <li key={`${s.kind}:${s.name}:${s.detail}`}>{stepText(m, s)}</li>
              ))}
          </ul>
        </div>
      ))}
    </section>
  );
}

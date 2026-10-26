// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Select, TypeTag } from "@plux/ui";
import { useState } from "react";
import { linkMessages } from "../plugin-canvas/links.messages.ts";
import {
  type ApplyMode,
  bindIssue,
  eventsOf,
  linkRecipe,
  linksOf,
  type NavigateLink,
  type RouteOption,
  redirectRecipe,
  removeLinkRecipe,
  targetIssues,
} from "../plugin-canvas/links.ts";
import { uuidv7 } from "../plugin-canvas/screens.ts";

export interface ActionsTabProps {
  readonly nodeId: string;
  /** The widget as the edit session holds it. */
  readonly node: { readonly type?: unknown } | undefined;
  readonly routes: readonly RouteOption[];
  readonly canEdit: boolean;
  /** Writes a recipe over the page's root through the edit pipeline (JSON Patch, one undo step). */
  readonly apply: (recipe: (root: unknown) => void) => void;
  readonly newId?: () => string;
}

/** F6 primary path: add a navigate action to an event of the selected widget, and fix links that broke. */
export function ActionsTab(props: ActionsTabProps) {
  const m = linkMessages(useLingui().i18n);
  const type = typeof props.node?.type === "string" ? props.node.type : undefined;
  const events = eventsOf(type);
  const [chosenEvent, setEvent] = useState<string | undefined>(undefined);
  const [chosenRoute, setRoute] = useState<string | undefined>(undefined);
  const [mode, setMode] = useState<ApplyMode>("replace");
  const newId = props.newId ?? uuidv7;
  const event = chosenEvent !== undefined && events.includes(chosenEvent) ? chosenEvent : events[0];
  const route =
    chosenRoute !== undefined && props.routes.some((r) => r.route === chosenRoute) ? chosenRoute : undefined;
  const target = route ?? props.routes[0]?.route;
  const issue = event === undefined ? undefined : bindIssue(props.node, event);
  const links = linksOf(props.node);
  const broken = new Set(
    targetIssues(props.node, props.routes).flatMap((i) => (i.kind === "target-missing" ? [key(i.link)] : [])),
  );
  const label = (r: RouteOption) => m.targetOption(r.title, r.own ? m.thisPlugin : m.otherPlugin);

  if (events.length === 0) {
    return <p className="text-body text-secondary">{m.noEvents}</p>;
  }
  return (
    <div className="flex flex-col gap-3">
      <section className="flex flex-col gap-2">
        <h3 className="text-label font-medium">{m.heading}</h3>
        <Select
          label={m.event}
          value={event ?? ""}
          options={events.map((e) => ({ value: e, label: e }))}
          disabled={!props.canEdit}
          onValueChange={setEvent}
        />
        {props.routes.length === 0 ? (
          <p className="text-body text-secondary">{m.noTarget}</p>
        ) : (
          <Select
            label={m.target}
            value={target ?? ""}
            options={props.routes.map((r) => ({ value: r.route, label: label(r) }))}
            disabled={!props.canEdit}
            onValueChange={setRoute}
          />
        )}
        {issue?.kind === "event-bound" && event !== undefined && (
          <fieldset className="flex flex-col gap-1">
            <legend className="text-body text-secondary">{m.bound(event)}</legend>
            {(["replace", "append"] as const).map((value) => (
              <label key={value} className="flex items-center gap-2 text-body">
                <input type="radio" name="link-mode" checked={mode === value} onChange={() => setMode(value)} />
                {value === "replace" ? m.replace : m.append}
              </label>
            ))}
          </fieldset>
        )}
        <Button
          disabled={!props.canEdit || event === undefined || target === undefined}
          onClick={() => {
            if (event !== undefined && target !== undefined) {
              props.apply(linkRecipe(props.nodeId, event, target, newId(), issue === undefined ? "replace" : mode));
            }
          }}
        >
          {m.add}
        </Button>
      </section>
      <section className="flex flex-col gap-2">
        <h3 className="text-label font-medium">{m.existing}</h3>
        {links.length === 0 ? (
          <p className="text-body text-secondary">{m.none}</p>
        ) : (
          <ul className="flex flex-col gap-2">
            {links.map((l) => (
              <li key={key(l)} className="flex flex-col gap-1 text-body">
                <span>
                  <TypeTag>{l.event}</TypeTag> {m.linkRow(l.event, l.route)}
                </span>
                {broken.has(key(l)) && <p role="alert">{m.missing(l.route)}</p>}
                <span className="flex gap-2">
                  {broken.has(key(l)) && (
                    <Button
                      size="sm"
                      variant="ghost"
                      disabled={!props.canEdit || target === undefined}
                      onClick={() => target !== undefined && props.apply(redirectRecipe(props.nodeId, l, target))}
                    >
                      {m.redirect}
                    </Button>
                  )}
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={!props.canEdit}
                    onClick={() => props.apply(removeLinkRecipe(props.nodeId, l))}
                  >
                    {m.remove}
                  </Button>
                </span>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}

const key = (l: NavigateLink): string => `${l.event}:${String(l.step)}`;

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Document } from "@plux/api";
import { useRouteContext } from "@tanstack/react-router";
import { type ReactNode, useMemo } from "react";
import type { RouterContext } from "../../router.tsx";
import { ActionGraphOverlay } from "../action-graph/action-graph-overlay.tsx";
import { extrasOf } from "../action-graph/graph-model.ts";
import { useNativeActions } from "../action-graph/use-native-actions.ts";
import { documentJson } from "../plugin-canvas/plugin-model.ts";
import { useAssetNames } from "../plugin-canvas/use-asset-facts.ts";
import { useFunctionList, useValidation } from "../problems/use-validation.ts";
import { browserValidator } from "../problems/validator-client.ts";

type Json = Record<string, unknown>;
const isObj = (v: unknown): v is Json => typeof v === "object" && v !== null && !Array.isArray(v);

/** The app's own event handlers in `app.json` (`triggers`), each with the pointer to its handler (#284). */
export function appHandlers(app: unknown): { readonly key: string; readonly pointer: readonly string[] }[] {
  const json = isObj(app) ? app : {};
  const fixed = ["onAppResume", "onAppPause", "onPushOpened", "onError"].map((key) => ({
    key,
    pointer: ["triggers", key],
  }));
  // A host event the app declares can be handled by name (HST-013); one handled already is listed too.
  const declared = Array.isArray(json["hostEvents"])
    ? json["hostEvents"].flatMap((e) => (isObj(e) && typeof e["name"] === "string" ? [e["name"]] : []))
    : [];
  const triggers = isObj(json["triggers"]) ? json["triggers"] : {};
  const handled = isObj(triggers["hostEvents"]) ? Object.keys(triggers["hostEvents"]) : [];
  const hostEvents = [...new Set([...declared, ...handled])].sort().map((name) => ({
    key: `hostEvents.${name}`,
    pointer: ["triggers", "hostEvents", name],
  }));
  const sources = isObj(triggers["dataSources"]) ? Object.keys(triggers["dataSources"]).sort() : [];
  const dataSources = sources.map((name) => ({
    key: `dataSources.${name}`,
    pointer: ["triggers", "dataSources", name],
  }));
  return [...fixed, ...hostEvents, ...dataSources];
}

/**
 * The app's own logic on the app canvas (#284): what the app does when it resumes or pauses, when a push is opened, on
 * an error, on each host event it declares and on its data sources' events — the Action graph editor a plugin's
 * widgets have, on `app.json`'s triggers, edited by the app canvas's session under the app lock and checked against
 * the whole project. Mounted only while open, so the validator starts only then. Timers and state watchers, lists
 * of their own, are edited in the app's JSON (#260).
 */
export function AppLogic(props: {
  readonly appId: string;
  readonly documents: readonly Document[] | undefined;
  readonly project: readonly Document[] | undefined;
  readonly onClose: () => void;
}): ReactNode {
  const { createValidator = browserValidator } = useRouteContext({ strict: false }) as Pick<
    RouterContext,
    "createValidator"
  >;
  const assetNames = useAssetNames(props.appId);
  const functionList = useFunctionList();
  const validation = useValidation(createValidator, props.project, props.documents, assetNames, functionList);
  const custom = useNativeActions(props.appId);
  const project = props.project;
  const extras = useMemo(
    () =>
      extrasOf(
        (project ?? []).map((d) => documentJson(d)),
        custom,
      ),
    [project, custom],
  );
  const app = props.documents?.find((d) => d.path === "app.json");
  const handlers = useMemo(
    () => ({ path: "app.json", events: appHandlers(app === undefined ? undefined : documentJson(app)) }),
    [app],
  );
  const checked = validation.state === "done" ? validation : undefined;
  return (
    <ActionGraphOverlay
      open
      onClose={props.onClose}
      documents={props.documents ?? []}
      pagePath={undefined}
      nodeId={undefined}
      handlers={handlers}
      extras={extras}
      validatePage={checked?.validatePage}
      language={checked?.language}
    />
  );
}

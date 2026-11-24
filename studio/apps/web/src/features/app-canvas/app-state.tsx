// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Document } from "@plux/api";
import { useRouteContext } from "@tanstack/react-router";
import { type ReactNode, useMemo } from "react";
import type { RouterContext } from "../../router.tsx";
import { useCanEdit, useEditSession } from "../editor/editor-provider.tsx";
import { uuidv7 } from "../plugin-canvas/screens.ts";
import { useAssetNames } from "../plugin-canvas/use-asset-facts.ts";
import { useValidation } from "../problems/use-validation.ts";
import { browserValidator } from "../problems/validator-client.ts";
import { loadedDocs } from "../safety/index.ts";
import { StateDesignerOverlay } from "../state-designer/index.ts";

/**
 * The app's own state on the app canvas (#283): the State designer a plugin has, on `app.json`'s `state`, edited by
 * the app canvas's session under the app lock and checked against the whole project. Mounted only while open, so the
 * validator starts only when it is used. The app's types are edited in its JSON (#260); the designer offers them.
 */
export function AppStateDesigner(props: {
  readonly appId: string;
  /** The app's own documents, which the app canvas's session edits. */
  readonly documents: readonly Document[] | undefined;
  /** The whole project's documents, every plugin's included. */
  readonly project: readonly Document[] | undefined;
  readonly onClose: () => void;
}): ReactNode {
  const { createValidator = browserValidator } = useRouteContext({ strict: false }) as Pick<
    RouterContext,
    "createValidator"
  >;
  const assetNames = useAssetNames(props.appId);
  const validation = useValidation(createValidator, props.project, props.documents, assetNames);
  const session = useEditSession();
  const editable = useCanEdit();
  const loaded = useMemo(() => loadedDocs(props.project ?? []), [props.project]);
  const appPath = props.documents?.some((d) => d.path === "app.json") === true ? "app.json" : undefined;
  return (
    <StateDesignerOverlay
      open
      onClose={props.onClose}
      session={session}
      editable={editable}
      pluginPath={appPath}
      pluginId=""
      documents={loaded}
      validatePage={validation.state === "done" ? validation.validatePage : undefined}
      newId={uuidv7}
    />
  );
}

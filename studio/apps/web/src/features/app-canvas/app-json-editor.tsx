// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Document } from "@plux/api";
import { useRouteContext } from "@tanstack/react-router";
import type { ReactNode } from "react";
import type { RouterContext } from "../../router.tsx";
import { JsonEditorOverlay, type LoadPublished } from "../json-editor/index.ts";
import { useAssetNames } from "../plugin-canvas/use-asset-facts.ts";
import { useValidation } from "../problems/use-validation.ts";
import { browserValidator } from "../problems/validator-client.ts";

/** The app's own documents have no plugin release to compare with: the comparison says nothing is published. */
const noPublished: LoadPublished = async () => null;

/**
 * The JSON editor of the app's own documents, from the app canvas (#260, #285): `app.json` — its navigation, native
 * routes, capabilities, state and the parts no form edits — the theme, the translation keys and the rest, edited by
 * the app canvas's own session under the app lock and checked by the validator as the plugin canvas checks a plugin's,
 * against the whole project. Mounted only while open, so the validator starts only when it is used.
 */
export function AppJsonEditor(props: {
  readonly appId: string;
  /** The app's own documents, which the app canvas's session edits. */
  readonly documents: readonly Document[] | undefined;
  /** The whole project's documents, every plugin's included, for the validator. */
  readonly project: readonly Document[] | undefined;
  readonly path?: string;
  readonly onClose: () => void;
}): ReactNode {
  const { createValidator = browserValidator } = useRouteContext({ strict: false }) as Pick<
    RouterContext,
    "createValidator"
  >;
  const assetNames = useAssetNames(props.appId);
  const validation = useValidation(createValidator, props.project, props.documents, assetNames);
  return (
    <JsonEditorOverlay
      open
      onClose={props.onClose}
      documents={props.documents ?? []}
      initialPath={props.path ?? "app.json"}
      validatePage={validation.state === "done" ? validation.validatePage : undefined}
      loadPublished={noPublished}
    />
  );
}

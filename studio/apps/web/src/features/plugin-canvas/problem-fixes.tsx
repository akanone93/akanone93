// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, FileDrop, Icon } from "@plux/ui";
import type { Problem } from "@plux/validator";
import { useQueryClient } from "@tanstack/react-query";
import { type ReactElement, useState } from "react";
import { withReason } from "../../components/refusal.tsx";
import type { EditSession } from "../editor/edit-session.ts";
import { uploadAsset } from "../panels/upload-asset.ts";
import { helpFor, type ProblemFix, withoutPlugin, withPlugin } from "../problems/problem-help.ts";
import { problemsMessages } from "../problems/problems.messages.ts";
import type { useAssetsTab } from "./use-assets-tab.ts";

export interface FixContext {
  /** Whether the JSON editor here can open a file. */
  readonly editable: (file: string) => boolean;
  readonly openJson: (file: string) => void;
  /** The app session that holds app.json under the app lock; null until it is ready. */
  readonly app: { readonly session: EditSession; readonly canEdit: boolean } | null;
  readonly assets: ReturnType<typeof useAssetsTab>;
}

/**
 * Under a finding in the Problems tab: what it means and the buttons that fix it (P1). A missing asset is uploaded again
 * under its name, so its entry and every use of it work again; app.json's plugins list is corrected in place; any file
 * the JSON editor can open opens at the finding.
 */
export function ProblemFixes({
  problem,
  ctx,
}: {
  readonly problem: Problem;
  readonly ctx: FixContext;
}): ReactElement | null {
  const m = problemsMessages(useLingui().i18n);
  const queries = useQueryClient();
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState<unknown>(null);
  const help = helpFor(problem, ctx.editable);
  if (help.why === undefined && help.fixes.length === 0) {
    return null;
  }
  const why = {
    assetMissing: m.whyAssetMissing,
    pluginNotListed: m.whyPluginNotListed,
    pluginNoDirectory: m.whyPluginNoDirectory,
    duplicateRoute: m.whyDuplicateRoute,
    unresolved: m.whyUnresolved,
  };
  const run = async (fix: ProblemFix, file?: File) => {
    setFailure(null);
    setBusy(true);
    try {
      switch (fix.kind) {
        case "uploadAsset":
          if (file !== undefined) {
            await uploadAsset(
              ctx.assets.client,
              { appId: ctx.assets.appId, session: ctx.assets.session, name: fix.file, file },
              { current: null },
              () => undefined,
            );
            await queries.invalidateQueries({ queryKey: ["assets", ctx.assets.appId] });
          }
          break;
        case "listPlugin":
        case "unlistPlugin":
          ctx.app?.session.edit(
            "app.json",
            (d) => {
              d["plugins"] =
                fix.kind === "listPlugin"
                  ? withPlugin(d["plugins"], fix.plugin)
                  : withoutPlugin(d["plugins"], fix.plugin);
            },
            { group: `problem-fix:${fix.kind}:${fix.plugin}` },
          );
          break;
        case "openJson":
          ctx.openJson(fix.file);
          break;
      }
    } catch (error) {
      setFailure(error);
    } finally {
      setBusy(false);
    }
  };
  const appWritable = ctx.app?.canEdit === true;
  return (
    <span className="problem-help">
      {help.why !== undefined && <span className="text-caption text-muted-foreground">{why[help.why]}</span>}
      <span className="flex flex-wrap gap-1.5">
        {help.fixes.map((fix) => {
          switch (fix.kind) {
            case "uploadAsset":
              return (
                <FileDrop
                  key={fix.kind}
                  variant="button"
                  label={m.fixUploadAsset(fix.file)}
                  disabled={busy}
                  onFiles={(files) => {
                    const file = files[0];
                    if (file !== undefined) {
                      void run(fix, file);
                    }
                  }}
                />
              );
            case "listPlugin":
            case "unlistPlugin":
              return (
                <Button
                  key={fix.kind}
                  size="sm"
                  variant="secondary"
                  disabled={busy || !appWritable}
                  onClick={() => void run(fix)}
                >
                  <Icon name="check-circle" size="pill" />
                  {fix.kind === "listPlugin" ? m.fixListPlugin(fix.plugin) : m.fixUnlistPlugin(fix.plugin)}
                </Button>
              );
            case "openJson":
              return (
                <Button key={fix.kind} size="sm" variant="ghost" onClick={() => void run(fix)}>
                  <Icon name="braces" size="pill" />
                  {m.fixOpenJson}
                </Button>
              );
          }
          return null;
        })}
      </span>
      {failure !== null && (
        <span role="alert" className="text-caption text-danger">
          {withReason(m.fixFailed, failure)}
        </span>
      )}
    </span>
  );
}

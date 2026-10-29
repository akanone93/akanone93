// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { FunctionAdminService, LanguageServerService } from "@plux/api";
import type { ReactElement } from "react";
import { useMemo } from "react";
import { useApp } from "../../lib/use-app.tsx";
import { CanEditContext } from "../editor/editor-provider.tsx";
import { LanguageBridge } from "../rust-editor/lsp-bridge.ts";
import { RustEditorPage } from "../rust-editor/rust-editor-overlay.tsx";
import { usePermissions } from "../security/use-security.ts";

/** A12: the Rust editor of a function as a full page, the same editor as the overlay (FN-033, STU-110). */
export function FunctionEditPage(): ReactElement {
  const { session } = useApp();
  const canEdit = usePermissions().has("function.deploy");
  const language = useMemo(() => createClient(LanguageServerService, session.transport), [session.transport]);
  const tests = useMemo(() => createClient(FunctionAdminService, session.transport), [session.transport]);
  const createBridge = useMemo(() => () => new LanguageBridge(language), [language]);
  return (
    <CanEditContext.Provider value={canEdit}>
      <RustEditorPage createBridge={createBridge} tests={tests} />
    </CanEditContext.Provider>
  );
}

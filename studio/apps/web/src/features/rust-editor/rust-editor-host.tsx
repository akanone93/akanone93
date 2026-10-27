// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { FunctionAdminService, LanguageServerService } from "@plux/api";
import { useMemo } from "react";
import { useApp } from "../../lib/use-app.tsx";
import { LanguageBridge } from "./lsp-bridge.ts";
import { RustEditorOverlay } from "./rust-editor-overlay.tsx";

export interface RustEditorHostProps {
  readonly open: boolean;
  readonly onClose: () => void;
}

/** The Rust editor of the plugin canvas, talking to the server through the session's transport. */
export function RustEditorHost({ open, onClose }: RustEditorHostProps) {
  const { session } = useApp();
  const language = useMemo(() => createClient(LanguageServerService, session.transport), [session.transport]);
  const tests = useMemo(() => createClient(FunctionAdminService, session.transport), [session.transport]);
  const createBridge = useMemo(() => () => new LanguageBridge(language), [language]);
  return <RustEditorOverlay open={open} onClose={onClose} createBridge={createBridge} tests={tests} />;
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Monaco's editor worker (diffing, word ranges); its own file so the build names the asset
// `monaco-editor-worker-<hash>.js`, the address the Trusted Types policy admits (S5.1).
import "monaco-editor/editor/editor.worker";

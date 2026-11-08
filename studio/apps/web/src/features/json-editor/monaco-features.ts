// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Monaco's editor features (code lens, inlay hints, suggestions, drag and drop, code actions and the rest), loaded
// before the first editor is created. `editor.api.js` is the core only; in monaco-editor 0.57 the features arrive
// with the module the language workers load (`internal/common/workers.js`), which the JSON support imports lazily,
// after an editor exists. Monaco reads its registered services once, when the first editor is created, so services
// registered later are unknown to it and every editor then threw "depends on UNKNOWN service" for those features.
// Importing them here, before any editor, registers the features and their services together; a Monaco upgrade that
// moves them fails monaco-features.test.ts.
import "monaco-editor/editor/editor.api.js";
import "monaco-editor/internal/common/workers.js";

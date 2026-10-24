// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/** Registers happy-dom globally so component tests run under `bun test` (studio-development §12). */

import { GlobalRegistrator } from "@happy-dom/global-registrator";

// Keep Bun's own network and binary globals: the BFF tests serve files through them.
const native = {
  AbortController: globalThis.AbortController,
  AbortSignal: globalThis.AbortSignal,
  Blob: globalThis.Blob,
  File: globalThis.File,
  FormData: globalThis.FormData,
  Headers: globalThis.Headers,
  Request: globalThis.Request,
  Response: globalThis.Response,
  fetch: globalThis.fetch,
};

GlobalRegistrator.register();
Object.assign(globalThis, native);

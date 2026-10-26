// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Worker entry of the validator (S2.1): instantiates the Go validator built by
// `make studio-validator` (hashed assets by Vite) and serves the validator protocol.

import "@plux/validator/wasm_exec.js";
import type { ProjectFile, PxlOp, ValidatorEngine } from "@plux/validator";
import wasmURL from "@plux/validator/validator.wasm?url";
import { serveValidatorWorker } from "@plux/validator/worker";

declare class Go {
  importObject: WebAssembly.Imports;
  run(instance: WebAssembly.Instance): Promise<void>;
}
type PluxValidatePage = (files: readonly ProjectFile[], file: string, data: string) => string;
type PluxPxl = (op: PxlOp, scope: string, src: string, offset: number) => string;
type PluxPxlSites = (file: string, data: string) => string;

const go = new Go();
const { instance } = await WebAssembly.instantiateStreaming(fetch(wasmURL), go.importObject);
void go.run(instance); // runs for the worker's lifetime; the module registers pluxValidatePage first

const module = self as unknown as { pluxValidatePage: PluxValidatePage; pluxPxl: PluxPxl; pluxPxlSites: PluxPxlSites };
const validate = module.pluxValidatePage;
const engine: ValidatorEngine = {
  load: (files) => validate(files, "", ""),
  validatePage: (file, data) => validate([], file, data),
  pxl: (op, scope, src, offset) => module.pluxPxl(op, scope, src, offset),
  pxlSites: (file, data) => module.pluxPxlSites(file, data),
};
serveValidatorWorker(self as unknown as Parameters<typeof serveValidatorWorker>[0], engine);

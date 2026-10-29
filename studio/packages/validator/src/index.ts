// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

export { ValidatorCancelled, ValidatorHost, type ValidatorWorkerLike } from "./host.ts";
export { codePointsBefore, type LineColumn, offsetAt, positionAt, utf16Index } from "./positions.ts";
export {
  type Problem,
  type ProjectFile,
  type PxlOp,
  parseProblems,
  parseValue,
  type ToolName,
  type ValidatorEngine,
  type ValidatorRequest,
  type ValidatorResponse,
} from "./protocol.ts";
export {
  createPxlProviders,
  type MonacoRange,
  markerSeverity,
  type PxlHoverResult,
  type PxlMarker,
  type PxlProviders,
  type PxlSignatureResult,
  type PxlSuggestion,
  pxlKeywords,
  pxlLanguageConfiguration,
  pxlLanguageId,
  pxlMonarch,
  pxlTriggerCharacters,
} from "./pxl-language.ts";
export {
  type PxlAnalysis,
  type PxlBackend,
  type PxlCompletion,
  type PxlCompletionKind,
  type PxlCompletions,
  type PxlDiagnostic,
  type PxlHover,
  type PxlScope,
  PxlService,
  type PxlSignature,
  type PxlSignatureHelp,
  type PxlSites,
  parseSites,
  scopeAt,
  serializeScope,
} from "./pxl-service.ts";
export { serveValidatorWorker, type ValidatorScope } from "./worker.ts";

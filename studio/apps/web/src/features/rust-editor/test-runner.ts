// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

export interface TestCaseResult {
  name: string;
  passed: boolean;
  message: string;
}

export interface TestRunState {
  status: "idle" | "running" | "finished" | "failed";
  results: TestCaseResult[];
  passed: number;
  failed: number;
  /** Build diagnostics as the server sent them, when the source did not build. */
  diagnostics: string;
  error: string;
}

export const idleRun: TestRunState = { status: "idle", results: [], passed: 0, failed: 0, diagnostics: "", error: "" };

/** The slice of `FunctionAdminService` the runner uses. */
export interface TestsClient {
  runTests(
    request: { sourceId: string },
    options: { signal: AbortSignal },
  ): AsyncIterable<{
    event:
      | { case: "result"; value: TestCaseResult }
      | { case: "finished"; value: { passed: number; failed: number; diagnostics: Uint8Array } }
      | { case: undefined; value?: undefined };
  }>;
}

/**
 * Runs the stored test cases of an uploaded source in the build sandbox (FN-034) and reports every state to `onState`,
 * one per case as it arrives. Resolves when the run ended; aborting `signal` stops it quietly.
 */
export async function runStoredTests(
  client: TestsClient,
  sourceId: string,
  onState: (state: TestRunState) => void,
  signal: AbortSignal,
): Promise<void> {
  let state: TestRunState = { ...idleRun, status: "running" };
  onState(state);
  try {
    for await (const { event } of client.runTests({ sourceId }, { signal })) {
      if (event.case === "result") {
        state = { ...state, results: [...state.results, event.value] };
      } else if (event.case === "finished") {
        const { passed, failed, diagnostics } = event.value;
        state = { ...state, status: "finished", passed, failed, diagnostics: new TextDecoder().decode(diagnostics) };
      }
      onState(state);
    }
    if (state.status === "running") {
      onState({ ...state, status: "failed", error: "the run ended without a summary" });
    }
  } catch (err) {
    if (!signal.aborted) {
      onState({ ...state, status: "failed", error: err instanceof Error ? err.message : "the run failed" });
    }
  }
}

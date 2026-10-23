<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# test/devloop

The scripted debugging session of exit criterion X2 (P10): a developer with no Studio
debugs a flow with the CLI alone (`DEV-041`, `DEV-021`, `DEV-020`, `FN-036`).

```bash
make devloop-session    # needs PLUX_TEST_DATABASE_URL (make test-db)
```

The session is a Go test, [`TestDevLoopSession_…`](../../backend/internal/devsession/devloop_session_integration_test.go),
and lives in `backend/internal/devsession` because it uses that package's database
fixtures and the server's internal packages, which a package outside `backend/` cannot
import. It builds the real `plux` binary and drives it as a developer would:

1. `plux dev` opens a session against an in-process server and prints the pairing link.
2. A simulated device pairs with the link's token and opens the real `/dev/socket`.
3. Saving an edit to a page of the project makes `plux dev` push it; the device receives
   the compiled bundle.
4. The device reports a failed action run (with its node path and steps), a state diff
   and a server function call whose log lines the server stored under the same trace ID.
5. `plux dev logs --filter action` prints the trace with its node path; `--filter
   state_diff` the change; `--filter function_call` the call with the server log lines
   merged into it.
6. `plux dev trace export` writes a trace file; `plux dev trace show` prints the same
   events as the live stream.

The emulator variant, with the real `plux_flutter` runtime on a device, runs in CI's
emulator job; this test is its server and CLI half.

The ten-minute quick start has its own timed dry run, `make quickstart-dry-run`
([quick-start guide](../../docs/guides/quick-start.md)).

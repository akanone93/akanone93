# ee — enterprise edition

Enterprise-edition modules, behind a clean interface boundary: the open-source core builds,
tests and runs without this directory (`GOV-033`), checked by `make go-test-core`. Production
use requires a licence file ([LICENSE](LICENSE), a draft for legal review); security
capabilities are never placed here (`GOV-032`). The split is recorded in
[ADR-0022](../docs/adr/0022-open-core-licensing.md) and [ADR-0084](../docs/adr/0084-enterprise-edition-and-licence-files.md).

| Path | What it is |
|---|---|
| `backend/license` | Verifies the signed licence file (Ed25519, the update-metadata envelope): valid, grace, expired (read-only), invalid, absent; re-verified hourly (`GOV-030`) |
| `backend/cmd/plux-server-ee` | The server with the licence verifier registered; enterprise features follow the licence |
| `backend/cmd/plux-license` | The maintainer's offline tool: `keygen`, `sign`, `verify`; never built into an image |

The core side is `backend/edition` (feature names, `Enabled`, `EnabledForWrite`, `Readable`;
no licence → every enterprise feature off) and `backend/edition/licensefile` (the public
parsing and verification used by `plux license inspect`). The installation reads its licence
from `PLUX_LICENSE_FILE`; `plux admin license` shows its state.

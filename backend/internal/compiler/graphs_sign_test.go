// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// A signTransaction step must name a transaction: a mutating operation of
// a source that declares the fields the user signs (SEC-027, SEC-028). An
// operation that is not one, or that does not exist, is refused before a
// device would ask the user to confirm a summary no request is held to.
func TestSignTransactionNamesATransaction_SEC_027(t *testing.T) {
	t.Parallel()
	dir := filepath.Join("..", "..", "..", "schema", "testdata", "documents", "plux_bank")
	const page = "plugins/banking/pages/transfer.page.json"
	const ptr = "/root/slots/body/slots/child/slots/child/children/17/events/onPressed"
	for transaction, code := range map[string]plxerr.Code{
		"payments.transfer": 0,
		"api.login":         plxerr.UnresolvedReference,
		"payments.refund":   plxerr.UnresolvedReference,
	} {
		m := project(t, dir)
		edit(t, m, page, func(doc map[string]any) {
			at(t, doc, "root/slots/body/slots/child/slots/child/children/17/events")["onPressed"] = raw(t, `{"steps": [
				{"id": "sign", "action": "signTransaction", "input": {"transaction": `+strconv.Quote(transaction)+`,
					"amount": {"$expr": "money(\"10.00\", \"EUR\")"}, "payee": "DE89370400440532013000"}}]}`)
		})
		res := compileFS(m)
		if code == 0 {
			// The fixture's function lives outside it, so only the step's own findings count here.
			for _, d := range res.Diagnostics {
				if d.Severity == plxerr.SeverityError && strings.HasPrefix(d.Path, ptr) {
					t.Errorf("%s: %s", transaction, d.Message)
				}
			}
			continue
		}
		wantDiag(t, res, code, page, ptr+"/steps/0/input/transaction")
	}
}

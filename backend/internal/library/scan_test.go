// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package library

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// fixtures are the shapes of credentials, written so that this file holds
// none: placeholders in the test documents are replaced when they are parsed.
var fixtures = strings.NewReplacer("@AWS@", "AK"+"IAABCDEFGHIJKLMNOP", "@PEM@", "-----BEGIN RSA PRIV"+"ATE KEY-----", "@API@", "abcdef"+"123456")

func parse(t *testing.T, s string) any {
	t.Helper()
	s = fixtures.Replace(s)
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// Verifies: STU-081.
// The public scan names secrets, sensitive fields and URLs that are not
// public https by JSON pointer and category, and passes ordinary content.
func TestScan_Findings_STU_081(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, doc string
		want      []Finding
	}{
		{"clean", `{"title":"Hello","link":"https://docs.example.com/page","password":"","token":"{{state.token}}"}`, nil},
		{"private key", `{"a":{"pem":"@PEM@\nMIIE"}}`, []Finding{{"/a/pem", CategorySecret}}},
		{"access key", `{"k":"@AWS@"}`, []Finding{{"/k", CategorySecret}}},
		{"named credential", `{"items":[{"apiKey":"@API@"}]}`, []Finding{{"/items/0/apiKey", CategorySecret}}},
		{"bearer", `{"h":"Bearer abcdefghijklmnopqrstuv"}`, []Finding{{"/h", CategorySecret}}},
		{"sensitive field", `{"fields":[{"name":"ssn","sensitive":true}]}`, []Finding{{"/fields/0", CategorySensitive}}},
		{"plain http", `{"u":"see http://example.com/x"}`, []Finding{{"/u", CategoryURL}}},
		{"loopback", `{"u":"https://localhost:8080/x"}`, []Finding{{"/u", CategoryURL}}},
		{"private address", `{"u":"https://10.1.2.3/x"}`, []Finding{{"/u", CategoryURL}}},
		{"internal host", `{"u":"https://billing.corp/x","v":"https://wiki.internal/y"}`, []Finding{{"/u", CategoryURL}, {"/v", CategoryURL}}},
		{"credentials in url", `{"u":"https://user:pw@example.com/"}`, []Finding{{"/u", CategoryURL}}},
		{"escaped pointer", `{"a/b":{"~":"@AWS@"}}`, []Finding{{"/a~1b/~0", CategorySecret}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := Scan(parse(t, c.doc))
			if len(got) != len(c.want) {
				t.Fatalf("Scan = %v, want %v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("finding %d = %v, want %v", i, got[i], c.want[i])
				}
			}
		})
	}
}

// Verifies: STU-081, SEC-092.
// The refusal names the places and never repeats a value.
func TestScanError_NamesNoValues_STU_081(t *testing.T) {
	t.Parallel()
	findings := Scan(parse(t, `{"a":"@AWS@"}`))
	err := scanError(findings)
	if !strings.Contains(err.Error(), "/a (secret)") || strings.Contains(err.Error(), "AK"+"IA") {
		t.Errorf("error = %q", err)
	}
	var pe *plxerr.Error
	if !errors.As(err, &pe) || pe.Code != plxerr.LibraryPublicScanFailed {
		t.Errorf("code = %v", err)
	}
}

// Verifies: STU-085.
// An instance conflicts only on what the adopted version lacks.
func TestConflictsOf_STU_085(t *testing.T) {
	t.Parallel()
	got := ConflictsOf([]Instance{
		{File: "plugins/a/pages/home.page.json", Path: "/root/children/1", Props: []string{"title", "tone"}, Slots: []string{"body"}},
		{File: "plugins/a/pages/home.page.json", Path: "/root/children/0", Props: []string{"title"}},
		{File: "plugins/b/pages/x.page.json", Path: "/root", Slots: []string{"footer", "body"}},
	}, []string{"title"}, []string{"body"})
	if len(got) != 2 || got[0].Path != "/root/children/1" || len(got[0].RemovedProps) != 1 || got[0].RemovedProps[0] != "tone" ||
		got[1].File != "plugins/b/pages/x.page.json" || got[1].RemovedSlots[0] != "footer" {
		t.Errorf("ConflictsOf = %+v", got)
	}
}

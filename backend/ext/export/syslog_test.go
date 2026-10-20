// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package export_test

import (
	"strings"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/ext/export"
)

// Verifies: SEC-143.
// A parameter value escapes exactly the three characters RFC 5424 section
// 6.3.3 names, and keeps the message valid UTF-8.
func TestEscapeParamValue_SEC_143(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, in, want string }{
		{"plain", "alice", "alice"},
		{"quote", `say "hi"`, `say \"hi\"`},
		{"backslash", `C:\dir`, `C:\\dir`},
		{"bracket", `a]b`, `a\]b`},
		{"opening bracket is not escaped", `a[b`, `a[b`},
		{"all three", `"\]`, `\"\\\]`},
		{"backslash then quote", `\"`, `\\\"`},
		{"unicode kept", "Zoë ✓", "Zoë ✓"},
		{"invalid UTF-8 replaced", "a\xffb", "a\uFFFDb"},
		{"empty", "", ""},
	} {
		if got := export.EscapeParamValue(c.in); got != c.want {
			t.Errorf("%s: EscapeParamValue(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

// Verifies: SEC-143.
// A message has the layout of RFC 5424 section 6: priority, version, a UTC
// microsecond timestamp, the header fields, the structured data and the
// message after a byte order mark.
func TestSyslogMessageLayout_SEC_143(t *testing.T) {
	t.Parallel()
	got, err := export.SyslogMessage{
		Facility: export.FacilityLogAudit, Severity: export.SeverityInformational,
		Time:     time.Date(2026, 10, 19, 14, 0, 0, 123456789, time.FixedZone("CEST", 2*3600)),
		Hostname: "plux-1", AppName: "plux-audit", MsgID: "app.created",
		Data: []export.SDElement{{ID: "plux@32473", Params: []export.SDParam{{Name: "actor", Value: `user:"a"]`}, {Name: "n", Value: "1"}}}},
		Text: []byte(`{"a":1}`),
	}.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	want := "<110>1 2026-10-19T12:00:00.123456Z plux-1 plux-audit - app.created " +
		`[plux@32473 actor="user:\"a\"\]" n="1"] ` + "\xEF\xBB\xBF" + `{"a":1}`
	if string(got) != want {
		t.Errorf("message =\n%q\nwant\n%q", got, want)
	}
}

// Verifies: SEC-143.
// Empty header fields are NILVALUE, characters outside printable ASCII are
// replaced and the fields are cut at their RFC limits; no structured data
// and no text give the shortest valid message.
func TestSyslogHeaderFields_SEC_143(t *testing.T) {
	t.Parallel()
	got, err := export.SyslogMessage{Facility: 1, Severity: 2, MsgID: strings.Repeat("x", 40) + " y", Hostname: "my host"}.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	want := "<10>1 - my_host - - " + strings.Repeat("x", 32) + " -"
	if string(got) != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

// Verifies: SEC-143.
// A structured-data identifier or parameter name that RFC 5424 forbids, and
// a priority out of range, are refused rather than sent malformed.
func TestSyslogRefusesInvalidInput_SEC_143(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		m    export.SyslogMessage
	}{
		{"empty id", export.SyslogMessage{Data: []export.SDElement{{ID: ""}}}},
		{"id with bracket", export.SyslogMessage{Data: []export.SDElement{{ID: "a]b"}}}},
		{"id with space", export.SyslogMessage{Data: []export.SDElement{{ID: "a b"}}}},
		{"long id", export.SyslogMessage{Data: []export.SDElement{{ID: strings.Repeat("a", 33)}}}},
		{"name with equals", export.SyslogMessage{Data: []export.SDElement{{ID: "a@1", Params: []export.SDParam{{Name: "a=b"}}}}}},
		{"name with quote", export.SyslogMessage{Data: []export.SDElement{{ID: "a@1", Params: []export.SDParam{{Name: `a"b`}}}}}},
		{"facility", export.SyslogMessage{Facility: 24}},
		{"severity", export.SyslogMessage{Severity: 8}},
		{"negative", export.SyslogMessage{Facility: -1}},
	} {
		if _, err := c.m.Marshal(); err == nil {
			t.Errorf("%s: no error", c.name)
		}
	}
}

// Verifies: SEC-143.
// The octet-counting framing prefixes the length in octets, not characters.
func TestFrameOctetCounting_SEC_143(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ in, want string }{
		{"hello", "5 hello"},
		{"", "0 "},
		{"é✓", "5 é✓"},
		{strings.Repeat("a", 100), "100 " + strings.Repeat("a", 100)},
	} {
		if got := string(export.Frame([]byte(c.in))); got != c.want {
			t.Errorf("Frame(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

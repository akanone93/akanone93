// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package auditexport

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/ext/export"
)

// Verifies: SEC-143.
// The header escapes a backslash and a pipe, and a line break becomes a
// space; the extension escapes a backslash and an equals sign, and writes a
// line break as \n or \r.
func TestCEFEscaping_SEC_143(t *testing.T) {
	t.Parallel()
	header := []struct{ name, in, want string }{
		{"plain", "Plux Server", "Plux Server"},
		{"pipe", "a|b", `a\|b`},
		{"backslash", `a\b`, `a\\b`},
		{"backslash before pipe", `a\|b`, `a\\\|b`},
		{"line breaks become spaces", "a\nb\rc", "a b c"},
		{"equals is not escaped in a header", "a=b", "a=b"},
	}
	for _, tc := range header {
		if got := cefHeaderField(tc.in); got != tc.want {
			t.Errorf("header %s: cefHeaderField(%q) = %q, want %q", tc.name, tc.in, got, tc.want)
		}
	}
	extension := []struct{ name, in, want string }{
		{"plain", "rec-1", "rec-1"},
		{"equals", "a=b", `a\=b`},
		{"backslash", `a\b`, `a\\b`},
		{"newline", "a\nb", `a\nb`},
		{"carriage return", "a\rb", `a\rb`},
		{"pipe is not escaped in an extension", "a|b", "a|b"},
	}
	for _, tc := range extension {
		if got := cefExtensionValue(tc.in); got != tc.want {
			t.Errorf("extension %s: cefExtensionValue(%q) = %q, want %q", tc.name, tc.in, got, tc.want)
		}
	}
}

// Verifies: SEC-143.
// A record becomes one CEF:0 line with the vendor, product, version, action
// and severity in the header, and the record's fields in a fixed order.
func TestCEFLine_SEC_143(t *testing.T) {
	t.Parallel()
	r := sampleRecords(1)[0]
	r.Action = "policy|changed"
	r.Detail = `a=b|c\d`
	line := cefLine(r, "1.2.3")
	millis := strconv.FormatInt(r.At.UnixMilli(), 10)
	want := "CEF:0|Plux|Plux Server|1.2.3|policy\\|changed|policy\\|changed|3|" +
		"rt=" + millis + " externalId=rec-1 suid=usr-1 suser=Ada src=203.0.113.7 " +
		"requestClientApplication=plux-test cn1Label=sequence cn1=1 " +
		"cs1Label=organization cs1=org-1 cs2Label=entryHash cs2=hash-1 " +
		"cs3Label=previousHash cs3= cs4Label=resource cs4=team:t1 " +
		`msg=a\=b|c\\d`
	if line != want {
		t.Errorf("cefLine:\n got %s\nwant %s", line, want)
	}
}

// Verifies: SEC-143.
// A CEF line cannot inject a second event: a line break in a value is
// written as an escape, so the message stays one line.
func TestCEFLineHoldsOneEvent_SEC_143(t *testing.T) {
	t.Parallel()
	r := sampleRecords(1)[0]
	r.Detail = "x\ny"
	r.Action = "a\nb"
	if line := cefLine(r, "1"); strings.ContainsAny(line, "\r\n") {
		t.Errorf("cefLine holds a line break: %q", line)
	}
}

// Verifies: SEC-143, GOV-030.
// Without a licence the CEF sink refuses the write with PLX-8600 and opens
// no connection.
func TestCEFSinkRefusedWithoutLicence_SEC_143(t *testing.T) {
	t.Parallel()
	sink, err := newCEFSink(export.Config{
		Destination: export.Destination{Kind: KindCEF, Address: "siem.example.com:6514"},
		Dial:        neverDialled(t),
		TLS:         testTLS(),
		Hostname:    "plux-1",
		Version:     "1.0.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	err = sink.Deliver(context.Background(), sampleRecords(2))
	if err == nil || !strings.Contains(err.Error(), "PLX-8600") {
		t.Errorf("Deliver without a licence = %v, want PLX-8600", err)
	}
}

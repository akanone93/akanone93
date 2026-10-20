// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auditexport

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/ext/export"
	"github.com/nightCode42/plux3/backend/internal/httpx"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Verifies: SEC-143.
// The wait after a failure doubles from ten seconds to a cap of fifteen
// minutes, and jitter spreads it over the upper half of each step.
func TestBackoff_SEC_143(t *testing.T) {
	t.Parallel()
	low := &Exporter{jitter: func() float64 { return 0 }}
	high := &Exporter{jitter: func() float64 { return 0.999999 }}
	for _, c := range []struct {
		failures int32
		step     time.Duration
	}{
		{1, 10 * time.Second},
		{2, 20 * time.Second},
		{3, 40 * time.Second},
		{6, 320 * time.Second},
		{7, 640 * time.Second},
		{8, 15 * time.Minute},
		{30, 15 * time.Minute},
		{1 << 30, 15 * time.Minute},
	} {
		if got := low.backoff(c.failures); got != c.step/2 {
			t.Errorf("backoff(%d) with no jitter = %s, want %s", c.failures, got, c.step/2)
		}
		if got := high.backoff(c.failures); got < c.step*99/100 || got >= c.step {
			t.Errorf("backoff(%d) with full jitter = %s, want just under %s", c.failures, got, c.step)
		}
	}
}

// Verifies: SEC-143.
// A recorded failure carries its PLX code, never the path or query of a URL
// (which can hold a token), and is bounded.
func TestDescribe_SEC_143(t *testing.T) {
	t.Parallel()
	cause := &url.Error{Op: "Post", URL: "https://siem.example/hook?token=s3cret", Err: errors.New("connection refused")}
	got := describe("splunk", cause)
	if !strings.HasPrefix(got, "PLX-8500 ") || !strings.Contains(got, "connection refused") || strings.Contains(got, "s3cret") {
		t.Errorf("describe = %q", got)
	}
	licensed := describe("x", plxerr.New(plxerr.EnterpriseFeatureNotLicensed, "not licensed"))
	if !strings.HasPrefix(licensed, "PLX-8600 ") {
		t.Errorf("a licence refusal keeps its code: %q", licensed)
	}
	long := describe("x", errors.New(strings.Repeat("é", 1000)))
	if len(long) > maxErrorLength || strings.ContainsRune(long, '\uFFFD') {
		t.Errorf("a long error is cut at %d bytes on a character boundary, got %d", maxErrorLength, len(long))
	}
	if strings.ContainsAny(describe("x", errors.New("a\nb\x00c")), "\n\x00") {
		t.Error("control characters survive")
	}
}

// Verifies: SEC-143, SEC-104.
// A destination on a private, loopback or link-local address is refused
// unless it was explicitly allowed, and the cloud metadata address is
// refused even then.
func TestCheckAddressRefusesPrivate_SEC_143(t *testing.T) {
	t.Parallel()
	for _, address := range []string{
		"127.0.0.1:6514", "10.0.0.5:6514", "192.168.1.1:6514", "172.16.0.1:6514",
		"[::1]:6514", "[fc00::1]:6514", "169.254.169.254:6514", "0.0.0.0:6514",
	} {
		err := checkAddress(KindSyslog, address, httpx.Policy{})
		if code, _ := plxerr.CodeOf(err); code != plxerr.OutboundRequestBlocked {
			t.Errorf("%s: err = %v, want PLX-%d", address, err, plxerr.OutboundRequestBlocked)
		}
	}
	for _, address := range []string{"127.0.0.1:6514", "10.0.0.5:6514", "[::1]:6514"} {
		if err := checkAddress(KindSyslog, address, httpx.PrivateNetworksPolicy()); err != nil {
			t.Errorf("%s allowed for development: %v", address, err)
		}
	}
	if err := checkAddress(KindSyslog, "169.254.169.254:6514", httpx.PrivateNetworksPolicy()); err == nil {
		t.Error("the metadata address is reachable even when private addresses are allowed")
	}
	for _, address := range []string{"", "siem.example.com", "siem.example.com:", ":6514", "siem.example.com:http-alt-x"} {
		if err := checkAddress(KindSyslog, address, httpx.Policy{}); err == nil {
			t.Errorf("%q accepted", address)
		}
	}
	if err := checkAddress(KindSyslog, "siem.example.com:6514", httpx.Policy{}); err != nil {
		t.Errorf("a public name is refused: %v", err)
	}
	if err := checkAddress(KindWebhook, "http://10.0.0.1/hook", httpx.Policy{}); err == nil {
		t.Error("a private webhook URL is accepted")
	}
	if err := checkAddress(KindWebhook, "ftp://siem.example.com/hook", httpx.Policy{}); err == nil {
		t.Error("a non-HTTP scheme is accepted")
	}
}

// Verifies: SEC-143, GOV-030.
// Without a licence the core refuses the enterprise kinds with PLX-8600
// before it looks at the address, and accepts syslog.
func TestEnterpriseKindsRefusedWithoutLicence_SEC_143(t *testing.T) {
	t.Parallel()
	o := opener{}
	for _, kind := range []string{KindCEF, KindOTLP, KindWebhook} {
		err := o.validate(context.Background(), kind, "siem.example.com:6514", false, "", Credentials{})
		if code, _ := plxerr.CodeOf(err); code != plxerr.EnterpriseFeatureNotLicensed {
			t.Errorf("%s: err = %v, want PLX-%d", kind, err, plxerr.EnterpriseFeatureNotLicensed)
		}
	}
	if err := o.validate(context.Background(), KindSyslog, "siem.example.com:6514", false, "", Credentials{}); err != nil {
		t.Errorf("syslog: %v", err)
	}
	if err := o.validate(context.Background(), "carrier-pigeon", "x:1", false, "", Credentials{}); err == nil {
		t.Error("an unknown kind is accepted")
	}
}

// Verifies: SEC-143.
// A syslog message names the organisation, actor, action, resource and entry
// hash in structured data, escapes what RFC 5424 requires, and carries the
// whole record as compact JSON.
func TestSyslogMessageCarriesTheEntry_SEC_143(t *testing.T) {
	t.Parallel()
	r := export.Record{
		ID: "id1", OrganizationID: "org1", Sequence: 7, At: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC),
		ActorKind: "user", ActorID: `u"1]`, ActorDisplay: "Ada", Action: "app.created", TargetKind: "app", TargetID: "a1",
		Detail: "x<y", PreviousHash: "aa", EntryHash: "bb",
	}
	got, err := SyslogMessage(r, "plux-1")
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	for _, want := range []string{
		"<110>1 2026-10-19T12:00:00.000000Z plux-1 plux-audit - app.created [plux@32473 ",
		`organization="org1"`, `actor="user:u\"1\]"`, `action="app.created"`, `resource="app:a1"`,
		`sequence="7"`, `entryHash="bb"`, `previousHash="aa"`,
		`{"id":"id1","organization":"org1","sequence":7,"time":"2026-10-19T12:00:00Z"`,
		`"detail":"x<y"`, `"previousHash":"aa","entryHash":"bb"}`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("message lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "\n") {
		t.Error("the message is not compact")
	}
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// Verifies: SEC-163.
// Every registered destination says who configures it, what it sends and
// what it does by default, and no kind is listed twice.
func TestRegistryDescribesEveryDestination_SEC_163(t *testing.T) {
	t.Parallel()
	seen := map[Kind]bool{}
	for _, d := range Destinations() {
		if seen[d.Kind] {
			t.Errorf("%s is registered twice", d.Kind)
		}
		seen[d.Kind] = true
		for field, v := range map[string]string{
			"name": d.Name, "purpose": d.Purpose, "setting": d.Setting, "data": d.Data,
			"configurator": string(d.ConfiguredBy), "default": string(d.Default),
		} {
			if v == "" {
				t.Errorf("%s has no %s", d.Kind, field)
			}
		}
		for _, h := range d.Hosts {
			if h != strings.ToLower(h) || strings.ContainsAny(h, ":/ ") {
				t.Errorf("%s lists %q, which is not a lower-case host name", d.Kind, h)
			}
		}
		if got, err := Lookup(d.Kind); err != nil || got.Name != d.Name {
			t.Errorf("Lookup(%s) = %v, %v", d.Kind, got.Name, err)
		}
	}
}

// Verifies: SEC-163.
// A client, a transport or a dialer cannot be built for a kind the
// registry does not list, nor for no kind at all.
func TestConstructorsRefuseAnUnregisteredKind_SEC_163(t *testing.T) {
	t.Parallel()
	for _, kind := range []Kind{"", "made.up"} {
		if _, err := NewClient(Options{Destination: kind}); !errors.Is(err, ErrUnregistered) {
			t.Errorf("NewClient(%q) = %v", kind, err)
		}
		if _, err := NewTransport(TransportOptions{Destination: kind}); !errors.Is(err, ErrUnregistered) {
			t.Errorf("NewTransport(%q) = %v", kind, err)
		}
		if _, err := GuardedDialer(kind, Policy{}); !errors.Is(err, ErrUnregistered) {
			t.Errorf("GuardedDialer(%q) = %v", kind, err)
		}
	}
}

// Verifies: SEC-163.
// A kind with fixed hosts refuses any other host before it resolves the
// name, even where the SSRF policy would allow the address.
func TestFixedHostsRefuseAnyOtherHost_SEC_163(t *testing.T) {
	t.Parallel()
	d, err := GuardedDialer(KindPush, Policy{AllowLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, address := range []string{"example.com:443", "127.0.0.1:443", "fcm.googleapis.com.evil.test:443", "no-port"} {
		if _, err := d.DialContext(context.Background(), "tcp", address); !errors.Is(err, ErrBlocked) {
			t.Errorf("DialContext(%q) = %v, want a refusal", address, err)
		}
	}

	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	c, err := NewClient(Options{Destination: KindPush, Policy: Policy{AllowLoopback: true}, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if resp, err := Get(context.Background(), c, srv.URL); err == nil {
		_ = resp.Body.Close()
		t.Error("a push client reached a host the push destination does not list")
	}
}

// Verifies: SEC-163.
// The match of a fixed host ignores case, and a kind without fixed hosts
// accepts any host (the SSRF policy then decides).
func TestDestinationAllows_SEC_163(t *testing.T) {
	t.Parallel()
	push, err := Lookup(KindPush)
	if err != nil {
		t.Fatal(err)
	}
	if !push.allows("FCM.Googleapis.com") || push.allows("example.com") {
		t.Error("a fixed-host destination matched the wrong hosts")
	}
	open, err := Lookup(KindNotify)
	if err != nil {
		t.Fatal(err)
	}
	if open.FixedHosts() || !open.allows("hooks.example.com") {
		t.Error("a destination without fixed hosts refused a host")
	}
}

// Verifies: SEC-163, DEP-040.
// The committed egress list is the one the registry renders.
func TestEgressListMatchesTheRegistry_SEC_163(t *testing.T) {
	t.Parallel()
	committed, err := os.ReadFile("../../../docs/operations/egress.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(committed) != RenderEgressList() {
		t.Error("docs/operations/egress.md differs from the registry; run make gen")
	}
}

// Verifies: SEC-163.
// The rendered list names every destination and every fixed host.
func TestRenderEgressListNamesEverything_SEC_163(t *testing.T) {
	t.Parallel()
	text := RenderEgressList()
	for _, d := range Destinations() {
		if !strings.Contains(text, "`"+string(d.Kind)+"`") {
			t.Errorf("the list lacks %s", d.Kind)
		}
		for _, h := range d.Hosts {
			if !strings.Contains(text, "`"+h+"`") {
				t.Errorf("the list lacks the host %s of %s", h, d.Kind)
			}
		}
	}
	for _, item := range Infrastructure() {
		if !strings.Contains(text, item) {
			t.Errorf("the list lacks %q", item)
		}
	}
}

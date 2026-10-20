// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package httpx

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// outboundPattern matches the ways code opens an outbound connection by
// itself instead of through a client built for a registered destination.
// httpx.NewClient, httpx.NewTransport and httpx.GuardedDialer are not
// listed: they take a Kind and fail for an unregistered one.
var outboundPattern = regexp.MustCompile(
	`\bhttp\.Client\{|\bhttp\.Transport\{|\bhttp\.(?:Get|Post|PostForm|Head)\(|\bhttp\.Default(?:Client|Transport)\b` +
		`|\bnet\.(?:Dial|DialTimeout|Dialer)\b|\btls\.Dial(?:er)?\b|\bsmtp\.Dial\b|\botlptrace\w*\b|\bhttpx\.New(\w+)\(`)

// registeredConstructors take a Kind, so a call to them is registered by
// construction.
var registeredConstructors = []string{"Client", "Transport"}

// notEgress lists, by path relative to the module root, the files that open
// connections without a destination kind, each with the reason it is not
// egress or is covered otherwise. A new outbound call outside this list
// fails TestNoUnregisteredOutboundCall_SEC_163: register a destination and
// build the client for it, or add the file here with its reason.
var notEgress = map[string]string{
	"cmd/plux/remote.go":              "the CLI talks to the server its user names, from the user's machine",
	"internal/auditexport/sinks.go":   "wraps the transport built for httpx.KindAuditExport",
	"internal/cache/sentinel.go":      "the operator's cache; infrastructure, listed as not egress",
	"internal/cache/valkey.go":        "the operator's cache; infrastructure, listed as not egress",
	"internal/document/scan.go":       "the operator's malware scanner; infrastructure, listed as not egress",
	"internal/egress/client.go":       "the function runner's tunnel to its egress proxy (httpx.KindFunctionEgress)",
	"internal/egress/server.go":       "the egress proxy itself: its upstream is httpx.KindFunctionEgress (FN-013)",
	"internal/fnrunner/host.go":       "function HTTP calls leave through the egress proxy (httpx.KindFunctionEgress)",
	"internal/gateway/egress.go":      "wraps the transport built for httpx.KindGatewayUpstream",
	"internal/gateway/send.go":        "sends over the transport built for httpx.KindGatewayUpstream",
	"internal/netsim/forward.go":      "the benchmark's network simulator; not part of the server",
	"internal/observability/trace.go": "the OTLP trace exporter of httpx.KindTraceExport; the SDK builds its own client",
	"internal/push/egress.go":         "wraps the client built for httpx.KindPush and restricts it to that kind's hosts",
	"internal/server/builder.go":      "mutual TLS between the roles of one installation",
	"internal/server/fnrunner.go":     "mutual TLS between the roles of one installation",
	"internal/server/function.go":     "mutual TLS between the roles of one installation",
	"internal/signing/pkcs11.go":      "a local unix socket to the PKCS#11 module",
	"internal/signing/vault.go":       "the operator's Vault; infrastructure, listed as not egress",
}

// Verifies: SEC-163.
// No source outside package httpx opens an outbound connection except in a
// file the test lists with its reason, so a new outbound call cannot ship
// without a registered destination.
func TestNoUnregisteredOutboundCall_SEC_163(t *testing.T) {
	t.Parallel()
	found := map[string][]string{}
	scanOutbound(t, "../..", "", found)
	scanOutbound(t, "../../../ee/backend", "ee/backend/", found)
	for file, matches := range found {
		if _, ok := notEgress[file]; !ok {
			t.Errorf("%s opens an outbound connection (%s) without a registered destination; build the client with httpx for a Kind", file, strings.Join(matches, ", "))
		}
	}
	for file := range notEgress {
		if _, ok := found[file]; !ok {
			t.Errorf("%s is on the not-egress list but no longer opens a connection; remove it", file)
		}
	}
}

// scanOutbound records, per file relative to root and prefixed with prefix,
// the outbound constructors the non-test sources under root use.
func scanOutbound(t *testing.T, root, prefix string, found map[string][]string) {
	t.Helper()
	if _, err := os.Stat(root); err != nil {
		return
	}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == "internal/httpx" || d.Name() == "testdata" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if m := outboundMatches(string(src)); len(m) > 0 {
			found[prefix+rel] = m
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// outboundMatches returns the distinct outbound constructors a source
// uses, ignoring comment lines and the constructors that take a Kind.
func outboundMatches(src string) []string {
	var out []string
	for _, line := range strings.Split(src, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		for _, m := range outboundPattern.FindAllStringSubmatch(line, -1) {
			if m[1] != "" && slices.Contains(registeredConstructors, m[1]) {
				continue
			}
			if !slices.Contains(out, m[0]) {
				out = append(out, m[0])
			}
		}
	}
	return out
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/ed25519"
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/edition/licensefile"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

const licenseUsage = "Usage: plux license inspect [--public-key <base64>]... [--json] <file>\n"

// license runs plux license inspect (GOV-030).
func (e env) license(args []string) int {
	if len(args) == 0 || args[0] != "inspect" {
		_, _ = fmt.Fprint(e.stderr, licenseUsage)
		return exitUsage
	}
	return e.licenseInspect(args[1:])
}

// keyFlags is a repeatable --public-key flag.
type keyFlags []string

func (k *keyFlags) String() string     { return strings.Join(*k, ",") }
func (k *keyFlags) Set(v string) error { *k = append(*k, v); return nil }

// licenseView is what plux license inspect reports.
type licenseView struct {
	Payload  licensefile.Payload `json:"payload"`
	Verified bool                `json:"verified"`
	// SignedBy are the identifiers of the keys that signed the file.
	SignedBy []string `json:"signedBy"`
	// Error says why the file did not verify.
	Error string `json:"error,omitempty"`
}

// licenseInspect shows a licence file's payload and whether a trusted key
// signed it. The file is read here and never sent anywhere; the keys are the
// ones compiled into this build plus any given with --public-key, which is
// how a licence signed by a test key is inspected. Whether the server
// enables features from it is decided by the server (GOV-033).
func (e env) licenseInspect(args []string) int {
	set := flag.NewFlagSet("license inspect", flag.ContinueOnError)
	var extra keyFlags
	set.Var(&extra, "public-key", "a base64 Ed25519 public key to trust besides the compiled-in ones (repeatable)")
	asJSON := set.Bool("json", false, "print the result as JSON on stdout")
	rest, code, ok := parse(set, args, e.stderr, licenseUsage+"\nShows a licence file's payload and whether it is signed by a trusted key.", 1)
	if !ok {
		return code
	}
	var public []ed25519.PublicKey
	for _, k := range extra {
		p, err := licensefile.ParsePublicKey(k)
		if err != nil {
			return e.fail("license inspect", usageError(err.Error()))
		}
		public = append(public, p)
	}
	raw, err := os.ReadFile(rest[0])
	if err != nil {
		return e.fail("license inspect", fmt.Errorf("read the licence: %w", err))
	}
	f, err := licensefile.Parse(raw)
	if err != nil {
		return e.fail("license inspect", err)
	}
	view := licenseView{Payload: f.Payload, SignedBy: []string{}}
	for _, s := range f.Signatures {
		view.SignedBy = append(view.SignedBy, s.KeyID)
	}
	slices.Sort(view.SignedBy)
	if err := f.Verify(licensefile.TrustedKeysWith(public...)); err != nil {
		view.Error = err.Error()
	} else {
		view.Verified = true
	}
	e.emit(*asJSON, view, view.text(time.Now()))
	if !view.Verified {
		return exitFailed
	}
	return exitOK
}

// text renders the view for a terminal.
func (v licenseView) text(now time.Time) string {
	p := v.Payload
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 4, 2, ' ', 0)
	row := func(k, val string) { _, _ = fmt.Fprintf(w, "%s\t%s\n", k, val) }
	row("License:", p.LicenseID)
	row("Organization:", p.Organization)
	row("Edition:", p.Edition)
	row("Issued:", p.IssuedAt)
	row("Not before:", p.NotBefore)
	row("Expires:", p.ExpiresAt+licenseCountdown(p.ExpiresAt, now))
	row("Grace days:", fmt.Sprint(p.Grace()))
	row("Features:", joinOrNone(p.Features))
	limits := make([]string, 0, len(p.Limits))
	for k, val := range p.Limits {
		limits = append(limits, fmt.Sprintf("%s=%d", k, val))
	}
	slices.Sort(limits)
	row("Limits:", joinOrNone(limits))
	if p.InstallationID != "" {
		row("Installation:", p.InstallationID)
	}
	row("Signed by:", joinOrNone(v.SignedBy))
	_ = w.Flush()
	if v.Verified {
		b.WriteString("Verified: signed by a trusted key\n")
	} else {
		b.WriteString("NOT VERIFIED: " + v.Error + "\n")
	}
	return b.String()
}

func joinOrNone(s []string) string {
	if len(s) == 0 {
		return "none"
	}
	return strings.Join(s, ", ")
}

// licenseCountdown says how far an instant is from now.
func licenseCountdown(expires string, now time.Time) string {
	t, err := time.Parse(time.RFC3339, expires)
	if err != nil {
		return ""
	}
	days := int(t.Sub(now).Hours() / 24)
	if t.After(now) {
		return fmt.Sprintf(" (in %d days)", days)
	}
	return fmt.Sprintf(" (%d days ago)", -days)
}

const adminUsage = "Usage: plux admin license [--server url] [--org id] [--json]\n"

// admin runs plux admin license: the installation's licence state, as the
// server reports it (GOV-030).
func (e env) admin(args []string) int {
	if len(args) == 0 || args[0] != "license" {
		_, _ = fmt.Fprint(e.stderr, adminUsage)
		return exitUsage
	}
	var c common
	set := flag.NewFlagSet("admin license", flag.ContinueOnError)
	c.register(set, false)
	if _, code, ok := parse(set, args[1:], e.stderr, adminUsage+"\nShows the state of the installation's enterprise licence and what it enables.", 0); !ok {
		return code
	}
	if err := c.resolve(); err != nil {
		return e.fail("admin license", err)
	}
	cl, err := e.connect(c, true)
	if err != nil {
		return e.fail("admin license", err)
	}
	client := pluxv1connect.NewLicenseServiceClient(e.httpClient, c.server, connect.WithInterceptors(headers{token: cl.token, org: c.org}))
	res, err := client.GetLicenseState(context.Background(), connect.NewRequest(&pluxv1.GetLicenseStateRequest{}))
	if err != nil {
		return e.fail("admin license", err)
	}
	m := res.Msg
	view := map[string]any{
		"state": m.GetState(), "licenseId": m.GetLicenseId(), "organization": m.GetOrganization(),
		"daysRemaining": m.GetDaysRemaining(), "features": nonNil(m.GetFeatures()), "limits": m.GetLimits(), "message": m.GetMessage(),
	}
	if m.GetExpiresAt() != nil {
		view["expiresAt"] = m.GetExpiresAt().AsTime().Format(time.RFC3339)
		view["graceEndsAt"] = m.GetGraceEndsAt().AsTime().Format(time.RFC3339)
	}
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "State:    %s\n%s\n", m.GetState(), m.GetMessage())
	if m.GetLicenseId() != "" {
		_, _ = fmt.Fprintf(&b, "License:  %s (%s)\nExpires:  %s (%d days)\nFeatures: %s\n",
			m.GetLicenseId(), m.GetOrganization(), m.GetExpiresAt().AsTime().Format(time.DateOnly), m.GetDaysRemaining(), joinOrNone(m.GetFeatures()))
	}
	return e.emit(c.json, view, b.String())
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

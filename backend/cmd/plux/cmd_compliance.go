// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/nightCode42/plux3/backend/ext/governance"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

const complianceUsage = "Usage: plux compliance report|verify [flags]\n"

// defaultReportWait is how long `compliance report` waits for the server's
// worker to sign a report.
const defaultReportWait = 30 * time.Second

// reportPoll is how often it asks while waiting.
const reportPoll = time.Second

// complianceFlags are the flags of the compliance subcommands.
type complianceFlags struct {
	from, to, format, out string
	wait                  time.Duration
	keys                  keyFlags
}

// compliance produces and verifies compliance reports (GOV-026): `report`
// asks the server for the signed report of a period, as the DSSE envelope or
// a PDF, and `verify` checks an envelope offline against the audit public key.
func (e env) compliance(args []string) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(e.stderr, complianceUsage)
		return exitUsage
	}
	var f complianceFlags
	switch args[0] {
	case "report":
		return e.complianceReport(args[1:], &f)
	case "verify":
		return e.complianceVerify(args[1:], &f)
	}
	_, _ = fmt.Fprintf(e.stderr, "%s compliance: unknown subcommand %q\n\n%s", name, args[0], complianceUsage)
	return exitUsage
}

// complianceReport requests a report, waits for its signature and writes it.
func (e env) complianceReport(args []string, f *complianceFlags) int {
	var c common
	set := flag.NewFlagSet("compliance report", flag.ContinueOnError)
	c.register(set, false)
	set.StringVar(&f.from, "from", "", "the start of the period, inclusive: `RFC 3339 or YYYY-MM-DD` (UTC)")
	set.StringVar(&f.to, "to", "", "the end of the period, exclusive: `RFC 3339 or YYYY-MM-DD` (UTC)")
	set.StringVar(&f.format, "format", "json", "`json` (the signed DSSE envelope) or pdf")
	set.StringVar(&f.out, "out", "", "write the report to this `file`; JSON goes to standard output without it")
	set.DurationVar(&f.wait, "wait", defaultReportWait, "how long to wait for the server to sign the report")
	_, code, ok := parse(set, args, e.stderr, "Usage: plux compliance report --from time --to time [--format json|pdf] [--out file] [--wait duration] [--json]\n\nGathers every production change of the period with its author, approvers, timestamps, content hashes and evidence into a report signed with the audit key (GOV-026).", 0)
	if !ok {
		return code
	}
	from, to, err := reportPeriod(f)
	if err == nil && f.format != "json" && f.format != "pdf" {
		err = usageError("--format is json or pdf")
	}
	if err == nil && f.format == "pdf" && f.out == "" {
		err = usageError("a PDF goes to a file: pass --out")
	}
	if err != nil {
		return e.fail("compliance", err)
	}
	if err := c.resolve(); err != nil {
		return e.fail("compliance", err)
	}
	opts, err := e.authOptions(c)
	if err != nil {
		return e.fail("compliance", err)
	}
	client := pluxv1connect.NewComplianceServiceClient(e.httpClient, c.server, opts)
	return e.fetchReport(context.Background(), client, c, *f, from, to)
}

// reportPeriod reads --from and --to.
func reportPeriod(f *complianceFlags) (from, to time.Time, err error) {
	if from, err = parseInstant(f.from); err != nil {
		return time.Time{}, time.Time{}, usageError("--from: " + err.Error())
	}
	if to, err = parseInstant(f.to); err != nil {
		return time.Time{}, time.Time{}, usageError("--to: " + err.Error())
	}
	return from, to, nil
}

// parseInstant reads an RFC 3339 time or a UTC date.
func parseInstant(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, errors.New("is required")
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("%q is not an RFC 3339 time or a YYYY-MM-DD date", s)
}

// fetchReport creates the report, waits until it is signed and writes it.
func (e env) fetchReport(ctx context.Context, client pluxv1connect.ComplianceServiceClient, c common, f complianceFlags, from, to time.Time) int {
	created, err := client.CreateComplianceReport(ctx, connect.NewRequest(&pluxv1.CreateComplianceReportRequest{From: timestamppb.New(from), To: timestamppb.New(to)}))
	if err != nil {
		return e.fail("compliance report", stepUp(err))
	}
	id := created.Msg.GetReport().GetId()
	deadline := time.Now().Add(f.wait)
	for {
		res, err := client.GetComplianceReport(ctx, connect.NewRequest(&pluxv1.GetComplianceReportRequest{Id: id, Format: f.format}))
		if err != nil {
			return e.fail("compliance report", err)
		}
		if res.Msg.GetReport().GetSigned() {
			return e.writeReport(c, f, res.Msg)
		}
		if !time.Now().Add(reportPoll).Before(deadline) {
			return e.fail("compliance report", fmt.Errorf("report %s is stored but not signed yet; fetch it later with --wait or ask an operator whether the worker runs", id))
		}
		time.Sleep(reportPoll)
	}
}

// writeReport writes a signed report to its file or to standard output.
func (e env) writeReport(c common, f complianceFlags, r *pluxv1.GetComplianceReportResponse) int {
	info := r.GetReport()
	if f.out == "" {
		_, _ = e.stdout.Write(r.GetContent())
		return exitOK
	}
	if err := os.WriteFile(f.out, r.GetContent(), 0o644); err != nil { //nolint:gosec // G306: a report is meant to be shared.
		return e.fail("compliance report", fmt.Errorf("write %s: %w", f.out, err))
	}
	return e.emit(c.json, map[string]any{"report": info.GetId(), "changes": info.GetChanges(), "keyId": info.GetKeyId(), "file": f.out},
		fmt.Sprintf("Wrote report %s (%d production changes, signed with key %s) to %s.", info.GetId(), info.GetChanges(), info.GetKeyId(), f.out))
}

// complianceVerify checks an envelope against the keys the auditor trusts.
// It needs no server and no account: the evidence is checked, not the
// server's word for it.
func (e env) complianceVerify(args []string, f *complianceFlags) int {
	var asJSON bool
	set := flag.NewFlagSet("compliance verify", flag.ContinueOnError)
	set.BoolVar(&asJSON, "json", false, "print the result as JSON on stdout")
	set.Var(&f.keys, "key", "a trusted audit public key as `id=base64` (repeatable); `plux-server audit public-key` prints it")
	rest, code, ok := parse(set, args, e.stderr, "Usage: plux compliance verify --key id=base64 [--json] <file>\n\nVerifies a compliance report's DSSE envelope offline: the signature is checked before the report is read, and a changed byte fails (GOV-026).", 1)
	if !ok {
		return code
	}
	keys, err := parseKeys(f.keys)
	if err != nil {
		return e.fail("compliance verify", err)
	}
	data, err := os.ReadFile(rest[0]) //nolint:gosec // G304: the file is the argument the user gave.
	if err != nil {
		return e.fail("compliance verify", err)
	}
	report, err := openReport(data, keys)
	if err != nil {
		return e.fail("compliance verify", err)
	}
	return e.emit(asJSON, map[string]any{
		"verified": true, "organization": report.OrganizationID, "from": report.From, "to": report.To, "changes": len(report.Changes),
	}, fmt.Sprintf("Verified: report of organisation %s, %s to %s, %d production changes.",
		report.OrganizationID, report.From.Format(time.RFC3339), report.To.Format(time.RFC3339), len(report.Changes)))
}

// openReport parses and verifies an envelope.
func openReport(data []byte, keys map[string]ed25519.PublicKey) (governance.Report, error) {
	env, err := governance.ParseEnvelope(data)
	if err != nil {
		return governance.Report{}, fmt.Errorf("not a compliance report: %w", err)
	}
	report, err := env.Open(keys)
	if err != nil {
		return governance.Report{}, fmt.Errorf("the report does not verify: %w", err)
	}
	return report, nil
}

// parseKeys reads the --key flags.
func parseKeys(specs []string) (map[string]ed25519.PublicKey, error) {
	if len(specs) == 0 {
		return nil, usageError("--key names the audit public key to trust, as id=base64")
	}
	keys := map[string]ed25519.PublicKey{}
	for _, spec := range specs {
		id, encoded, ok := strings.Cut(spec, "=")
		raw, err := base64.StdEncoding.DecodeString(encoded)
		if !ok || id == "" || err != nil || len(raw) != ed25519.PublicKeySize {
			return nil, usageError(fmt.Sprintf("--key %q is not id=base64 of a 32-byte Ed25519 public key", id))
		}
		keys[id] = ed25519.PublicKey(raw)
	}
	return keys, nil
}

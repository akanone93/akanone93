// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/nightCode42/plux3/backend/internal/loadgen"
)

// Exit codes: 0 success, 1 a failed run, 2 a usage error.
const (
	exitOK     = 0
	exitFailed = 1
	exitUsage  = 2
)

// main delegates to run so that the logic is testable.
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

// options are the parsed flags.
type options struct {
	cfg           loadgen.Config
	speedup       float64
	caFile        string
	maxErrorRatio float64
	function      string
	functionInput string
	functionVer   uint
	deviceIDsFile string
}

// run executes one load run, prints its report as JSON on stdout and
// returns the process exit code.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	opts, err := parseArgs(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		_, _ = fmt.Fprintln(stderr, "plux-load:", err)
		return exitUsage
	}
	fleet, err := loadgen.NewFleet(opts.cfg)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "plux-load:", err)
		return exitUsage
	}
	regErr := fleet.Register(ctx)
	var report loadgen.Report
	if regErr == nil {
		if err := writeDeviceIDs(opts.deviceIDsFile, fleet.DeviceIDs()); err != nil {
			_, _ = fmt.Fprintln(stderr, "plux-load:", err)
			return exitFailed
		}
		report = fleet.Run(ctx)
	} else {
		report = fleet.Report(0)
	}
	if err := printReport(stdout, report); err != nil {
		_, _ = fmt.Fprintln(stderr, "plux-load:", err)
		return exitFailed
	}
	if regErr != nil {
		_, _ = fmt.Fprintln(stderr, "plux-load:", regErr)
		return exitFailed
	}
	return verdict(report, opts.maxErrorRatio, stderr)
}

// writeDeviceIDs writes the device IDs, one per line, to a file; an empty
// name writes nothing.
func writeDeviceIDs(name string, ids []string) error {
	if name == "" {
		return nil
	}
	var b strings.Builder
	for _, id := range ids {
		b.WriteString(id + "\n")
	}
	if err := os.WriteFile(name, []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("plux-load: write the device IDs: %w", err)
	}
	return nil
}

// printReport writes the report as indented JSON.
func printReport(w io.Writer, r loadgen.Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		return fmt.Errorf("plux-load: %w", err)
	}
	return nil
}

// verdict fails a run whose error ratio exceeds the limit, so that a
// pipeline notices a server that stopped answering.
func verdict(r loadgen.Report, limit float64, stderr io.Writer) int {
	var requests int64
	for _, op := range r.Operations {
		requests += op.Requests
	}
	if requests == 0 {
		_, _ = fmt.Fprintln(stderr, "plux-load: no request was made")
		return exitFailed
	}
	if ratio := float64(r.Errors()) / float64(requests); ratio > limit {
		_, _ = fmt.Fprintf(stderr, "plux-load: %d of %d requests failed (%.4f), more than %.4f\n", r.Errors(), requests, ratio, limit)
		return exitFailed
	}
	return exitOK
}

// parseArgs reads the flags into a run's configuration.
func parseArgs(args []string, stderr io.Writer) (options, error) {
	var o options
	fs := flag.NewFlagSet("plux-load", flag.ContinueOnError)
	fs.SetOutput(stderr)
	c := &o.cfg
	fs.StringVar(&c.BaseURL, "server", "", "base `URL` of the server, as its publicBaseURL names it")
	fs.StringVar(&c.AppID, "app", "", "`ID` of the app the devices belong to")
	fs.StringVar(&c.Environment, "env", "staging", "environment key; it must not be a production one (development evidence)")
	fs.StringVar(&c.Channel, "channel", "", "channel the devices follow; empty is the default")
	fs.IntVar(&c.Devices, "devices", 100, "number of synthetic devices")
	fs.DurationVar(&c.Duration, "duration", time.Minute, "how long the registered fleet runs")
	fs.IntVar(&c.Concurrency, "register-concurrency", 32, "registrations in flight at once")
	fs.Float64Var(&o.speedup, "speedup", 1, "run the pacing this many times faster; reported rates are also given at real pacing")
	fs.Uint64Var(&c.Seed, "seed", 1, "seed of the pacing jitter")
	fs.BoolVar(&c.ForwardedFor, "forwarded-for", false, "give each device its own X-Forwarded-For address (needs a trusted proxy; the load overlay)")
	fs.BoolVar(&c.InjectErrors, "inject-errors", false, "report an error event with every minute of use: a synthetic regression for a rollout's health gate")
	fs.Int64Var(&c.ErrorRelease, "error-release", 0, "with -inject-errors, only devices running this release `sequence` report the error")
	fs.StringVar(&o.deviceIDsFile, "device-ids-file", "", "write the IDs of the registered devices, one per line, to this `file` before the run starts (for audit scripts)")
	fs.BoolVar(&c.PushTokens, "push-tokens", false, "register a push token for each device (needs push.enabled)")
	fs.StringVar(&o.caFile, "ca-file", "", "PEM `file` of the certificate authority to trust, for the development CA")
	fs.Float64Var(&o.maxErrorRatio, "max-error-ratio", 0.001, "fail the run when more than this share of requests fail")
	fs.StringVar(&o.function, "function", "", "`name` of a server function the devices call now and then")
	fs.UintVar(&o.functionVer, "function-version", 1, "version of that function")
	fs.StringVar(&o.functionInput, "function-input", "{}", "JSON input of that function")
	if err := fs.Parse(args); err != nil {
		return o, fmt.Errorf("plux-load: %w", err)
	}
	return o.finish()
}

// finish derives the pacing, the function and the HTTP client from the
// flags.
func (o options) finish() (options, error) {
	if o.speedup <= 0 {
		return o, errors.New("-speedup must be positive")
	}
	c := &o.cfg
	c.Scale = o.speedup
	c.Pacing = loadgen.DefaultPacing().Scaled(o.speedup)
	if o.function != "" {
		if !json.Valid([]byte(o.functionInput)) {
			return o, errors.New("-function-input is not valid JSON")
		}
		c.Function = &loadgen.Function{Name: o.function, Version: uint32(o.functionVer), Input: []byte(o.functionInput)} //nolint:gosec // a function version is small
	}
	client, err := httpClient(o.caFile)
	if err != nil {
		return o, err
	}
	c.HTTP = client
	return o, nil
}

// httpClient returns the client for all calls: one that trusts the given
// certificate authority in addition to the system's, when one is named.
func httpClient(caFile string) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone() //nolint:forcetypeassert // the default transport is an *http.Transport
	transport.MaxIdleConnsPerHost = 256
	if caFile != "" {
		pem, err := os.ReadFile(caFile) //nolint:gosec // the operator names the file
		if err != nil {
			return nil, fmt.Errorf("plux-load: %w", err)
		}
		pool, err := x509.SystemCertPool()
		if err != nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, errors.New("plux-load: -ca-file holds no certificate")
		}
		transport.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS13}
	}
	return &http.Client{Transport: transport, Timeout: 30 * time.Second}, nil
}
